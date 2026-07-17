package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"audiobookshelf/internal/core"
)

// TestEmpiricalRefactor_EndpointRegistration verifies that the settings routes are registered
// correctly, support the expected HTTP methods, return 405 Method Not Allowed for unsupported methods,
// and enforce authentication wrapper checks.
func TestEmpiricalRefactor_EndpointRegistration(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Alter series table to add missing column for prefix recomputation in tests
	_, err := db.Exec(`ALTER TABLE series ADD COLUMN nameIgnorePrefix TEXT`)
	if err != nil {
		t.Fatalf("failed to alter series table: %v", err)
	}

	// Seed user and API key for authentication
	_, err = db.Exec(`INSERT INTO users (id, username, type, isActive, permissions) VALUES ('user-1', 'admin', 'admin', 1, '{}')`)
	if err != nil {
		t.Fatalf("failed to insert user: %v", err)
	}
	_, err = db.Exec(`INSERT INTO apiKeys (id, isActive, expiresAt, userId, name, createdAt) VALUES ('key-123', 1, '', 'user-1', 'testkey', '2026-07-16')`)
	if err != nil {
		t.Fatalf("failed to insert API key: %v", err)
	}

	mux := http.NewServeMux()
	cfg := &core.Config{
		RouterBasePath: "/subpath",
	}
	registerSettingsRoutes(mux, cfg, db)

	tests := []struct {
		name           string
		method         string
		path           string
		useAuth        bool
		body           string
		expectedStatus int
	}{
		{
			name:           "GET settings - authorized",
			method:         "GET",
			path:           "/subpath/api/settings",
			useAuth:        true,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "GET settings - unauthorized",
			method:         "GET",
			path:           "/subpath/api/settings",
			useAuth:        false,
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "POST settings - method not allowed",
			method:         "POST",
			path:           "/subpath/api/settings",
			useAuth:        true,
			expectedStatus: http.StatusMethodNotAllowed,
		},
		{
			name:           "PATCH settings - authorized",
			method:         "PATCH",
			path:           "/subpath/api/settings",
			useAuth:        true,
			body:           `{"dateFormat": "YYYY-MM-DD"}`,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "PATCH sorting-prefixes - authorized",
			method:         "PATCH",
			path:           "/subpath/api/sorting-prefixes",
			useAuth:        true,
			body:           `{"sortingPrefixes": ["the", "a"]}`,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "GET sorting-prefixes - method not allowed",
			method:         "GET",
			path:           "/subpath/api/sorting-prefixes",
			useAuth:        true,
			expectedStatus: http.StatusMethodNotAllowed,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var bodyReader *bytes.Reader
			if tc.body != "" {
				bodyReader = bytes.NewReader([]byte(tc.body))
			} else {
				bodyReader = bytes.NewReader(nil)
			}

			req := httptest.NewRequest(tc.method, tc.path, bodyReader)
			if tc.useAuth {
				req.Header.Set("Authorization", "Bearer key-123")
			}

			rr := httptest.NewRecorder()
			mux.ServeHTTP(rr, req)

			if rr.Code != tc.expectedStatus {
				t.Errorf("Expected status %d, got %d. Body: %s", tc.expectedStatus, rr.Code, rr.Body.String())
			}
		})
	}

	// Give background recomputation goroutine a chance to run/finish before closing DB
	time.Sleep(100 * time.Millisecond)
}

// TestEmpiricalRefactor_AsyncPrefixRecomputation verifies that ignore prefix recomputation
// is triggered asynchronously and correctly updates books, podcasts, and series in the database.
func TestEmpiricalRefactor_AsyncPrefixRecomputation(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Alter series table to add missing column for prefix recomputation in tests
	_, err := db.Exec(`ALTER TABLE series ADD COLUMN nameIgnorePrefix TEXT`)
	if err != nil {
		t.Fatalf("failed to alter series table: %v", err)
	}

	// Seed user and API key
	_, err = db.Exec(`INSERT INTO users (id, username, type, isActive, permissions) VALUES ('user-1', 'admin', 'admin', 1, '{}')`)
	if err != nil {
		t.Fatalf("failed to insert user: %v", err)
	}
	_, err = db.Exec(`INSERT INTO apiKeys (id, isActive, expiresAt, userId, name, createdAt) VALUES ('key-123', 1, '', 'user-1', 'testkey', '2026-07-16')`)
	if err != nil {
		t.Fatalf("failed to insert API key: %v", err)
	}

	// Seed books, podcasts, and series with prefixes
	_, err = db.Exec(`INSERT INTO books (id, title, titleIgnorePrefix) VALUES ('b-1', 'The Hobbit', 'The Hobbit')`)
	if err != nil {
		t.Fatalf("failed to insert book: %v", err)
	}
	_, err = db.Exec(`INSERT INTO podcasts (id, title, titleIgnorePrefix) VALUES ('p-1', 'A Podcast Story', 'A Podcast Story')`)
	if err != nil {
		t.Fatalf("failed to insert podcast: %v", err)
	}
	_, err = db.Exec(`INSERT INTO series (id, name, nameIgnorePrefix) VALUES ('s-1', 'The Wheel of Time', 'The Wheel of Time')`)
	if err != nil {
		t.Fatalf("failed to insert series: %v", err)
	}

	mux := http.NewServeMux()
	cfg := &core.Config{
		RouterBasePath: "",
	}
	registerSettingsRoutes(mux, cfg, db)

	// Send PATCH to /api/sorting-prefixes
	payload := map[string]interface{}{
		"sortingPrefixes": []string{"the", "a"},
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest("PATCH", "/api/sorting-prefixes", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer key-123")
	rr := httptest.NewRecorder()

	// Measure start time to check asynchronous behavior
	start := time.Now()
	mux.ServeHTTP(rr, req)
	duration := time.Since(start)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK, got %d. Body: %s", rr.Code, rr.Body.String())
	}

	// Check that the request returned very quickly (should be well under 100ms)
	t.Logf("PATCH request completed in %s", duration)
	if duration > 100*time.Millisecond {
		t.Logf("Warning: PATCH request took %s, which is longer than expected for async triggering", duration)
	}

	// Poll database to verify that the async recomputation completed successfully
	var bookIgnore, podcastIgnore, seriesIgnore string
	success := false
	maxAttempts := 20
	for i := 0; i < maxAttempts; i++ {
		err = db.QueryRow("SELECT titleIgnorePrefix FROM books WHERE id = 'b-1'").Scan(&bookIgnore)
		if err != nil {
			t.Fatalf("failed to query book: %v", err)
		}
		err = db.QueryRow("SELECT titleIgnorePrefix FROM podcasts WHERE id = 'p-1'").Scan(&podcastIgnore)
		if err != nil {
			t.Fatalf("failed to query podcast: %v", err)
		}
		err = db.QueryRow("SELECT nameIgnorePrefix FROM series WHERE id = 's-1'").Scan(&seriesIgnore)
		if err != nil {
			t.Fatalf("failed to query series: %v", err)
		}

		if bookIgnore == "Hobbit" && podcastIgnore == "Podcast Story" && seriesIgnore == "Wheel of Time" {
			success = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if !success {
		t.Fatalf("Asynchronous recomputation did not complete within the timeout. bookIgnore=%q, podcastIgnore=%q, seriesIgnore=%q", bookIgnore, podcastIgnore, seriesIgnore)
	}

	t.Log("Asynchronous recomputation completed successfully and verified in DB")
}
