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
)

func TestHandleCreateUpdateMeProgress(t *testing.T) {
	db := setupProgressTestDB(t)
	defer db.Close()

	// Seed user
	_, err := db.Exec(`INSERT INTO users (id, username, type, isActive, extraData) VALUES ('user-1', 'testuser', 'user', 1, '{}')`)
	if err != nil {
		t.Fatalf("Failed to seed user: %v", err)
	}

	// Seed libraryItem (book)
	_, err = db.Exec(`INSERT INTO libraryItems (id, mediaId, mediaType, title) VALUES ('item-1', 'book-1', 'book', 'Test Book')`)
	if err != nil {
		t.Fatalf("Failed to seed libraryItem: %v", err)
	}

	userSess := &core.UserSession{ID: "user-1", Username: "testuser", Type: "user", IsActive: true}

	t.Run("Create media progress success", func(t *testing.T) {
		payload := map[string]interface{}{
			"duration":    1200.0,
			"currentTime": 10.0,
		}
		body, _ := json.Marshal(payload)

		req := httptest.NewRequest("PATCH", "/api/me/progress/item-1", bytes.NewReader(body))
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, userSess))
		rr := httptest.NewRecorder()

		handleCreateUpdateMeProgress(db).ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("Expected status %d, got %d. Body: %s", http.StatusOK, rr.Code, rr.Body.String())
		}

		// Verify mediaProgresses in DB
		var duration, currentTime float64
		var isFinished int
		err = db.QueryRow("SELECT duration, currentTime, isFinished FROM mediaProgresses WHERE userId = ? AND mediaItemId = ?", "user-1", "book-1").Scan(&duration, &currentTime, &isFinished)
		if err != nil {
			t.Fatalf("Failed to query media progress from DB: %v", err)
		}

		if duration != 1200.0 || currentTime != 10.0 || isFinished != 0 {
			t.Errorf("Unexpected progress values: duration=%f, currentTime=%f, isFinished=%d", duration, currentTime, isFinished)
		}
	})

	t.Run("Update media progress and verify auto-finish", func(t *testing.T) {
		payload := map[string]interface{}{
			"currentTime": 1195.0,
		}
		body, _ := json.Marshal(payload)

		req := httptest.NewRequest("PATCH", "/api/me/progress/item-1", bytes.NewReader(body))
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, userSess))
		rr := httptest.NewRecorder()

		handleCreateUpdateMeProgress(db).ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("Expected status %d, got %d. Body: %s", http.StatusOK, rr.Code, rr.Body.String())
		}

		var isFinished int
		var finishedAt sql.NullString
		err = db.QueryRow("SELECT isFinished, finishedAt FROM mediaProgresses WHERE userId = ? AND mediaItemId = ?", "user-1", "book-1").Scan(&isFinished, &finishedAt)
		if err != nil {
			t.Fatalf("Failed to query progress: %v", err)
		}

		if isFinished != 1 || !finishedAt.Valid || finishedAt.String == "" {
			t.Errorf("Expected progress to be finished, got isFinished=%d, finishedAt=%v", isFinished, finishedAt)
		}
	})
}
