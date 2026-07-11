package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"audiobookshelf/internal/core"
)

func setupSessionsTestDB(t *testing.T) *sql.DB {
	db := setupUsersTestDB(t)

	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS sessions (
			id TEXT PRIMARY KEY,
			userId TEXT,
			ipAddress TEXT,
			userAgent TEXT,
			refreshToken TEXT,
			expiresAt TEXT,
			lastRefreshToken TEXT,
			lastRefreshTokenExpiresAt TEXT,
			createdAt TEXT,
			updatedAt TEXT
		)
	`)
	if err != nil {
		t.Fatalf("failed to create sessions table: %v", err)
	}

	return db
}

func TestActiveLoginSessions(t *testing.T) {
	db := setupSessionsTestDB(t)
	defer db.Close()

	// 1. Insert test users
	_, err := db.Exec(`
		INSERT INTO users (id, username, type, pash, token, isActive, permissions, extraData, bookmarks, createdAt, updatedAt)
		VALUES 
		('user-1', 'user1', 'user', 'somehash', 'sometoken1', 1, '{}', '{}', '[]', '2026-07-11T00:00:00Z', '2026-07-11T00:00:00Z'),
		('user-2', 'user2', 'user', 'somehash', 'sometoken2', 1, '{}', '{}', '[]', '2026-07-11T00:00:00Z', '2026-07-11T00:00:00Z'),
		('admin-1', 'admin1', 'admin', 'somehash', 'sometoken3', 1, '{}', '{}', '[]', '2026-07-11T00:00:00Z', '2026-07-11T00:00:00Z')
	`)
	if err != nil {
		t.Fatalf("failed to insert test users: %v", err)
	}

	// 2. Insert test sessions for user-1
	_, err = db.Exec(`
		INSERT INTO sessions (id, userId, ipAddress, userAgent, refreshToken, expiresAt, lastRefreshToken, lastRefreshTokenExpiresAt, createdAt, updatedAt)
		VALUES 
		('sess-1', 'user-1', '127.0.0.1', 'Chrome', 'rt-1', '2026-08-11T00:00:00Z', 'lrt-1', '2026-07-11T12:00:00Z', '2026-07-11T00:00:00Z', '2026-07-11T00:00:00Z'),
		('sess-2', 'user-1', '10.0.0.2', 'Firefox', 'rt-2', '2026-08-11T00:00:00Z', NULL, NULL, '2026-07-11T00:00:00Z', '2026-07-11T00:00:00Z')
	`)
	if err != nil {
		t.Fatalf("failed to insert test sessions: %v", err)
	}

	user1SessionContext := &core.UserSession{
		ID:                 "user-1",
		Username:           "user1",
		Type:               "user",
		IsActive:           true,
		AccessAllLibraries: true,
	}

	user2SessionContext := &core.UserSession{
		ID:                 "user-2",
		Username:           "user2",
		Type:               "user",
		IsActive:           true,
		AccessAllLibraries: true,
	}

	adminSessionContext := &core.UserSession{
		ID:                 "admin-1",
		Username:           "admin1",
		Type:               "admin",
		IsActive:           true,
		AccessAllLibraries: true,
	}

	t.Run("GetSessions_Self", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/users/user-1/sessions", nil)
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, user1SessionContext))
		req.AddCookie(&http.Cookie{Name: "refresh_token", Value: "rt-1"})
		rr := httptest.NewRecorder()

		handleUserCRUD(db).ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("Expected 200 OK, got %d. Body: %s", rr.Code, rr.Body.String())
		}

		var sessions []UserSessionResponse
		if err := json.Unmarshal(rr.Body.Bytes(), &sessions); err != nil {
			t.Fatalf("failed to unmarshal response: %v", err)
		}

		if len(sessions) != 2 {
			t.Errorf("Expected 2 sessions, got %d", len(sessions))
		}

		// sess-1 should have isCurrent == true because refresh_token cookie matches rt-1
		foundCurrent := false
		for _, s := range sessions {
			if s.ID == "sess-1" {
				if !s.IsCurrent {
					t.Errorf("Expected sess-1 to be marked as current")
				}
				foundCurrent = true
			} else if s.ID == "sess-2" {
				if s.IsCurrent {
					t.Errorf("Expected sess-2 to not be marked as current")
				}
			}
		}
		if !foundCurrent {
			t.Errorf("Did not find sess-1 in returned sessions")
		}
	})

	t.Run("GetSessions_Forbidden_OtherUser", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/users/user-1/sessions", nil)
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, user2SessionContext))
		rr := httptest.NewRecorder()

		handleUserCRUD(db).ServeHTTP(rr, req)

		if rr.Code != http.StatusForbidden {
			t.Errorf("Expected 403 Forbidden, got %d", rr.Code)
		}
	})

	t.Run("GetSessions_Admin_OtherUser", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/users/user-1/sessions", nil)
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, adminSessionContext))
		rr := httptest.NewRecorder()

		handleUserCRUD(db).ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("Expected 200 OK, got %d", rr.Code)
		}

		var sessions []UserSessionResponse
		if err := json.Unmarshal(rr.Body.Bytes(), &sessions); err != nil {
			t.Fatalf("failed to unmarshal response: %v", err)
		}

		if len(sessions) != 2 {
			t.Errorf("Expected 2 sessions, got %d", len(sessions))
		}
	})

	t.Run("DeleteSession_Forbidden_OtherUser", func(t *testing.T) {
		req := httptest.NewRequest("DELETE", "/api/users/user-1/sessions/sess-2", nil)
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, user2SessionContext))
		rr := httptest.NewRecorder()

		handleUserCRUD(db).ServeHTTP(rr, req)

		if rr.Code != http.StatusForbidden {
			t.Errorf("Expected 403 Forbidden, got %d", rr.Code)
		}

		// Verify session still exists
		var count int
		db.QueryRow("SELECT COUNT(*) FROM sessions WHERE id = 'sess-2'").Scan(&count)
		if count != 1 {
			t.Errorf("Expected session to not be deleted, but it was")
		}
	})

	t.Run("DeleteSession_Self_NotCurrent", func(t *testing.T) {
		req := httptest.NewRequest("DELETE", "/api/users/user-1/sessions/sess-2", nil)
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, user1SessionContext))
		rr := httptest.NewRecorder()

		handleUserCRUD(db).ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("Expected 200 OK, got %d. Body: %s", rr.Code, rr.Body.String())
		}

		// Verify session is deleted
		var count int
		db.QueryRow("SELECT COUNT(*) FROM sessions WHERE id = 'sess-2'").Scan(&count)
		if count != 0 {
			t.Errorf("Expected session to be deleted, but it still exists")
		}

		// Verify cookie was NOT cleared (since it wasn't the current session)
		foundClearCookie := false
		for _, cookie := range rr.Result().Cookies() {
			if cookie.Name == "refresh_token" && cookie.Value == "" {
				foundClearCookie = true
			}
		}
		if foundClearCookie {
			t.Errorf("Did not expect refresh_token cookie to be cleared")
		}
	})

	t.Run("DeleteSession_Self_Current", func(t *testing.T) {
		req := httptest.NewRequest("DELETE", "/api/users/user-1/sessions/sess-1", nil)
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, user1SessionContext))
		req.AddCookie(&http.Cookie{Name: "refresh_token", Value: "rt-1"})
		rr := httptest.NewRecorder()

		handleUserCRUD(db).ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("Expected 200 OK, got %d. Body: %s", rr.Code, rr.Body.String())
		}

		// Verify session is deleted
		var count int
		db.QueryRow("SELECT COUNT(*) FROM sessions WHERE id = 'sess-1'").Scan(&count)
		if count != 0 {
			t.Errorf("Expected session to be deleted, but it still exists")
		}

		// Verify cookie WAS cleared
		foundClearCookie := false
		for _, cookie := range rr.Result().Cookies() {
			if cookie.Name == "refresh_token" && cookie.Value == "" {
				foundClearCookie = true
			}
		}
		if !foundClearCookie {
			t.Errorf("Expected refresh_token cookie to be cleared")
		}
	})
}
