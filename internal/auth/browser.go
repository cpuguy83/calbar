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

// BrowserAuth provides authentication via MSAL interactive browser auth.
// This is used as a fallback when the broker is not available.
type BrowserAuth struct {
	client   public.Client
	clientID string
	scopes   []string

	mu          sync.Mutex
	cachedToken *Token
	account     public.Account
}

// NewBrowserAuth creates a new interactive browser auth client.
func NewBrowserAuth(clientID string, scopes []string) (*BrowserAuth, error) {
	if clientID == "" {
		clientID = DefaultBrowserClientID
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

	return &BrowserAuth{
		client:   client,
		clientID: clientID,
		scopes:   scopes,
	}, nil
}

// GetToken acquires an access token, using cached token if valid.
func (b *BrowserAuth) GetToken(ctx context.Context) (*Token, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Return cached token if still valid
	if b.cachedToken != nil && time.Now().Add(5*time.Minute).Before(b.cachedToken.ExpiresOn) {
		return b.cachedToken, nil
	}

	// Try to get accounts from cache
	accounts, err := b.client.Accounts(ctx)
	if err != nil {
		slog.Debug("could not get cached accounts", "error", err)
	}

	// Try silent auth with cached accounts
	for _, acct := range accounts {
		result, err := b.client.AcquireTokenSilent(ctx, b.scopes, public.WithSilentAccount(acct))
		if err == nil {
			b.account = acct
			b.cachedToken = &Token{
				AccessToken: result.AccessToken,
				ExpiresOn:   result.ExpiresOn,
				AccountID:   acct.HomeAccountID,
			}
			return b.cachedToken, nil
		}
		slog.Debug("silent auth failed for account", "account", acct.PreferredUsername, "error", err)
	}

	// Fall back to interactive browser auth.
	slog.Info("no cached credentials, starting interactive browser auth")
	token, err := b.acquireTokenInteractive(ctx)
	if err != nil {
		return nil, err
	}

	b.cachedToken = token
	return token, nil
}

// acquireTokenInteractive performs browser-based interactive auth.
func (b *BrowserAuth) acquireTokenInteractive(ctx context.Context) (*Token, error) {
	result, err := b.client.AcquireTokenInteractive(ctx, b.scopes)
	if err != nil {
		return nil, fmt.Errorf("interactive browser auth: %w", err)
	}

	b.account = result.Account

	return &Token{
		AccessToken: result.AccessToken,
		ExpiresOn:   result.ExpiresOn,
		AccountID:   result.Account.HomeAccountID,
	}, nil
}

// Close is a no-op for browser auth.
func (b *BrowserAuth) Close() error {
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
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}

	return os.WriteFile(t.path, data, 0o600)
}

// getCacheFilePath returns the path for the token cache file.
func getCacheFilePath() (string, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cacheDir, "calbar", "msal_token_cache.json"), nil
}
