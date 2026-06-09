# Package internal/auth

This package implements external OIDC authentication.

## Go Signatures

```go
package auth

import (
	"context"
	"net/http"
)

type OIDCSettings struct {
	IssuerURL                   string   `json:"authOpenIDIssuerURL"`
	ClientID                    string   `json:"authOpenIDClientID"`
	ClientSecret                string   `json:"authOpenIDClientSecret"`
	RedirectURL                 string   `json:"authOpenIDRedirectURL"`
	AutoRegister                bool     `json:"authOpenIDAutoRegister"`
	MatchExistingBy             string   `json:"authOpenIDMatchExistingBy"` // "email" or "username"
	MobileRedirectURIs          []string `json:"authOpenIDMobileRedirectURIs"`
	GroupClaim                  string   `json:"authOpenIDGroupClaim"`
	AdvancedPermsClaim          string   `json:"authOpenIDAdvancedPermsClaim"`
	SubfolderForRedirectURLs    bool     `json:"authOpenIDSubfolderForRedirectURLs"`
}

type OIDCHandler struct {
	Settings OIDCSettings
}

// NewOIDCHandler constructs an OIDC authentication handler.
func NewOIDCHandler(settings OIDCSettings) *OIDCHandler

// HandleLogin redirects the browser to the OIDC authorization endpoint.
func (h *OIDCHandler) HandleLogin(w http.ResponseWriter, r *http.Request)

// HandleCallback processes authorization code redirection, retrieves tokens, and validates claims.
// Returns the OIDC user details.
func (h *OIDCHandler) HandleCallback(w http.ResponseWriter, r *http.Request) (map[string]interface{}, error)
```

## Behavioral Notes
- **HandleLogin**: Performs discovery of the OIDC provider configurations using `IssuerURL` and initiates the authorization flow.
- **HandleCallback**: Exchanged authorization code for token, validates signature/expiry of token, and extracts user details (email, username, groups) to facilitate login/auto-registration.
