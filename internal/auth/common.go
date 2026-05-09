package auth

import "time"

const (
	// DefaultClientID is the Edge browser client ID, used for SSO and token acquisition.
	DefaultClientID = "d7b530a4-7680-4c23-a8bf-c52c121d2e87"

	// DefaultRedirectURI is the native-app redirect URI used by Microsoft auth flows.
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
