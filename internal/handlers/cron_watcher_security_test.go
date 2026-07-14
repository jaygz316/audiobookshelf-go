package handlers

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"audiobookshelf/internal/core"
)

func TestCronAndWatcherSecurity(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	adminSession := &core.UserSession{
		ID:       "admin-user",
		Username: "admin",
		Type:     "admin",
		IsActive: true,
	}

	userSession := &core.UserSession{
		ID:       "normal-user",
		Username: "user",
		Type:     "user",
		IsActive: true,
	}

	t.Run("ValidateCron_Admin_Success", func(t *testing.T) {
		payload := `{"expression": "0 0 * * *"}`
		req := httptest.NewRequest("POST", "/api/validate-cron", bytes.NewBufferString(payload))
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, adminSession))
		rr := httptest.NewRecorder()

		http.HandlerFunc(handleValidateCron).ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected status 200 OK for admin, got %d", rr.Code)
		}
	})

	t.Run("ValidateCron_User_Forbidden", func(t *testing.T) {
		payload := `{"expression": "0 0 * * *"}`
		req := httptest.NewRequest("POST", "/api/validate-cron", bytes.NewBufferString(payload))
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, userSession))
		rr := httptest.NewRecorder()

		http.HandlerFunc(handleValidateCron).ServeHTTP(rr, req)

		if rr.Code != http.StatusForbidden {
			t.Errorf("Expected status 403 Forbidden for normal user, got %d", rr.Code)
		}
	})

	t.Run("WatcherUpdate_Admin_Success", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/watcher/update", nil)
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, adminSession))
		rr := httptest.NewRecorder()

		http.HandlerFunc(handleWatcherUpdate).ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected status 200 OK for admin, got %d", rr.Code)
		}
	})

	t.Run("WatcherUpdate_User_Forbidden", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/watcher/update", nil)
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, userSession))
		rr := httptest.NewRecorder()

		http.HandlerFunc(handleWatcherUpdate).ServeHTTP(rr, req)

		if rr.Code != http.StatusForbidden {
			t.Errorf("Expected status 403 Forbidden for normal user, got %d", rr.Code)
		}
	})
}
