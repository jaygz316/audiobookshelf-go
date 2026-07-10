package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"audiobookshelf/internal/core"
)

func TestNotificationsHandlers(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	adminSession := &core.UserSession{
		ID:       "admin1",
		Username: "adminuser",
		Type:     "admin",
		IsActive: true,
	}

	userSession := &core.UserSession{
		ID:       "user1",
		Username: "regularuser",
		Type:     "user",
		IsActive: true,
	}

	t.Run("Unauthorized - GET", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/notifications", nil)
		rr := httptest.NewRecorder()
		handler := handleGetNotifications(db)
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Errorf("Expected status 401, got %d", rr.Code)
		}
	})

	t.Run("Unauthorized - POST", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/notifications", bytes.NewReader([]byte(`{}`)))
		rr := httptest.NewRecorder()
		handler := handleUpdateNotifications(db)
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Errorf("Expected status 401, got %d", rr.Code)
		}
	})

	t.Run("Forbidden - GET", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/notifications", nil)
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, userSession))
		rr := httptest.NewRecorder()
		handler := handleGetNotifications(db)
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusForbidden {
			t.Errorf("Expected status 403, got %d", rr.Code)
		}
	})

	t.Run("Forbidden - POST", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/notifications", bytes.NewReader([]byte(`{}`)))
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, userSession))
		rr := httptest.NewRecorder()
		handler := handleUpdateNotifications(db)
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusForbidden {
			t.Errorf("Expected status 403, got %d", rr.Code)
		}
	})

	t.Run("GET Returns Defaults on empty DB", func(t *testing.T) {
		// Clean up settings database first
		_, err := db.Exec("DELETE FROM settings WHERE key = 'notification-settings'")
		if err != nil {
			t.Fatalf("Failed to clean settings table: %v", err)
		}

		req := httptest.NewRequest("GET", "/api/notifications", nil)
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, adminSession))
		rr := httptest.NewRecorder()
		handler := handleGetNotifications(db)
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", rr.Code)
		}

		var resp map[string]interface{}
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to unmarshal response: %v. Body: %s", err, rr.Body.String())
		}

		if resp["data"] != nil {
			t.Errorf("Expected data to be null, got %v", resp["data"])
		}

		settings, ok := resp["settings"].(map[string]interface{})
		if !ok {
			t.Fatalf("Response does not contain settings map")
		}

		if settings["appriseApiUrl"] != nil {
			t.Errorf("Expected appriseApiUrl to be null, got %v", settings["appriseApiUrl"])
		}

		if settings["maxNotificationQueue"] != float64(25) {
			t.Errorf("Expected maxNotificationQueue to be 25, got %v", settings["maxNotificationQueue"])
		}

		if settings["maxFailedAttempts"] != float64(5) {
			t.Errorf("Expected maxFailedAttempts to be 5, got %v", settings["maxFailedAttempts"])
		}

		notifications, ok := settings["notifications"].([]interface{})
		if !ok {
			t.Fatalf("Expected notifications to be slice, got %T", settings["notifications"])
		}
		if len(notifications) != 0 {
			t.Errorf("Expected notifications to be empty, got length %d", len(notifications))
		}
	})

	t.Run("POST Updates Settings", func(t *testing.T) {
		apiUrl := "https://apprise.example.com"
		reqPayload := map[string]interface{}{
			"appriseApiUrl":        apiUrl,
			"maxNotificationQueue": 50,
			"maxFailedAttempts":    10,
			"notifications": []map[string]interface{}{
				{
					"id":            "notif1",
					"eventName":     "testEvent",
					"urls":          []string{"https://callback.example.com/notif"},
					"titleTemplate": "Test Title",
					"bodyTemplate":  "Test Body",
					"enabled":       true,
				},
			},
		}

		body, _ := json.Marshal(reqPayload)
		req := httptest.NewRequest("POST", "/api/notifications", bytes.NewReader(body))
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, adminSession))
		rr := httptest.NewRecorder()
		handler := handleUpdateNotifications(db)
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d. Body: %s", rr.Code, rr.Body.String())
		}

		var resp map[string]interface{}
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to unmarshal response: %v", err)
		}

		settings, ok := resp["settings"].(map[string]interface{})
		if !ok {
			t.Fatalf("Response does not contain settings map")
		}

		if settings["appriseApiUrl"] != apiUrl {
			t.Errorf("Expected appriseApiUrl to be %q, got %v", apiUrl, settings["appriseApiUrl"])
		}

		if settings["maxNotificationQueue"] != float64(50) {
			t.Errorf("Expected maxNotificationQueue to be 50, got %v", settings["maxNotificationQueue"])
		}

		if settings["maxFailedAttempts"] != float64(10) {
			t.Errorf("Expected maxFailedAttempts to be 10, got %v", settings["maxFailedAttempts"])
		}

		notifications, ok := settings["notifications"].([]interface{})
		if !ok || len(notifications) != 1 {
			t.Fatalf("Expected 1 notification, got %v", settings["notifications"])
		}

		notif := notifications[0].(map[string]interface{})
		if notif["id"] != "notif1" {
			t.Errorf("Expected notification id 'notif1', got %v", notif["id"])
		}
	})

	t.Run("GET Returns Updated Settings", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/notifications", nil)
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, adminSession))
		rr := httptest.NewRecorder()
		handler := handleGetNotifications(db)
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", rr.Code)
		}

		var resp map[string]interface{}
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to unmarshal response: %v", err)
		}

		settings, ok := resp["settings"].(map[string]interface{})
		if !ok {
			t.Fatalf("Response does not contain settings map")
		}

		if settings["appriseApiUrl"] != "https://apprise.example.com" {
			t.Errorf("Expected appriseApiUrl to be https://apprise.example.com, got %v", settings["appriseApiUrl"])
		}

		if settings["maxNotificationQueue"] != float64(50) {
			t.Errorf("Expected maxNotificationQueue to be 50, got %v", settings["maxNotificationQueue"])
		}

		if settings["maxFailedAttempts"] != float64(10) {
			t.Errorf("Expected maxFailedAttempts to be 10, got %v", settings["maxFailedAttempts"])
		}

		notifications, ok := settings["notifications"].([]interface{})
		if !ok || len(notifications) != 1 {
			t.Fatalf("Expected 1 notification, got %v", settings["notifications"])
		}
	})

	t.Run("PATCH Merges Settings", func(t *testing.T) {
		// Only update maxFailedAttempts
		reqPayload := map[string]interface{}{
			"maxFailedAttempts": 15,
		}

		body, _ := json.Marshal(reqPayload)
		req := httptest.NewRequest("PATCH", "/api/notifications", bytes.NewReader(body))
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, adminSession))
		rr := httptest.NewRecorder()
		handler := handleUpdateNotifications(db)
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d. Body: %s", rr.Code, rr.Body.String())
		}

		var resp map[string]interface{}
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to unmarshal response: %v", err)
		}

		settings, ok := resp["settings"].(map[string]interface{})
		if !ok {
			t.Fatalf("Response does not contain settings map")
		}

		// Check merged settings: maxFailedAttempts is updated, other settings are kept
		if settings["appriseApiUrl"] != "https://apprise.example.com" {
			t.Errorf("Expected appriseApiUrl to remain https://apprise.example.com, got %v", settings["appriseApiUrl"])
		}

		if settings["maxNotificationQueue"] != float64(50) {
			t.Errorf("Expected maxNotificationQueue to remain 50, got %v", settings["maxNotificationQueue"])
		}

		if settings["maxFailedAttempts"] != float64(15) {
			t.Errorf("Expected maxFailedAttempts to be 15, got %v", settings["maxFailedAttempts"])
		}
	})

	t.Run("Invalid Payload - Invalid JSON", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/notifications", bytes.NewReader([]byte(`{invalid json`)))
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, adminSession))
		rr := httptest.NewRecorder()
		handler := handleUpdateNotifications(db)
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("Expected status 400, got %d", rr.Code)
		}
	})

	t.Run("Invalid Payload - Invalid maxNotificationQueue <= 0", func(t *testing.T) {
		reqPayload := map[string]interface{}{
			"maxNotificationQueue": 0,
		}

		body, _ := json.Marshal(reqPayload)
		req := httptest.NewRequest("POST", "/api/notifications", bytes.NewReader(body))
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, adminSession))
		rr := httptest.NewRecorder()
		handler := handleUpdateNotifications(db)
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("Expected status 400, got %d", rr.Code)
		}
	})

	t.Run("Invalid Payload - Invalid maxFailedAttempts <= 0", func(t *testing.T) {
		reqPayload := map[string]interface{}{
			"maxFailedAttempts": -1,
		}

		body, _ := json.Marshal(reqPayload)
		req := httptest.NewRequest("POST", "/api/notifications", bytes.NewReader(body))
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, adminSession))
		rr := httptest.NewRecorder()
		handler := handleUpdateNotifications(db)
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("Expected status 400, got %d", rr.Code)
		}
	})

	t.Run("Invalid Payload - Notifications slice null", func(t *testing.T) {
		reqPayload := map[string]interface{}{
			"notifications": nil,
		}

		body, _ := json.Marshal(reqPayload)
		req := httptest.NewRequest("POST", "/api/notifications", bytes.NewReader(body))
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, adminSession))
		rr := httptest.NewRecorder()
		handler := handleUpdateNotifications(db)
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("Expected status 400, got %d", rr.Code)
		}
	})

	t.Run("GET /api/notifications/test triggers default test notification", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/notifications/test", nil)
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, adminSession))
		rr := httptest.NewRecorder()
		handler := handleSendDefaultTestNotification(db)
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d. Body: %s", rr.Code, rr.Body.String())
		}
		var resp map[string]interface{}
		_ = json.Unmarshal(rr.Body.Bytes(), &resp)
		if resp["success"] != true {
			t.Errorf("Expected success to be true, got %v", resp["success"])
		}
	})

	t.Run("GET /api/notifications/{id}/test triggers targeted notification", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/notifications/notif1/test", nil)
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, adminSession))
		rr := httptest.NewRecorder()
		handler := handleSendTestNotification(db)
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d. Body: %s", rr.Code, rr.Body.String())
		}
	})

	t.Run("PATCH /api/notifications/{id} updates single target", func(t *testing.T) {
		payload := map[string]interface{}{
			"enabled": false,
		}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest("PATCH", "/api/notifications/notif1", bytes.NewReader(body))
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, adminSession))
		rr := httptest.NewRecorder()
		handler := handleUpdateNotification(db)
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d. Body: %s", rr.Code, rr.Body.String())
		}

		var resp map[string]interface{}
		_ = json.Unmarshal(rr.Body.Bytes(), &resp)
		settings := resp["settings"].(map[string]interface{})
		notifications := settings["notifications"].([]interface{})
		notif := notifications[0].(map[string]interface{})
		if notif["enabled"] != false {
			t.Errorf("Expected enabled to be false, got %v", notif["enabled"])
		}
	})

	t.Run("DELETE /api/notifications/{id} removes target", func(t *testing.T) {
		req := httptest.NewRequest("DELETE", "/api/notifications/notif1", nil)
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, adminSession))
		rr := httptest.NewRecorder()
		handler := handleDeleteNotification(db)
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d. Body: %s", rr.Code, rr.Body.String())
		}

		var resp map[string]interface{}
		_ = json.Unmarshal(rr.Body.Bytes(), &resp)
		settings := resp["settings"].(map[string]interface{})
		notifications := settings["notifications"].([]interface{})
		if len(notifications) != 0 {
			t.Errorf("Expected 0 notifications left, got %d", len(notifications))
		}
	})
}
