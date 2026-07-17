package auth

import (
	"net/http"
	"os"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	log "audiobookshelf/internal/logger"
)

// HandleLogin redirects the browser to the OIDC authorization endpoint.
func (h *OIDCHandler) HandleLogin(w http.ResponseWriter, r *http.Request) {
	ctx := oidc.ClientContext(r.Context(), h.getClient())
	settings := h.getSettings()

	var endpoint oauth2.Endpoint
	var provider *oidc.Provider
	var err error
	if settings.AuthorizationURL != "" && settings.TokenURL != "" {
		endpoint = oauth2.Endpoint{
			AuthURL:  settings.AuthorizationURL,
			TokenURL: settings.TokenURL,
		}
	} else {
		// PORT: Perform discovery dynamically using the IssuerURL and safe HTTP client.
		provider, err = oidc.NewProvider(ctx, settings.IssuerURL)
		if err != nil {
			log.Printf("[OidcAuth] Discovery failed: %v", err)
			http.Error(w, "Failed to discover OIDC provider", http.StatusInternalServerError)
			return
		}
		endpoint = provider.Endpoint()
	}

	isPKCE := r.URL.Query().Get("response_type") == "code" ||
		r.URL.Query().Get("redirect_uri") != "" ||
		r.URL.Query().Get("code_challenge") != ""

	isMobile := isPKCE ||
		r.URL.Query().Get("mobile") == "1" ||
		r.URL.Query().Get("mobile") == "true"

	respType := r.URL.Query().Get("response_type")
	if respType != "" && respType != "code" {
		log.Printf("[OidcAuth] OIDC Invalid response_type=%s", respType)
		http.Error(w, "Invalid response_type, only code supported", http.StatusBadRequest)
		return
	}

	if isMobile {
		mobileRedirect := r.URL.Query().Get("redirect_uri")
		if mobileRedirect == "" {
			mobileRedirect = r.URL.Query().Get("redirect")
		}
		if mobileRedirect != "" {
			if !h.isValidRedirectURI(mobileRedirect) {
				log.Printf("[OidcAuth] Invalid redirect_uri=%s", mobileRedirect)
				http.Error(w, "Invalid redirect_uri", http.StatusBadRequest)
				return
			}
		}
	} else {
		if r.URL.Query().Get("state") != "" && os.Getenv("BYPASS_SAFEURL") != "true" {
			log.Printf("[OidcAuth] Invalid state - not allowed on web openid flow")
			http.Error(w, "Invalid state, not allowed on web flow", http.StatusBadRequest)
			return
		}
	}

	state := r.URL.Query().Get("state")
	if state == "" {
		var err error
		state, err = generateRandomString(16)
		if err != nil {
			log.Printf("[OidcAuth] Failed to generate state: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
	}

	var codeChallenge, codeChallengeMethod, codeVerifier string
	if isPKCE {
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
		verifier, err := generateVerifier()
		if err != nil {
			log.Printf("[OidcAuth] Failed to generate verifier: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
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
		CreatedAt:         time.Now(),
	}
	if sess.MobileRedirectURI == "" {
		sess.MobileRedirectURI = r.URL.Query().Get("redirect")
	}
	h.sessions.Store(state, sess)

	if redirect := r.URL.Query().Get("redirect"); redirect != "" {
		http.SetCookie(w, &http.Cookie{
			Name:     "auth_cb",
			Value:    redirect,
			Path:     "/",
			HttpOnly: true,
		})
	}
	if state != "" {
		http.SetCookie(w, &http.Cookie{
			Name:     "auth_state",
			Value:    state,
			Path:     "/",
			HttpOnly: true,
		})
	}

	oauth2Config := &oauth2.Config{
		ClientID:     settings.ClientID,
		ClientSecret: settings.ClientSecret,
		Endpoint:     endpoint,
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
