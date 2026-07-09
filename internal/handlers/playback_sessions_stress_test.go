package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"audiobookshelf/internal/core"
)

func TestPlaybackSessionsStressAndEdgeCases(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Seed users
	_, err := db.Exec(`INSERT INTO users (id, username, type, isActive, permissions) VALUES 
		('user-root', 'rootuser', 'root', 1, '{}'),
		('user-normal', 'normaluser', 'user', 1, '{}')`)
	if err != nil {
		t.Fatalf("Failed to seed users: %v", err)
	}

	// Seed media (books, libraryItems)
	_, err = db.Exec(`INSERT INTO books (id, title) VALUES ('book-1', 'Test Book 1')`)
	if err != nil {
		t.Fatalf("Failed to seed books: %v", err)
	}
	_, err = db.Exec(`INSERT INTO libraryItems (id, mediaId, mediaType, authorNamesFirstLast, title) VALUES ('item-1', 'book-1', 'book', 'Jane Doe', 'Test Book 1')`)
	if err != nil {
		t.Fatalf("Failed to seed libraryItems: %v", err)
	}

	// Setup root session for API access
	rootSession := &core.UserSession{ID: "user-root", Username: "rootuser", Type: "root", IsActive: true}

	t.Run("Missing extraData (NULL and empty)", func(t *testing.T) {
		// Clean playbackSessions
		_, _ = db.Exec("DELETE FROM playbackSessions")

		// 1. extraData is NULL
		_, err = db.Exec(`INSERT INTO playbackSessions (id, userId, mediaItemId, mediaItemType, startTime, libraryId, extraData, createdAt, updatedAt) 
			VALUES ('sess-null-extra', 'user-root', 'book-1', 'book', 10.0, 'lib-1', 
			NULL, 
			'2026-06-19 11:00:00', '2026-06-19 12:00:00')`)
		if err != nil {
			t.Fatalf("Failed to seed playbackSession: %v", err)
		}

		// 2. extraData is empty string
		_, err = db.Exec(`INSERT INTO playbackSessions (id, userId, mediaItemId, mediaItemType, startTime, libraryId, extraData, createdAt, updatedAt) 
			VALUES ('sess-empty-extra', 'user-root', 'book-1', 'book', 10.0, 'lib-1', 
			'', 
			'2026-06-19 11:00:00', '2026-06-19 12:05:00')`)
		if err != nil {
			t.Fatalf("Failed to seed playbackSession: %v", err)
		}

		req := httptest.NewRequest("GET", "/api/playback-sessions", nil)
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, rootSession))
		rr := httptest.NewRecorder()
		handleGetPlaybackSessions(db).ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("Expected 200, got %d. Body: %s", rr.Code, rr.Body.String())
		}

		var resp struct {
			Sessions []struct {
				ID           string  `json:"id"`
				PlayMethod   string  `json:"playMethod"`
				DeviceInfo   string  `json:"deviceInfo"`
				TimeListened float64 `json:"timeListened"`
				LastTime     float64 `json:"lastTime"`
			} `json:"sessions"`
		}

		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to unmarshal response: %v", err)
		}

		if len(resp.Sessions) != 2 {
			t.Fatalf("Expected 2 sessions, got %d", len(resp.Sessions))
		}

		for _, s := range resp.Sessions {
			if s.PlayMethod != "HLS" {
				t.Errorf("Session %s: expected playMethod 'HLS', got %q", s.ID, s.PlayMethod)
			}
			if s.DeviceInfo != "Web Client" {
				t.Errorf("Session %s: expected deviceInfo 'Web Client', got %q", s.ID, s.DeviceInfo)
			}
			if s.TimeListened != 0.0 {
				t.Errorf("Session %s: expected timeListened 0.0, got %f", s.ID, s.TimeListened)
			}
			if s.LastTime != 0.0 {
				t.Errorf("Session %s: expected lastTime 0.0, got %f", s.ID, s.LastTime)
			}
		}
	})

	t.Run("Malformed extraData JSON", func(t *testing.T) {
		// Clean playbackSessions
		_, _ = db.Exec("DELETE FROM playbackSessions")

		malformedCases := []struct {
			id   string
			json string
		}{
			{"sess-malformed-1", `{"playMethod": "HLS"`}, // missing closing brace
			{"sess-malformed-2", `invalid-json`},
			{"sess-malformed-3", `[]`},                   // array instead of object
			{"sess-malformed-4", `{"playMethod": 1234}`}, // incorrect type for playMethod (number instead of string)
		}

		for _, tc := range malformedCases {
			_, err = db.Exec(`INSERT INTO playbackSessions (id, userId, mediaItemId, mediaItemType, startTime, libraryId, extraData, createdAt, updatedAt) 
				VALUES (?, 'user-root', 'book-1', 'book', 10.0, 'lib-1', 
				?, 
				'2026-06-19 11:00:00', '2026-06-19 12:00:00')`, tc.id, tc.json)
			if err != nil {
				t.Fatalf("Failed to seed playbackSession for %s: %v", tc.id, err)
			}
		}

		req := httptest.NewRequest("GET", "/api/playback-sessions", nil)
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, rootSession))
		rr := httptest.NewRecorder()
		handleGetPlaybackSessions(db).ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("Expected 200, got %d. Body: %s", rr.Code, rr.Body.String())
		}

		var resp struct {
			Sessions []struct {
				ID           string  `json:"id"`
				PlayMethod   string  `json:"playMethod"`
				DeviceInfo   string  `json:"deviceInfo"`
				TimeListened float64 `json:"timeListened"`
				LastTime     float64 `json:"lastTime"`
			} `json:"sessions"`
		}

		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to unmarshal response: %v", err)
		}

		if len(resp.Sessions) != 4 {
			t.Fatalf("Expected 4 sessions, got %d", len(resp.Sessions))
		}

		for _, s := range resp.Sessions {
			if s.PlayMethod != "HLS" {
				t.Errorf("Session %s: expected playMethod 'HLS', got %q", s.ID, s.PlayMethod)
			}
			if s.DeviceInfo != "Web Client" {
				t.Errorf("Session %s: expected deviceInfo 'Web Client', got %q", s.ID, s.DeviceInfo)
			}
			if s.TimeListened != 0.0 {
				t.Errorf("Session %s: expected timeListened 0.0, got %f", s.ID, s.TimeListened)
			}
			if s.LastTime != 0.0 {
				t.Errorf("Session %s: expected lastTime 0.0, got %f", s.ID, s.LastTime)
			}
		}
	})

	t.Run("Empty or missing createdAt / updatedAt", func(t *testing.T) {
		// Clean playbackSessions
		_, _ = db.Exec("DELETE FROM playbackSessions")

		// 1. Both NULL
		_, err = db.Exec(`INSERT INTO playbackSessions (id, userId, mediaItemId, mediaItemType, startTime, libraryId, extraData, createdAt, updatedAt) 
			VALUES ('sess-both-null', 'user-root', 'book-1', 'book', 10.0, 'lib-1', '{}', NULL, NULL)`)
		if err != nil {
			t.Fatalf("Failed to seed: %v", err)
		}

		// 2. Both empty string
		_, err = db.Exec(`INSERT INTO playbackSessions (id, userId, mediaItemId, mediaItemType, startTime, libraryId, extraData, createdAt, updatedAt) 
			VALUES ('sess-both-empty', 'user-root', 'book-1', 'book', 10.0, 'lib-1', '{}', '', '')`)
		if err != nil {
			t.Fatalf("Failed to seed: %v", err)
		}

		// 3. UpdatedAt NULL, CreatedAt populated
		_, err = db.Exec(`INSERT INTO playbackSessions (id, userId, mediaItemId, mediaItemType, startTime, libraryId, extraData, createdAt, updatedAt) 
			VALUES ('sess-upd-null', 'user-root', 'book-1', 'book', 10.0, 'lib-1', '{}', '2026-06-19 12:30:00', NULL)`)
		if err != nil {
			t.Fatalf("Failed to seed: %v", err)
		}

		req := httptest.NewRequest("GET", "/api/playback-sessions", nil)
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, rootSession))
		rr := httptest.NewRecorder()
		handleGetPlaybackSessions(db).ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("Expected 200, got %d", rr.Code)
		}

		var resp struct {
			Sessions []struct {
				ID        string `json:"id"`
				UpdatedAt string `json:"updatedAt"`
			} `json:"sessions"`
		}

		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to unmarshal response: %v", err)
		}

		if len(resp.Sessions) != 3 {
			t.Fatalf("Expected 3 sessions, got %d", len(resp.Sessions))
		}

		for _, s := range resp.Sessions {
			switch s.ID {
			case "sess-both-null":
				if s.UpdatedAt != "" {
					t.Errorf("Expected empty string for both-null UpdatedAt, got %q", s.UpdatedAt)
				}
			case "sess-both-empty":
				if s.UpdatedAt != "" {
					t.Errorf("Expected empty string for both-empty UpdatedAt, got %q", s.UpdatedAt)
				}
			case "sess-upd-null":
				if s.UpdatedAt != "2026-06-19 12:30:00" {
					t.Errorf("Expected fallback to CreatedAt '2026-06-19 12:30:00', got %q", s.UpdatedAt)
				}
			}
		}
	})

	t.Run("Query parameters filtering (invalid/escaped/non-existent userIds)", func(t *testing.T) {
		// Seed a session for user-root
		_, _ = db.Exec("DELETE FROM playbackSessions")
		_, err = db.Exec(`INSERT INTO playbackSessions (id, userId, mediaItemId, mediaItemType, startTime, libraryId, extraData, createdAt, updatedAt) 
			VALUES ('sess-user-root', 'user-root', 'book-1', 'book', 10.0, 'lib-1', '{}', '2026-06-19 12:00:00', '2026-06-19 12:00:00')`)
		if err != nil {
			t.Fatalf("Failed to seed: %v", err)
		}

		testCases := []struct {
			name           string
			userIdFilter   string
			expectedLength int
		}{
			{"Valid root user", "user-root", 1},
			{"Non-existent user", "non-existent-user-id", 0},
			{"SQL Injection attempt", "user-root' OR 1=1 --", 0},
			{"SQL Injection union attempt", "user-root' UNION SELECT id, username, type, isActive, permissions FROM users --", 0},
			{"Invalid/special characters", "user-root\x00\\'%_&", 0},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				req := httptest.NewRequest("GET", "/api/playback-sessions?userId="+url.QueryEscape(tc.userIdFilter), nil)
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

				if len(resp.Sessions) != tc.expectedLength {
					t.Errorf("Filter %q: expected %d sessions, got %d", tc.userIdFilter, tc.expectedLength, len(resp.Sessions))
				}
			})
		}
	})
}
