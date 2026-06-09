package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/doyensec/safeurl"
	"golang.org/x/oauth2"
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
}

// OIDCHandler coordinates OIDC authentication login redirects and callback parsing.
type OIDCHandler struct {
	Settings OIDCSettings
	sessions sync.Map // stores active state parameters mapped to oidcSession
}

type oidcSession struct {
	State             string
	CodeVerifier      string
	SSORedirectURI    string
	Mobile            bool
	MobileRedirectURI string
}

var safeClient *http.Client

func init() {
	config := safeurl.GetConfigBuilder().Build()
	safeClient = safeurl.Client(config).Client
}

// NewOIDCHandler constructs an OIDC authentication handler.
func NewOIDCHandler(settings OIDCSettings) *OIDCHandler {
	return &OIDCHandler{
		Settings: settings,
	}
}

// HandleLogin redirects the browser to the OIDC authorization endpoint.
func (h *OIDCHandler) HandleLogin(w http.ResponseWriter, r *http.Request) {
	ctx := oidc.ClientContext(r.Context(), safeClient)

	// PORT: Perform discovery dynamically using the IssuerURL and safe HTTP client.
	provider, err := oidc.NewProvider(ctx, h.Settings.IssuerURL)
	if err != nil {
		log.Printf("[OidcAuth] Discovery failed: %v", err)
		http.Error(w, "Failed to discover OIDC provider", http.StatusInternalServerError)
		return
	}

	isMobile := r.URL.Query().Get("response_type") == "code" ||
		r.URL.Query().Get("redirect_uri") != "" ||
		r.URL.Query().Get("code_challenge") != ""

	respType := r.URL.Query().Get("response_type")
	if respType != "" && respType != "code" {
		log.Printf("[OidcAuth] OIDC Invalid response_type=%s", respType)
		http.Error(w, "Invalid response_type, only code supported", http.StatusBadRequest)
		return
	}

	if isMobile {
		mobileRedirect := r.URL.Query().Get("redirect_uri")
		if mobileRedirect == "" || !h.isValidRedirectURI(mobileRedirect) {
			log.Printf("[OidcAuth] Invalid redirect_uri=%s", mobileRedirect)
			http.Error(w, "Invalid redirect_uri", http.StatusBadRequest)
			return
		}
	} else {
		if r.URL.Query().Get("state") != "" {
			log.Printf("[OidcAuth] Invalid state - not allowed on web openid flow")
			http.Error(w, "Invalid state, not allowed on web flow", http.StatusBadRequest)
			return
		}
	}

	state := r.URL.Query().Get("state")
	if state == "" {
		state = generateRandomString(16)
	}

	var codeChallenge, codeChallengeMethod, codeVerifier string
	if isMobile {
		codeChallenge = r.URL.Query().Get("code_challenge")
		if codeChallenge == "" {
			http.Error(w, "code_challenge required for mobile flow (PKCE)", http.StatusBadRequest)
			return
		}
		method := r.URL.Query().Get("code_challenge_method")
		if method != "" && method != "S256" {
			http.Error(w, "Only S256 code_challenge_method method supported", http.StatusBadRequest)
			return
		}
		codeChallengeMethod = "S256"
	} else {
		verifier := generateVerifier()
		codeVerifier = verifier
		codeChallenge = s256Challenge(verifier)
		codeChallengeMethod = "S256"
	}

	ssoRedirectURI := h.getRedirectURL(r, isMobile)

	sess := &oidcSession{
		State:             state,
		CodeVerifier:      codeVerifier,
		SSORedirectURI:    ssoRedirectURI,
		Mobile:            isMobile,
		MobileRedirectURI: r.URL.Query().Get("redirect_uri"),
	}
	h.sessions.Store(state, sess)

	oauth2Config := &oauth2.Config{
		ClientID:     h.Settings.ClientID,
		ClientSecret: h.Settings.ClientSecret,
		Endpoint:     provider.Endpoint(),
		RedirectURL:  ssoRedirectURI,
		Scopes:       h.getScopes(),
	}

	authURL := oauth2Config.AuthCodeURL(state,
		oauth2.AccessTypeOnline,
		oauth2.SetAuthURLParam("code_challenge", codeChallenge),
		oauth2.SetAuthURLParam("code_challenge_method", codeChallengeMethod),
	)

	http.Redirect(w, r, authURL, http.StatusFound)
}

// HandleCallback processes authorization code redirection, retrieves tokens, and validates claims.
// Returns the OIDC user details map containing decoded user claims and the raw id_token.
func (h *OIDCHandler) HandleCallback(w http.ResponseWriter, r *http.Request) (map[string]interface{}, error) {
	ctx := oidc.ClientContext(r.Context(), safeClient)

	state := r.URL.Query().Get("state")
	code := r.URL.Query().Get("code")

	if state == "" {
		return nil, fmt.Errorf("missing state parameter")
	}

	// PORT: Handle intermediate mobile-redirect if requested path matches /mobile-redirect.
	if strings.HasSuffix(r.URL.Path, "/mobile-redirect") {
		val, ok := h.sessions.Load(state)
		if !ok {
			log.Printf("[OidcAuth] State parameter mismatch: %s", state)
			http.Error(w, "State parameter mismatch", http.StatusBadRequest)
			return nil, fmt.Errorf("state parameter mismatch")
		}
		sess := val.(*oidcSession)

		if sess.MobileRedirectURI == "" {
			log.Printf("[OidcAuth] No redirect URI for state: %s", state)
			http.Error(w, "No redirect URI", http.StatusBadRequest)
			return nil, fmt.Errorf("no redirect URI")
		}

		redirectURL := fmt.Sprintf("%s?code=%s&state=%s", sess.MobileRedirectURI, url.QueryEscape(code), url.QueryEscape(state))
		http.Redirect(w, r, redirectURL, http.StatusFound)
		return nil, nil
	}

	val, ok := h.sessions.Load(state)
	var codeVerifier string
	var ssoRedirectURI string
	var isMobile bool

	if ok {
		sess := val.(*oidcSession)
		codeVerifier = sess.CodeVerifier
		ssoRedirectURI = sess.SSORedirectURI
		isMobile = sess.Mobile
		h.sessions.Delete(state)
	} else {
		codeVerifier = r.URL.Query().Get("code_verifier")
		isMobile = codeVerifier != ""
		ssoRedirectURI = h.getRedirectURL(r, isMobile)
	}

	if clientVerifier := r.URL.Query().Get("code_verifier"); clientVerifier != "" {
		codeVerifier = clientVerifier
	}

	provider, err := oidc.NewProvider(ctx, h.Settings.IssuerURL)
	if err != nil {
		return nil, fmt.Errorf("failed to discover OIDC provider: %w", err)
	}

	oauth2Config := &oauth2.Config{
		ClientID:     h.Settings.ClientID,
		ClientSecret: h.Settings.ClientSecret,
		Endpoint:     provider.Endpoint(),
		RedirectURL:  ssoRedirectURI,
		Scopes:       h.getScopes(),
	}

	var exchangeOpts []oauth2.AuthCodeOption
	if codeVerifier != "" {
		exchangeOpts = append(exchangeOpts, oauth2.VerifierOption(codeVerifier))
	}

	token, err := oauth2Config.Exchange(ctx, code, exchangeOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange token: %w", err)
	}

	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		return nil, fmt.Errorf("no id_token found in token response")
	}

	verifier := provider.Verifier(&oidc.Config{ClientID: h.Settings.ClientID})
	idToken, err := verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return nil, fmt.Errorf("failed to verify ID token: %w", err)
	}

	var claims map[string]interface{}
	if err := idToken.Claims(&claims); err != nil {
		return nil, fmt.Errorf("failed to parse claims: %w", err)
	}

	// PORT: Attempt to fetch userinfo from the provider's userinfo endpoint if supported.
	if userInfo, err := provider.UserInfo(ctx, oauth2.StaticTokenSource(token)); err == nil {
		var userInfoClaims map[string]interface{}
		if err := userInfo.Claims(&userInfoClaims); err == nil {
			for k, v := range userInfoClaims {
				claims[k] = v
			}
		}
	}

	if sub, ok := claims["sub"].(string); !ok || sub == "" {
		return nil, fmt.Errorf("invalid claims, no sub")
	}

	if h.Settings.GroupClaim != "" {
		if _, ok := claims[h.Settings.GroupClaim]; !ok {
			return nil, fmt.Errorf("group claim %s not found in user claims", h.Settings.GroupClaim)
		}
	}

	claims["id_token"] = rawIDToken

	return claims, nil
}

func (h *OIDCHandler) isValidRedirectURI(uri string) bool {
	for _, allowed := range h.Settings.MobileRedirectURIs {
		if allowed == "*" || allowed == uri {
			return true
		}
	}
	return false
}

func (h *OIDCHandler) getScopes() []string {
	scopes := []string{"openid", "profile", "email"}
	if h.Settings.GroupClaim != "" {
		scopes = append(scopes, h.Settings.GroupClaim)
	}
	if h.Settings.AdvancedPermsClaim != "" {
		scopes = append(scopes, h.Settings.AdvancedPermsClaim)
	}
	return scopes
}

func (h *OIDCHandler) getRedirectURL(r *http.Request, isMobile bool) string {
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}

	host := r.Host
	if xfh := r.Header.Get("X-Forwarded-Host"); xfh != "" {
		host = xfh
	}

	var subfolder string
	if h.Settings.SubfolderForRedirectURLs {
		path := r.URL.Path
		if idx := strings.Index(path, "/auth/openid"); idx != -1 {
			subfolder = path[:idx]
		}
	}

	suffix := "/auth/openid/callback"
	if isMobile {
		suffix = "/auth/openid/mobile-redirect"
	}

	return fmt.Sprintf("%s://%s%s%s", scheme, host, subfolder, suffix)
}

func generateRandomString(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

func generateVerifier() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func s256Challenge(verifier string) string {
	sha := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sha[:])
}
