package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"audiobookshelf/internal/core"
	idb "audiobookshelf/internal/db"
)

func TestBookmarkHandlers_ChallengerEdgeCases(t *testing.T) {
	db := prepareTestDBForBookmarks(t)
	defer db.Close()

	// Seed a test user
	_, err := db.Exec(`INSERT INTO users (id, username, type, isActive, token, bookmarks, createdAt, updatedAt) 
		VALUES ('challenger-user', 'cuser', 'user', 1, 'token-c', '[]', '2026-06-08 12:00:00.000', '2026-06-08 12:00:00.000')`)
	if err != nil {
		t.Fatalf("Failed to seed user: %v", err)
	}

	userSess := &core.UserSession{
		ID:       "challenger-user",
		Username: "cuser",
		Type:     "user",
		IsActive: true,
	}

	// 1. Test unauthorized access (No UserContextKey)
	t.Run("Create Bookmark - Unauthorized", func(t *testing.T) {
		payload := map[string]interface{}{
			"time":  12.34,
			"title": "Unauth Title",
		}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest("POST", "/api/me/item/book-1/bookmark", bytes.NewReader(body))
		rr := httptest.NewRecorder()

		handler := handleMeCreateBookmark(db)
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Errorf("Expected 401 Unauthorized, got %d", rr.Code)
		}
	})

	// 2. Test invalid request body
	t.Run("Create Bookmark - Invalid JSON", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/me/item/book-1/bookmark", bytes.NewReader([]byte("{invalid json")))
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, userSess))
		rr := httptest.NewRecorder()

		handler := handleMeCreateBookmark(db)
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("Expected 400 Bad Request, got %d", rr.Code)
		}
	})

	// 3. Test empty title
	t.Run("Create Bookmark - Empty Title", func(t *testing.T) {
		payload := map[string]interface{}{
			"time":  12.34,
			"title": "",
		}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest("POST", "/api/me/item/book-1/bookmark", bytes.NewReader(body))
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, userSess))
		rr := httptest.NewRecorder()

		handler := handleMeCreateBookmark(db)
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("Expected 400 Bad Request, got %d", rr.Code)
		}
	})

	// 4. Test invalid library item IDs containing slash
	t.Run("Create Bookmark - Invalid ID with slash", func(t *testing.T) {
		payload := map[string]interface{}{
			"time":  12.34,
			"title": "Valid Title",
		}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest("POST", "/api/me/item/book/abc/bookmark", bytes.NewReader(body))
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, userSess))
		rr := httptest.NewRecorder()

		handler := handleMeCreateBookmark(db)
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("Expected 400 Bad Request, got %d", rr.Code)
		}
	})

	// 5. Test update non-existent bookmark
	t.Run("Update Bookmark - Non-existent", func(t *testing.T) {
		payload := map[string]interface{}{
			"time":  9999.9,
			"title": "Non-existent Title",
		}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest("PATCH", "/api/me/item/book-1/bookmark", bytes.NewReader(body))
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, userSess))
		rr := httptest.NewRecorder()

		handler := handleMeUpdateBookmark(db)
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusNotFound {
			t.Errorf("Expected 404 Not Found, got %d", rr.Code)
		}
	})

	// 6. Test delete non-existent bookmark
	t.Run("Delete Bookmark - Non-existent", func(t *testing.T) {
		req := httptest.NewRequest("DELETE", "/api/me/item/book-1/bookmark/9999.9", nil)
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, userSess))
		rr := httptest.NewRecorder()

		handler := handleMeRemoveBookmark(db)
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusNotFound {
			t.Errorf("Expected 404 Not Found, got %d", rr.Code)
		}
	})

	// 7. Success case: Create, Update, Delete flow
	t.Run("Create Update Delete Success Flow", func(t *testing.T) {
		// Create
		payload := map[string]interface{}{
			"time":  500.25,
			"title": "Flow Bookmark",
			"note":  "Initial note",
			"color": "#ffffff",
			"cfi":   "cfi-value",
		}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest("POST", "/api/me/item/book-flow/bookmark", bytes.NewReader(body))
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, userSess))
		rr := httptest.NewRecorder()

		handleMeCreateBookmark(db).ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("Create failed: %s", rr.Body.String())
		}

		// Update
		payloadUpdate := map[string]interface{}{
			"time":  500.25,
			"title": "Flow Bookmark Updated",
			"note":  "Updated note",
			"color": "#000000",
			"cfi":   "cfi-updated",
		}
		bodyUpdate, _ := json.Marshal(payloadUpdate)
		reqUpdate := httptest.NewRequest("PATCH", "/api/me/item/book-flow/bookmark", bytes.NewReader(bodyUpdate))
		reqUpdate = reqUpdate.WithContext(context.WithValue(reqUpdate.Context(), core.UserContextKey, userSess))
		rrUpdate := httptest.NewRecorder()

		handleMeUpdateBookmark(db).ServeHTTP(rrUpdate, reqUpdate)
		if rrUpdate.Code != http.StatusOK {
			t.Fatalf("Update failed: %s", rrUpdate.Body.String())
		}

		// Verify values in DB
		user, err := idb.GetUserFullByID(context.Background(), db, userSess.ID)
		if err != nil {
			t.Fatalf("GetUser failed: %v", err)
		}
		var bookmarks []Bookmark
		json.Unmarshal(user.Bookmarks, &bookmarks)
		if len(bookmarks) != 1 {
			t.Fatalf("Expected 1 bookmark, got %d", len(bookmarks))
		}
		if bookmarks[0].Title != "Flow Bookmark Updated" || bookmarks[0].Cfi != "cfi-updated" {
			t.Errorf("Values not updated correctly in DB: %+v", bookmarks[0])
		}

		// Delete
		reqDelete := httptest.NewRequest("DELETE", "/api/me/item/book-flow/bookmark/500.25", nil)
		reqDelete = reqDelete.WithContext(context.WithValue(reqDelete.Context(), core.UserContextKey, userSess))
		rrDelete := httptest.NewRecorder()

		handleMeRemoveBookmark(db).ServeHTTP(rrDelete, reqDelete)
		if rrDelete.Code != http.StatusOK {
			t.Fatalf("Delete failed: %s", rrDelete.Body.String())
		}

		// Verify empty
		user, _ = idb.GetUserFullByID(context.Background(), db, userSess.ID)
		json.Unmarshal(user.Bookmarks, &bookmarks)
		if len(bookmarks) != 0 {
			t.Errorf("Expected 0 bookmarks, got %d", len(bookmarks))
		}
	})
}
