package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/AzureAD/microsoft-authentication-library-for-go/apps/cache"
	"github.com/AzureAD/microsoft-authentication-library-for-go/apps/public"
)

// DeviceCodeAuth provides authentication via device code flow.
// This is used as a fallback when the broker is not available.
type DeviceCodeAuth struct {
	client   public.Client
	clientID string
	scopes   []string

	mu          sync.Mutex
	cachedToken *Token
	account     public.Account
}

// NewDeviceCodeAuth creates a new device code auth client.
func NewDeviceCodeAuth(clientID string, scopes []string) (*DeviceCodeAuth, error) {
	if clientID == "" {
		clientID = DefaultClientID
	}

	// Set up cache file
	cacheFile, err := getCacheFilePath()
	if err != nil {
		slog.Warn("could not determine cache file path", "error", err)
	}

	var opts []public.Option
	opts = append(opts, public.WithAuthority(DefaultAuthority))

	if cacheFile != "" {
		accessor := &tokenCacheAccessor{path: cacheFile}
		opts = append(opts, public.WithCache(accessor))
	}

	client, err := public.New(clientID, opts...)
	if err != nil {
		return nil, fmt.Errorf("create MSAL client: %w", err)
	}

	return &DeviceCodeAuth{
		client:   client,
		clientID: clientID,
		scopes:   scopes,
	}, nil
}

// GetToken acquires an access token, using cached token if valid.
func (d *DeviceCodeAuth) GetToken(ctx context.Context) (*Token, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Return cached token if still valid
	if d.cachedToken != nil && time.Now().Add(5*time.Minute).Before(d.cachedToken.ExpiresOn) {
		return d.cachedToken, nil
	}

	// Try to get accounts from cache
	accounts, err := d.client.Accounts(ctx)
	if err != nil {
		slog.Debug("could not get cached accounts", "error", err)
	}

	// Try silent auth with cached accounts
	for _, acct := range accounts {
		result, err := d.client.AcquireTokenSilent(ctx, d.scopes, public.WithSilentAccount(acct))
		if err == nil {
			d.account = acct
			d.cachedToken = &Token{
				AccessToken: result.AccessToken,
				ExpiresOn:   result.ExpiresOn,
				AccountID:   acct.HomeAccountID,
			}
			return d.cachedToken, nil
		}
		slog.Debug("silent auth failed for account", "account", acct.PreferredUsername, "error", err)
	}

	// Fall back to device code flow
	slog.Info("no cached credentials, starting device code flow")
	token, err := d.acquireTokenWithDeviceCode(ctx)
	if err != nil {
		return nil, err
	}

	d.cachedToken = token
	return token, nil
}

// acquireTokenWithDeviceCode performs the device code flow.
func (d *DeviceCodeAuth) acquireTokenWithDeviceCode(ctx context.Context) (*Token, error) {
	dc, err := d.client.AcquireTokenByDeviceCode(ctx, d.scopes)
	if err != nil {
		return nil, fmt.Errorf("start device code flow: %w", err)
	}

	// Print instructions for user
	fmt.Fprintf(os.Stderr, "\n"+
		"To sign in, use a web browser to open the page %s\n"+
		"and enter the code %s to authenticate.\n\n",
		dc.Result.VerificationURL,
		dc.Result.UserCode)

	// Wait for authentication
	result, err := dc.AuthenticationResult(ctx)
	if err != nil {
		return nil, fmt.Errorf("device code auth: %w", err)
	}

	// Get the account for future silent auth
	accounts, _ := d.client.Accounts(ctx)
	for _, acct := range accounts {
		if acct.HomeAccountID == result.Account.HomeAccountID {
			d.account = acct
			break
		}
	}

	return &Token{
		AccessToken: result.AccessToken,
		ExpiresOn:   result.ExpiresOn,
		AccountID:   result.Account.HomeAccountID,
	}, nil
}

// Close is a no-op for device code auth.
func (d *DeviceCodeAuth) Close() error {
	return nil
}

// tokenCacheAccessor implements cache.ExportReplace for MSAL token caching.
type tokenCacheAccessor struct {
	path string
}

func (t *tokenCacheAccessor) Replace(ctx context.Context, cache cache.Unmarshaler, hints cache.ReplaceHints) error {
	data, err := os.ReadFile(t.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return cache.Unmarshal(data)
}

func (t *tokenCacheAccessor) Export(ctx context.Context, cache cache.Marshaler, hints cache.ExportHints) error {
	data, err := cache.Marshal()
	if err != nil {
		return err
	}

	// Ensure directory exists
	dir := filepath.Dir(t.path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	return os.WriteFile(t.path, data, 0600)
}

// getCacheFilePath returns the path for the token cache file.
func getCacheFilePath() (string, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cacheDir, "calbar", "msal_token_cache.json"), nil
}
