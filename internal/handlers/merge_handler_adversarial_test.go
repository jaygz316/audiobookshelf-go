package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"audiobookshelf/internal/core"
)

func TestMergeAudioFiles_Adversarial(t *testing.T) {
	// Backup and restore MetadataPath
	oldMetaPath := MetadataPath
	defer func() { MetadataPath = oldMetaPath }()

	tempDir := t.TempDir()
	MetadataPath = tempDir

	// Helper to seed a library item and book
	seedDB := func(t *testing.T, db *sql.DB, itemID, mediaID, mediaType string, files []MergeAudioFile) {
		bFiles, err := json.Marshal(files)
		if err != nil {
			t.Fatalf("failed to marshal files: %v", err)
		}

		_, err = db.Exec(`INSERT INTO books (id, title, audioFiles, chapters) VALUES (?, 'Test Book', ?, '[]')`, mediaID, bFiles)
		if err != nil {
			t.Fatalf("failed to insert book: %v", err)
		}

		_, err = db.Exec(`INSERT INTO libraryItems (id, mediaId, mediaType, path, size, updatedAt) VALUES (?, ?, ?, ?, 1000, '2026-06-08 12:00:00.000')`, itemID, mediaID, mediaType, tempDir)
		if err != nil {
			t.Fatalf("failed to insert library item: %v", err)
		}
	}

	t.Run("Unauthorized - No User Session", func(t *testing.T) {
		db := setupTestDB(t)
		defer db.Close()

		req := httptest.NewRequest("POST", "/api/items/item-123/merge", nil)
		rr := httptest.NewRecorder()

		handler := handleMergeAudioFiles(db)
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Errorf("Expected status 401 Unauthorized, got %d: %s", rr.Code, rr.Body.String())
		}
	})

	t.Run("Forbidden - Non-Admin/Non-Root User", func(t *testing.T) {
		db := setupTestDB(t)
		defer db.Close()

		userSess := &core.UserSession{
			ID:       "user-normal",
			Username: "normal",
			Type:     "user",
			IsActive: true,
		}

		req := httptest.NewRequest("POST", "/api/items/item-123/merge", nil)
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, userSess))
		rr := httptest.NewRecorder()

		handler := handleMergeAudioFiles(db)
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusForbidden {
			t.Errorf("Expected status 403 Forbidden, got %d: %s", rr.Code, rr.Body.String())
		}
	})

	t.Run("Method Not Allowed", func(t *testing.T) {
		db := setupTestDB(t)
		defer db.Close()

		userSess := &core.UserSession{
			ID:       "user-admin",
			Username: "admin",
			Type:     "admin",
			IsActive: true,
		}

		req := httptest.NewRequest("GET", "/api/items/item-123/merge", nil)
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, userSess))
		rr := httptest.NewRecorder()

		handler := handleMergeAudioFiles(db)
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusMethodNotAllowed {
			t.Errorf("Expected status 405 Method Not Allowed, got %d: %s", rr.Code, rr.Body.String())
		}
	})

	t.Run("Invalid Path - Path Traversal in Item ID", func(t *testing.T) {
		db := setupTestDB(t)
		defer db.Close()

		userSess := &core.UserSession{
			ID:       "user-admin",
			Username: "admin",
			Type:     "admin",
			IsActive: true,
		}

		// Traversal in the middle of path
		req := httptest.NewRequest("POST", "/api/items/item-..-123/merge", nil)
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, userSess))
		rr := httptest.NewRecorder()

		handler := handleMergeAudioFiles(db)
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("Expected status 400 Bad Request, got %d: %s", rr.Code, rr.Body.String())
		}
	})

	t.Run("Invalid Path - Backslash Traversal in Item ID", func(t *testing.T) {
		db := setupTestDB(t)
		defer db.Close()

		userSess := &core.UserSession{
			ID:       "user-admin",
			Username: "admin",
			Type:     "admin",
			IsActive: true,
		}

		req := httptest.NewRequest("POST", `/api/items/item-\..\merge`, nil)
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, userSess))
		rr := httptest.NewRecorder()

		handler := handleMergeAudioFiles(db)
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("Expected status 400 Bad Request, got %d: %s", rr.Code, rr.Body.String())
		}
	})

	t.Run("Item Not Found", func(t *testing.T) {
		db := setupTestDB(t)
		defer db.Close()

		userSess := &core.UserSession{
			ID:       "user-admin",
			Username: "admin",
			Type:     "admin",
			IsActive: true,
		}

		req := httptest.NewRequest("POST", "/api/items/nonexistent-item/merge", nil)
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, userSess))
		rr := httptest.NewRecorder()

		handler := handleMergeAudioFiles(db)
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusNotFound {
			t.Errorf("Expected status 404 Not Found, got %d: %s", rr.Code, rr.Body.String())
		}
	})

	t.Run("Invalid MediaType - Not a Book", func(t *testing.T) {
		db := setupTestDB(t)
		defer db.Close()

		userSess := &core.UserSession{
			ID:       "user-admin",
			Username: "admin",
			Type:     "admin",
			IsActive: true,
		}

		// Seed a library item as 'podcast' instead of 'book'
		seedDB(t, db, "item-123", "podcast-123", "podcast", nil)

		req := httptest.NewRequest("POST", "/api/items/item-123/merge", nil)
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, userSess))
		rr := httptest.NewRecorder()

		handler := handleMergeAudioFiles(db)
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("Expected status 400 Bad Request, got %d: %s", rr.Code, rr.Body.String())
		}
	})

	t.Run("Empty Active Files - Zero Files", func(t *testing.T) {
		db := setupTestDB(t)
		defer db.Close()

		userSess := &core.UserSession{
			ID:       "user-admin",
			Username: "admin",
			Type:     "admin",
			IsActive: true,
		}

		seedDB(t, db, "item-123", "book-123", "book", []MergeAudioFile{})

		req := httptest.NewRequest("POST", "/api/items/item-123/merge", nil)
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, userSess))
		rr := httptest.NewRecorder()

		handler := handleMergeAudioFiles(db)
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("Expected status 400 Bad Request, got %d: %s", rr.Code, rr.Body.String())
		}
	})

	t.Run("Single Active File - Less than 2 files", func(t *testing.T) {
		db := setupTestDB(t)
		defer db.Close()

		userSess := &core.UserSession{
			ID:       "user-admin",
			Username: "admin",
			Type:     "admin",
			IsActive: true,
		}

		// Single active file
		safePath := filepath.Join(tempDir, "track1.mp3")
		_ = os.WriteFile(safePath, []byte("dummy audio content"), 0644)

		files := []MergeAudioFile{
			{
				Index:   0,
				Exclude: false,
			},
		}
		files[0].Metadata.Path = safePath
		files[0].Metadata.Filename = "track1.mp3"

		seedDB(t, db, "item-123", "book-123", "book", files)

		req := httptest.NewRequest("POST", "/api/items/item-123/merge", nil)
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, userSess))
		rr := httptest.NewRecorder()

		handler := handleMergeAudioFiles(db)
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("Expected status 400 Bad Request, got %d: %s", rr.Code, rr.Body.String())
		}
	})

	t.Run("Unsafe Audio File Path", func(t *testing.T) {
		db := setupTestDB(t)
		defer db.Close()

		userSess := &core.UserSession{
			ID:       "user-admin",
			Username: "admin",
			Type:     "admin",
			IsActive: true,
		}

		// Let's create two files. One is outside of tempDir and not in libraryFolders/MetadataPath.
		// Wait, if it doesn't exist, it might fail on os.NotExist check first. So let's create a file outside tempDir.
		unsafeDir := t.TempDir() // a different temp dir than tempDir (MetadataPath)
		file1 := filepath.Join(unsafeDir, "unsafe_track1.mp3")
		file2 := filepath.Join(tempDir, "safe_track2.mp3")

		_ = os.WriteFile(file1, []byte("dummy 1"), 0644)
		_ = os.WriteFile(file2, []byte("dummy 2"), 0644)

		files := []MergeAudioFile{
			{Index: 0, Exclude: false},
			{Index: 1, Exclude: false},
		}
		files[0].Metadata.Path = file1
		files[0].Metadata.Filename = "unsafe_track1.mp3"
		files[1].Metadata.Path = file2
		files[1].Metadata.Filename = "safe_track2.mp3"

		seedDB(t, db, "item-123", "book-123", "book", files)

		// Seed a libraryFolder pointing only to tempDir
		_, err := db.Exec(`INSERT INTO libraryFolders (id, path, libraryId) VALUES ('folder-123', ?, 'lib-123')`, tempDir)
		if err != nil {
			t.Fatalf("failed to insert folder: %v", err)
		}

		req := httptest.NewRequest("POST", "/api/items/item-123/merge", nil)
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, userSess))
		rr := httptest.NewRecorder()

		handler := handleMergeAudioFiles(db)
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusForbidden {
			t.Errorf("Expected status 403 Forbidden (unsafe path), got %d: %s", rr.Code, rr.Body.String())
		}
	})

	t.Run("Audio File Not Found On Disk", func(t *testing.T) {
		db := setupTestDB(t)
		defer db.Close()

		userSess := &core.UserSession{
			ID:       "user-admin",
			Username: "admin",
			Type:     "admin",
			IsActive: true,
		}

		// Two files, but they don't actually exist on disk
		file1 := filepath.Join(tempDir, "nonexistent_track1.mp3")
		file2 := filepath.Join(tempDir, "nonexistent_track2.mp3")

		files := []MergeAudioFile{
			{Index: 0, Exclude: false},
			{Index: 1, Exclude: false},
		}
		files[0].Metadata.Path = file1
		files[0].Metadata.Filename = "nonexistent_track1.mp3"
		files[1].Metadata.Path = file2
		files[1].Metadata.Filename = "nonexistent_track2.mp3"

		seedDB(t, db, "item-123", "book-123", "book", files)

		// Seed libraryFolder
		_, err := db.Exec(`INSERT INTO libraryFolders (id, path, libraryId) VALUES ('folder-123', ?, 'lib-123')`, tempDir)
		if err != nil {
			t.Fatalf("failed to insert folder: %v", err)
		}

		req := httptest.NewRequest("POST", "/api/items/item-123/merge", nil)
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, userSess))
		rr := httptest.NewRecorder()

		handler := handleMergeAudioFiles(db)
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("Expected status 400 Bad Request, got %d: %s", rr.Code, rr.Body.String())
		}
	})
}
