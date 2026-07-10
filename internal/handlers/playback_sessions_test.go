package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"audiobookshelf/internal/core"
)

func TestGetPlaybackSessionsHandler(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Seed users
	_, err := db.Exec(`INSERT INTO users (id, username, type, isActive, permissions) VALUES 
		('user-root', 'rootuser', 'root', 1, '{}'),
		('user-admin', 'adminuser', 'admin', 1, '{}'),
		('user-normal', 'normaluser', 'user', 1, '{}')`)
	if err != nil {
		t.Fatalf("Failed to seed users: %v", err)
	}

	// Seed media (books, libraryItems, podcasts, podcastEpisodes)
	_, err = db.Exec(`INSERT INTO books (id, title) VALUES ('book-1', 'The Great Novel')`)
	if err != nil {
		t.Fatalf("Failed to seed books: %v", err)
	}
	_, err = db.Exec(`INSERT INTO libraryItems (id, mediaId, mediaType, authorNamesFirstLast, title) VALUES ('item-1', 'book-1', 'book', 'Jane Doe', 'The Great Novel')`)
	if err != nil {
		t.Fatalf("Failed to seed libraryItems: %v", err)
	}

	_, err = db.Exec(`INSERT INTO podcasts (id, title, author) VALUES ('podcast-1', 'Tech Talk', 'Alice Smith')`)
	if err != nil {
		t.Fatalf("Failed to seed podcasts: %v", err)
	}

	_, err = db.Exec(`INSERT INTO podcastEpisodes (id, podcastId, title) VALUES ('episode-1', 'podcast-1', 'Episode One')`)
	if err != nil {
		t.Fatalf("Failed to seed podcastEpisodes: %v", err)
	}

	// Seed playbackSessions
	// session 1: book, full extraData, root user, updatedAt = '2026-06-19 12:00:00'
	_, err = db.Exec(`INSERT INTO playbackSessions (id, userId, mediaItemId, mediaItemType, startTime, libraryId, extraData, createdAt, updatedAt) 
		VALUES ('sess-1', 'user-root', 'book-1', 'book', 10.0, 'lib-1', 
		'{"playMethod":"HLS","deviceInfo":"iPhone","timeListened":120.5,"lastTime":45.0}', 
		'2026-06-19 11:00:00', '2026-06-19 12:00:00')`)
	if err != nil {
		t.Fatalf("Failed to seed playbackSession 1: %v", err)
	}

	// session 2: podcastEpisode, empty/missing extraData, admin user, updatedAt = '2026-06-19 13:00:00'
	_, err = db.Exec(`INSERT INTO playbackSessions (id, userId, mediaItemId, mediaItemType, startTime, libraryId, extraData, createdAt, updatedAt) 
		VALUES ('sess-2', 'user-admin', 'episode-1', 'podcastEpisode', 0.0, 'lib-1', 
		'', 
		'2026-06-19 13:00:00', '2026-06-19 13:00:00')`)
	if err != nil {
		t.Fatalf("Failed to seed playbackSession 2: %v", err)
	}

	// session 3: podcast, invalid JSON extraData, normal user, updatedAt = '2026-06-19 10:00:00'
	_, err = db.Exec(`INSERT INTO playbackSessions (id, userId, mediaItemId, mediaItemType, startTime, libraryId, extraData, createdAt, updatedAt) 
		VALUES ('sess-3', 'user-normal', 'podcast-1', 'podcast', 5.0, 'lib-1', 
		'invalid-json', 
		'2026-06-19 10:00:00', '2026-06-19 10:00:00')`)
	if err != nil {
		t.Fatalf("Failed to seed playbackSession 3: %v", err)
	}

	rootSession := &core.UserSession{ID: "user-root", Username: "rootuser", Type: "root", IsActive: true}
	adminSession := &core.UserSession{ID: "user-admin", Username: "adminuser", Type: "admin", IsActive: true}
	normalSession := &core.UserSession{ID: "user-normal", Username: "normaluser", Type: "user", IsActive: true}

	t.Run("Unauthorized missing session", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/playback-sessions", nil)
		rr := httptest.NewRecorder()
		handleGetPlaybackSessions(db).ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Errorf("Expected 401, got %d", rr.Code)
		}
	})

	t.Run("Forbidden for non-admin non-root user", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/playback-sessions", nil)
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, normalSession))
		rr := httptest.NewRecorder()
		handleGetPlaybackSessions(db).ServeHTTP(rr, req)

		if rr.Code != http.StatusForbidden {
			t.Errorf("Expected 403, got %d", rr.Code)
		}
	})

	t.Run("Get all sessions ordered by updatedAt DESC", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/playback-sessions", nil)
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, adminSession))
		rr := httptest.NewRecorder()
		handleGetPlaybackSessions(db).ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("Expected 200, got %d", rr.Code)
		}

		var resp struct {
			Sessions []struct {
				ID            string  `json:"id"`
				UserID        string  `json:"userId"`
				Username      string  `json:"username"`
				MediaItemID   string  `json:"mediaItemId"`
				MediaItemType string  `json:"mediaItemType"`
				Title         string  `json:"title"`
				Author        string  `json:"author"`
				StartTime     float64 `json:"startTime"`
				TimeListened  float64 `json:"timeListened"`
				LastTime      float64 `json:"lastTime"`
				UpdatedAt     string  `json:"updatedAt"`
				PlayMethod    string  `json:"playMethod"`
				DeviceInfo    string  `json:"deviceInfo"`
			} `json:"sessions"`
		}

		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to unmarshal response: %v", err)
		}

		if len(resp.Sessions) != 3 {
			t.Fatalf("Expected 3 sessions, got %d", len(resp.Sessions))
		}

		// Order should be sess-2 (13:00), sess-1 (12:00), sess-3 (10:00)
		if resp.Sessions[0].ID != "sess-2" || resp.Sessions[1].ID != "sess-1" || resp.Sessions[2].ID != "sess-3" {
			t.Errorf("Unexpected session ordering: %s, %s, %s", resp.Sessions[0].ID, resp.Sessions[1].ID, resp.Sessions[2].ID)
		}

		// Verify sess-2 (podcastEpisode): fallback playMethod & deviceInfo, resolved title/author
		s2 := resp.Sessions[0]
		if s2.Title != "Episode One" {
			t.Errorf("Expected title 'Episode One', got %q", s2.Title)
		}
		if s2.Author != "Alice Smith" {
			t.Errorf("Expected author 'Alice Smith', got %q", s2.Author)
		}
		if s2.PlayMethod != "HLS" || s2.DeviceInfo != "Web Client" {
			t.Errorf("Expected fallbacks for sess-2: %q, %q", s2.PlayMethod, s2.DeviceInfo)
		}

		// Verify sess-1 (book): full extraData
		s1 := resp.Sessions[1]
		if s1.Title != "The Great Novel" {
			t.Errorf("Expected title 'The Great Novel', got %q", s1.Title)
		}
		if s1.Author != "Jane Doe" {
			t.Errorf("Expected author 'Jane Doe', got %q", s1.Author)
		}
		if s1.PlayMethod != "HLS" || s1.DeviceInfo != "iPhone" || s1.TimeListened != 120.5 || s1.LastTime != 45.0 {
			t.Errorf("Unexpected session data for sess-1: %+v", s1)
		}
	})

	t.Run("Filter sessions by userId", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/playback-sessions?userId=user-root", nil)
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, rootSession))
		rr := httptest.NewRecorder()
		handleGetPlaybackSessions(db).ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("Expected 200, got %d", rr.Code)
		}

		var resp struct {
			Sessions []struct {
				ID string `json:"id"`
			} `json:"sessions"`
		}

		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to unmarshal response: %v", err)
		}

		if len(resp.Sessions) != 1 {
			t.Fatalf("Expected 1 session for user-root, got %d", len(resp.Sessions))
		}

		if resp.Sessions[0].ID != "sess-1" {
			t.Errorf("Expected sess-1, got %s", resp.Sessions[0].ID)
		}
	})

	t.Run("DELETE playback-session forbidden non-owner", func(t *testing.T) {
		req := httptest.NewRequest("DELETE", "/api/playback-sessions/sess-1", nil)
		// normalSession is user-normal, sess-1 belongs to user-root
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, normalSession))
		rr := httptest.NewRecorder()
		handleClosePlaybackSession(db, "sess-1").ServeHTTP(rr, req)

		if rr.Code != http.StatusForbidden {
			t.Errorf("Expected 403, got %d", rr.Code)
		}
	})

	t.Run("DELETE playback-session non-existent", func(t *testing.T) {
		req := httptest.NewRequest("DELETE", "/api/playback-sessions/non-existent-sess", nil)
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, rootSession))
		rr := httptest.NewRecorder()
		handleClosePlaybackSession(db, "non-existent-sess").ServeHTTP(rr, req)

		if rr.Code != http.StatusNotFound {
			t.Errorf("Expected 404, got %d", rr.Code)
		}
	})

	t.Run("DELETE playback-session allowed owner", func(t *testing.T) {
		req := httptest.NewRequest("DELETE", "/api/playback-sessions/sess-3", nil)
		// sess-3 belongs to user-normal (normalSession)
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, normalSession))
		rr := httptest.NewRecorder()
		handleClosePlaybackSession(db, "sess-3").ServeHTTP(rr, req)

		if rr.Code != http.StatusNoContent {
			t.Errorf("Expected 204, got %d", rr.Code)
		}

		// Verify deletion
		var count int
		err := db.QueryRow("SELECT COUNT(*) FROM playbackSessions WHERE id = 'sess-3'").Scan(&count)
		if err != nil {
			t.Fatalf("Failed to query DB: %v", err)
		}
		if count != 0 {
			t.Errorf("Expected session to be deleted, count: %d", count)
		}
	})

	t.Run("DELETE playback-session allowed admin", func(t *testing.T) {
		req := httptest.NewRequest("DELETE", "/api/playback-sessions/sess-1", nil)
		// sess-1 belongs to user-root, adminuser is user-admin (adminSession)
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, adminSession))
		rr := httptest.NewRecorder()
		handleClosePlaybackSession(db, "sess-1").ServeHTTP(rr, req)

		if rr.Code != http.StatusNoContent {
			t.Errorf("Expected 204, got %d", rr.Code)
		}

		// Verify deletion
		var count int
		err := db.QueryRow("SELECT COUNT(*) FROM playbackSessions WHERE id = 'sess-1'").Scan(&count)
		if err != nil {
			t.Fatalf("Failed to query DB: %v", err)
		}
		if count != 0 {
			t.Errorf("Expected session to be deleted, count: %d", count)
		}
	})
}
