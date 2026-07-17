package auth

import (
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/doyensec/safeurl"
)

// OIDCSettings holds configuration for OIDC authentication.
type OIDCSettings struct {
	IssuerURL                string   `json:"authOpenIDIssuerURL"`
	ClientID                 string   `json:"authOpenIDClientID"`
	ClientSecret             string   `json:"authOpenIDClientSecret"`
	RedirectURL              string   `json:"authOpenIDRedirectURL"`
	AutoRegister             bool     `json:"authOpenIDAutoRegister"`
	MatchExistingBy          string   `json:"authOpenIDMatchExistingBy"` // "email" or "username"
	MobileRedirectURIs       []string `json:"authOpenIDMobileRedirectURIs"`
	GroupClaim               string   `json:"authOpenIDGroupClaim"`
	AdvancedPermsClaim       string   `json:"authOpenIDAdvancedPermsClaim"`
	SubfolderForRedirectURLs bool     `json:"authOpenIDSubfolderForRedirectURLs"`
	AuthorizationURL         string   `json:"authOpenIDAuthorizationURL"`
	TokenURL                 string   `json:"authOpenIDTokenURL"`
	UserInfoURL              string   `json:"authOpenIDUserInfoURL"`
	JwksURL                  string   `json:"authOpenIDJwksURL"`
	LogoutURL                string   `json:"authOpenIDLogoutURL"`
	TokenSigningAlgorithm    string   `json:"authOpenIDTokenSigningAlgorithm"`
}

// OIDCHandler coordinates OIDC authentication login redirects and callback parsing.
type OIDCHandler struct {
	mu       sync.RWMutex
	Settings OIDCSettings
	sessions sync.Map // stores active state parameters mapped to oidcSession
	client   *http.Client
}

type oidcSession struct {
	State             string
	CodeVerifier      string
	SSORedirectURI    string
	Mobile            bool
	MobileRedirectURI string
	CreatedAt         time.Time
}

var safeClient *http.Client

func init() {
	config := safeurl.GetConfigBuilder().Build()
	safeClient = safeurl.Client(config).Client
}

// NewOIDCHandler constructs an OIDC authentication handler.
func NewOIDCHandler(settings OIDCSettings, client *http.Client) *OIDCHandler {
	h := &OIDCHandler{
		Settings: settings,
		client:   client,
	}
	go h.startCleanupLoop()
	return h
}

func (h *OIDCHandler) startCleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	for range ticker.C {
		h.sessions.Range(func(key, value interface{}) bool {
			sess, ok := value.(*oidcSession)
			if ok && time.Since(sess.CreatedAt) > 10*time.Minute {
				h.sessions.Delete(key)
			}
			return true
		})
	}
}

// getClient returns the injected http.Client or falls back to the safeClient.
func (h *OIDCHandler) getClient() *http.Client {
	if h.client != nil {
		return h.client
	}
	if os.Getenv("BYPASS_SAFEURL") == "true" {
		return http.DefaultClient
	}
	return safeClient
}

// getSettings thread-safely returns a copy of the handler's settings.
func (h *OIDCHandler) getSettings() OIDCSettings {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.Settings
}

// UpdateSettings thread-safely updates the handler's settings.
func (h *OIDCHandler) UpdateSettings(settings OIDCSettings) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.Settings = settings
}
