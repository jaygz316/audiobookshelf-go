package handlers

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"audiobookshelf/internal/core"
	idb "audiobookshelf/internal/db"
)

// TestEmpirical_PrefixRecompute_NoDeadlock checks that recomputing prefixes
// for books, podcasts, and series under MaxOpenConns=1 does not deadlock.
func TestEmpirical_PrefixRecompute_NoDeadlock(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open memory db: %v", err)
	}
	db.SetMaxOpenConns(1)
	defer db.Close()

	// Setup necessary tables
	setupQueries := []string{
		`CREATE TABLE books (id TEXT PRIMARY KEY, title TEXT, titleIgnorePrefix TEXT)`,
		`CREATE TABLE podcasts (id TEXT PRIMARY KEY, title TEXT, titleIgnorePrefix TEXT)`,
		`CREATE TABLE series (id TEXT PRIMARY KEY, name TEXT, nameIgnorePrefix TEXT)`,
	}
	for _, q := range setupQueries {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("Failed setup query: %v", err)
		}
	}

	// Insert test data
	_, err = db.Exec(`INSERT INTO books (id, title, titleIgnorePrefix) VALUES ('b1', 'The Hobbit', 'The Hobbit')`)
	if err != nil {
		t.Fatalf("Failed to insert book: %v", err)
	}
	_, err = db.Exec(`INSERT INTO podcasts (id, title, titleIgnorePrefix) VALUES ('p1', 'A Podcast Story', 'A Podcast Story')`)
	if err != nil {
		t.Fatalf("Failed to insert podcast: %v", err)
	}
	_, err = db.Exec(`INSERT INTO series (id, name, nameIgnorePrefix) VALUES ('s1', 'The Wheel of Time', 'The Wheel of Time')`)
	if err != nil {
		t.Fatalf("Failed to insert series: %v", err)
	}

	prefixes := []string{"the", "a"}

	t.Run("BooksPrefixRecomputation", func(t *testing.T) {
		done := make(chan struct{})
		go func() {
			recomputeBooksIgnorePrefixes(db, prefixes)
			close(done)
		}()

		select {
		case <-done:
			// Success
			var val string
			err := db.QueryRow("SELECT titleIgnorePrefix FROM books WHERE id = 'b1'").Scan(&val)
			if err != nil {
				t.Fatalf("Query failed: %v", err)
			}
			if val != "Hobbit" {
				t.Errorf("Expected 'Hobbit', got '%s'", val)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("DEADLOCK DETECTED in recomputeBooksIgnorePrefixes under MaxOpenConns=1")
		}
	})

	t.Run("PodcastsPrefixRecomputation", func(t *testing.T) {
		done := make(chan struct{})
		go func() {
			recomputePodcastsIgnorePrefixes(db, prefixes)
			close(done)
		}()

		select {
		case <-done:
			// Success
			var val string
			err := db.QueryRow("SELECT titleIgnorePrefix FROM podcasts WHERE id = 'p1'").Scan(&val)
			if err != nil {
				t.Fatalf("Query failed: %v", err)
			}
			if val != "Podcast Story" {
				t.Errorf("Expected 'Podcast Story', got '%s'", val)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("DEADLOCK DETECTED in recomputePodcastsIgnorePrefixes under MaxOpenConns=1")
		}
	})

	t.Run("SeriesPrefixRecomputation", func(t *testing.T) {
		done := make(chan struct{})
		go func() {
			recomputeSeriesIgnorePrefixes(db, prefixes)
			close(done)
		}()

		select {
		case <-done:
			// Success
			var val string
			err := db.QueryRow("SELECT nameIgnorePrefix FROM series WHERE id = 's1'").Scan(&val)
			if err != nil {
				t.Fatalf("Query failed: %v", err)
			}
			if val != "Wheel of Time" {
				t.Errorf("Expected 'Wheel of Time', got '%s'", val)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("DEADLOCK DETECTED in recomputeSeriesIgnorePrefixes under MaxOpenConns=1")
		}
	})
}

// TestEmpirical_CustomMetadataProvider_MediaTypeValidation verifies that invalid
// mediaType values are strictly validated and result in a 400 Bad Request.
func TestEmpirical_CustomMetadataProvider_MediaTypeValidation(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open memory db: %v", err)
	}
	defer db.Close()

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS customMetadataProviders (id TEXT PRIMARY KEY, name TEXT, mediaType TEXT, url TEXT, authHeaderValue TEXT, extraData TEXT, createdAt TEXT, updatedAt TEXT)`)
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	rootSession := &core.UserSession{
		ID:       "root-user",
		Username: "root",
		Type:     "root",
		IsActive: true,
	}

	tests := []struct {
		name           string
		mediaType      interface{} // can test non-string types too
		expectedStatus int
		expectErrorMsg string
	}{
		{
			name:           "Valid book mediaType",
			mediaType:      "book",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Valid podcast mediaType",
			mediaType:      "podcast",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Invalid mediaType - empty",
			mediaType:      "",
			expectedStatus: http.StatusBadRequest,
			expectErrorMsg: "Name, url and mediaType are required",
		},
		{
			name:           "Invalid mediaType - wrong value",
			mediaType:      "other",
			expectedStatus: http.StatusBadRequest,
			expectErrorMsg: "mediaType must be book or podcast",
		},
		{
			name:           "Invalid mediaType - uppercase",
			mediaType:      "BOOK",
			expectedStatus: http.StatusBadRequest,
			expectErrorMsg: "mediaType must be book or podcast",
		},
		{
			name:           "Invalid mediaType - space padded",
			mediaType:      "book ",
			expectedStatus: http.StatusBadRequest,
			expectErrorMsg: "mediaType must be book or podcast",
		},
		{
			name:           "Invalid mediaType - float",
			mediaType:      12.34,
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			payload := map[string]interface{}{
				"name":      "Test Provider",
				"url":       "http://localhost/metadata",
				"mediaType": tc.mediaType,
			}
			body, _ := json.Marshal(payload)
			req := httptest.NewRequest("POST", "/api/custom-metadata-providers", bytes.NewReader(body))
			req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, rootSession))
			rr := httptest.NewRecorder()

			handleCreateCustomMetadataProvider(db).ServeHTTP(rr, req)

			if rr.Code != tc.expectedStatus {
				t.Errorf("Expected status %d, got %d. Body: %s", tc.expectedStatus, rr.Code, rr.Body.String())
			}

			if tc.expectedStatus == http.StatusBadRequest && tc.expectErrorMsg != "" {
				var errResp map[string]string
				if err := json.NewDecoder(rr.Body).Decode(&errResp); err != nil {
					t.Fatalf("Failed to parse error response: %v", err)
				}
				if errResp["error"] != tc.expectErrorMsg {
					t.Errorf("Expected error message %q, got %q", tc.expectErrorMsg, errResp["error"])
				}
			}
		})
	}
}

// TestEmpirical_CustomMetadataProvider_Concurrency stresses the custom metadata provider
// creation with concurrent valid and invalid requests.
func TestEmpirical_CustomMetadataProvider_Concurrency(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "abs-providers-concurrency-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "abs.db")
	db, err := idb.InitDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to open db: %v", err)
	}
	defer db.Close()

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS customMetadataProviders (id TEXT PRIMARY KEY, name TEXT, mediaType TEXT, url TEXT, authHeaderValue TEXT, extraData TEXT, createdAt TEXT, updatedAt TEXT)`)
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	rootSession := &core.UserSession{
		ID:       "root-user",
		Username: "root",
		Type:     "root",
		IsActive: true,
	}

	var wg sync.WaitGroup
	workers := 20
	requestsPerWorker := 10
	errCh := make(chan error, workers*requestsPerWorker)

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < requestsPerWorker; j++ {
				// Alternate between valid and invalid mediaType
				mediaType := "book"
				expectedStatus := http.StatusOK
				if (workerID+j)%2 == 0 {
					mediaType = "invalid"
					expectedStatus = http.StatusBadRequest
				}

				payload := map[string]interface{}{
					"name":      fmt.Sprintf("Provider-%d-%d", workerID, j),
					"url":       "http://localhost/metadata",
					"mediaType": mediaType,
				}
				body, _ := json.Marshal(payload)
				req := httptest.NewRequest("POST", "/api/custom-metadata-providers", bytes.NewReader(body))
				req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, rootSession))
				rr := httptest.NewRecorder()

				handleCreateCustomMetadataProvider(db).ServeHTTP(rr, req)

				if rr.Code != expectedStatus {
					errCh <- fmt.Errorf("Worker %d, req %d got status %d, expected %d. Body: %s", workerID, j, rr.Code, expectedStatus, rr.Body.String())
				}
			}
		}(i)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Error(err)
	}
}
