package handlers

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"

	"audiobookshelf/internal/core"
)

func setupProgressTestDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open memory db: %v", err)
	}
	db.SetMaxOpenConns(1)

	queries := []string{
		`CREATE TABLE users (id TEXT PRIMARY KEY, username TEXT, email TEXT, pash TEXT, type TEXT, token TEXT, isActive INTEGER, isLocked INTEGER, lastSeen INTEGER, permissions TEXT, bookmarks TEXT, extraData TEXT, createdAt TEXT, updatedAt TEXT)`,
		`CREATE TABLE series (id TEXT PRIMARY KEY, name TEXT)`,
		`CREATE TABLE libraryItems (id TEXT PRIMARY KEY, mediaId TEXT, mediaType TEXT, title TEXT)`,
		`CREATE TABLE books (id TEXT PRIMARY KEY, title TEXT)`,
		`CREATE TABLE podcasts (id TEXT PRIMARY KEY, title TEXT, autoDeletePlayed INTEGER)`,
		`CREATE TABLE podcastEpisodes (id TEXT PRIMARY KEY, podcastId TEXT, title TEXT, audioFile TEXT)`,
		`CREATE TABLE mediaProgresses (
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
		)`,
		`CREATE TABLE playbackSessions (
			id TEXT PRIMARY KEY, 
			userId TEXT, 
			mediaItemId TEXT, 
			mediaItemType TEXT, 
			startTime REAL, 
			libraryId TEXT, 
			extraData TEXT, 
			createdAt TEXT, 
			updatedAt TEXT
		)`,
	}

	for _, q := range queries {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("Failed to execute query %q: %v", q, err)
		}
	}
	return db
}

func TestSeriesContinueListeningDiscrepancy(t *testing.T) {
	db := setupProgressTestDB(t)
	defer db.Close()

	// Seed user and series
	_, err := db.Exec(`INSERT INTO users (id, username, type, isActive, extraData) VALUES ('user-1', 'testuser', 'user', 1, '{}')`)
	if err != nil {
		t.Fatalf("Failed to seed user: %v", err)
	}
	_, err = db.Exec(`INSERT INTO series (id, name) VALUES ('series-123', 'My Test Series')`)
	if err != nil {
		t.Fatalf("Failed to seed series: %v", err)
	}

	userSess := &core.UserSession{ID: "user-1", Username: "testuser", Type: "user", IsActive: true}

	// Test 1: Direct call to handleRemoveSeriesFromContinueListening with path "/api/me/series/series-123/remove"
	{
		req := httptest.NewRequest("POST", "/api/me/series/series-123/remove", nil)
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, userSess))
		rr := httptest.NewRecorder()

		handleRemoveSeriesFromContinueListening(db).ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d. Body: %s", http.StatusOK, rr.Code, rr.Body.String())
		}
	}

	// Test 2: Direct call to handleReaddSeriesFromContinueListening with path "/api/me/series/series-123/readd"
	{
		req := httptest.NewRequest("POST", "/api/me/series/series-123/readd", nil)
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, userSess))
		rr := httptest.NewRecorder()

		handleReaddSeriesFromContinueListening(db).ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d. Body: %s", http.StatusOK, rr.Code, rr.Body.String())
		}
	}
}
