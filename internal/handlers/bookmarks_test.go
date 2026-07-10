package handlers

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"audiobookshelf/internal/core"
	idb "audiobookshelf/internal/db"
)

func prepareTestDBForBookmarks(t *testing.T) *sql.DB {
	db := setupTestDB(t)

	// Drop inadequate tables created by setupTestDB
	_, _ = db.Exec(`DROP TABLE IF EXISTS users`)

	// Recreate users table with full schema required by GetUserFullByID
	_, err := db.Exec(`CREATE TABLE users (
		id TEXT PRIMARY KEY,
		username TEXT,
		email TEXT,
		pash TEXT,
		type TEXT,
		token TEXT,
		isActive INTEGER,
		isLocked INTEGER,
		lastSeen INTEGER,
		permissions TEXT,
		bookmarks TEXT,
		extraData TEXT,
		createdAt TEXT,
		updatedAt TEXT
	)`)
	if err != nil {
		t.Fatalf("Failed to create users table: %v", err)
	}

	return db
}

func TestBookmarkHandlers(t *testing.T) {
	db := prepareTestDBForBookmarks(t)
	defer db.Close()

	// Seed a test user
	_, err := db.Exec(`INSERT INTO users (id, username, type, isActive, token, bookmarks, createdAt, updatedAt) 
		VALUES ('user-1', 'testuser', 'user', 1, 'token-123', '[]', '2026-06-08 12:00:00.000', '2026-06-08 12:00:00.000')`)
	if err != nil {
		t.Fatalf("Failed to seed user: %v", err)
	}

	userSess := &core.UserSession{
		ID:       "user-1",
		Username: "testuser",
		Type:     "user",
		IsActive: true,
	}

	// 1. Create Bookmark (POST /api/me/item/:id/bookmark)
	t.Run("Create Bookmark", func(t *testing.T) {
		payload := map[string]interface{}{
			"time":  123.45,
			"title": "My Test Bookmark",
		}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest("POST", "/api/me/item/book-abc/bookmark", bytes.NewReader(body))
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, userSess))
		rr := httptest.NewRecorder()

		handler := handleMeCreateBookmark(db)
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("Expected status 200 OK, got %d: %s", rr.Code, rr.Body.String())
		}

		var created Bookmark
		if err := json.Unmarshal(rr.Body.Bytes(), &created); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if created.LibraryItemID != "book-abc" {
			t.Errorf("Expected libraryItemId 'book-abc', got '%s'", created.LibraryItemID)
		}
		if created.Time != 123.45 {
			t.Errorf("Expected time 123.45, got %f", created.Time)
		}
		if created.Title != "My Test Bookmark" {
			t.Errorf("Expected title 'My Test Bookmark', got '%s'", created.Title)
		}

		// Verify database state
		user, err := idb.GetUserFullByID(context.Background(), db, "user-1")
		if err != nil {
			t.Fatalf("Failed to query user: %v", err)
		}
		var bookmarks []Bookmark
		json.Unmarshal(user.Bookmarks, &bookmarks)
		if len(bookmarks) != 1 {
			t.Errorf("Expected 1 bookmark in db, got %d", len(bookmarks))
		} else {
			if bookmarks[0].Title != "My Test Bookmark" {
				t.Errorf("Expected title 'My Test Bookmark' in db, got '%s'", bookmarks[0].Title)
			}
		}
	})

	// 2. Update Bookmark (PATCH /api/me/item/:id/bookmark)
	t.Run("Update Bookmark", func(t *testing.T) {
		payload := map[string]interface{}{
			"time":  123.45,
			"title": "My Updated Bookmark Title",
		}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest("PATCH", "/api/me/item/book-abc/bookmark", bytes.NewReader(body))
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, userSess))
		rr := httptest.NewRecorder()

		handler := handleMeUpdateBookmark(db)
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("Expected status 200 OK, got %d: %s", rr.Code, rr.Body.String())
		}

		var updated Bookmark
		if err := json.Unmarshal(rr.Body.Bytes(), &updated); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if updated.Title != "My Updated Bookmark Title" {
			t.Errorf("Expected updated title 'My Updated Bookmark Title', got '%s'", updated.Title)
		}

		// Verify database state
		user, err := idb.GetUserFullByID(context.Background(), db, "user-1")
		if err != nil {
			t.Fatalf("Failed to query user: %v", err)
		}
		var bookmarks []Bookmark
		json.Unmarshal(user.Bookmarks, &bookmarks)
		if len(bookmarks) != 1 || bookmarks[0].Title != "My Updated Bookmark Title" {
			t.Errorf("DB bookmark was not updated properly: %v", bookmarks)
		}
	})

	// 3. Remove Bookmark (DELETE /api/me/item/:id/bookmark/:time)
	t.Run("Remove Bookmark", func(t *testing.T) {
		req := httptest.NewRequest("DELETE", "/api/me/item/book-abc/bookmark/123.45", nil)
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, userSess))
		rr := httptest.NewRecorder()

		handler := handleMeRemoveBookmark(db)
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("Expected status 200 OK, got %d: %s", rr.Code, rr.Body.String())
		}

		// Verify database state
		user, err := idb.GetUserFullByID(context.Background(), db, "user-1")
		if err != nil {
			t.Fatalf("Failed to query user: %v", err)
		}
		var bookmarks []Bookmark
		json.Unmarshal(user.Bookmarks, &bookmarks)
		if len(bookmarks) != 0 {
			t.Errorf("Expected 0 bookmarks in db, got %d", len(bookmarks))
		}
	})
}
