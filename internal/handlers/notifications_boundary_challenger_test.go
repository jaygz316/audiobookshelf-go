package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"audiobookshelf/internal/core"
)

func TestNotificationsBoundaryChecks(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	adminSession := &core.UserSession{
		ID:       "admin1",
		Username: "adminuser",
		Type:     "admin",
		IsActive: true,
	}

	t.Run("maxNotificationQueue extremely large (causing json unmarshal error or float overflow)", func(t *testing.T) {
		// 1. Value larger than 64-bit int max
		largeQueueStr := `{"maxNotificationQueue": 999999999999999999999999999999999999999999}`
		req := httptest.NewRequest("POST", "/api/notifications", bytes.NewReader([]byte(largeQueueStr)))
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, adminSession))
		rr := httptest.NewRecorder()
		handleUpdateNotifications(db).ServeHTTP(rr, req)

		// Expect StatusBadRequest because unmarshal into validatedSettings.MaxNotificationQueue (int) will fail
		if rr.Code != http.StatusBadRequest {
			t.Errorf("Expected status 400 for overflow int, got %d. Body: %s", rr.Code, rr.Body.String())
		}
	})

	t.Run("maxNotificationQueue max 32-bit and 64-bit int (valid values but extremely large boundaries)", func(t *testing.T) {
		// Max int32: 2147483647
		payload := map[string]interface{}{
			"maxNotificationQueue": 2147483647,
		}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest("POST", "/api/notifications", bytes.NewReader(body))
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, adminSession))
		rr := httptest.NewRecorder()
		handleUpdateNotifications(db).ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected status 200 for max int32, got %d. Body: %s", rr.Code, rr.Body.String())
		}

		// Verify it was saved correctly
		var valStr string
		err := db.QueryRow("SELECT value FROM settings WHERE key = 'notification-settings'").Scan(&valStr)
		if err != nil {
			t.Fatalf("Failed to scan settings: %v", err)
		}
		if !strings.Contains(valStr, "2147483647") {
			t.Errorf("Expected saved settings to contain 2147483647, got %s", valStr)
		}
	})

	t.Run("notifications list with hundreds of entries", func(t *testing.T) {
		// Generate 500 notifications
		notifications := make([]map[string]interface{}, 500)
		for i := 0; i < 500; i++ {
			notifications[i] = map[string]interface{}{
				"id":            fmt.Sprintf("notif_%d", i),
				"libraryId":     nil,
				"eventName":     fmt.Sprintf("event_%d", i),
				"urls":          []string{fmt.Sprintf("https://callback.example.com/%d", i)},
				"titleTemplate": "Title",
				"bodyTemplate":  "Body",
				"enabled":       true,
			}
		}

		payload := map[string]interface{}{
			"notifications": notifications,
		}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest("POST", "/api/notifications", bytes.NewReader(body))
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, adminSession))
		rr := httptest.NewRecorder()
		handleUpdateNotifications(db).ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("Expected status 200 for 500 notifications, got %d. Body: %s", rr.Code, rr.Body.String())
		}

		// Retrieve setting and verify size
		reqGet := httptest.NewRequest("GET", "/api/notifications", nil)
		reqGet = reqGet.WithContext(context.WithValue(reqGet.Context(), core.UserContextKey, adminSession))
		rrGet := httptest.NewRecorder()
		handleGetNotifications(db).ServeHTTP(rrGet, reqGet)

		var resp map[string]interface{}
		if err := json.Unmarshal(rrGet.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to unmarshal GET response: %v", err)
		}
		settings := resp["settings"].(map[string]interface{})
		retrievedNotifs := settings["notifications"].([]interface{})
		if len(retrievedNotifs) != 500 {
			t.Errorf("Expected 500 notifications, retrieved %d", len(retrievedNotifs))
		}
	})

	t.Run("appriseApiUrl set to null", func(t *testing.T) {
		payload := map[string]interface{}{
			"appriseApiUrl": nil,
		}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest("POST", "/api/notifications", bytes.NewReader(body))
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, adminSession))
		rr := httptest.NewRecorder()
		handleUpdateNotifications(db).ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected status 200 for null appriseApiUrl, got %d. Body: %s", rr.Code, rr.Body.String())
		}

		// Verify DB contains null for appriseApiUrl
		var valStr string
		err := db.QueryRow("SELECT value FROM settings WHERE key = 'notification-settings'").Scan(&valStr)
		if err != nil {
			t.Fatalf("Failed to query settings: %v", err)
		}
		if !strings.Contains(valStr, `"appriseApiUrl":null`) {
			t.Errorf("Expected saved settings to contain null appriseApiUrl, got %s", valStr)
		}
	})

	t.Run("appriseApiUrl set to empty string", func(t *testing.T) {
		payload := map[string]interface{}{
			"appriseApiUrl": "",
		}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest("POST", "/api/notifications", bytes.NewReader(body))
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, adminSession))
		rr := httptest.NewRecorder()
		handleUpdateNotifications(db).ServeHTTP(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("Expected status 400 for empty string, got %d. Body: %s", rr.Code, rr.Body.String())
		}
	})

	t.Run("appriseApiUrl set to invalid formats", func(t *testing.T) {
		invalidUrls := []string{
			"not-a-valid-url",
			"http://",
			"ftp://invalid-scheme.com",
			"http://[::1]:invalidport",
		}

		for _, invalidUrl := range invalidUrls {
			payload := map[string]interface{}{
				"appriseApiUrl": invalidUrl,
			}
			body, _ := json.Marshal(payload)
			req := httptest.NewRequest("POST", "/api/notifications", bytes.NewReader(body))
			req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, adminSession))
			rr := httptest.NewRecorder()
			handleUpdateNotifications(db).ServeHTTP(rr, req)

			if rr.Code != http.StatusBadRequest {
				t.Errorf("Expected status 400 for URL %q, got %d. Body: %s", invalidUrl, rr.Code, rr.Body.String())
			}
		}
	})

	t.Run("notifications URLs invalid formats", func(t *testing.T) {
		invalidNotifUrls := [][]string{
			{"not-a-valid-url"},
			{"http://"},
			{"mailto:test@example.com"},
			{"https://"},
		}

		for _, urls := range invalidNotifUrls {
			payload := map[string]interface{}{
				"notifications": []map[string]interface{}{
					{
						"id":            "notif_invalid",
						"eventName":     "testEvent",
						"urls":          urls,
						"titleTemplate": "Title",
						"bodyTemplate":  "Body",
						"enabled":       true,
					},
				},
			}
			body, _ := json.Marshal(payload)
			req := httptest.NewRequest("POST", "/api/notifications", bytes.NewReader(body))
			req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, adminSession))
			rr := httptest.NewRecorder()
			handleUpdateNotifications(db).ServeHTTP(rr, req)

			if rr.Code != http.StatusBadRequest {
				t.Errorf("Expected status 400 for notification URL list %v, got %d. Body: %s", urls, rr.Code, rr.Body.String())
			}
		}
	})
}
