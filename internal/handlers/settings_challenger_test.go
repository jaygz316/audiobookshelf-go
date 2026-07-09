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

// TestChallenger_PrefixRecomputeDeadlock verifies that prefix recomputation (e.g. books, podcasts, series)
// does not deadlock when the database pool size is constrained to 1 open connection.
// This handles the scenario where we do a Query (releasing/holding the connection) and then perform
// updates in the middle of iterating the rows.
func TestChallenger_PrefixRecomputeDeadlock(t *testing.T) {
	// Set up a memory DB with MaxOpenConns = 1
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open memory db: %v", err)
	}
	db.SetMaxOpenConns(1)
	defer db.Close()

	// Create tables needed
	_, err = db.Exec(`CREATE TABLE books (id TEXT PRIMARY KEY, title TEXT, titleIgnorePrefix TEXT)`)
	if err != nil {
		t.Fatalf("Failed to create books table: %v", err)
	}
	_, err = db.Exec(`CREATE TABLE podcasts (id TEXT PRIMARY KEY, title TEXT, titleIgnorePrefix TEXT)`)
	if err != nil {
		t.Fatalf("Failed to create podcasts table: %v", err)
	}
	_, err = db.Exec(`CREATE TABLE series (id TEXT PRIMARY KEY, name TEXT, nameIgnorePrefix TEXT)`)
	if err != nil {
		t.Fatalf("Failed to create series table: %v", err)
	}

	// Insert data where the prefix ignore column needs to be updated
	_, err = db.Exec(`INSERT INTO books (id, title, titleIgnorePrefix) VALUES ('book-1', 'The Hobbit', 'The Hobbit')`)
	if err != nil {
		t.Fatalf("Failed to insert book: %v", err)
	}
	_, err = db.Exec(`INSERT INTO podcasts (id, title, titleIgnorePrefix) VALUES ('pod-1', 'The Daily Podcast', 'The Daily Podcast')`)
	if err != nil {
		t.Fatalf("Failed to insert podcast: %v", err)
	}
	_, err = db.Exec(`INSERT INTO series (id, name, nameIgnorePrefix) VALUES ('series-1', 'The Lord of the Rings', 'The Lord of the Rings')`)
	if err != nil {
		t.Fatalf("Failed to insert series: %v", err)
	}

	// Test book recompute deadlock
	t.Run("BookRecomputeDeadlock", func(t *testing.T) {
		done := make(chan bool, 1)
		go func() {
			recomputeBooksIgnorePrefixes(db, []string{"the"})
			done <- true
		}()

		select {
		case <-done:
			t.Log("recomputeBooksIgnorePrefixes finished successfully without deadlock")
		case <-time.After(1 * time.Second):
			t.Errorf("[DEADLOCK DETECTED] recomputeBooksIgnorePrefixes timed out while updating book under MaxOpenConns=1")
		}
	})

	// Test podcast recompute deadlock
	t.Run("PodcastRecomputeDeadlock", func(t *testing.T) {
		done := make(chan bool, 1)
		go func() {
			recomputePodcastsIgnorePrefixes(db, []string{"the"})
			done <- true
		}()

		select {
		case <-done:
			t.Log("recomputePodcastsIgnorePrefixes finished successfully without deadlock")
		case <-time.After(1 * time.Second):
			t.Errorf("[DEADLOCK DETECTED] recomputePodcastsIgnorePrefixes timed out while updating podcast under MaxOpenConns=1")
		}
	})

	// Test series recompute deadlock
	t.Run("SeriesRecomputeDeadlock", func(t *testing.T) {
		done := make(chan bool, 1)
		go func() {
			recomputeSeriesIgnorePrefixes(db, []string{"the"})
			done <- true
		}()

		select {
		case <-done:
			t.Log("recomputeSeriesIgnorePrefixes finished successfully without deadlock")
		case <-time.After(1 * time.Second):
			t.Errorf("[DEADLOCK DETECTED] recomputeSeriesIgnorePrefixes timed out while updating series under MaxOpenConns=1")
		}
	})
}

// TestChallenger_ConcurrentReadWriteStress stresses the SQLite WAL database with multiple concurrent
// read and write workers to confirm that the settings, API keys, custom metadata providers,
// and user handlers do not produce locks or failures under heavy load.
func TestChallenger_ConcurrentReadWriteStress(t *testing.T) {
	// Use a temporary file for SQLite to test WAL mode concurrency
	tmpDir, err := os.MkdirTemp("", "abs-stress-")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "abs.db")
	
	// Open using InitDB to set the WAL and busy_timeout settings
	db, err := idb.InitDB(dbPath)
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	defer db.Close()

	// Build extra tables needed by handlers
	extraQueries := []string{
		`CREATE TABLE IF NOT EXISTS users (id TEXT PRIMARY KEY, username TEXT, type TEXT, isActive INTEGER, permissions TEXT, extraData TEXT, pash TEXT, token TEXT, email TEXT, bookmarks TEXT, createdAt TEXT, updatedAt TEXT, lastSeen TEXT, isLocked INTEGER)`,
		`CREATE TABLE IF NOT EXISTS customMetadataProviders (id TEXT PRIMARY KEY, name TEXT, mediaType TEXT, url TEXT, authHeaderValue TEXT, extraData TEXT, createdAt TEXT, updatedAt TEXT)`,
		`CREATE TABLE IF NOT EXISTS playbackSessions (id TEXT PRIMARY KEY, userId TEXT, mediaItemId TEXT, mediaItemType TEXT, startTime REAL, libraryId TEXT, extraData TEXT, createdAt TEXT, updatedAt TEXT)`,
		`CREATE TABLE IF NOT EXISTS libraries (id TEXT PRIMARY KEY, name TEXT, displayOrder INTEGER, icon TEXT, mediaType TEXT, provider TEXT, lastScan TEXT, lastScanVersion TEXT, settings TEXT, createdAt TEXT, updatedAt TEXT)`,
		`CREATE TABLE IF NOT EXISTS books (id TEXT PRIMARY KEY, title TEXT, titleIgnorePrefix TEXT, subtitle TEXT, publishedYear TEXT, publishedDate TEXT, publisher TEXT, description TEXT, isbn TEXT, asin TEXT, language TEXT, explicit INTEGER, abridged INTEGER, coverPath TEXT, duration REAL, narrators BLOB, audioFiles BLOB, ebookFile BLOB, chapters BLOB, tags BLOB, genres BLOB)`,
		`CREATE TABLE IF NOT EXISTS podcasts (id TEXT PRIMARY KEY, title TEXT, titleIgnorePrefix TEXT, author TEXT, releaseDate TEXT, feedURL TEXT, imageURL TEXT, description TEXT, itunesPageURL TEXT, itunesId TEXT, itunesArtistId TEXT, language TEXT, podcastType TEXT, explicit INTEGER, autoDownloadEpisodes INTEGER, autoDownloadSchedule TEXT, lastEpisodeCheck TEXT, maxEpisodesToKeep INTEGER, maxNewEpisodesToDownload INTEGER, coverPath TEXT, tags BLOB, genres BLOB, numEpisodes INTEGER)`,
		`CREATE TABLE IF NOT EXISTS series (id TEXT PRIMARY KEY, name TEXT, nameIgnorePrefix TEXT)`,
	}
	for _, q := range extraQueries {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("failed to run extra query %q: %v", q, err)
		}
	}

	// Insert root user
	_, err = db.Exec(`INSERT INTO users (id, username, type, isActive, permissions) VALUES ('root-user', 'root', 'root', 1, '{}')`)
	if err != nil {
		t.Fatalf("failed to insert root user: %v", err)
	}

	rootSession := &core.UserSession{
		ID:       "root-user",
		Username: "root",
		Type:     "root",
		IsActive: true,
	}

	// Start concurrent workers
	var wg sync.WaitGroup
	numWorkers := 15
	opsPerWorker := 20
	errCh := make(chan error, numWorkers*opsPerWorker)

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			
			for j := 0; j < opsPerWorker; j++ {
				opType := (workerID + j) % 5
				switch opType {
				case 0:
					// Read settings
					req := httptest.NewRequest("GET", "/api/settings", nil)
					req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, rootSession))
					rr := httptest.NewRecorder()
					handleGetServerSettings(db).ServeHTTP(rr, req)
					if rr.Code != http.StatusOK {
						errCh <- fmt.Errorf("worker %d op %d: GET settings failed with status %d: %s", workerID, j, rr.Code, rr.Body.String())
					}
				case 1:
					// Patch settings
					payload := map[string]interface{}{
						"dateFormat": fmt.Sprintf("YYYY-MM-DD-%d", workerID*j),
					}
					body, _ := json.Marshal(payload)
					req := httptest.NewRequest("PATCH", "/api/settings", bytes.NewReader(body))
					req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, rootSession))
					rr := httptest.NewRecorder()
					handleUpdateServerSettings(db).ServeHTTP(rr, req)
					if rr.Code != http.StatusOK {
						errCh <- fmt.Errorf("worker %d op %d: PATCH settings failed with status %d: %s", workerID, j, rr.Code, rr.Body.String())
					}
				case 2:
					// Get and Patch auth settings
					req := httptest.NewRequest("GET", "/api/auth-settings", nil)
					req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, rootSession))
					rr := httptest.NewRecorder()
					handleGetAuthSettings(db).ServeHTTP(rr, req)
					if rr.Code != http.StatusOK {
						errCh <- fmt.Errorf("worker %d op %d: GET auth-settings failed: %d", workerID, j, rr.Code)
					}

					payload := map[string]interface{}{
						"authLoginCustomMessage": fmt.Sprintf("Welcome worker %d-%d", workerID, j),
					}
					body, _ := json.Marshal(payload)
					reqPatch := httptest.NewRequest("PATCH", "/api/auth-settings", bytes.NewReader(body))
					reqPatch = reqPatch.WithContext(context.WithValue(reqPatch.Context(), core.UserContextKey, rootSession))
					rrPatch := httptest.NewRecorder()
					handleUpdateAuthSettings(db).ServeHTTP(rrPatch, reqPatch)
					if rrPatch.Code != http.StatusOK {
						errCh <- fmt.Errorf("worker %d op %d: PATCH auth-settings failed: %d", workerID, j, rrPatch.Code)
					}
				case 3:
					// Create and delete custom metadata provider
					provID := fmt.Sprintf("prov-%d-%d", workerID, j)
					payload := map[string]interface{}{
						"name":      fmt.Sprintf("Provider %s", provID),
						"url":       "http://localhost",
						"mediaType": "book",
					}
					body, _ := json.Marshal(payload)
					reqPost := httptest.NewRequest("POST", "/api/custom-metadata-providers", bytes.NewReader(body))
					reqPost = reqPost.WithContext(context.WithValue(reqPost.Context(), core.UserContextKey, rootSession))
					rrPost := httptest.NewRecorder()
					handleCreateCustomMetadataProvider(db).ServeHTTP(rrPost, reqPost)
					if rrPost.Code != http.StatusOK {
						errCh <- fmt.Errorf("worker %d op %d: POST provider failed: %d: %s", workerID, j, rrPost.Code, rrPost.Body.String())
						continue
					}

					var resp map[string]interface{}
					json.Unmarshal(rrPost.Body.Bytes(), &resp)
					provMap := resp["provider"].(map[string]interface{})
					insertedID := provMap["id"].(string)

					// Delete it
					reqDel := httptest.NewRequest("DELETE", "/api/custom-metadata-providers/"+insertedID, nil)
					reqDel = reqDel.WithContext(context.WithValue(reqDel.Context(), core.UserContextKey, rootSession))
					rrDel := httptest.NewRecorder()
					handleDeleteCustomMetadataProvider(db).ServeHTTP(rrDel, reqDel)
					if rrDel.Code != http.StatusOK {
						errCh <- fmt.Errorf("worker %d op %d: DELETE provider failed: %d: %s", workerID, j, rrDel.Code, rrDel.Body.String())
					}
				case 4:
					// Users list
					req := httptest.NewRequest("GET", "/api/users", nil)
					req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, rootSession))
					rr := httptest.NewRecorder()
					handleGetUsers(db).ServeHTTP(rr, req)
					if rr.Code != http.StatusOK {
						errCh <- fmt.Errorf("worker %d op %d: GET users failed: %d: %s", workerID, j, rr.Code, rr.Body.String())
					}
				}
			}
		}(i)
	}

	wg.Wait()
	close(errCh)

	hasErrors := false
	for err := range errCh {
		t.Errorf("Concurrency error: %v", err)
		hasErrors = true
	}
	if !hasErrors {
		t.Log("Passed concurrent read/write stress testing without errors or database locks!")
	}
}

// TestChallenger_MalformedPayloads challenges settings update and custom metadata providers endpoints
// with invalid JSON and wrong types to ensure they are handled gracefully and don't panic.
func TestChallenger_MalformedPayloads(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Create customMetadataProviders table if it doesn't exist in setupTestDB
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS customMetadataProviders (id TEXT PRIMARY KEY, name TEXT, mediaType TEXT, url TEXT, authHeaderValue TEXT, extraData TEXT, createdAt TEXT, updatedAt TEXT)`)
	if err != nil {
		t.Fatalf("Failed to create customMetadataProviders table: %v", err)
	}

	rootSession := &core.UserSession{
		ID:       "root-user",
		Username: "root",
		Type:     "root",
		IsActive: true,
	}

	// 1. PATCH /api/settings with invalid JSON structure
	t.Run("PATCH_Settings_InvalidJSON", func(t *testing.T) {
		req := httptest.NewRequest("PATCH", "/api/settings", bytes.NewBufferString(`{invalid json`))
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, rootSession))
		rr := httptest.NewRecorder()
		handleUpdateServerSettings(db).ServeHTTP(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("Expected 400 Bad Request for malformed JSON, got %d", rr.Code)
		}
	})

	// 2. PATCH /api/settings with invalid types (e.g. passing a string for a boolean key)
	t.Run("PATCH_Settings_InvalidTypes", func(t *testing.T) {
		payload := map[string]interface{}{
			"watchLibraryChanges": "not-a-boolean",
			"sortingPrefixes":     12345, // not a list
		}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest("PATCH", "/api/settings", bytes.NewReader(body))
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, rootSession))
		rr := httptest.NewRecorder()
		handleUpdateServerSettings(db).ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected 200 OK because the handler should handle invalid types gracefully, got %d", rr.Code)
		}
		var resp map[string]interface{}
		if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if _, ok := resp["serverSettings"]; !ok {
			t.Errorf("Response missing serverSettings")
		}
	})

	// 3. POST /api/custom-metadata-providers with invalid/missing parameters
	t.Run("POST_CustomMetadataProvider_MissingParams", func(t *testing.T) {
		payload := map[string]interface{}{
			"name": "", // empty name
			"url":  "http://test",
		}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest("POST", "/api/custom-metadata-providers", bytes.NewReader(body))
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, rootSession))
		rr := httptest.NewRecorder()
		handleCreateCustomMetadataProvider(db).ServeHTTP(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("Expected 400 Bad Request for missing params, got %d", rr.Code)
		}
	})

	// 4. POST /api/custom-metadata-providers with invalid mediaType (e.g. not book/podcast)
	t.Run("POST_CustomMetadataProvider_InvalidMediaType", func(t *testing.T) {
		payload := map[string]interface{}{
			"name":      "Test Provider",
			"url":       "http://test",
			"mediaType": "invalid-media-type",
		}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest("POST", "/api/custom-metadata-providers", bytes.NewReader(body))
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, rootSession))
		rr := httptest.NewRecorder()
		handleCreateCustomMetadataProvider(db).ServeHTTP(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("Expected 400 Bad Request for invalid mediaType, got %d", rr.Code)
		}
	})
}

// TestChallenger_ConcurrentPrefixRecompute triggers prefix recomputation concurrently in multiple goroutines
// to verify if SQLite handles concurrent updates on the same rows without lock failures, and whether any
// silent update failures occur.
func TestChallenger_ConcurrentPrefixRecompute(t *testing.T) {
	// Create a temp file SQLite database
	tmpDir, err := os.MkdirTemp("", "abs-recompute-")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "abs.db")
	db, err := idb.InitDB(dbPath)
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	defer db.Close()

	// Ensure books table exists
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS books (id TEXT PRIMARY KEY, title TEXT, titleIgnorePrefix TEXT)`)
	if err != nil {
		t.Fatalf("failed to create books table: %v", err)
	}

	// Insert a book that needs update
	_, err = db.Exec(`INSERT INTO books (id, title, titleIgnorePrefix) VALUES ('book-1', 'The Hobbit', 'The Hobbit')`)
	if err != nil {
		t.Fatalf("failed to insert book: %v", err)
	}

	// Spawn multiple concurrent recomputes
	var wg sync.WaitGroup
	numConcurrently := 10
	
	for i := 0; i < numConcurrently; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			// Alternate prefixes to trigger changes
			var prefixes []string
			if workerID%2 == 0 {
				prefixes = []string{"the"}
			} else {
				prefixes = []string{"a"}
			}
			recomputeBooksIgnorePrefixes(db, prefixes)
		}(i)
	}

	wg.Wait()

	// Verify if the database is in a consistent state
	var titleIgnorePrefix string
	err = db.QueryRow("SELECT titleIgnorePrefix FROM books WHERE id = 'book-1'").Scan(&titleIgnorePrefix)
	if err != nil {
		t.Fatalf("failed to query book: %v", err)
	}
	t.Logf("Final titleIgnorePrefix: %s", titleIgnorePrefix)
}

