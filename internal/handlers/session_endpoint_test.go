package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"audiobookshelf/internal/core"
)

func TestGetSession_Unauthorized(t *testing.T) {
	db := setupUsersTestDB(t)
	defer db.Close()

	handler := handleGetSession(db)

	req := httptest.NewRequest("GET", "/api/v1/session", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("Expected status code %d, got %d", http.StatusUnauthorized, rr.Code)
	}
}

func TestGetSession_Authorized(t *testing.T) {
	db := setupUsersTestDB(t)
	defer db.Close()

	// Insert test user
	_, err := db.Exec(`
		INSERT INTO users (id, username, type, token, isActive, permissions, extraData)
		VALUES ('user-id-123', 'testuser', 'user', 'user-token', 1, '{}', '{}')
	`)
	if err != nil {
		t.Fatalf("failed to insert test user: %v", err)
	}

	handler := handleGetSession(db)

	// Create request with user session context
	req := httptest.NewRequest("GET", "/api/v1/session", nil)
	userSession := &core.UserSession{
		ID:                 "user-id-123",
		Username:           "testuser",
		Type:               "user",
		IsActive:           true,
		AccessAllLibraries: true,
	}
	ctx := context.WithValue(req.Context(), core.UserContextKey, userSession)
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected status code %d, got %d. Body: %s", http.StatusOK, rr.Code, rr.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp["id"] != "user-id-123" {
		t.Errorf("Expected user ID 'user-id-123', got '%v'", resp["id"])
	}
	if resp["username"] != "testuser" {
		t.Errorf("Expected username 'testuser', got '%v'", resp["username"])
	}
}

func TestGetSession_UserNotFound(t *testing.T) {
	db := setupUsersTestDB(t)
	defer db.Close()

	handler := handleGetSession(db)

	// Create request with user session context for a non-existent user
	req := httptest.NewRequest("GET", "/api/v1/session", nil)
	userSession := &core.UserSession{
		ID:                 "non-existent-id",
		Username:           "ghost",
		Type:               "user",
		IsActive:           true,
		AccessAllLibraries: true,
	}
	ctx := context.WithValue(req.Context(), core.UserContextKey, userSession)
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("Expected status code %d, got %d", http.StatusUnauthorized, rr.Code)
	}
}
