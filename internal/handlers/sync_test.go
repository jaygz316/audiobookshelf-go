package handlers

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"audiobookshelf/internal/core"
)

func setupSyncTestDB(t *testing.T) *sql.DB {
	db := setupTestDB(t)
	_, _ = db.Exec("DROP TABLE mediaProgresses")
	_, err := db.Exec(`
		CREATE TABLE mediaProgresses (
			id TEXT PRIMARY KEY,
			userId TEXT,
			mediaItemId TEXT,
			mediaItemType TEXT,
			duration REAL,
			currentTime REAL,
			isFinished INTEGER,
			hideFromContinueListening INTEGER,
			ebookLocation TEXT,
			ebookProgress REAL,
			finishedAt TEXT,
			extraData TEXT,
			podcastId TEXT,
			createdAt TEXT,
			updatedAt TEXT
		)
	`)
	if err != nil {
		t.Fatalf("Failed to recreate mediaProgresses table: %v", err)
	}
	return db
}

func TestSyncLocalProgress(t *testing.T) {
	db := setupSyncTestDB(t)
	defer db.Close()

	// Seed users
	_, err := db.Exec(`INSERT INTO users (id, username, type, isActive, permissions) VALUES ('user-1', 'testuser', 'user', 1, '{}')`)
	if err != nil {
		t.Fatalf("Failed to seed user: %v", err)
	}

	// Seed book & library item
	_, err = db.Exec(`INSERT INTO books (id, title) VALUES ('book-1', 'Test Book')`)
	if err != nil {
		t.Fatalf("Failed to seed book: %v", err)
	}
	_, err = db.Exec(`INSERT INTO libraryItems (id, mediaId, mediaType, title) VALUES ('item-1', 'book-1', 'book', 'Test Book')`)
	if err != nil {
		t.Fatalf("Failed to seed library item: %v", err)
	}

	userSess := &core.UserSession{
		ID:       "user-1",
		Username: "testuser",
		Type:     "user",
		IsActive: true,
	}

	t.Run("Valid Sync - Creates Record", func(t *testing.T) {
		payload := LocalMediaProgressPayload{
			LocalMediaProgress: []LocalMediaProgressItem{
				{
					LibraryItemID: "item-1",
					Duration:      1000.0,
					CurrentTime:   func() *float64 { f := 250.0; return &f }(),
					IsFinished:    false,
					UpdatedAt:     float64(time.Now().UnixMilli()),
				},
			},
		}

		body, _ := json.Marshal(payload)
		req := httptest.NewRequest("POST", "/api/me/sync-local-progress", bytes.NewReader(body))
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, userSess))
		rr := httptest.NewRecorder()

		handleSyncLocalProgress(db).ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("Expected 200 OK, got %d. Body: %s", rr.Code, rr.Body.String())
		}

		// Verify database
		var count int
		var currTime float64
		err := db.QueryRow(`SELECT COUNT(*), currentTime FROM mediaProgresses WHERE userId = 'user-1' AND mediaItemId = 'book-1'`).Scan(&count, &currTime)
		if err != nil {
			t.Fatalf("Failed to query mediaProgresses: %v", err)
		}
		if count != 1 {
			t.Errorf("Expected 1 progress record, got %d", count)
		}
		if currTime != 250.0 {
			t.Errorf("Expected currentTime to be 250.0, got %f", currTime)
		}
	})

	t.Run("Stale Sync - Does Not Overwrite Newer Server Record", func(t *testing.T) {
		// Update record on server to be very recent
		serverTimeStr := "2026-07-14 02:00:00.000 +00:00"
		_, err := db.Exec(`UPDATE mediaProgresses SET currentTime = 500.0, updatedAt = ? WHERE userId = 'user-1' AND mediaItemId = 'book-1'`, serverTimeStr)
		if err != nil {
			t.Fatalf("Failed to update mediaProgresses: %v", err)
		}

		// Client sends an older progress (recorded in the past)
		pastTimeMs := float64(time.Date(2026, 7, 14, 1, 0, 0, 0, time.UTC).UnixMilli())
		payload := LocalMediaProgressPayload{
			LocalMediaProgress: []LocalMediaProgressItem{
				{
					LibraryItemID: "item-1",
					Duration:      1000.0,
					CurrentTime:   func() *float64 { f := 100.0; return &f }(),
					IsFinished:    false,
					UpdatedAt:     pastTimeMs,
				},
			},
		}

		body, _ := json.Marshal(payload)
		req := httptest.NewRequest("POST", "/api/me/sync-local-progress", bytes.NewReader(body))
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, userSess))
		rr := httptest.NewRecorder()

		handleSyncLocalProgress(db).ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("Expected 200 OK, got %d. Body: %s", rr.Code, rr.Body.String())
		}

		// Verify database currentTime is still 500.0 (newer server version)
		var currTime float64
		err = db.QueryRow(`SELECT currentTime FROM mediaProgresses WHERE userId = 'user-1' AND mediaItemId = 'book-1'`).Scan(&currTime)
		if err != nil {
			t.Fatalf("Failed to query mediaProgresses: %v", err)
		}
		if currTime != 500.0 {
			t.Errorf("Expected newer server progress (500.0) to be preserved, got %f", currTime)
		}
	})
}

func TestSyncLocalSessions(t *testing.T) {
	db := setupSyncTestDB(t)
	defer db.Close()

	// Seed users
	_, err := db.Exec(`INSERT INTO users (id, username, type, isActive, permissions) VALUES ('user-1', 'testuser', 'user', 1, '{}')`)
	if err != nil {
		t.Fatalf("Failed to seed user: %v", err)
	}

	// Seed book & library item
	_, err = db.Exec(`INSERT INTO books (id, title) VALUES ('book-1', 'Test Book')`)
	if err != nil {
		t.Fatalf("Failed to seed book: %v", err)
	}
	_, err = db.Exec(`INSERT INTO libraryItems (id, mediaId, mediaType, title) VALUES ('item-1', 'book-1', 'book', 'Test Book')`)
	if err != nil {
		t.Fatalf("Failed to seed library item: %v", err)
	}

	userSess := &core.UserSession{
		ID:       "user-1",
		Username: "testuser",
		Type:     "user",
		IsActive: true,
	}

	t.Run("Valid Sessions Sync", func(t *testing.T) {
		startedAt := time.Now().Add(-1 * time.Hour).UnixMilli()
		updatedAt := time.Now().UnixMilli()

		payload := LocalSessionsPayload{
			Sessions: []LocalSessionItem{
				{
					ID:            "sess-uuid-1",
					LibraryID:     "lib-1",
					LibraryItemID: "item-1",
					TimeListening: 120.0,
					StartTime:     0.0,
					CurrentTime:   120.0,
					StartedAt:     float64(startedAt),
					UpdatedAt:     float64(updatedAt),
					Duration:      1000.0,
					PlayMethod:    "HLS",
					MediaPlayer:   "native",
					DeviceInfo: map[string]interface{}{
						"browserName": "Absorb App",
						"osName":      "Android",
					},
				},
			},
		}

		body, _ := json.Marshal(payload)
		req := httptest.NewRequest("POST", "/api/session/local-all", bytes.NewReader(body))
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, userSess))
		rr := httptest.NewRecorder()

		handleSyncLocalSessions(db).ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("Expected 200 OK, got %d. Body: %s", rr.Code, rr.Body.String())
		}

		var resp SyncSessionsResponse
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to unmarshal response: %v", err)
		}

		if len(resp.Results) != 1 {
			t.Fatalf("Expected 1 result, got %d", len(resp.Results))
		}

		if !resp.Results[0].Success {
			t.Errorf("Expected session sync success to be true")
		}

		if !resp.Results[0].ProgressSynced {
			t.Errorf("Expected progressSynced to be true")
		}

		// Verify playback session in database
		var count int
		var extraStr string
		err := db.QueryRow(`SELECT COUNT(*), extraData FROM playbackSessions WHERE id = 'sess-uuid-1'`).Scan(&count, &extraStr)
		if err != nil {
			t.Fatalf("Failed to query playbackSessions: %v", err)
		}
		if count != 1 {
			t.Errorf("Expected 1 playback session in DB, got %d", count)
		}

		var extra map[string]interface{}
		json.Unmarshal([]byte(extraStr), &extra)
		if extra["deviceInfo"] != "Absorb App / Android" {
			t.Errorf("Expected deviceInfo stringified to 'Absorb App / Android', got %q", extra["deviceInfo"])
		}

		// Verify mediaProgresses update
		var progressCount int
		var progressTime float64
		err = db.QueryRow(`SELECT COUNT(*), currentTime FROM mediaProgresses WHERE userId = 'user-1' AND mediaItemId = 'book-1'`).Scan(&progressCount, &progressTime)
		if err != nil {
			t.Fatalf("Failed to query mediaProgresses: %v", err)
		}
		if progressCount != 1 {
			t.Errorf("Expected 1 progress record, got %d", progressCount)
		}
		if progressTime != 120.0 {
			t.Errorf("Expected progress currentTime to be updated to 120.0, got %f", progressTime)
		}
	})
}

func TestSyncLocalSession(t *testing.T) {
	db := setupSyncTestDB(t)
	defer db.Close()

	// Seed libraryItems and mock users
	ctx := context.Background()
	_, err := db.Exec(`INSERT INTO users (id, username, type, isActive, permissions) VALUES ('user-1', 'testuser', 'user', 1, '{}')`)
	if err != nil {
		t.Fatalf("Failed to seed user: %v", err)
	}
	_, err = db.Exec(`INSERT INTO books (id, title) VALUES ('book-1', 'Test Book')`)
	if err != nil {
		t.Fatalf("Failed to seed book: %v", err)
	}
	_, err = db.Exec(`INSERT INTO libraryItems (id, mediaId, mediaType, title) VALUES ('book-1', 'book-1', 'book', 'Test Book')`)
	if err != nil {
		t.Fatalf("Failed to seed libraryItems: %v", err)
	}

	t.Run("Sync single local session", func(t *testing.T) {
		sessionItem := LocalSessionItem{
			ID:            "sess-single-1",
			LibraryID:     "lib-1",
			LibraryItemID: "book-1",
			TimeListening: 60.0,
			StartTime:     0.0,
			CurrentTime:   180.0,
			Duration:      1000.0,
			StartedAt:     "2026-07-10T10:00:00Z",
			UpdatedAt:     "2026-07-10T10:30:00Z",
			PlayMethod:    0, // Direct Play
			DeviceInfo: map[string]interface{}{
				"clientName": "ShelfPlayer",
				"osName":     "iOS",
			},
		}

		body, err := json.Marshal(sessionItem)
		if err != nil {
			t.Fatalf("Failed to marshal body: %v", err)
		}

		req := httptest.NewRequest("POST", "/api/session/local", bytes.NewReader(body))
		userSess := &core.UserSession{ID: "user-1"}
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, userSess))

		rr := httptest.NewRecorder()
		handleSyncLocalSession(db).ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d. Body: %s", rr.Code, rr.Body.String())
		}

		var result SyncSessionResult
		if err := json.NewDecoder(rr.Body).Decode(&result); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if result.ID != "sess-single-1" {
			t.Errorf("Expected session ID 'sess-single-1', got %q", result.ID)
		}
		if !result.Success {
			t.Error("Expected Success to be true")
		}
		if !result.ProgressSynced {
			t.Error("Expected ProgressSynced to be true")
		}

		// Verify database state
		var count int
		var extraStr string
		err = db.QueryRowContext(ctx, `SELECT count(*), extraData FROM playbackSessions WHERE id = 'sess-single-1'`).Scan(&count, &extraStr)
		if err != nil {
			t.Fatalf("Failed to query playbackSessions: %v", err)
		}
		if count != 1 {
			t.Errorf("Expected 1 playback session, got %d", count)
		}

		var extra map[string]interface{}
		json.Unmarshal([]byte(extraStr), &extra)
		if extra["deviceInfo"] != "ShelfPlayer / iOS" {
			t.Errorf("Expected deviceInfo stringified to 'ShelfPlayer / iOS', got %q", extra["deviceInfo"])
		}

		// Verify mediaProgresses
		var progressCount int
		var progressTime float64
		err = db.QueryRow(`SELECT count(*), currentTime FROM mediaProgresses WHERE userId = 'user-1' AND mediaItemId = 'book-1'`).Scan(&progressCount, &progressTime)
		if err != nil {
			t.Fatalf("Failed to query mediaProgresses: %v", err)
		}
		if progressCount != 1 {
			t.Errorf("Expected 1 progress record, got %d", progressCount)
		}
		if progressTime != 180.0 {
			t.Errorf("Expected progress currentTime to be updated to 180.0, got %f", progressTime)
		}
	})
}
