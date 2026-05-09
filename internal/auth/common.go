package auth

import "time"

const (
	// DefaultBrokerClientID is the Edge browser client ID, used with Microsoft Identity Broker SSO.
	DefaultBrokerClientID = "d7b530a4-7680-4c23-a8bf-c52c121d2e87"

	// DefaultBrowserClientID is the Microsoft Outlook public client ID.
	// Its app registration supports loopback redirects used by MSAL browser auth.
	DefaultBrowserClientID = "d3590ed6-52b3-4102-aeff-aad2292ab01c"

	// DefaultClientID is kept as the broker default for existing internal callers.
	DefaultClientID = DefaultBrokerClientID

	// DefaultRedirectURI is the native-app redirect URI used by Microsoft broker requests.
	DefaultRedirectURI = "https://login.microsoftonline.com/common/oauth2/nativeclient"

	// DefaultAuthority is used when no tenant-specific realm is available.
	DefaultAuthority = "https://login.microsoftonline.com/common"
)

// Token represents an OAuth2 access token.
type Token struct {
	AccessToken string
	ExpiresOn   time.Time
	AccountID   string
}
