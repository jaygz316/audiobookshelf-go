package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"audiobookshelf/internal/core"
)

func TestMetadataProviders_EmptyAndPopulated(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// 1. Get initial active providers
	req := httptest.NewRequest("GET", "/api/search/providers", nil)
	rr := httptest.NewRecorder()
	handleGetMetadataProviders(db).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected 200 OK, got %d", rr.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse JSON response: %v", err)
	}

	providers, ok := resp["providers"].(map[string]interface{})
	if !ok {
		t.Fatalf("Expected providers map")
	}

	books, ok := providers["books"].([]interface{})
	if !ok || len(books) == 0 {
		t.Fatalf("Expected non-empty books array")
	}

	podcasts, ok := providers["podcasts"].([]interface{})
	if !ok || len(podcasts) == 0 {
		t.Fatalf("Expected non-empty podcasts array")
	}

	// 2. Insert custom providers
	_, err := db.Exec(`INSERT INTO customMetadataProviders (id, name, mediaType, url, authHeaderValue, extraData, createdAt, updatedAt) 
		VALUES ('test-book-id', 'My Custom Book Provider', 'book', 'https://book.com', 'Bearer token', '{}', 12345, 12345)`)
	if err != nil {
		t.Fatalf("Failed to insert custom book provider: %v", err)
	}

	_, err = db.Exec(`INSERT INTO customMetadataProviders (id, name, mediaType, url, authHeaderValue, extraData, createdAt, updatedAt) 
		VALUES ('test-podcast-id', 'My Custom Podcast Provider', 'podcast', 'https://podcast.com', NULL, '{}', 12345, 12345)`)
	if err != nil {
		t.Fatalf("Failed to insert custom podcast provider: %v", err)
	}

	// 3. Request again and verify custom providers are present
	rr = httptest.NewRecorder()
	handleGetMetadataProviders(db).ServeHTTP(rr, req)

	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse JSON response: %v", err)
	}

	providers = resp["providers"].(map[string]interface{})
	books = providers["books"].([]interface{})
	podcasts = providers["podcasts"].([]interface{})

	// Check custom book provider exists
	foundBook := false
	for _, item := range books {
		m := item.(map[string]interface{})
		if m["value"] == "custom-test-book-id" {
			foundBook = true
			if m["text"] != "My Custom Book Provider" {
				t.Errorf("Expected text 'My Custom Book Provider', got %q", m["text"])
			}
		}
	}
	if !foundBook {
		t.Errorf("Custom book provider not found in response")
	}

	// Check custom podcast provider exists
	foundPodcast := false
	for _, item := range podcasts {
		m := item.(map[string]interface{})
		if m["value"] == "custom-test-podcast-id" {
			foundPodcast = true
			if m["text"] != "My Custom Podcast Provider" {
				t.Errorf("Expected text 'My Custom Podcast Provider', got %q", m["text"])
			}
		}
	}
	if !foundPodcast {
		t.Errorf("Custom podcast provider not found in response")
	}
}

func TestCreateCustomMetadataProvider_InvalidInputs(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	adminSession := &core.UserSession{
		ID:       "admin-user",
		Username: "admin",
		Type:     "admin",
		IsActive: true,
	}

	testCases := []struct {
		name           string
		payload        string
		expectedStatus int
	}{
		{
			name:           "Empty Name",
			payload:        `{"name":"","url":"http://test.com","mediaType":"book"}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Empty URL",
			payload:        `{"name":"Test","url":"","mediaType":"book"}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Empty MediaType",
			payload:        `{"name":"Test","url":"http://test.com","mediaType":""}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Invalid Scheme ftp",
			payload:        `{"name":"Test","url":"ftp://test.com","mediaType":"book"}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Invalid Scheme javascript",
			payload:        `{"name":"Test","url":"javascript:alert(1)","mediaType":"book"}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Invalid Scheme data",
			payload:        `{"name":"Test","url":"data:text/html,test","mediaType":"book"}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Invalid Scheme none",
			payload:        `{"name":"Test","url":"test.com","mediaType":"book"}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Invalid MediaType other",
			payload:        `{"name":"Test","url":"http://test.com","mediaType":"other"}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Malformed JSON",
			payload:        `{"name":"Test",`,
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/api/custom-metadata-providers", bytes.NewBufferString(tc.payload))
			req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, adminSession))
			rr := httptest.NewRecorder()

			handleCreateCustomMetadataProvider(db).ServeHTTP(rr, req)

			if rr.Code != tc.expectedStatus {
				t.Errorf("Expected status %d, got %d. Response: %s", tc.expectedStatus, rr.Code, rr.Body.String())
			}
		})
	}
}

func TestCustomMetadataProviders_PermissionsAndMissingContext(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	nonAdminSession := &core.UserSession{
		ID:       "user-1",
		Username: "user",
		Type:     "user",
		IsActive: true,
	}

	t.Run("Get_MissingSession", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/custom-metadata-providers", nil)
		rr := httptest.NewRecorder()
		handleGetCustomMetadataProviders(db).ServeHTTP(rr, req)
		if rr.Code != http.StatusForbidden {
			t.Errorf("Expected 403 Forbidden, got %d", rr.Code)
		}
	})

	t.Run("Create_MissingSession", func(t *testing.T) {
		payload := `{"name":"Test","url":"http://test.com","mediaType":"book"}`
		req := httptest.NewRequest("POST", "/api/custom-metadata-providers", bytes.NewBufferString(payload))
		rr := httptest.NewRecorder()
		handleCreateCustomMetadataProvider(db).ServeHTTP(rr, req)
		if rr.Code != http.StatusForbidden {
			t.Errorf("Expected 403 Forbidden, got %d", rr.Code)
		}
	})

	t.Run("Delete_MissingSession", func(t *testing.T) {
		req := httptest.NewRequest("DELETE", "/api/custom-metadata-providers/some-id", nil)
		rr := httptest.NewRecorder()
		handleDeleteCustomMetadataProvider(db).ServeHTTP(rr, req)
		if rr.Code != http.StatusForbidden {
			t.Errorf("Expected 403 Forbidden, got %d", rr.Code)
		}
	})

	t.Run("Get_NonAdmin", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/custom-metadata-providers", nil)
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, nonAdminSession))
		rr := httptest.NewRecorder()
		handleGetCustomMetadataProviders(db).ServeHTTP(rr, req)
		if rr.Code != http.StatusForbidden {
			t.Errorf("Expected 403 Forbidden, got %d", rr.Code)
		}
	})
}

func TestDeleteCustomMetadataProvider_FallbackLibraries(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	adminSession := &core.UserSession{
		ID:       "admin-user",
		Username: "admin",
		Type:     "admin",
		IsActive: true,
	}

	// 1. Insert mock libraries
	// Book library using custom provider
	_, err := db.Exec(`INSERT INTO libraries (id, name, mediaType, provider, settings, createdAt, updatedAt) 
		VALUES ('lib-book', 'My Books', 'book', 'custom-target-id', '{}', 'now', 'now')`)
	if err != nil {
		t.Fatalf("Failed to insert mock book library: %v", err)
	}

	// Podcast library using custom provider
	_, err = db.Exec(`INSERT INTO libraries (id, name, mediaType, provider, settings, createdAt, updatedAt) 
		VALUES ('lib-podcast', 'My Podcasts', 'podcast', 'custom-target-id', '{}', 'now', 'now')`)
	if err != nil {
		t.Fatalf("Failed to insert mock podcast library: %v", err)
	}

	// Another book library using a different provider (should remain unchanged)
	_, err = db.Exec(`INSERT INTO libraries (id, name, mediaType, provider, settings, createdAt, updatedAt) 
		VALUES ('lib-book-other', 'Other Books', 'book', 'audible', '{}', 'now', 'now')`)
	if err != nil {
		t.Fatalf("Failed to insert other book library: %v", err)
	}

	// 2. Perform Delete of target-id
	req := httptest.NewRequest("DELETE", "/api/custom-metadata-providers/target-id", nil)
	req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, adminSession))
	rr := httptest.NewRecorder()

	handleDeleteCustomMetadataProvider(db).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected 200 OK, got %d: %s", rr.Code, rr.Body.String())
	}

	// 3. Verify fallbacks in DB
	var bookProv string
	err = db.QueryRow("SELECT provider FROM libraries WHERE id = 'lib-book'").Scan(&bookProv)
	if err != nil {
		t.Fatalf("Failed to query lib-book: %v", err)
	}
	if bookProv != "google" {
		t.Errorf("Expected lib-book provider to fallback to 'google', got %q", bookProv)
	}

	var podcastProv string
	err = db.QueryRow("SELECT provider FROM libraries WHERE id = 'lib-podcast'").Scan(&podcastProv)
	if err != nil {
		t.Fatalf("Failed to query lib-podcast: %v", err)
	}
	if podcastProv != "itunes" {
		t.Errorf("Expected lib-podcast provider to fallback to 'itunes', got %q", podcastProv)
	}

	var otherProv string
	err = db.QueryRow("SELECT provider FROM libraries WHERE id = 'lib-book-other'").Scan(&otherProv)
	if err != nil {
		t.Fatalf("Failed to query lib-book-other: %v", err)
	}
	if otherProv != "audible" {
		t.Errorf("Expected lib-book-other provider to remain 'audible', got %q", otherProv)
	}
}

func TestCustomMetadataProviders_Concurrency(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	adminSession := &core.UserSession{
		ID:       "admin-user",
		Username: "admin",
		Type:     "admin",
		IsActive: true,
	}

	const concurrencyCount = 50
	var wg sync.WaitGroup
	wg.Add(concurrencyCount * 3) // 3 actions per worker: Create, Get, Delete

	// We'll run multiple goroutines creating, getting and deleting custom providers concurrently
	for i := 0; i < concurrencyCount; i++ {
		go func(workerID int) {
			defer wg.Done()
			// 1. Create provider
			payload := `{"name":"Concur Provider","url":"https://api.concur.com/v1","mediaType":"book"}`
			req := httptest.NewRequest("POST", "/api/custom-metadata-providers", bytes.NewBufferString(payload))
			req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, adminSession))
			rr := httptest.NewRecorder()
			handleCreateCustomMetadataProvider(db).ServeHTTP(rr, req)
			if rr.Code != http.StatusOK {
				t.Errorf("Worker %d failed to create provider", workerID)
			}
		}(i)

		go func(workerID int) {
			defer wg.Done()
			// 2. Get active providers (no auth required)
			req := httptest.NewRequest("GET", "/api/search/providers", nil)
			rr := httptest.NewRecorder()
			handleGetMetadataProviders(db).ServeHTTP(rr, req)
			if rr.Code != http.StatusOK {
				t.Errorf("Worker %d failed to get active providers", workerID)
			}
		}(i)

		go func(workerID int) {
			defer wg.Done()
			// 3. Get custom providers (admin required)
			req := httptest.NewRequest("GET", "/api/custom-metadata-providers", nil)
			req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, adminSession))
			rr := httptest.NewRecorder()
			handleGetCustomMetadataProviders(db).ServeHTTP(rr, req)
			if rr.Code != http.StatusOK {
				t.Errorf("Worker %d failed to get custom providers", workerID)
			}
		}(i)
	}

	wg.Wait()
}
