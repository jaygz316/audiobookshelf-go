package e2e

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var (
	testPrivKey *rsa.PrivateKey
	testPubKey  *rsa.PublicKey
)

func init() {
	var err error
	testPrivKey, err = rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	testPubKey = &testPrivKey.PublicKey
}

// StartMockOIDCServer spins up an in-process OIDC provider.
func StartMockOIDCServer() *httptest.Server {
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
		w.Header().Set("Content-Type", "application/json")
		email := "user@example.com"
		sub := "user123"
		username := "mockuser"
		groups := []string{"admin", "users"}

		code := r.FormValue("code")
		if code == "code-alice" {
			email = "alice@example.com"
			sub = "sub-alice"
			username = "alice"
			groups = []string{"users"}
		} else if code == "code-group-test" {
			email = "groupuser@example.com"
			sub = "sub-groupuser"
			username = "groupuser"
			groups = []string{"Audiobook-Listeners"}
		}

		token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
			"iss":                serverURL,
			"sub":                sub,
			"aud":                "test-client-id",
			"exp":                time.Now().Add(time.Hour).Unix(),
			"iat":                time.Now().Unix(),
			"email":              email,
			"email_verified":     true,
			"preferred_username": username,
			"name":               username,
			"groups":             groups,
		})
		token.Header["kid"] = "mock-key-id"
		signedToken, err := token.SignedString(testPrivKey)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}

		resp := map[string]interface{}{
			"access_token": "mock-access-token-" + sub,
			"token_type":   "Bearer",
			"expires_in":   3600,
			"id_token":     signedToken,
		}
		json.NewEncoder(w).Encode(resp)
	})

	mux.HandleFunc("/userinfo", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		authHeader := r.Header.Get("Authorization")

		sub := "user123"
		email := "user@example.com"
		username := "mockuser"
		groups := []string{"admin", "users"}

		if authHeader == "Bearer mock-access-token-sub-alice" {
			email = "alice@example.com"
			sub = "sub-alice"
			username = "alice"
			groups = []string{"users"}
		} else if authHeader == "Bearer mock-access-token-sub-groupuser" {
			email = "groupuser@example.com"
			sub = "sub-groupuser"
			username = "groupuser"
			groups = []string{"Audiobook-Listeners"}
		}

		resp := map[string]interface{}{
			"sub":                sub,
			"email":              email,
			"email_verified":     true,
			"preferred_username": username,
			"name":               username,
			"groups":             groups,
		}
		json.NewEncoder(w).Encode(resp)
	})

	mux.HandleFunc("/auth", func(w http.ResponseWriter, r *http.Request) {
		redirectURI := r.FormValue("redirect_uri")
		state := r.FormValue("state")
		code := "mock-auth-code"
		if state == "state-alice" {
			code = "code-alice"
		} else if state == "state-group" {
			code = "code-group-test"
		}
		if redirectURI != "" {
			target := redirectURI + "?code=" + code + "&state=" + state
			http.Redirect(w, r, target, http.StatusFound)
		} else {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("Mock authorization login page"))
		}
	})

	server := httptest.NewServer(mux)
	serverURL = server.URL
	return server
}
