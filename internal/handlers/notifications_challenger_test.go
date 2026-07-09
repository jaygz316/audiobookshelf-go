package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"audiobookshelf/internal/core"
)

// TestNotificationsConcurrentUpdates checks how the handlers respond under race conditions/concurrency stress.
func TestNotificationsConcurrentUpdates(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	db.SetMaxOpenConns(1)

	adminSession := &core.UserSession{
		ID:       "admin1",
		Username: "adminuser",
		Type:     "admin",
		IsActive: true,
	}

	const workers = 50
	var wg sync.WaitGroup
	wg.Add(workers)

	statusCodes := make([]int, workers)
	var mu sync.Mutex

	for i := 0; i < workers; i++ {
		go func(workerID int) {
			defer wg.Done()

			var req *http.Request
			var handler http.HandlerFunc

			// Mix GET, POST (full update) and PATCH (partial update) requests
			switch workerID % 3 {
			case 0:
				// PATCH to update maxNotificationQueue
				payload := map[string]interface{}{
					"maxNotificationQueue": 10 + workerID,
				}
				body, _ := json.Marshal(payload)
				req = httptest.NewRequest("PATCH", "/api/notifications", bytes.NewReader(body))
				handler = handleUpdateNotifications(db)
			case 1:
				// POST to update settings completely
				payload := map[string]interface{}{
					"appriseApiUrl":        fmt.Sprintf("https://apprise%d.example.com", workerID),
					"maxNotificationQueue": 100 + workerID,
					"maxFailedAttempts":    5 + workerID%5,
					"notifications":        []interface{}{},
				}
				body, _ := json.Marshal(payload)
				req = httptest.NewRequest("POST", "/api/notifications", bytes.NewReader(body))
				handler = handleUpdateNotifications(db)
			default:
				// GET settings
				req = httptest.NewRequest("GET", "/api/notifications", nil)
				handler = handleGetNotifications(db)
			}

			req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, adminSession))
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			mu.Lock()
			statusCodes[workerID] = rr.Code
			mu.Unlock()
		}(i)
	}

	wg.Wait()

	// Verify that no request causes a crash or panic.
	// Since SQLite is single-threaded or may return lock errors (500) under high concurrency,
	// returning 200 or 500 is considered a graceful response (no panics).
	for i, code := range statusCodes {
		if code != http.StatusOK && code != http.StatusInternalServerError {
			t.Errorf("Worker %d returned unexpected status code: %d", i, code)
		}
	}

	// Verify database integrity by retrieving final settings
	req := httptest.NewRequest("GET", "/api/notifications", nil)
	req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, adminSession))
	rr := httptest.NewRecorder()
	handleGetNotifications(db).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Failed to retrieve final settings, status: %d, body: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to unmarshal final settings response: %v", err)
	}
}

// TestNotificationsSQLInjection checks if SQLite parsing/execution errors can be triggered via injection payloads.
func TestNotificationsSQLInjection(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	adminSession := &core.UserSession{
		ID:       "admin1",
		Username: "adminuser",
		Type:     "admin",
		IsActive: true,
	}

	sqlInjectionPayloads := []string{
		"'; DROP TABLE settings; --",
		"\" OR \"1\"=\"1",
		"' UNION SELECT key, value FROM settings --",
		"'; INSERT INTO settings (key, value) VALUES ('injected', 'true'); --",
	}

	for _, payloadStr := range sqlInjectionPayloads {
		t.Run(fmt.Sprintf("Payload: %s", payloadStr), func(t *testing.T) {
			payload := map[string]interface{}{
				"appriseApiUrl":        payloadStr,
				"maxNotificationQueue": 25,
				"maxFailedAttempts":    5,
				"notifications":        []interface{}{},
			}
			body, _ := json.Marshal(payload)

			req := httptest.NewRequest("POST", "/api/notifications", bytes.NewReader(body))
			req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, adminSession))
			rr := httptest.NewRecorder()
			handleUpdateNotifications(db).ServeHTTP(rr, req)

			if rr.Code != http.StatusOK && rr.Code != http.StatusBadRequest {
				t.Errorf("Expected status 200 or 400, got %d", rr.Code)
			}

			// Verify settings table was NOT dropped
			var count int
			err := db.QueryRow("SELECT COUNT(*) FROM settings").Scan(&count)
			if err != nil {
				t.Fatalf("SQL Injection corrupted the database: %v", err)
			}

			// Verify no unauthorized rows were inserted
			var val string
			err = db.QueryRow("SELECT value FROM settings WHERE key = 'injected'").Scan(&val)
			if err == nil {
				t.Errorf("SQL Injection successfully inserted a new key!")
			}
		})
	}
}

// TestNotificationsPayloadMalformations checks how the handler behaves with malformed payloads.
func TestNotificationsPayloadMalformations(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	adminSession := &core.UserSession{
		ID:       "admin1",
		Username: "adminuser",
		Type:     "admin",
		IsActive: true,
	}

	testCases := []struct {
		name         string
		payload      []byte
		expectedCode int
	}{
		{
			name:         "Corrupted JSON",
			payload:      []byte(`{"appriseApiUrl": "http://example.com", "maxNotificationQueue": 25,`),
			expectedCode: http.StatusBadRequest,
		},
		{
			name:         "Wrong type for maxNotificationQueue (string)",
			payload:      []byte(`{"maxNotificationQueue": "twenty-five"}`),
			expectedCode: http.StatusBadRequest,
		},
		{
			name:         "Wrong type for maxFailedAttempts (boolean)",
			payload:      []byte(`{"maxFailedAttempts": true}`),
			expectedCode: http.StatusBadRequest,
		},
		{
			name:         "Wrong type for notifications (string)",
			payload:      []byte(`{"notifications": "not-an-array"}`),
			expectedCode: http.StatusBadRequest,
		},
		{
			name:         "Wrong type for notification enabled field (string)",
			payload:      []byte(`{"notifications": [{"id": "notif1", "enabled": "yes"}]}`),
			expectedCode: http.StatusBadRequest,
		},
		{
			name:         "Wrong type for urls field (number)",
			payload:      []byte(`{"notifications": [{"id": "notif1", "urls": 123}]}`),
			expectedCode: http.StatusBadRequest,
		},
		{
			name:         "Negative maxNotificationQueue",
			payload:      []byte(`{"maxNotificationQueue": -5}`),
			expectedCode: http.StatusBadRequest,
		},
		{
			name:         "Zero maxFailedAttempts",
			payload:      []byte(`{"maxFailedAttempts": 0}`),
			expectedCode: http.StatusBadRequest,
		},
		{
			name:         "Null notifications array",
			payload:      []byte(`{"notifications": null}`),
			expectedCode: http.StatusBadRequest,
		},
		{
			name:         "Empty JSON body",
			payload:      []byte(``),
			expectedCode: http.StatusBadRequest,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/api/notifications", bytes.NewReader(tc.payload))
			req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, adminSession))
			rr := httptest.NewRecorder()
			handleUpdateNotifications(db).ServeHTTP(rr, req)

			if rr.Code != tc.expectedCode {
				t.Errorf("Expected status %d, got %d. Body: %s", tc.expectedCode, rr.Code, rr.Body.String())
			}
		})
	}
}
