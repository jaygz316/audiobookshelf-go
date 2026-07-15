package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"audiobookshelf/internal/core"
)

func TestHandleSearchLibrary(t *testing.T) {
	db := setupTestDBShared(t)
	defer db.Close()

	// Seed some search data
	_, _ = db.Exec(`INSERT INTO users (id, username, type, isActive, permissions) VALUES ('user1', 'testuser', 'admin', 1, '{}')`)
	_, _ = db.Exec(`INSERT INTO libraries (id, name, mediaType) VALUES ('lib1', 'Library 1', 'book')`)

	// Seed a book library item
	_, _ = db.Exec(`INSERT INTO libraryItems (id, libraryId, mediaId, mediaType, title) VALUES ('li1', 'lib1', 'book1', 'book', 'Harry Potter')`)
	_, _ = db.Exec(`INSERT INTO books (id, title, explicit, tags, genres, narrators) VALUES ('book1', 'Harry Potter', 0, '["magic"]', '["Fantasy"]', '["Stephen Fry"]')`)

	// Seed an author with non-null createdAt and updatedAt
	_, _ = db.Exec(`INSERT INTO authors (id, libraryId, name, createdAt, updatedAt) VALUES ('auth1', 'lib1', 'J.K. Rowling', '2023-01-01 00:00:00', '2023-01-01 00:00:00')`)

	handler := HandleSearchLibrary(db, "lib1")

	// Create test request
	req := httptest.NewRequest("GET", "/api/libraries/lib1/search?q=Harry", nil)
	// Set user context
	user := &core.UserSession{
		ID:                       "user1",
		Username:                 "testuser",
		Type:                     "admin",
		CanAccessExplicitContent: true,
	}
	ctx := context.WithValue(req.Context(), core.UserContextKey, user)
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status code 200, got %d. Body: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	books, ok := resp["book"].([]interface{})
	if !ok || len(books) != 1 {
		t.Errorf("Expected 1 book result, got %+v", resp["book"])
	}

	authors, ok := resp["authors"].([]interface{})
	if !ok || len(authors) != 0 { // 'Harry' shouldn't match 'J.K. Rowling'
		t.Errorf("Expected 0 author results, got %+v", resp["authors"])
	}

	// Test searching for JK Rowling
	req2 := httptest.NewRequest("GET", "/api/libraries/lib1/search?q=Rowling", nil)
	req2 = req2.WithContext(context.WithValue(req2.Context(), core.UserContextKey, user))
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req2)

	var resp2 map[string]interface{}
	_ = json.Unmarshal(rr2.Body.Bytes(), &resp2)
	authors2, _ := resp2["authors"].([]interface{})
	if len(authors2) != 1 {
		t.Errorf("Expected 1 author result for Rowling, got %d", len(authors2))
	}
}
