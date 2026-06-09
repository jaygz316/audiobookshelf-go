package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var (
	testPrivKey *rsa.PrivateKey
	testPubKey  *rsa.PublicKey
)

func init() {
	// Bypass loopback blocking in safeurl for testing
	safeClient = http.DefaultClient

	// Generate RSA keypair for JWT signing
	var err error
	testPrivKey, err = rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	testPubKey = &testPrivKey.PublicKey
}

func startMockOIDCServer(
	t *testing.T,
	customTokenHandler func(w http.ResponseWriter, r *http.Request, serverURL string),
	customUserInfoHandler func(w http.ResponseWriter, r *http.Request),
) *httptest.Server {
	mux := http.NewServeMux()
	var serverURL string

	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		config := map[string]interface{}{
			"issuer":                                serverURL,
			"authorization_endpoint":                serverURL + "/auth",
			"token_endpoint":                        serverURL + "/token",
			"jwks_uri":                              serverURL + "/keys",
			"userinfo_endpoint":                     serverURL + "/userinfo",
			"response_types_supported":              []string{"code"},
			"subject_types_supported":               []string{"public"},
			"id_token_signing_alg_values_supported": []string{"RS256"},
		}
		json.NewEncoder(w).Encode(config)
	})

	mux.HandleFunc("/keys", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		eBytes := big.NewInt(int64(testPubKey.E)).Bytes()
		eStr := base64.RawURLEncoding.EncodeToString(eBytes)
		nStr := base64.RawURLEncoding.EncodeToString(testPubKey.N.Bytes())

		jwk := map[string]interface{}{
			"kty": "RSA",
			"use": "sig",
			"alg": "RS256",
			"kid": "mock-key-id",
			"n":   nStr,
			"e":   eStr,
		}
		jwks := map[string]interface{}{
			"keys": []interface{}{jwk},
		}
		json.NewEncoder(w).Encode(jwks)
	})

	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		if customTokenHandler != nil {
			customTokenHandler(w, r, serverURL)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
			"iss":    serverURL,
			"sub":    "user123",
			"aud":    "test-client-id",
			"exp":    time.Now().Add(time.Hour).Unix(),
			"iat":    time.Now().Unix(),
			"email":  "user@example.com",
			"groups": []string{"admin", "users"},
		})
		token.Header["kid"] = "mock-key-id"
		signedToken, err := token.SignedString(testPrivKey)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}

		resp := map[string]interface{}{
			"access_token": "mock-access-token",
			"token_type":   "Bearer",
			"expires_in":   3600,
			"id_token":     signedToken,
		}
		json.NewEncoder(w).Encode(resp)
	})

	mux.HandleFunc("/userinfo", func(w http.ResponseWriter, r *http.Request) {
		if customUserInfoHandler != nil {
			customUserInfoHandler(w, r)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		authHeader := r.Header.Get("Authorization")
		if authHeader != "Bearer mock-access-token" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		resp := map[string]interface{}{
			"sub":    "user123",
			"email":  "user@example.com",
			"groups": []string{"admin", "users"},
		}
		json.NewEncoder(w).Encode(resp)
	})

	server := httptest.NewServer(mux)
	serverURL = server.URL
	return server
}

func TestHandleLogin_Web(t *testing.T) {
	server := startMockOIDCServer(t, nil, nil)
	defer server.Close()

	settings := OIDCSettings{
		IssuerURL:    server.URL,
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
	}
	h := NewOIDCHandler(settings)

	req := httptest.NewRequest("GET", "/auth/openid/login", nil)
	req.Host = "example.com"
	rec := httptest.NewRecorder()

	h.HandleLogin(rec, req)

	resp := rec.Result()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("expected status 302, got %d", resp.StatusCode)
	}

	locVal := resp.Header.Get("Location")
	if locVal == "" {
		t.Fatal("expected Location header")
	}

	loc, err := url.Parse(locVal)
	if err != nil {
		t.Fatalf("failed to parse Location: %v", err)
	}

	expectedAuthPrefix := server.URL + "/auth"
	if !strings.HasPrefix(locVal, expectedAuthPrefix) {
		t.Errorf("expected redirect to start with %s, got %s", expectedAuthPrefix, locVal)
	}

	q := loc.Query()
	if q.Get("client_id") != "test-client-id" {
		t.Errorf("expected client_id test-client-id, got %s", q.Get("client_id"))
	}
	if q.Get("response_type") != "code" {
		t.Errorf("expected response_type code, got %s", q.Get("response_type"))
	}
	if q.Get("code_challenge") == "" {
		t.Errorf("expected code_challenge to be present")
	}
	if q.Get("code_challenge_method") != "S256" {
		t.Errorf("expected code_challenge_method S256, got %s", q.Get("code_challenge_method"))
	}

	expectedRedirect := "http://example.com/auth/openid/callback"
	if q.Get("redirect_uri") != expectedRedirect {
		t.Errorf("expected redirect_uri %s, got %s", expectedRedirect, q.Get("redirect_uri"))
	}

	scope := q.Get("scope")
	for _, expectedScope := range []string{"openid", "profile", "email"} {
		if !strings.Contains(scope, expectedScope) {
			t.Errorf("expected scope to contain %s, got %s", expectedScope, scope)
		}
	}

	state := q.Get("state")
	if state == "" {
		t.Fatal("expected state parameter")
	}

	sessVal, ok := h.sessions.Load(state)
	if !ok {
		t.Fatal("expected session for state to be stored")
	}
	sess := sessVal.(*oidcSession)
	if sess.State != state {
		t.Errorf("expected stored state to match, got %s", sess.State)
	}
	if sess.CodeVerifier == "" {
		t.Errorf("expected stored CodeVerifier to be populated")
	}
	if sess.SSORedirectURI != expectedRedirect {
		t.Errorf("expected stored SSORedirectURI to be %s, got %s", expectedRedirect, sess.SSORedirectURI)
	}
	if sess.Mobile {
		t.Errorf("expected Mobile to be false")
	}
}

func TestHandleLogin_Web_WithState(t *testing.T) {
	server := startMockOIDCServer(t, nil, nil)
	defer server.Close()

	settings := OIDCSettings{
		IssuerURL:    server.URL,
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
	}
	h := NewOIDCHandler(settings)

	// In web flow, sending a state query parameter on login is an error
	req := httptest.NewRequest("GET", "/auth/openid/login?state=user-provided-state", nil)
	rec := httptest.NewRecorder()

	h.HandleLogin(rec, req)

	resp := rec.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", resp.StatusCode)
	}
}

func TestHandleLogin_Web_InvalidResponseType(t *testing.T) {
	server := startMockOIDCServer(t, nil, nil)
	defer server.Close()

	settings := OIDCSettings{
		IssuerURL:    server.URL,
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
	}
	h := NewOIDCHandler(settings)

	req := httptest.NewRequest("GET", "/auth/openid/login?response_type=token", nil)
	rec := httptest.NewRecorder()

	h.HandleLogin(rec, req)

	resp := rec.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", resp.StatusCode)
	}
}

func TestHandleLogin_Mobile_Success(t *testing.T) {
	server := startMockOIDCServer(t, nil, nil)
	defer server.Close()

	settings := OIDCSettings{
		IssuerURL:          server.URL,
		ClientID:           "test-client-id",
		ClientSecret:       "test-client-secret",
		MobileRedirectURIs: []string{"audiobookshelf://oauth", "another://uri"},
	}
	h := NewOIDCHandler(settings)

	reqURL := "/auth/openid/login?response_type=code&redirect_uri=audiobookshelf://oauth&code_challenge=challenge123&code_challenge_method=S256&state=mobile-state"
	req := httptest.NewRequest("GET", reqURL, nil)
	req.Host = "example.com"
	rec := httptest.NewRecorder()

	h.HandleLogin(rec, req)

	resp := rec.Result()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("expected status 302, got %d", resp.StatusCode)
	}

	locVal := resp.Header.Get("Location")
	loc, err := url.Parse(locVal)
	if err != nil {
		t.Fatalf("failed to parse Location: %v", err)
	}

	q := loc.Query()
	if q.Get("state") != "mobile-state" {
		t.Errorf("expected state mobile-state, got %s", q.Get("state"))
	}
	if q.Get("code_challenge") != "challenge123" {
		t.Errorf("expected code_challenge challenge123, got %s", q.Get("code_challenge"))
	}
	expectedRedirect := "http://example.com/auth/openid/mobile-redirect"
	if q.Get("redirect_uri") != expectedRedirect {
		t.Errorf("expected redirect_uri %s, got %s", expectedRedirect, q.Get("redirect_uri"))
	}

	sessVal, ok := h.sessions.Load("mobile-state")
	if !ok {
		t.Fatal("expected session for state to be stored")
	}
	sess := sessVal.(*oidcSession)
	if !sess.Mobile {
		t.Errorf("expected sess.Mobile to be true")
	}
	if sess.MobileRedirectURI != "audiobookshelf://oauth" {
		t.Errorf("expected MobileRedirectURI to be audiobookshelf://oauth, got %s", sess.MobileRedirectURI)
	}
}

func TestHandleLogin_Mobile_WildcardRedirect(t *testing.T) {
	server := startMockOIDCServer(t, nil, nil)
	defer server.Close()

	settings := OIDCSettings{
		IssuerURL:          server.URL,
		ClientID:           "test-client-id",
		ClientSecret:       "test-client-secret",
		MobileRedirectURIs: []string{"*"},
	}
	h := NewOIDCHandler(settings)

	reqURL := "/auth/openid/login?response_type=code&redirect_uri=arbitrary://uri&code_challenge=challenge123"
	req := httptest.NewRequest("GET", reqURL, nil)
	rec := httptest.NewRecorder()

	h.HandleLogin(rec, req)

	resp := rec.Result()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("expected status 302, got %d", resp.StatusCode)
	}
}

func TestHandleLogin_Mobile_InvalidRedirect(t *testing.T) {
	server := startMockOIDCServer(t, nil, nil)
	defer server.Close()

	settings := OIDCSettings{
		IssuerURL:          server.URL,
		ClientID:           "test-client-id",
		ClientSecret:       "test-client-secret",
		MobileRedirectURIs: []string{"audiobookshelf://oauth"},
	}
	h := NewOIDCHandler(settings)

	reqURL := "/auth/openid/login?response_type=code&redirect_uri=attacker://uri&code_challenge=challenge123"
	req := httptest.NewRequest("GET", reqURL, nil)
	rec := httptest.NewRecorder()

	h.HandleLogin(rec, req)

	resp := rec.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", resp.StatusCode)
	}
}

func TestHandleLogin_Mobile_MissingChallenge(t *testing.T) {
	server := startMockOIDCServer(t, nil, nil)
	defer server.Close()

	settings := OIDCSettings{
		IssuerURL:          server.URL,
		ClientID:           "test-client-id",
		ClientSecret:       "test-client-secret",
		MobileRedirectURIs: []string{"audiobookshelf://oauth"},
	}
	h := NewOIDCHandler(settings)

	reqURL := "/auth/openid/login?response_type=code&redirect_uri=audiobookshelf://oauth"
	req := httptest.NewRequest("GET", reqURL, nil)
	rec := httptest.NewRecorder()

	h.HandleLogin(rec, req)

	resp := rec.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", resp.StatusCode)
	}
}

func TestHandleLogin_Mobile_InvalidChallengeMethod(t *testing.T) {
	server := startMockOIDCServer(t, nil, nil)
	defer server.Close()

	settings := OIDCSettings{
		IssuerURL:          server.URL,
		ClientID:           "test-client-id",
		ClientSecret:       "test-client-secret",
		MobileRedirectURIs: []string{"audiobookshelf://oauth"},
	}
	h := NewOIDCHandler(settings)

	reqURL := "/auth/openid/login?response_type=code&redirect_uri=audiobookshelf://oauth&code_challenge=foo&code_challenge_method=plain"
	req := httptest.NewRequest("GET", reqURL, nil)
	rec := httptest.NewRecorder()

	h.HandleLogin(rec, req)

	resp := rec.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", resp.StatusCode)
	}
}

func TestHandleCallback_Web_Success(t *testing.T) {
	server := startMockOIDCServer(t, nil, nil)
	defer server.Close()

	settings := OIDCSettings{
		IssuerURL:    server.URL,
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
	}
	h := NewOIDCHandler(settings)

	state := "test-state-123"
	codeVerifier := "verifier1234567890123456789012345678901234567890"
	ssoRedirect := "http://example.com/auth/openid/callback"
	sess := &oidcSession{
		State:          state,
		CodeVerifier:   codeVerifier,
		SSORedirectURI: ssoRedirect,
		Mobile:         false,
	}
	h.sessions.Store(state, sess)

	reqURL := fmt.Sprintf("/auth/openid/callback?state=%s&code=test-auth-code", state)
	req := httptest.NewRequest("GET", reqURL, nil)
	req.Host = "example.com"
	rec := httptest.NewRecorder()

	claims, err := h.HandleCallback(rec, req)
	if err != nil {
		t.Fatalf("HandleCallback failed: %v", err)
	}

	if claims == nil {
		t.Fatal("expected claims, got nil")
	}

	if claims["sub"] != "user123" {
		t.Errorf("expected sub user123, got %v", claims["sub"])
	}
	if claims["email"] != "user@example.com" {
		t.Errorf("expected email user@example.com, got %v", claims["email"])
	}

	if _, ok := claims["id_token"].(string); !ok {
		t.Error("expected raw id_token in claims")
	}

	if _, ok := h.sessions.Load(state); ok {
		t.Error("expected session to be deleted after callback")
	}
}

func TestHandleCallback_MobileRedirect_Success(t *testing.T) {
	server := startMockOIDCServer(t, nil, nil)
	defer server.Close()

	settings := OIDCSettings{
		IssuerURL:    server.URL,
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
	}
	h := NewOIDCHandler(settings)

	state := "mobile-state-123"
	sess := &oidcSession{
		State:             state,
		SSORedirectURI:    "http://example.com/auth/openid/mobile-redirect",
		Mobile:            true,
		MobileRedirectURI: "audiobookshelf://oauth",
	}
	h.sessions.Store(state, sess)

	reqURL := fmt.Sprintf("/auth/openid/mobile-redirect?state=%s&code=auth-code-123", state)
	req := httptest.NewRequest("GET", reqURL, nil)
	rec := httptest.NewRecorder()

	claims, err := h.HandleCallback(rec, req)
	if err != nil {
		t.Fatalf("HandleCallback failed: %v", err)
	}

	if claims != nil {
		t.Errorf("expected claims to be nil, got %v", claims)
	}

	resp := rec.Result()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("expected status 302, got %d", resp.StatusCode)
	}

	loc := resp.Header.Get("Location")
	expectedLocation := "audiobookshelf://oauth?code=auth-code-123&state=mobile-state-123"
	if loc != expectedLocation {
		t.Errorf("expected redirect location %s, got %s", expectedLocation, loc)
	}
}

func TestHandleCallback_MobileExchange_Success(t *testing.T) {
	server := startMockOIDCServer(t, nil, nil)
	defer server.Close()

	settings := OIDCSettings{
		IssuerURL:    server.URL,
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
	}
	h := NewOIDCHandler(settings)

	// In this flow, mobile app calls callback endpoint directly with code_verifier
	reqURL := "/auth/openid/callback?state=dummy-state&code=test-auth-code&code_verifier=xyz123"
	req := httptest.NewRequest("GET", reqURL, nil)
	req.Host = "example.com"
	rec := httptest.NewRecorder()

	claims, err := h.HandleCallback(rec, req)
	if err != nil {
		t.Fatalf("HandleCallback failed: %v", err)
	}

	if claims == nil {
		t.Fatal("expected claims, got nil")
	}

	if claims["sub"] != "user123" {
		t.Errorf("expected sub user123, got %v", claims["sub"])
	}
}

func TestHandleCallback_MissingState(t *testing.T) {
	settings := OIDCSettings{
		IssuerURL: "http://example.com",
	}
	h := NewOIDCHandler(settings)

	req := httptest.NewRequest("GET", "/auth/openid/callback?code=some-code", nil)
	rec := httptest.NewRecorder()

	claims, err := h.HandleCallback(rec, req)
	if err == nil {
		t.Fatal("expected error for missing state")
	}
	if !strings.Contains(err.Error(), "missing state parameter") {
		t.Errorf("expected missing state error, got: %v", err)
	}
	if claims != nil {
		t.Errorf("expected nil claims, got %v", claims)
	}
}

func TestHandleCallback_MobileRedirect_StateMismatch(t *testing.T) {
	settings := OIDCSettings{
		IssuerURL: "http://example.com",
	}
	h := NewOIDCHandler(settings)

	req := httptest.NewRequest("GET", "/auth/openid/mobile-redirect?state=unknown-state&code=code", nil)
	rec := httptest.NewRecorder()

	claims, err := h.HandleCallback(rec, req)
	if err == nil {
		t.Fatal("expected error for state mismatch")
	}
	if !strings.Contains(err.Error(), "state parameter mismatch") {
		t.Errorf("expected state parameter mismatch error, got: %v", err)
	}
	if claims != nil {
		t.Errorf("expected nil claims, got %v", claims)
	}
	if rec.Result().StatusCode != http.StatusBadRequest {
		t.Errorf("expected BadRequest status, got %d", rec.Result().StatusCode)
	}
}

func TestHandleCallback_MobileRedirect_MissingRedirectURI(t *testing.T) {
	settings := OIDCSettings{
		IssuerURL: "http://example.com",
	}
	h := NewOIDCHandler(settings)

	state := "mobile-state-123"
	sess := &oidcSession{
		State:             state,
		SSORedirectURI:    "http://example.com/auth/openid/mobile-redirect",
		Mobile:            true,
		MobileRedirectURI: "",
	}
	h.sessions.Store(state, sess)

	req := httptest.NewRequest("GET", "/auth/openid/mobile-redirect?state=mobile-state-123&code=code", nil)
	rec := httptest.NewRecorder()

	claims, err := h.HandleCallback(rec, req)
	if err == nil {
		t.Fatal("expected error for missing redirect URI")
	}
	if !strings.Contains(err.Error(), "no redirect URI") {
		t.Errorf("expected no redirect URI error, got: %v", err)
	}
	if claims != nil {
		t.Errorf("expected nil claims, got %v", claims)
	}
	if rec.Result().StatusCode != http.StatusBadRequest {
		t.Errorf("expected BadRequest status, got %d", rec.Result().StatusCode)
	}
}

func TestHandleCallback_TokenExchangeFailure(t *testing.T) {
	server := startMockOIDCServer(t, func(w http.ResponseWriter, r *http.Request, serverURL string) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error": "invalid_grant", "error_description": "code is invalid"}`))
	}, nil)
	defer server.Close()

	settings := OIDCSettings{
		IssuerURL:    server.URL,
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
	}
	h := NewOIDCHandler(settings)

	state := "state-123"
	sess := &oidcSession{
		State:          state,
		CodeVerifier:   "verifier",
		SSORedirectURI: "http://example.com/auth/openid/callback",
	}
	h.sessions.Store(state, sess)

	req := httptest.NewRequest("GET", "/auth/openid/callback?state=state-123&code=bad-code", nil)
	req.Host = "example.com"
	rec := httptest.NewRecorder()

	claims, err := h.HandleCallback(rec, req)
	if err == nil {
		t.Fatal("expected error from token exchange")
	}
	if !strings.Contains(err.Error(), "failed to exchange token") {
		t.Errorf("expected failed to exchange token error, got: %v", err)
	}
	if claims != nil {
		t.Errorf("expected nil claims, got %v", claims)
	}
}

func TestHandleCallback_GroupClaim_Success(t *testing.T) {
	server := startMockOIDCServer(t, nil, nil)
	defer server.Close()

	settings := OIDCSettings{
		IssuerURL:    server.URL,
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
		GroupClaim:   "groups",
	}
	h := NewOIDCHandler(settings)

	state := "state-123"
	sess := &oidcSession{
		State:          state,
		CodeVerifier:   "verifier",
		SSORedirectURI: "http://example.com/auth/openid/callback",
	}
	h.sessions.Store(state, sess)

	req := httptest.NewRequest("GET", "/auth/openid/callback?state=state-123&code=code", nil)
	req.Host = "example.com"
	rec := httptest.NewRecorder()

	claims, err := h.HandleCallback(rec, req)
	if err != nil {
		t.Fatalf("expected success, got err: %v", err)
	}
	if claims == nil {
		t.Fatal("expected claims")
	}

	scopes := h.getScopes()
	hasGroupsScope := false
	for _, s := range scopes {
		if s == "groups" {
			hasGroupsScope = true
		}
	}
	if !hasGroupsScope {
		t.Error("expected groups to be in scopes")
	}
}

func TestHandleCallback_GroupClaim_Missing(t *testing.T) {
	server := startMockOIDCServer(t, func(w http.ResponseWriter, r *http.Request, serverURL string) {
		w.Header().Set("Content-Type", "application/json")
		token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
			"iss":   serverURL,
			"sub":   "user123",
			"aud":   "test-client-id",
			"exp":   time.Now().Add(time.Hour).Unix(),
			"iat":   time.Now().Unix(),
			"email": "user@example.com",
		})
		token.Header["kid"] = "mock-key-id"
		signedToken, err := token.SignedString(testPrivKey)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}

		resp := map[string]interface{}{
			"access_token": "mock-access-token",
			"token_type":   "Bearer",
			"expires_in":   3600,
			"id_token":     signedToken,
		}
		json.NewEncoder(w).Encode(resp)
	}, nil)
	defer server.Close()

	settings := OIDCSettings{
		IssuerURL:    server.URL,
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
		GroupClaim:   "non_existent_groups_claim",
	}
	h := NewOIDCHandler(settings)

	state := "state-123"
	sess := &oidcSession{
		State:          state,
		CodeVerifier:   "verifier",
		SSORedirectURI: "http://example.com/auth/openid/callback",
	}
	h.sessions.Store(state, sess)

	req := httptest.NewRequest("GET", "/auth/openid/callback?state=state-123&code=code", nil)
	req.Host = "example.com"
	rec := httptest.NewRecorder()

	claims, err := h.HandleCallback(rec, req)
	if err == nil {
		t.Fatal("expected error due to missing group claim")
	}
	if !strings.Contains(err.Error(), "group claim non_existent_groups_claim not found") {
		t.Errorf("expected missing group claim error, got: %v", err)
	}
	if claims != nil {
		t.Errorf("expected nil claims, got %v", claims)
	}
}

func TestHandleLogin_DiscoveryFailure(t *testing.T) {
	settings := OIDCSettings{
		IssuerURL: "http://127.0.0.1:0", // Invalid port
	}
	h := NewOIDCHandler(settings)

	req := httptest.NewRequest("GET", "/auth/openid/login", nil)
	rec := httptest.NewRecorder()

	h.HandleLogin(rec, req)

	resp := rec.Result()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected 500 status on discovery failure, got %d", resp.StatusCode)
	}
}

func TestHandleCallback_DiscoveryFailure(t *testing.T) {
	settings := OIDCSettings{
		IssuerURL: "http://127.0.0.1:0", // Invalid port
	}
	h := NewOIDCHandler(settings)

	req := httptest.NewRequest("GET", "/auth/openid/callback?state=some-state&code=code", nil)
	rec := httptest.NewRecorder()

	claims, err := h.HandleCallback(rec, req)
	if err == nil {
		t.Fatal("expected error on discovery failure")
	}
	if !strings.Contains(err.Error(), "failed to discover OIDC provider") {
		t.Errorf("expected failed to discover OIDC provider, got %v", err)
	}
	if claims != nil {
		t.Errorf("expected nil claims, got %v", claims)
	}
}

func TestHandleCallback_MissingSub(t *testing.T) {
	server := startMockOIDCServer(t, func(w http.ResponseWriter, r *http.Request, serverURL string) {
		w.Header().Set("Content-Type", "application/json")
		token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
			"iss":   serverURL,
			"aud":   "test-client-id",
			"exp":   time.Now().Add(time.Hour).Unix(),
			"iat":   time.Now().Unix(),
			"email": "user@example.com",
		})
		token.Header["kid"] = "mock-key-id"
		signedToken, err := token.SignedString(testPrivKey)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}

		resp := map[string]interface{}{
			"access_token": "mock-access-token",
			"token_type":   "Bearer",
			"expires_in":   3600,
			"id_token":     signedToken,
		}
		json.NewEncoder(w).Encode(resp)
	}, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Userinfo explicitly does NOT contain sub
		resp := map[string]interface{}{
			"email": "user@example.com",
		}
		json.NewEncoder(w).Encode(resp)
	})
	defer server.Close()

	settings := OIDCSettings{
		IssuerURL:    server.URL,
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
	}
	h := NewOIDCHandler(settings)

	state := "state-123"
	sess := &oidcSession{
		State:          state,
		CodeVerifier:   "verifier",
		SSORedirectURI: "http://example.com/auth/openid/callback",
	}
	h.sessions.Store(state, sess)

	req := httptest.NewRequest("GET", "/auth/openid/callback?state=state-123&code=code", nil)
	req.Host = "example.com"
	rec := httptest.NewRecorder()

	claims, err := h.HandleCallback(rec, req)
	if err == nil {
		t.Fatal("expected error due to missing sub claim")
	}
	if !strings.Contains(err.Error(), "invalid claims, no sub") {
		t.Errorf("expected no sub error, got: %v", err)
	}
	if claims != nil {
		t.Errorf("expected nil claims, got %v", claims)
	}
}

func TestHandleCallback_UserInfoMerge(t *testing.T) {
	server := startMockOIDCServer(t, func(w http.ResponseWriter, r *http.Request, serverURL string) {
		w.Header().Set("Content-Type", "application/json")
		token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
			"iss": serverURL,
			"sub": "user123",
			"aud": "test-client-id",
			"exp": time.Now().Add(time.Hour).Unix(),
			"iat": time.Now().Unix(),
		})
		token.Header["kid"] = "mock-key-id"
		signedToken, err := token.SignedString(testPrivKey)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		resp := map[string]interface{}{
			"access_token": "mock-access-token",
			"token_type":   "Bearer",
			"expires_in":   3600,
			"id_token":     signedToken,
		}
		json.NewEncoder(w).Encode(resp)
	}, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]interface{}{
			"sub":          "user123",
			"email":        "userinfo-email@example.com",
			"custom_claim": "hello-world",
		}
		json.NewEncoder(w).Encode(resp)
	})
	defer server.Close()

	settings := OIDCSettings{
		IssuerURL:    server.URL,
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
	}
	h := NewOIDCHandler(settings)

	state := "state-123"
	sess := &oidcSession{
		State:          state,
		CodeVerifier:   "verifier",
		SSORedirectURI: "http://example.com/auth/openid/callback",
	}
	h.sessions.Store(state, sess)

	req := httptest.NewRequest("GET", "/auth/openid/callback?state=state-123&code=code", nil)
	req.Host = "example.com"
	rec := httptest.NewRecorder()

	claims, err := h.HandleCallback(rec, req)
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}

	if claims["email"] != "userinfo-email@example.com" {
		t.Errorf("expected email to be merged from userinfo: userinfo-email@example.com, got: %v", claims["email"])
	}
	if claims["custom_claim"] != "hello-world" {
		t.Errorf("expected custom_claim to be merged from userinfo: hello-world, got: %v", claims["custom_claim"])
	}
}

func TestHandleLogin_SubfolderForRedirectURLs(t *testing.T) {
	server := startMockOIDCServer(t, nil, nil)
	defer server.Close()

	settings := OIDCSettings{
		IssuerURL:                server.URL,
		ClientID:                 "test-client-id",
		ClientSecret:             "test-client-secret",
		SubfolderForRedirectURLs: true,
	}
	h := NewOIDCHandler(settings)

	req := httptest.NewRequest("GET", "/my-app/auth/openid/login", nil)
	req.Host = "example.com"
	rec := httptest.NewRecorder()

	h.HandleLogin(rec, req)

	resp := rec.Result()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("expected status 302, got %d", resp.StatusCode)
	}

	locVal := resp.Header.Get("Location")
	loc, err := url.Parse(locVal)
	if err != nil {
		t.Fatalf("failed to parse Location: %v", err)
	}

	q := loc.Query()
	expectedRedirect := "http://example.com/my-app/auth/openid/callback"
	if q.Get("redirect_uri") != expectedRedirect {
		t.Errorf("expected redirect_uri %s, got %s", expectedRedirect, q.Get("redirect_uri"))
	}
}
