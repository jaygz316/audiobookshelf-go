package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"audiobookshelf/internal/core"
)

// TestChallenger_AccessControl challenges GET, POST, and DELETE on api-keys endpoints
// to confirm that regular users (non-admin, non-root) are rejected with 403 Forbidden.
func TestChallenger_AccessControl(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Insert test user
	_, err := db.Exec(`INSERT INTO users (id, username, type, isActive, permissions) VALUES ('user1', 'regularuser', 'user', 1, '{}')`)
	if err != nil {
		t.Fatalf("Failed to insert user: %v", err)
	}

	// Normal user session
	userSession := &core.UserSession{
		ID:       "user1",
		Username: "regularuser",
		Type:     "user",
		IsActive: true,
	}

	// 1. GET /api/api-keys
	t.Run("GET_AccessControl", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/api-keys", nil)
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, userSession))
		rr := httptest.NewRecorder()
		handleGetApiKeys(db).ServeHTTP(rr, req)

		if rr.Code != http.StatusForbidden {
			t.Errorf("Expected 403 Forbidden for GET /api/api-keys, got %d", rr.Code)
		}
	})

	// 2. POST /api/api-keys
	t.Run("POST_AccessControl", func(t *testing.T) {
		reqPayload := CreateApiKeyRequest{
			Name:   "New API Key",
			UserID: "user1",
		}
		body, _ := json.Marshal(reqPayload)
		req := httptest.NewRequest("POST", "/api/api-keys", bytes.NewReader(body))
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, userSession))
		rr := httptest.NewRecorder()
		handlePostApiKey(db).ServeHTTP(rr, req)

		if rr.Code != http.StatusForbidden {
			t.Errorf("Expected 403 Forbidden for POST /api/api-keys, got %d", rr.Code)
		}
	})

	// 3. DELETE /api/api-keys/{id}
	t.Run("DELETE_AccessControl", func(t *testing.T) {
		req := httptest.NewRequest("DELETE", "/api/api-keys/some-key-id", nil)
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, userSession))
		rr := httptest.NewRecorder()
		handleDeleteApiKey(db).ServeHTTP(rr, req)

		if rr.Code != http.StatusForbidden {
			t.Errorf("Expected 403 Forbidden for DELETE /api/api-keys/{id}, got %d", rr.Code)
		}
	})
}

// TestChallenger_InputValidation challenges API key creation to confirm that
// empty or whitespace-only name values are rejected with 400 Bad Request.
func TestChallenger_InputValidation(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	_, err := db.Exec(`INSERT INTO users (id, username, type, isActive, permissions) VALUES ('user1', 'testuser', 'admin', 1, '{}')`)
	if err != nil {
		t.Fatalf("Failed to insert test user: %v", err)
	}

	adminSession := &core.UserSession{
		ID:       "user1",
		Username: "testuser",
		Type:     "admin",
		IsActive: true,
	}

	// 1. Empty name
	t.Run("EmptyName", func(t *testing.T) {
		reqPayload := CreateApiKeyRequest{
			Name:   "",
			UserID: "user1",
		}
		body, _ := json.Marshal(reqPayload)
		req := httptest.NewRequest("POST", "/api/api-keys", bytes.NewReader(body))
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, adminSession))
		rr := httptest.NewRecorder()
		handlePostApiKey(db).ServeHTTP(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("Expected 400 Bad Request for empty name, got %d", rr.Code)
		}
	})

	// 2. Whitespace-only name
	t.Run("WhitespaceName", func(t *testing.T) {
		reqPayload := CreateApiKeyRequest{
			Name:   "   ",
			UserID: "user1",
		}
		body, _ := json.Marshal(reqPayload)
		req := httptest.NewRequest("POST", "/api/api-keys", bytes.NewReader(body))
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, adminSession))
		rr := httptest.NewRecorder()
		handlePostApiKey(db).ServeHTTP(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("Expected 400 Bad Request for whitespace-only name, got %d", rr.Code)
		}
	})
}

// TestChallenger_TrailingSlashOnDelete confirms that a DELETE request with a
// trailing slash (i.e., /api/api-keys/{id}/) successfully deletes the key from the DB.
func TestChallenger_TrailingSlashOnDelete(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	adminSession := &core.UserSession{
		ID:       "user1",
		Username: "testuser",
		Type:     "admin",
		IsActive: true,
	}

	// Insert API key
	testKeyID := "testkey1234567890abcdef1234567890abcdef12345"
	_, err := db.Exec(`INSERT INTO apiKeys (id, isActive, expiresAt, userId, name, createdAt) VALUES (?, 1, '', 'user1', 'Test Key', '')`, testKeyID)
	if err != nil {
		t.Fatalf("Failed to insert key: %v", err)
	}

	// Try to delete with trailing slash
	req := httptest.NewRequest("DELETE", "/api/api-keys/"+testKeyID+"/", nil)
	req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, adminSession))
	rr := httptest.NewRecorder()
	handleDeleteApiKey(db).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200 OK, got %d", rr.Code)
	}

	// Verify the key is deleted from the database
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM apiKeys WHERE id = ?", testKeyID).Scan(&count)
	if err != nil {
		t.Fatalf("Failed to query DB: %v", err)
	}
	if count != 0 {
		t.Errorf("API key was not deleted from the database when using a trailing slash")
	}
}

// TestChallenger_Performance_JWTSkipCheckAPIKey verifies that standard JWT tokens
// skip the CheckAPIKey DB lookup. We test this by dropping the apiKeys table entirely
// before doing request authentication using a valid standard JWT token. Since the
// middleware skips the CheckAPIKey query, the request will authenticate successfully.
func TestChallenger_Performance_JWTSkipCheckAPIKey(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Create sessions table (which is queried for standard JWT authentication)
	_, err := db.Exec(`CREATE TABLE sessions (id TEXT PRIMARY KEY, userId TEXT)`)
	if err != nil {
		t.Fatalf("Failed to create sessions table: %v", err)
	}

	// Insert root user and session
	rootUserID := "root-user-uuid"
	_, err = db.Exec(`INSERT INTO users (id, username, type, isActive, extraData, permissions) VALUES (?, 'root', 'root', 1, '{}', '{}')`, rootUserID)
	if err != nil {
		t.Fatalf("Failed to insert root user: %v", err)
	}

	_, err = db.Exec(`INSERT INTO sessions (id, userId) VALUES ('sess1', ?)`, rootUserID)
	if err != nil {
		t.Fatalf("Failed to insert session: %v", err)
	}

	// Generate standard JWT token (with dots, type != "api")
	secret := getTokenSecret(db)
	claims := &core.AuthClaims{
		UserID:   rootUserID,
		Username: "root",
		Type:     "root",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
		},
	}
	tokenObj := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	token, err := tokenObj.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("Failed to sign token: %v", err)
	}

	// Drop the apiKeys table entirely
	_, err = db.Exec(`DROP TABLE apiKeys`)
	if err != nil {
		t.Fatalf("Failed to drop apiKeys table: %v", err)
	}

	// Run AuthMiddleware
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mw := AuthMiddleware(db, secret, nextHandler)

	req := httptest.NewRequest("GET", "/api/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()

	mw.ServeHTTP(rr, req)

	// Since apiKeys table was dropped, if the middleware attempted to call CheckAPIKey
	// or query the apiKeys table, it would return an error and not 200 OK.
	// Since standard JWT bypasses the DB lookup for API keys entirely, it succeeds.
	if rr.Code != http.StatusOK {
		t.Errorf("Expected standard JWT to succeed even with dropped apiKeys table, got %d. Body: %s", rr.Code, rr.Body.String())
	}
}
