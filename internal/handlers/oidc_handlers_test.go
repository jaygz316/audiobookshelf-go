package handlers

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
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"audiobookshelf/internal/auth"
	"audiobookshelf/internal/core"
)

func TestOIDCAuthIntegration(t *testing.T) {
	// 1. Setup RSA key pair for mock OIDC server token signing
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate RSA key: %v", err)
	}
	pubKey := &privKey.PublicKey

	// 2. Setup mock OIDC server
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
		eBytes := big.NewInt(int64(pubKey.E)).Bytes()
		eStr := base64.RawURLEncoding.EncodeToString(eBytes)
		nStr := base64.RawURLEncoding.EncodeToString(pubKey.N.Bytes())

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
		w.Header().Set("Content-Type", "application/json")
		token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
			"iss":    serverURL,
			"sub":    "user-oidc-123",
			"aud":    "test-client-id",
			"exp":    time.Now().Add(time.Hour).Unix(),
			"iat":    time.Now().Unix(),
			"email":  "oidcuser@example.com",
			"groups": []string{"users"},
		})
		token.Header["kid"] = "mock-key-id"
		signedToken, err := token.SignedString(privKey)
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
		w.Header().Set("Content-Type", "application/json")
		authHeader := r.Header.Get("Authorization")
		if authHeader != "Bearer mock-access-token" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		userInfo := map[string]interface{}{
			"sub":                "user-oidc-123",
			"email":              "oidcuser@example.com",
			"preferred_username": "oidcuser",
		}
		json.NewEncoder(w).Encode(userInfo)
	})

	oidcServer := httptest.NewServer(mux)
	defer oidcServer.Close()
	serverURL = oidcServer.URL

	// 3. Setup test database
	db := setupSessionsTestDB(t)
	defer db.Close()

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT,
			createdAt TEXT,
			updatedAt TEXT
		)
	`)
	if err != nil {
		t.Fatalf("failed to create settings table: %v", err)
	}

	// Insert OIDC configuration settings
	settingsMap := map[string]interface{}{
		"authOpenIDIssuerURL":       serverURL,
		"authOpenIDClientID":        "test-client-id",
		"authOpenIDClientSecret":    "test-client-secret",
		"authOpenIDRedirectURL":     "http://localhost/auth/openid/callback",
		"authOpenIDAutoRegister":    true,
		"authOpenIDMatchExistingBy": "email",
	}
	settingsBytes, _ := json.Marshal(settingsMap)
	_, err = db.Exec("INSERT INTO settings (key, value, createdAt, updatedAt) VALUES ('server-settings', ?, 'now', 'now')", string(settingsBytes))
	if err != nil {
		t.Fatalf("failed to insert settings: %v", err)
	}

	// Setup token secret for database
	_, err = db.Exec("INSERT INTO settings (key, value, createdAt, updatedAt) VALUES ('tokenSecret', 'my-very-secret-key-12345', 'now', 'now')")
	if err != nil {
		t.Fatalf("failed to insert token secret settings: %v", err)
	}

	// 4. Instantiate and set global OIDC handler
	s, err := getOIDCSettings(db)
	if err != nil {
		t.Fatalf("failed to read OIDC settings: %v", err)
	}
	globalOIDCHandlerMu.Lock()
	globalOIDCHandler = auth.NewOIDCHandler(s, oidcServer.Client())
	globalOIDCHandlerMu.Unlock()

	// 5. Invoke OIDC Login route to start session flow and generate state
	loginReq := httptest.NewRequest("GET", "/auth/openid", nil)
	loginRec := httptest.NewRecorder()

	// Invoke Login handler directly
	muxLogin := http.NewServeMux()
	muxLogin.HandleFunc("/auth/openid", func(w http.ResponseWriter, r *http.Request) {
		globalOIDCHandler.HandleLogin(w, r)
	})
	muxLogin.ServeHTTP(loginRec, loginReq)

	if loginRec.Code != http.StatusFound {
		t.Fatalf("expected 302 redirect from /auth/openid, got %d", loginRec.Code)
	}

	redirectLocation := loginRec.Header().Get("Location")
	parsedURL, err := url.Parse(redirectLocation)
	if err != nil {
		t.Fatalf("failed to parse redirect location: %v", err)
	}
	state := parsedURL.Query().Get("state")
	if state == "" {
		t.Fatal("expected state parameter in redirect URL")
	}

	// Extract auth_state cookie from redirect response
	var authStateCookie *http.Cookie
	for _, cookie := range loginRec.Result().Cookies() {
		if cookie.Name == "auth_state" {
			authStateCookie = cookie
			break
		}
	}
	if authStateCookie == nil {
		t.Fatal("expected auth_state cookie to be set")
	}

	// 6. Invoke Callback route with mock code and state
	callbackReq := httptest.NewRequest("GET", fmt.Sprintf("/auth/openid/callback?code=mock-code&state=%s", state), nil)
	callbackReq.AddCookie(authStateCookie)
	callbackRec := httptest.NewRecorder()

	handleOIDCCallback(db).ServeHTTP(callbackRec, callbackReq)

	if callbackRec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK from /auth/openid/callback, got %d. Body: %s", callbackRec.Code, callbackRec.Body.String())
	}

	// Parse callback JSON response
	var loginResp map[string]interface{}
	if err := json.Unmarshal(callbackRec.Body.Bytes(), &loginResp); err != nil {
		t.Fatalf("failed to unmarshal callback response: %v", err)
	}

	userMap, ok := loginResp["user"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected user object in callback response, got: %+v", loginResp)
	}

	accessToken, ok := userMap["accessToken"].(string)
	if !ok || accessToken == "" {
		t.Fatal("expected accessToken in response user object")
	}

	// Verify that the refresh_token cookie was set
	var refreshTokenCookie *http.Cookie
	for _, cookie := range callbackRec.Result().Cookies() {
		if cookie.Name == "refresh_token" {
			refreshTokenCookie = cookie
			break
		}
	}
	if refreshTokenCookie == nil {
		t.Fatal("expected refresh_token cookie to be set in response")
	}
	if refreshTokenCookie.Value == "" {
		t.Fatal("expected non-empty refresh_token value in cookie")
	}

	// Verify that a user session was created in the database sessions table
	var sessionCount int
	err = db.QueryRow("SELECT COUNT(*) FROM sessions WHERE userId = ?", userMap["id"]).Scan(&sessionCount)
	if err != nil {
		t.Fatalf("failed to count sessions in DB: %v", err)
	}
	if sessionCount != 1 {
		t.Fatalf("expected exactly 1 session in DB, got %d", sessionCount)
	}

	// 7. Verify that the generated accessToken is valid and accepted by AuthMiddlewareWrapper
	// Set up the API endpoint with the AuthMiddlewareWrapper
	apiMux := http.NewServeMux()
	apiMux.Handle("/api/me", AuthMiddlewareWrapper(db, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uSess, ok := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if !ok || uSess == nil {
			http.Error(w, "no user context", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(uSess)
	})))

	apiReq := httptest.NewRequest("GET", "/api/me", nil)
	apiReq.Header.Set("Authorization", "Bearer "+accessToken)
	apiRec := httptest.NewRecorder()

	apiMux.ServeHTTP(apiRec, apiReq)

	if apiRec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK from AuthMiddleware protected route, got %d. Body: %s", apiRec.Code, apiRec.Body.String())
	}

	var userSession core.UserSession
	if err := json.Unmarshal(apiRec.Body.Bytes(), &userSession); err != nil {
		t.Fatalf("failed to unmarshal user session: %v", err)
	}

	if userSession.ID != userMap["id"] {
		t.Errorf("expected session userID %s, got %s", userMap["id"], userSession.ID)
	}
	if userSession.Username != "oidcuser" {
		t.Errorf("expected username 'oidcuser', got %q", userSession.Username)
	}
}
