package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"audiobookshelf/internal/core"
)

func TestListeningStatsAndHistory(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Seed users
	_, err := db.Exec(`INSERT INTO users (id, username, type, isActive, permissions) VALUES 
		('user-root', 'rootuser', 'root', 1, '{}'),
		('user-normal', 'normaluser', 'user', 1, '{}'),
		('user-other', 'otheruser', 'user', 1, '{}')`)
	if err != nil {
		t.Fatalf("Failed to seed users: %v", err)
	}

	// Seed media
	_, err = db.Exec(`INSERT INTO books (id, title) VALUES ('book-1', 'Test Book 1')`)
	if err != nil {
		t.Fatalf("Failed to seed books: %v", err)
	}
	_, err = db.Exec(`INSERT INTO libraryItems (id, mediaId, mediaType, authorNamesFirstLast, title) VALUES ('item-1', 'book-1', 'book', 'Jane Doe', 'Test Book 1')`)
	if err != nil {
		t.Fatalf("Failed to seed libraryItems: %v", err)
	}

	todayUTC := time.Now().UTC()
	todayStr := todayUTC.Format("2006-01-02 15:04:05.000 +00:00")

	// Seed playbackSessions for user-normal
	// Session 1: 120 seconds today
	_, err = db.Exec(`INSERT INTO playbackSessions (id, userId, mediaItemId, mediaItemType, startTime, libraryId, extraData, createdAt, updatedAt) 
		VALUES ('sess-1', 'user-normal', 'book-1', 'book', 10.0, 'lib-1', 
		'{"playMethod":"HLS","deviceInfo":"Web Client","timeListened":120.0,"lastTime":130.0}', 
		?, ?)`, todayStr, todayStr)
	if err != nil {
		t.Fatalf("Failed to seed playbackSession 1: %v", err)
	}

	// Session 2: 60 seconds on a specific past date (e.g. 2026-06-10)
	_, err = db.Exec(`INSERT INTO playbackSessions (id, userId, mediaItemId, mediaItemType, startTime, libraryId, extraData, createdAt, updatedAt) 
		VALUES ('sess-2', 'user-normal', 'book-1', 'book', 0.0, 'lib-1', 
		'{"playMethod":"HLS","deviceInfo":"Mobile App","timeListened":60.0,"lastTime":60.0}', 
		'2026-06-10 12:00:00.000 +00:00', '2026-06-10 12:01:00.000 +00:00')`)
	if err != nil {
		t.Fatalf("Failed to seed playbackSession 2: %v", err)
	}

	// Seed mediaProgresses for user-normal
	_, err = db.Exec(`INSERT INTO mediaProgresses (id, userId, mediaItemId, isFinished, currentTime, updatedAt) 
		VALUES ('prog-1', 'user-normal', 'book-1', 1, 120.0, ?)`, todayStr)
	if err != nil {
		t.Fatalf("Failed to seed mediaProgresses: %v", err)
	}

	normalSession := &core.UserSession{ID: "user-normal", Username: "normaluser", Type: "user", IsActive: true}
	otherSession := &core.UserSession{ID: "user-other", Username: "otheruser", Type: "user", IsActive: true}
	rootSession := &core.UserSession{ID: "user-root", Username: "rootuser", Type: "root", IsActive: true}

	t.Run("GET /api/me/listening-stats success", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/me/listening-stats", nil)
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, normalSession))
		rr := httptest.NewRecorder()
		handleGetMeListeningStats(db).ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected 200, got %d", rr.Code)
		}

		var stats ListeningStatsResponse
		if err := json.NewDecoder(rr.Body).Decode(&stats); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if stats.TotalTime != 180.0 {
			t.Errorf("Expected TotalTime 180.0, got %f", stats.TotalTime)
		}
		if stats.Today != 120.0 {
			t.Errorf("Expected Today 120.0, got %f", stats.Today)
		}
		if stats.Days["2026-06-10"] != 60.0 {
			t.Errorf("Expected past day 60.0, got %f", stats.Days["2026-06-10"])
		}
		if len(stats.RecentSessions) != 2 {
			t.Errorf("Expected 2 recent sessions, got %d", len(stats.RecentSessions))
		}
		if stats.ItemsFinished != 1 {
			t.Errorf("Expected ItemsFinished 1, got %d", stats.ItemsFinished)
		}
		if stats.DaysListened != 2 {
			t.Errorf("Expected DaysListened 2, got %d", stats.DaysListened)
		}
	})

	t.Run("GET /api/me/listening-sessions success", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/me/listening-sessions?page=0&itemsPerPage=1", nil)
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, normalSession))
		rr := httptest.NewRecorder()
		handleGetMeListeningSessions(db).ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected 200, got %d", rr.Code)
		}

		var resp map[string]interface{}
		if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		sessionsList, ok := resp["sessions"].([]interface{})
		if !ok || len(sessionsList) != 1 {
			t.Errorf("Expected 1 session in paginated list, got %v", resp["sessions"])
		}
		if int(resp["total"].(float64)) != 2 {
			t.Errorf("Expected total 2, got %v", resp["total"])
		}
	})

	t.Run("GET /api/users/{id}/listening-stats permissions", func(t *testing.T) {
		// Other user tries to access normaluser's stats -> Forbidden
		req := httptest.NewRequest("GET", "/api/users/user-normal/listening-stats", nil)
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, otherSession))
		rr := httptest.NewRecorder()

		// Serve using user CRUD router (handleUserCRUD)
		handleUserCRUD(db).ServeHTTP(rr, req)
		if rr.Code != http.StatusForbidden {
			t.Errorf("Expected 403 Forbidden, got %d", rr.Code)
		}

		// Root user tries to access normaluser's stats -> Success
		req2 := httptest.NewRequest("GET", "/api/users/user-normal/listening-stats", nil)
		req2 = req2.WithContext(context.WithValue(req2.Context(), core.UserContextKey, rootSession))
		rr2 := httptest.NewRecorder()
		handleUserCRUD(db).ServeHTTP(rr2, req2)
		if rr2.Code != http.StatusOK {
			t.Errorf("Expected 200 OK, got %d", rr2.Code)
		}
	})

	t.Run("GET /api/server-listening-stats permissions and result", func(t *testing.T) {
		// Normal user tries to access server stats -> Forbidden
		req := httptest.NewRequest("GET", "/api/server-listening-stats", nil)
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, normalSession))
		rr := httptest.NewRecorder()
		handleGetServerListeningStats(db).ServeHTTP(rr, req)
		if rr.Code != http.StatusForbidden {
			t.Errorf("Expected 403 Forbidden, got %d", rr.Code)
		}

		// Admin/Root user tries to access server stats -> Success
		req2 := httptest.NewRequest("GET", "/api/server-listening-stats", nil)
		req2 = req2.WithContext(context.WithValue(req2.Context(), core.UserContextKey, rootSession))
		rr2 := httptest.NewRecorder()
		handleGetServerListeningStats(db).ServeHTTP(rr2, req2)
		if rr2.Code != http.StatusOK {
			t.Errorf("Expected 200 OK, got %d", rr2.Code)
		}

		var stats ServerListeningStatsResponse
		if err := json.NewDecoder(rr2.Body).Decode(&stats); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		// TotalTime should be 180 (user-normal has 180, user-other has 0, root has 0)
		if stats.TotalTime != 180.0 {
			t.Errorf("Expected TotalTime 180.0, got %f", stats.TotalTime)
		}

		// TopUsers should contain normaluser with 180.0
		if stats.TopUsers["normaluser"] != 180.0 {
			t.Errorf("Expected top user normaluser 180.0, got %v", stats.TopUsers)
		}

		if stats.ItemsFinished != 1 {
			t.Errorf("Expected ItemsFinished 1, got %d", stats.ItemsFinished)
		}
		if stats.DaysListened != 2 {
			t.Errorf("Expected DaysListened 2, got %d", stats.DaysListened)
		}
	})
}
