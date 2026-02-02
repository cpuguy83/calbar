// Package auth provides authentication for Microsoft services via the identity broker.
package auth

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
)

const (
	// D-Bus service details for Microsoft Identity Broker
	brokerService   = "com.microsoft.identity.broker1"
	brokerPath      = "/com/microsoft/identity/broker1"
	brokerInterface = "com.microsoft.identity.Broker1"

	// Broker protocol version - must be "0.0" for current broker
	brokerProtocolVersion = "0.0"

	// Edge browser client ID - works for SSO and token acquisition
	DefaultClientID = "d7b530a4-7680-4c23-a8bf-c52c121d2e87"

	// Default redirect URI for native apps
	DefaultRedirectURI = "https://login.microsoftonline.com/common/oauth2/nativeclient"

	// Default authority (used when no tenant-specific realm available)
	DefaultAuthority = "https://login.microsoftonline.com/common"

	// Authorization type for token acquisition
	AuthTypeToken = 1
)

var (
	ErrBrokerNotAvailable = errors.New("microsoft identity broker not available")
	ErrNoAccounts         = errors.New("no accounts found in broker")
	ErrAuthFailed         = errors.New("authentication failed")
)

// Token represents an OAuth2 access token.
type Token struct {
	AccessToken string
	ExpiresOn   time.Time
	AccountID   string
}

// Broker is a client for the Microsoft Identity Broker D-Bus service.
type Broker struct {
	conn      *dbus.Conn
	clientID  string
	scopes    []string
	sessionID string

	mu            sync.Mutex
	cachedToken   *Token
	cachedAccount map[string]any // Full account object from broker
}

// NewBroker creates a new broker client.
func NewBroker(clientID string, scopes []string) *Broker {
	if clientID == "" {
		clientID = DefaultClientID
	}
	return &Broker{
		clientID:  clientID,
		scopes:    scopes,
		sessionID: generateSessionID(),
	}
}

// generateSessionID creates a UUID v4 for the session.
func generateSessionID() string {
	b := make([]byte, 16)
	rand.Read(b)
	// Set version (4) and variant (RFC 4122)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// connect establishes a D-Bus connection if not already connected.
func (b *Broker) connect() error {
	if b.conn != nil {
		return nil
	}

	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return fmt.Errorf("connect to session bus: %w", err)
	}
	b.conn = conn
	return nil
}

// Close closes the D-Bus connection.
func (b *Broker) Close() error {
	if b.conn != nil {
		return b.conn.Close()
	}
	return nil
}

// IsAvailable checks if the broker is available on D-Bus.
func (b *Broker) IsAvailable(ctx context.Context) bool {
	if err := b.connect(); err != nil {
		return false
	}

	// Try to get broker version as a health check
	_, err := b.getLinuxBrokerVersion(ctx)
	return err == nil
}

// GetToken acquires an access token, using cached token if valid.
func (b *Broker) GetToken(ctx context.Context) (*Token, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Return cached token if still valid (with 5 minute buffer)
	if b.cachedToken != nil && time.Now().Add(5*time.Minute).Before(b.cachedToken.ExpiresOn) {
		slog.Debug("using cached token", "expires", b.cachedToken.ExpiresOn)
		return b.cachedToken, nil
	}

	if err := b.connect(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBrokerNotAvailable, err)
	}

	// Try silent auth first if we have a cached account
	if b.cachedAccount != nil {
		token, err := b.acquireTokenSilently(ctx, b.cachedAccount)
		if err == nil {
			b.cachedToken = token
			return token, nil
		}
		slog.Debug("silent auth with cached account failed", "error", err)
	}

	// Get accounts and try silent auth with first available
	accounts, err := b.getAccounts(ctx)
	if err != nil {
		return nil, fmt.Errorf("get accounts: %w", err)
	}
	if len(accounts) == 0 {
		return nil, ErrNoAccounts
	}

	for _, acct := range accounts {
		token, err := b.acquireTokenSilently(ctx, acct)
		if err == nil {
			b.cachedAccount = acct
			b.cachedToken = token
			return token, nil
		}
		username, _ := acct["username"].(string)
		slog.Debug("silent auth failed for account", "username", username, "error", err)
	}

	return nil, fmt.Errorf("%w: all accounts failed silent auth", ErrAuthFailed)
}

// callBroker makes a D-Bus call to the broker.
// Signature: (protocolVersion, sessionId, requestJson) -> responseJson
func (b *Broker) callBroker(ctx context.Context, method string, request any) (map[string]any, error) {
	reqJSON, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	slog.Debug("calling broker", "method", method)

	obj := b.conn.Object(brokerService, brokerPath)
	call := obj.CallWithContext(ctx, brokerInterface+"."+method, 0,
		brokerProtocolVersion, b.sessionID, string(reqJSON))

	if call.Err != nil {
		return nil, fmt.Errorf("dbus call %s: %w", method, call.Err)
	}

	var respStr string
	if err := call.Store(&respStr); err != nil {
		return nil, fmt.Errorf("store response: %w", err)
	}

	slog.Debug("broker response", "method", method)

	var resp map[string]any
	if err := json.Unmarshal([]byte(respStr), &resp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	// Check for broker errors
	if errObj, ok := resp["error"].(map[string]any); ok {
		errJSON, _ := json.Marshal(errObj)
		return nil, fmt.Errorf("broker error: %s", errJSON)
	}
	if errMsg, ok := resp["error"].(string); ok && errMsg != "" {
		return nil, fmt.Errorf("broker error: %s", errMsg)
	}

	return resp, nil
}

// getLinuxBrokerVersion gets the broker version (used as health check).
func (b *Broker) getLinuxBrokerVersion(ctx context.Context) (string, error) {
	resp, err := b.callBroker(ctx, "getLinuxBrokerVersion", map[string]any{})
	if err != nil {
		return "", err
	}
	version, _ := resp["linuxBrokerVersion"].(string)
	return version, nil
}

// getAccounts retrieves cached accounts from the broker.
func (b *Broker) getAccounts(ctx context.Context) ([]map[string]any, error) {
	req := map[string]any{
		"clientId":    b.clientID,
		"redirectUri": DefaultRedirectURI,
	}

	resp, err := b.callBroker(ctx, "getAccounts", req)
	if err != nil {
		return nil, err
	}

	// Extract accounts from response
	accounts, ok := resp["accounts"].([]any)
	if !ok {
		return []map[string]any{}, nil
	}

	result := make([]map[string]any, 0, len(accounts))
	for _, acc := range accounts {
		if accMap, ok := acc.(map[string]any); ok {
			result = append(result, accMap)
		}
	}

	return result, nil
}

// getAuthParameters builds the authParameters object for token requests.
func (b *Broker) getAuthParameters(account map[string]any) map[string]any {
	// Use tenant-specific authority if we have realm info
	authority := DefaultAuthority
	if account != nil {
		if realm, ok := account["realm"].(string); ok && realm != "" {
			authority = "https://login.microsoftonline.com/" + realm
		}
	}

	scopes := b.scopes
	if len(scopes) == 0 {
		scopes = []string{"https://graph.microsoft.com/.default"}
	}

	params := map[string]any{
		"account":           account,
		"authority":         authority,
		"authorizationType": AuthTypeToken,
		"clientId":          b.clientID,
		"redirectUri":       DefaultRedirectURI,
		"requestedScopes":   scopes,
	}

	// Add username if available
	if account != nil {
		if username, ok := account["username"].(string); ok {
			params["username"] = username
		}
	}

	return params
}

// acquireTokenSilently gets a token without user interaction.
func (b *Broker) acquireTokenSilently(ctx context.Context, account map[string]any) (*Token, error) {
	req := map[string]any{
		"authParameters": b.getAuthParameters(account),
	}

	resp, err := b.callBroker(ctx, "acquireTokenSilently", req)
	if err != nil {
		return nil, err
	}

	// Extract token from response - can be at top level or nested
	accessToken, _ := resp["accessToken"].(string)
	if accessToken == "" {
		// Try nested brokerTokenResponse
		if tokenResp, ok := resp["brokerTokenResponse"].(map[string]any); ok {
			accessToken, _ = tokenResp["accessToken"].(string)
			if errObj, ok := tokenResp["error"].(map[string]any); ok {
				errJSON, _ := json.Marshal(errObj)
				return nil, fmt.Errorf("token response error: %s", errJSON)
			}
		}
	}

	if accessToken == "" {
		return nil, errors.New("no access token in response")
	}

	// Parse expiration - try different field names and formats
	var expiresOn time.Time
	if exp, ok := resp["expiresOn"].(float64); ok {
		expiresOn = time.Unix(int64(exp), 0)
	} else if exp, ok := resp["expiresOn"].(int64); ok {
		expiresOn = time.Unix(exp, 0)
	} else {
		// Default to 1 hour from now if not specified
		expiresOn = time.Now().Add(time.Hour)
	}

	// Get account ID from response or account object
	accountID, _ := resp["accountId"].(string)
	if accountID == "" {
		accountID, _ = account["localAccountId"].(string)
	}

	return &Token{
		AccessToken: accessToken,
		ExpiresOn:   expiresOn,
		AccountID:   accountID,
	}, nil
}
