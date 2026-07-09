package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"audiobookshelf/internal/core"
)

func TestApiKeysHandlers(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Insert a test user
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

	t.Run("Create API Key", func(t *testing.T) {
		reqPayload := CreateApiKeyRequest{
			Name:      "Test Key",
			UserID:    "user1",
			ExpiresAt: time.Now().Add(1 * time.Hour).UTC().Format(time.RFC3339),
		}
		body, _ := json.Marshal(reqPayload)
		req := httptest.NewRequest("POST", "/api/api-keys", bytes.NewReader(body))
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, adminSession))
		rr := httptest.NewRecorder()

		handler := handlePostApiKey(db)
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d. Body: %s", rr.Code, rr.Body.String())
		}

		var resp map[string]interface{}
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to unmarshal response: %v", err)
		}

		if resp["success"] != true {
			t.Errorf("Expected success true, got %v", resp["success"])
		}

		apiKey, ok := resp["apiKey"].(map[string]interface{})
		if !ok {
			t.Fatalf("Response does not contain apiKey map")
		}

		if apiKey["name"] != "Test Key" {
			t.Errorf("Expected name 'Test Key', got %v", apiKey["name"])
		}
		if apiKey["userId"] != "user1" {
			t.Errorf("Expected userId 'user1', got %v", apiKey["userId"])
		}
		if apiKey["username"] != "testuser" {
			t.Errorf("Expected username 'testuser', got %v", apiKey["username"])
		}
		if apiKey["token"] == "" {
			t.Errorf("Expected token to be non-empty")
		}
	})

	t.Run("Create API Key for Non-Existent User", func(t *testing.T) {
		reqPayload := CreateApiKeyRequest{
			Name:   "Test Key Invalid",
			UserID: "user_nonexistent",
		}
		body, _ := json.Marshal(reqPayload)
		req := httptest.NewRequest("POST", "/api/api-keys", bytes.NewReader(body))
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, adminSession))
		rr := httptest.NewRecorder()

		handler := handlePostApiKey(db)
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("Expected status 400, got %d", rr.Code)
		}
	})

	t.Run("Get API Keys", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/api-keys", nil)
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, adminSession))
		rr := httptest.NewRecorder()

		handler := handleGetApiKeys(db)
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", rr.Code)
		}

		var resp map[string]interface{}
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to unmarshal response: %v", err)
		}

		apiKeys, ok := resp["apiKeys"].([]interface{})
		if !ok {
			t.Fatalf("Response does not contain apiKeys slice")
		}

		if len(apiKeys) != 1 {
			t.Errorf("Expected 1 API key, got %d", len(apiKeys))
		}

		firstKey := apiKeys[0].(map[string]interface{})
		if firstKey["name"] != "Test Key" {
			t.Errorf("Expected name 'Test Key', got %v", firstKey["name"])
		}
		if firstKey["username"] != "testuser" {
			t.Errorf("Expected username 'testuser', got %v", firstKey["username"])
		}
	})

	t.Run("Delete API Key", func(t *testing.T) {
		// Fetch the API key ID first
		var apiKeyID string
		err := db.QueryRow("SELECT id FROM apiKeys WHERE name = 'Test Key'").Scan(&apiKeyID)
		if err != nil {
			t.Fatalf("Failed to get API key ID: %v", err)
		}

		req := httptest.NewRequest("DELETE", "/api/api-keys/"+apiKeyID, nil)
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, adminSession))
		rr := httptest.NewRecorder()

		handler := handleDeleteApiKey(db)
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", rr.Code)
		}

		// Verify deletion
		var count int
		err = db.QueryRow("SELECT COUNT(*) FROM apiKeys WHERE id = ?", apiKeyID).Scan(&count)
		if err != nil {
			t.Fatalf("Failed to query API key count: %v", err)
		}
		if count != 0 {
			t.Errorf("Expected API key to be deleted, but it still exists")
		}
	})
}

func TestAuthMiddlewareAPIKey(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Insert test user
	_, err := db.Exec(`INSERT INTO users (id, username, type, isActive, permissions) VALUES ('user1', 'testuser', 'admin', 1, '{}')`)
	if err != nil {
		t.Fatalf("Failed to insert test user: %v", err)
	}

	// Insert active valid API key
	activeToken := "1234567890abcdef1234567890abcdef1234567890abcdef"
	_, err = db.Exec(`
		INSERT INTO apiKeys (id, isActive, expiresAt, userId, name, createdAt)
		VALUES (?, 1, ?, 'user1', 'Active Key', '2026-06-19T00:00:00Z')
	`, activeToken, time.Now().Add(10*time.Minute).UTC().Format(time.RFC3339))
	if err != nil {
		t.Fatalf("Failed to insert active API key: %v", err)
	}

	// Insert expired API key
	expiredToken := "expired567890abcdef1234567890abcdef1234567890abcdef"
	_, err = db.Exec(`
		INSERT INTO apiKeys (id, isActive, expiresAt, userId, name, createdAt)
		VALUES (?, 1, ?, 'user1', 'Expired Key', '2026-06-19T00:00:00Z')
	`, expiredToken, time.Now().Add(-10*time.Minute).UTC().Format(time.RFC3339))
	if err != nil {
		t.Fatalf("Failed to insert expired API key: %v", err)
	}

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userSess, ok := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if !ok || userSess == nil {
			http.Error(w, "no user context", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("Authenticated as " + userSess.Username))
	})

	mw := AuthMiddleware(db, "secret", nextHandler)

	t.Run("Valid API Key via Bearer Header", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/me", nil)
		req.Header.Set("Authorization", "Bearer "+activeToken)
		rr := httptest.NewRecorder()

		mw.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d. Body: %s", rr.Code, rr.Body.String())
		}
		if !strings.Contains(rr.Body.String(), "Authenticated as testuser") {
			t.Errorf("Expected response body to contain 'Authenticated as testuser', got: %s", rr.Body.String())
		}
	})

	t.Run("Valid API Key via Token Query Param", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/me?token="+activeToken, nil)
		rr := httptest.NewRecorder()

		mw.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", rr.Code)
		}
	})

	t.Run("Expired API Key", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/me", nil)
		req.Header.Set("Authorization", "Bearer "+expiredToken)
		rr := httptest.NewRecorder()

		mw.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Errorf("Expected status 401, got %d", rr.Code)
		}
	})

	t.Run("Non-Existent API Key", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/me", nil)
		req.Header.Set("Authorization", "Bearer fakekey12345")
		rr := httptest.NewRecorder()

		mw.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Errorf("Expected status 401, got %d", rr.Code)
		}
	})
}
