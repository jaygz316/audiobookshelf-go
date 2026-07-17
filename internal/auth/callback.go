package auth

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	log "audiobookshelf/internal/logger"
)

// HandleCallback processes authorization code redirection, retrieves tokens, and validates claims.
// Returns the OIDC user details map containing decoded user claims and the raw id_token.
func (h *OIDCHandler) HandleCallback(w http.ResponseWriter, r *http.Request) (map[string]interface{}, error) {
	ctx := oidc.ClientContext(r.Context(), h.getClient())
	settings := h.getSettings()

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

	var provider *oidc.Provider
	var endpoint oauth2.Endpoint
	var err error

	if settings.AuthorizationURL != "" && settings.TokenURL != "" {
		endpoint = oauth2.Endpoint{
			AuthURL:  settings.AuthorizationURL,
			TokenURL: settings.TokenURL,
		}
	} else {
		provider, err = oidc.NewProvider(ctx, settings.IssuerURL)
		if err != nil {
			return nil, fmt.Errorf("failed to discover OIDC provider: %w", err)
		}
		endpoint = provider.Endpoint()
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
		if codeVerifier == "" {
			return nil, fmt.Errorf("state parameter mismatch")
		}
		isMobile = true
		ssoRedirectURI = h.getRedirectURL(r, isMobile)
	}

	if clientVerifier := r.URL.Query().Get("code_verifier"); clientVerifier != "" {
		codeVerifier = clientVerifier
	}

	oauth2Config := &oauth2.Config{
		ClientID:     settings.ClientID,
		ClientSecret: settings.ClientSecret,
		Endpoint:     endpoint,
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

	var verifier *oidc.IDTokenVerifier
	if settings.JwksURL != "" {
		keySet := oidc.NewRemoteKeySet(ctx, settings.JwksURL)
		config := &oidc.Config{ClientID: settings.ClientID}
		if settings.TokenSigningAlgorithm != "" {
			config.SupportedSigningAlgs = []string{settings.TokenSigningAlgorithm}
		}
		verifier = oidc.NewVerifier(settings.IssuerURL, keySet, config)
	} else {
		if provider == nil {
			provider, err = oidc.NewProvider(ctx, settings.IssuerURL)
			if err != nil {
				return nil, fmt.Errorf("failed to verify ID token issuer: %w", err)
			}
		}
		verifier = provider.Verifier(&oidc.Config{ClientID: settings.ClientID})
	}

	idToken, err := verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return nil, fmt.Errorf("failed to verify ID token: %w", err)
	}

	var claims map[string]interface{}
	if err := idToken.Claims(&claims); err != nil {
		return nil, fmt.Errorf("failed to parse claims: %w", err)
	}

	// PORT: Attempt to fetch userinfo from the provider's userinfo endpoint if supported.
	var userInfoClaims map[string]interface{}
	if settings.UserInfoURL != "" {
		req, err := http.NewRequestWithContext(ctx, "GET", settings.UserInfoURL, nil)
		if err == nil {
			req.Header.Set("Authorization", "Bearer "+token.AccessToken)
			resp, err := h.getClient().Do(req)
			if err == nil {
				defer resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					_ = json.NewDecoder(resp.Body).Decode(&userInfoClaims)
				}
			}
		}
	} else if provider != nil {
		if userInfo, err := provider.UserInfo(ctx, oauth2.StaticTokenSource(token)); err == nil {
			_ = userInfo.Claims(&userInfoClaims)
		}
	}

	if userInfoClaims != nil {
		for k, v := range userInfoClaims {
			claims[k] = v
		}
	}

	if sub, ok := claims["sub"].(string); !ok || sub == "" {
		return nil, fmt.Errorf("invalid claims, no sub")
	}

	if settings.GroupClaim != "" {
		gVal, ok := claims[settings.GroupClaim]
		if !ok {
			return nil, fmt.Errorf("group claim %s not found in user claims", settings.GroupClaim)
		}

		highestRole, err := mapGroupClaims(gVal)
		if err != nil {
			return nil, err
		}
		claims["mapped_role"] = highestRole
	}

	claims["id_token"] = rawIDToken

	return claims, nil
}
