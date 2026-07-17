package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"audiobookshelf/internal/core"
)

func TestMergeAudioFiles_Unauthorized(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	handler := handleMergeAudioFiles(db)

	t.Run("NoUserSession", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/items/item-123/merge", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Errorf("Expected 401 Unauthorized, got %d: %s", rr.Code, rr.Body.String())
		}
	})

	t.Run("ForbiddenUser", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/items/item-123/merge", nil)
		userSess := &core.UserSession{
			ID:       "user-normal",
			Username: "normal",
			Type:     "user", // not admin/root
			IsActive: true,
		}
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, userSess))
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusForbidden {
			t.Errorf("Expected 403 Forbidden, got %d: %s", rr.Code, rr.Body.String())
		}
	})
}

func TestMergeAudioFiles_InvalidMethod(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	handler := handleMergeAudioFiles(db)
	userSess := &core.UserSession{
		ID:       "user-admin",
		Username: "admin",
		Type:     "admin",
		IsActive: true,
	}

	methods := []string{"GET", "PUT", "DELETE", "PATCH"}
	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/api/items/item-123/merge", nil)
			req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, userSess))
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusMethodNotAllowed {
				t.Errorf("Expected 405 Method Not Allowed for %s, got %d", method, rr.Code)
			}
		})
	}
}

func TestMergeAudioFiles_InvalidPath(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	handler := handleMergeAudioFiles(db)
	userSess := &core.UserSession{
		ID:       "user-admin",
		Username: "admin",
		Type:     "admin",
		IsActive: true,
	}

	invalidPaths := []string{
		"/api/items/item-123",
		"/api/items/item-123/merge/extra",
		"/merge",
		"/a/merge",
	}

	for _, path := range invalidPaths {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest("POST", path, nil)
			req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, userSess))
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusBadRequest {
				t.Errorf("Expected 400 Bad Request for path %s, got %d: %s", path, rr.Code, rr.Body.String())
			}
		})
	}
}

func TestPrepareMergeContext_EdgeCases(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	tempDir := t.TempDir()

	// Seed library folder to allow safe paths
	_, err := db.Exec(`INSERT INTO libraryFolders (id, path, libraryId) VALUES ('folder-123', ?, 'lib-123')`, tempDir)
	if err != nil {
		t.Fatalf("Failed to seed libraryFolder: %v", err)
	}

	t.Run("InvalidItemIDTraversal", func(t *testing.T) {
		_, status, err := prepareMergeContext(db, "item/../escape")
		if err == nil || status != http.StatusBadRequest {
			t.Errorf("Expected bad request status, got %d, err: %v", status, err)
		}
	})

	t.Run("ItemNotFound", func(t *testing.T) {
		_, status, err := prepareMergeContext(db, "nonexistent-item")
		if err == nil || status != http.StatusNotFound {
			t.Errorf("Expected 404 Not Found, got %d, err: %v", status, err)
		}
	})

	t.Run("NotABook", func(t *testing.T) {
		// Seed a library item that is a podcast
		_, err := db.Exec(`INSERT INTO libraryItems (id, mediaId, mediaType, path, size) VALUES ('item-podcast', 'podcast-123', 'podcast', ?, 1000)`, tempDir)
		if err != nil {
			t.Fatalf("Failed to seed library item: %v", err)
		}

		_, status, err := prepareMergeContext(db, "item-podcast")
		if err == nil || status != http.StatusBadRequest {
			t.Errorf("Expected 400 Bad Request for podcast, got %d, err: %v", status, err)
		}
	})

	t.Run("EmptyAudioFiles", func(t *testing.T) {
		_, err = db.Exec(`INSERT INTO libraryItems (id, mediaId, mediaType, path, size) VALUES ('item-empty-files', 'book-empty', 'book', ?, 1000)`, tempDir)
		if err != nil {
			t.Fatalf("Failed to seed library item: %v", err)
		}

		// Seed a book with empty audio files list
		_, err = db.Exec(`INSERT INTO books (id, title, audioFiles) VALUES ('book-empty', 'Empty Book', '[]')`)
		if err != nil {
			t.Fatalf("Failed to seed book: %v", err)
		}

		_, status, err := prepareMergeContext(db, "item-empty-files")
		if err == nil || status != http.StatusBadRequest {
			t.Errorf("Expected 400 Bad Request, got %d, err: %v", status, err)
		}
	})

	t.Run("OneAudioFileOnly", func(t *testing.T) {
		_, err = db.Exec(`INSERT INTO libraryItems (id, mediaId, mediaType, path, size) VALUES ('item-one-file', 'book-one', 'book', ?, 1000)`, tempDir)
		if err != nil {
			t.Fatalf("Failed to seed library item: %v", err)
		}

		file1 := filepath.Join(tempDir, "one.m4a")
		createTestAudioFile(t, file1, 1)

		audioFiles := []MergeAudioFile{
			{
				Index:    0,
				Exclude:  false,
				Duration: 1.0,
				Title:    "Track 1",
			},
		}
		audioFiles[0].Metadata.Path = file1
		audioFiles[0].Metadata.Filename = "one.m4a"
		audioFiles[0].Metadata.Size = 100

		bAudioFiles, _ := json.Marshal(audioFiles)
		_, err = db.Exec(`INSERT INTO books (id, title, audioFiles) VALUES ('book-one', 'One File Book', ?)`, bAudioFiles)
		if err != nil {
			t.Fatalf("Failed to seed book: %v", err)
		}

		_, status, err := prepareMergeContext(db, "item-one-file")
		if err == nil || status != http.StatusBadRequest {
			t.Errorf("Expected 400 Bad Request for 1 file, got %d, err: %v", status, err)
		}
	})

	t.Run("UnsafePath", func(t *testing.T) {
		_, err = db.Exec(`INSERT INTO libraryItems (id, mediaId, mediaType, path, size) VALUES ('item-unsafe', 'book-unsafe', 'book', ?, 1000)`, tempDir)
		if err != nil {
			t.Fatalf("Failed to seed library item: %v", err)
		}

		// A path that is outside the library folder (tempDir)
		unsafeFile1 := "/etc/passwd"
		unsafeFile2 := "/etc/hosts"

		audioFiles := []MergeAudioFile{
			{Index: 0, Exclude: false, Duration: 1.0},
			{Index: 1, Exclude: false, Duration: 2.0},
		}
		audioFiles[0].Metadata.Path = unsafeFile1
		audioFiles[1].Metadata.Path = unsafeFile2

		bAudioFiles, _ := json.Marshal(audioFiles)
		_, err = db.Exec(`INSERT INTO books (id, title, audioFiles) VALUES ('book-unsafe', 'Unsafe Book', ?)`, bAudioFiles)
		if err != nil {
			t.Fatalf("Failed to seed book: %v", err)
		}

		_, status, err := prepareMergeContext(db, "item-unsafe")
		if err == nil || status != http.StatusForbidden {
			t.Errorf("Expected 403 Forbidden for unsafe paths, got %d, err: %v", status, err)
		}
	})

	t.Run("FileNotFoundOnDisk", func(t *testing.T) {
		_, err = db.Exec(`INSERT INTO libraryItems (id, mediaId, mediaType, path, size) VALUES ('item-missing-disk', 'book-missing-disk', 'book', ?, 1000)`, tempDir)
		if err != nil {
			t.Fatalf("Failed to seed library item: %v", err)
		}

		// Files that don't exist under tempDir
		missingFile1 := filepath.Join(tempDir, "missing1.m4a")
		missingFile2 := filepath.Join(tempDir, "missing2.m4a")

		audioFiles := []MergeAudioFile{
			{Index: 0, Exclude: false, Duration: 1.0},
			{Index: 1, Exclude: false, Duration: 2.0},
		}
		audioFiles[0].Metadata.Path = missingFile1
		audioFiles[1].Metadata.Path = missingFile2

		bAudioFiles, _ := json.Marshal(audioFiles)
		_, err = db.Exec(`INSERT INTO books (id, title, audioFiles) VALUES ('book-missing-disk', 'Missing Disk Book', ?)`, bAudioFiles)
		if err != nil {
			t.Fatalf("Failed to seed book: %v", err)
		}

		_, status, err := prepareMergeContext(db, "item-missing-disk")
		if err == nil || status != http.StatusBadRequest {
			t.Errorf("Expected 400 Bad Request for missing files on disk, got %d, err: %v", status, err)
		}
	})
}

func TestMergeAudioFiles_FFmpegFailure(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	tempDir := t.TempDir()

	// Seed library folder to allow safe paths
	_, err := db.Exec(`INSERT INTO libraryFolders (id, path, libraryId) VALUES ('folder-123', ?, 'lib-123')`, tempDir)
	if err != nil {
		t.Fatalf("Failed to seed libraryFolder: %v", err)
	}

	file1 := filepath.Join(tempDir, "bad1.wav")
	file2 := filepath.Join(tempDir, "bad2.wav")

	// Create files containing invalid audio data (just dummy text files)
	if err := os.WriteFile(file1, []byte("invalid data"), 0644); err != nil {
		t.Fatalf("Failed to create file1: %v", err)
	}
	if err := os.WriteFile(file2, []byte("invalid data"), 0644); err != nil {
		t.Fatalf("Failed to create file2: %v", err)
	}

	audioFiles := []MergeAudioFile{
		{Index: 0, Exclude: false, Duration: 1.0},
		{Index: 1, Exclude: false, Duration: 2.0},
	}
	audioFiles[0].Metadata.Path = file1
	audioFiles[0].Metadata.Filename = "bad1.wav"
	audioFiles[1].Metadata.Path = file2
	audioFiles[1].Metadata.Filename = "bad2.wav"

	bAudioFiles, _ := json.Marshal(audioFiles)
	_, err = db.Exec(`INSERT INTO books (id, title, audioFiles) VALUES ('book-bad', 'Bad Book', ?)`, bAudioFiles)
	if err != nil {
		t.Fatalf("Failed to seed book: %v", err)
	}

	_, err = db.Exec(`INSERT INTO libraryItems (id, mediaId, mediaType, path, size) VALUES ('item-bad', 'book-bad', 'book', ?, 1000)`, tempDir)
	if err != nil {
		t.Fatalf("Failed to seed library item: %v", err)
	}

	// Make the request as admin
	userSess := &core.UserSession{
		ID:       "user-admin",
		Username: "admin",
		Type:     "admin",
		IsActive: true,
	}

	req := httptest.NewRequest("POST", "/api/items/item-bad/merge", nil)
	req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, userSess))
	rr := httptest.NewRecorder()

	handler := handleMergeAudioFiles(db)
	handler.ServeHTTP(rr, req)

	// Since they are invalid audio files, and firstExt is ".wav", UseCopy will be false, and FFmpeg transcoding will fail.
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("Expected 500 Internal Server Error, got %d: %s", rr.Code, rr.Body.String())
	}
}
