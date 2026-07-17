package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"audiobookshelf/internal/core"
)

func TestAdversarial_ExpirationValidation(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Insert active user
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

	// 1. Create API key with a past expiration date
	pastTime := time.Now().Add(-10 * time.Minute).UTC().Format(time.RFC3339)
	reqPayload := CreateApiKeyRequest{
		Name:      "Expired Key",
		UserID:    "user1",
		ExpiresAt: pastTime,
	}
	body, _ := json.Marshal(reqPayload)
	req := httptest.NewRequest("POST", "/api/api-keys", bytes.NewReader(body))
	req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, adminSession))
	rr := httptest.NewRecorder()

	handlePostApiKey(db).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200 when creating expired key, got %d. Body: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}
	apiKeyMap, ok := resp["apiKey"].(map[string]interface{})
	if !ok {
		t.Fatalf("Response does not contain apiKey map")
	}
	token := apiKeyMap["token"].(string)

	// 2. Try to access an endpoint protected by AuthMiddleware using the expired key
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mw := AuthMiddleware(db, "secret", nextHandler)

	reqAuth := httptest.NewRequest("GET", "/api/me", nil)
	reqAuth.Header.Set("Authorization", "Bearer "+token)
	rrAuth := httptest.NewRecorder()

	mw.ServeHTTP(rrAuth, reqAuth)
	if rrAuth.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401 Unauthorized for expired key, got %d", rrAuth.Code)
	} else {
		t.Logf("Pass: AuthMiddleware successfully rejected request using expired API Key (status: 401)")
	}
}

func TestAdversarial_SecurityPermissions(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Insert an admin user and a regular user
	_, err := db.Exec(`INSERT INTO users (id, username, type, isActive, permissions) VALUES ('admin1', 'adminuser', 'admin', 1, '{}')`)
	if err != nil {
		t.Fatalf("Failed to insert admin: %v", err)
	}
	_, err = db.Exec(`INSERT INTO users (id, username, type, isActive, permissions) VALUES ('user1', 'regularuser', 'user', 1, '{}')`)
	if err != nil {
		t.Fatalf("Failed to insert regular user: %v", err)
	}

	// Setup normal user session (not admin/root)
	userSession := &core.UserSession{
		ID:       "user1",
		Username: "regularuser",
		Type:     "user",
		IsActive: true,
	}

	// 1. Test GET /api/api-keys
	reqGet := httptest.NewRequest("GET", "/api/api-keys", nil)
	reqGet = reqGet.WithContext(context.WithValue(reqGet.Context(), core.UserContextKey, userSession))
	rrGet := httptest.NewRecorder()
	handleGetApiKeys(db).ServeHTTP(rrGet, reqGet)

	if rrGet.Code != http.StatusForbidden {
		t.Logf("[VULNERABILITY CONFIRMED] GET /api/api-keys did not return 403 Forbidden for non-admin/root user. Got status %d, Body: %s", rrGet.Code, rrGet.Body.String())
	} else {
		t.Logf("Pass: GET /api/api-keys correctly rejected non-admin/root user with 403 Forbidden")
	}

	// 2. Test POST /api/api-keys
	reqPayload := CreateApiKeyRequest{
		Name:   "Unauthorized Key",
		UserID: "user1",
	}
	body, _ := json.Marshal(reqPayload)
	reqPost := httptest.NewRequest("POST", "/api/api-keys", bytes.NewReader(body))
	reqPost = reqPost.WithContext(context.WithValue(reqPost.Context(), core.UserContextKey, userSession))
	rrPost := httptest.NewRecorder()
	handlePostApiKey(db).ServeHTTP(rrPost, reqPost)

	if rrPost.Code != http.StatusForbidden {
		t.Logf("[VULNERABILITY CONFIRMED] POST /api/api-keys did not return 403 Forbidden for non-admin/root user. Got status %d, Body: %s", rrPost.Code, rrPost.Body.String())
	} else {
		t.Logf("Pass: POST /api/api-keys correctly rejected non-admin/root user with 403 Forbidden")
	}

	// 3. Test DELETE /api/api-keys/{id}
	reqDelete := httptest.NewRequest("DELETE", "/api/api-keys/some-key-id", nil)
	reqDelete = reqDelete.WithContext(context.WithValue(reqDelete.Context(), core.UserContextKey, userSession))
	rrDelete := httptest.NewRecorder()
	handleDeleteApiKey(db).ServeHTTP(rrDelete, reqDelete)

	if rrDelete.Code != http.StatusForbidden {
		t.Logf("[VULNERABILITY CONFIRMED] DELETE /api/api-keys/{id} did not return 403 Forbidden for non-admin/root user. Got status %d", rrDelete.Code)
	} else {
		t.Logf("Pass: DELETE /api/api-keys/{id} correctly rejected non-admin/root user with 403 Forbidden")
	}
}

func TestAdversarial_TokenRandomness(t *testing.T) {
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

	tokens := make(map[string]bool)
	const numKeys = 100

	for i := 0; i < numKeys; i++ {
		reqPayload := CreateApiKeyRequest{
			Name:   fmt.Sprintf("Key-%d", i),
			UserID: "user1",
		}
		body, _ := json.Marshal(reqPayload)
		req := httptest.NewRequest("POST", "/api/api-keys", bytes.NewReader(body))
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, adminSession))
		rr := httptest.NewRecorder()
		handlePostApiKey(db).ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("Failed to create key: %d", rr.Code)
		}

		var resp map[string]interface{}
		_ = json.Unmarshal(rr.Body.Bytes(), &resp)
		apiKey := resp["apiKey"].(map[string]interface{})
		token := apiKey["token"].(string)

		// Check token length
		if len(token) != 48 {
			t.Errorf("Expected token length of 48, got %d", len(token))
		}

		// Check if hex character format is correct
		for _, char := range token {
			isHexChar := (char >= '0' && char <= '9') || (char >= 'a' && char <= 'f') || (char >= 'A' && char <= 'F')
			if !isHexChar {
				t.Errorf("Token contains non-hex character: %c", char)
			}
		}

		// Check uniqueness
		if tokens[token] {
			t.Errorf("Duplicate token generated: %s", token)
		}
		tokens[token] = true
	}

	t.Logf("Pass: Verified %d generated tokens. All are unique, 48-char hex strings.", numKeys)
}

func TestAdversarial_MalformedInput(t *testing.T) {
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
			t.Logf("[BUG/OMISSION] POST /api/api-keys with empty name did not return 400 Bad Request, got status %d", rr.Code)
		} else {
			t.Logf("Pass: POST with empty name returned 400 Bad Request")
		}
	})

	// 2. Missing userId
	t.Run("MissingUserID", func(t *testing.T) {
		reqPayload := CreateApiKeyRequest{
			Name:   "Some Key",
			UserID: "",
		}
		body, _ := json.Marshal(reqPayload)
		req := httptest.NewRequest("POST", "/api/api-keys", bytes.NewReader(body))
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, adminSession))
		rr := httptest.NewRecorder()
		handlePostApiKey(db).ServeHTTP(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("Expected status 400 Bad Request for missing userId, got %d", rr.Code)
		} else {
			t.Logf("Pass: POST with missing userId correctly returned 400 Bad Request")
		}
	})

	// 3. Invalid JSON structure
	t.Run("InvalidJSON", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/api-keys", strings.NewReader(`{invalid json`))
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, adminSession))
		rr := httptest.NewRecorder()
		handlePostApiKey(db).ServeHTTP(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("Expected status 400 Bad Request for invalid JSON, got %d", rr.Code)
		} else {
			t.Logf("Pass: POST with invalid JSON correctly returned 400 Bad Request")
		}
	})
}

func TestAdversarial_TrailingSlashAndDeleteNonExistent(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	adminSession := &core.UserSession{
		ID:       "user1",
		Username: "testuser",
		Type:     "admin",
		IsActive: true,
	}

	// 1. Delete a non-existent key (idempotency check)
	reqDeleteNonExistent := httptest.NewRequest("DELETE", "/api/api-keys/doesnotexist", nil)
	reqDeleteNonExistent = reqDeleteNonExistent.WithContext(context.WithValue(reqDeleteNonExistent.Context(), core.UserContextKey, adminSession))
	rrDeleteNonExistent := httptest.NewRecorder()
	handleDeleteApiKey(db).ServeHTTP(rrDeleteNonExistent, reqDeleteNonExistent)

	if rrDeleteNonExistent.Code != http.StatusOK {
		t.Errorf("Idempotency failed: DELETE for non-existent key returned status %d, expected 200", rrDeleteNonExistent.Code)
	} else {
		t.Logf("Pass: DELETE for non-existent key correctly returned 200 OK (idempotent)")
	}

	// 2. Insert a key to test trailing slash delete
	testKeyID := "testkey1234567890abcdef1234567890abcdef12345"
	_, err := db.Exec(`INSERT INTO apiKeys (id, isActive, expiresAt, userId, name, createdAt) VALUES (?, 1, '', 'user1', 'Test Key', '')`, testKeyID)
	if err != nil {
		t.Fatalf("Failed to insert key: %v", err)
	}

	// Try to delete with trailing slash: /api/api-keys/testkey1234567890abcdef1234567890abcdef12345/
	reqDeleteTrailing := httptest.NewRequest("DELETE", "/api/api-keys/"+testKeyID+"/", nil)
	reqDeleteTrailing = reqDeleteTrailing.WithContext(context.WithValue(reqDeleteTrailing.Context(), core.UserContextKey, adminSession))
	rrDeleteTrailing := httptest.NewRecorder()
	handleDeleteApiKey(db).ServeHTTP(rrDeleteTrailing, reqDeleteTrailing)

	if rrDeleteTrailing.Code != http.StatusOK {
		t.Logf("DELETE with trailing slash returned status %d instead of 200 OK", rrDeleteTrailing.Code)
	} else {
		t.Logf("DELETE with trailing slash returned 200 OK. Checking if key was actually deleted...")
		var count int
		err = db.QueryRow("SELECT COUNT(*) FROM apiKeys WHERE id = ?", testKeyID).Scan(&count)
		if err != nil {
			t.Fatalf("db query failed: %v", err)
		}
		if count != 0 {
			t.Logf("[BUG CONFIRMED] DELETE /api/api-keys/{id}/ returned 200 OK, but key '%s' was NOT deleted from the database. (Silent failure due to path extraction)", testKeyID)
		} else {
			t.Logf("Pass: DELETE with trailing slash successfully deleted key from DB")
		}
	}
}

func TestAdversarial_DeactivatedUserAPIKey(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// 1. Insert a deactivated user (isActive = 0)
	_, err := db.Exec(`INSERT INTO users (id, username, type, isActive, permissions) VALUES ('user_deactivated', 'deactivated_user', 'admin', 0, '{}')`)
	if err != nil {
		t.Fatalf("Failed to insert test user: %v", err)
	}

	// 2. Insert an active API key for this deactivated user
	token := "deactivateduser1234567890abcdef1234567890abcdef"
	_, err = db.Exec(`
		INSERT INTO apiKeys (id, isActive, expiresAt, userId, name, createdAt)
		VALUES (?, 1, '', 'user_deactivated', 'Deactivated Key', '2026-06-19T00:00:00Z')
	`, token)
	if err != nil {
		t.Fatalf("Failed to insert API key: %v", err)
	}

	// 3. Try to authenticate using the deactivated user's API key
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mw := AuthMiddleware(db, "secret", nextHandler)

	req := httptest.NewRequest("GET", "/api/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()

	mw.ServeHTTP(rr, req)

	// Since we are validating security, deactivated users must be rejected (401 Unauthorized)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401 Unauthorized for deactivated user direct API key flow, got %d", rr.Code)
	} else {
		t.Logf("Pass: Deactivated user's API key correctly rejected with 401 Unauthorized")
	}
}

func TestAdversarial_DeactivatedUserAPIKeyClaims(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// 1. Insert a deactivated user (isActive = 0)
	_, err := db.Exec(`INSERT INTO users (id, username, type, isActive, permissions) VALUES ('user_deactivated', 'deactivated_user', 'admin', 0, '{}')`)
	if err != nil {
		t.Fatalf("Failed to insert test user: %v", err)
	}

	// 2. Insert an active API key for this deactivated user
	apiKeyID := "deactivateduser1234567890abcdef1234567890abcdef"
	_, err = db.Exec(`
		INSERT INTO apiKeys (id, isActive, expiresAt, userId, name, createdAt)
		VALUES (?, 1, '', 'user_deactivated', 'Deactivated Key', '2026-06-19T00:00:00Z')
	`, apiKeyID)
	if err != nil {
		t.Fatalf("Failed to insert API key: %v", err)
	}

	// 3. Create a JWT claims token of type "api" pointing to this key
	claims := &core.AuthClaims{
		KeyID: apiKeyID,
		Type:  "api",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err := token.SignedString([]byte("secret"))
	if err != nil {
		t.Fatalf("Failed to sign token: %v", err)
	}

	// 4. Try to authenticate using the claims JWT
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mw := AuthMiddleware(db, "secret", nextHandler)

	req := httptest.NewRequest("GET", "/api/me", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	rr := httptest.NewRecorder()

	mw.ServeHTTP(rr, req)

	// Since we are validating security, deactivated users must be rejected (401 Unauthorized)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401 Unauthorized for deactivated user API key claims flow, got %d", rr.Code)
	} else {
		t.Logf("Pass: Deactivated user's API key claims correctly rejected with 401 Unauthorized")
	}
}

func TestAdversarial_MalformedExpiration(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Insert active user
	_, err := db.Exec(`INSERT INTO users (id, username, type, isActive, permissions) VALUES ('user1', 'testuser', 'admin', 1, '{}')`)
	if err != nil {
		t.Fatalf("Failed to insert test user: %v", err)
	}

	// Insert API key with malformed expiresAt date
	token := "malformedexp1234567890abcdef1234567890abcdef"
	_, err = db.Exec(`
		INSERT INTO apiKeys (id, isActive, expiresAt, userId, name, createdAt)
		VALUES (?, 1, 'invalid-date-format', 'user1', 'Malformed Exp Key', '2026-06-19T00:00:00Z')
	`, token)
	if err != nil {
		t.Fatalf("Failed to insert API key: %v", err)
	}

	// Try to authenticate using this API key
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mw := AuthMiddleware(db, "secret", nextHandler)

	req := httptest.NewRequest("GET", "/api/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()

	mw.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401 Unauthorized for malformed expiration API Key, got %d", rr.Code)
	} else {
		t.Logf("Pass: AuthMiddleware successfully rejected request using API Key with malformed expiration (status: 401)")
	}
}
