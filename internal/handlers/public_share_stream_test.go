package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"audiobookshelf/internal/share"

	"golang.org/x/crypto/bcrypt"
)

func TestPublicShareStream(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	testTempDir := t.TempDir()
	MetadataPath = testTempDir

	// Initialize managers
	reinitManagers(db)

	// Create dummy library items: one single-file item, one directory item
	// 1. Single-file library item:
	tmpFile, err := os.CreateTemp(testTempDir, "dummy-audio-*.m4b")
	if err != nil {
		t.Fatalf("Failed to create temp audio file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	_, _ = tmpFile.WriteString("dummy audio content")
	tmpFile.Close()

	_, err = db.Exec(`
		INSERT INTO libraryItems (id, ino, libraryId, libraryFolderId, path, relPath, isFile, mtime, ctime, birthtime, createdAt, updatedAt, isMissing, isInvalid, mediaType, mediaId, size, title)
		VALUES ('item-file', '123', 'lib1', 'folder1', ?, ?, 1, '123456', '123456', '123456', '123456', '123456', 0, 0, 'book', 'book-file', 1000, 'Test Book File')
	`, tmpFile.Name(), filepath.Base(tmpFile.Name()))
	if err != nil {
		t.Fatalf("Failed to insert library item: %v", err)
	}

	_, err = db.Exec(`
		INSERT INTO books (id, title, duration, narrators, audioFiles, genres, tags)
		VALUES ('book-file', 'Test Book File', 120.0, '[]', '[]', '[]', '[]')
	`)
	if err != nil {
		t.Fatalf("Failed to insert book: %v", err)
	}

	// 2. Directory library item:
	tmpDir := filepath.Join(testTempDir, "dummy-album")
	if err := os.Mkdir(tmpDir, 0755); err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	track1Path := filepath.Join(tmpDir, "track1.mp3")
	if err := os.WriteFile(track1Path, []byte("track1 content"), 0644); err != nil {
		t.Fatalf("Failed to create track1: %v", err)
	}
	track2Path := filepath.Join(tmpDir, "track2.mp3")
	if err := os.WriteFile(track2Path, []byte("track2 content"), 0644); err != nil {
		t.Fatalf("Failed to create track2: %v", err)
	}
	nonAudioPath := filepath.Join(tmpDir, "readme.txt")
	if err := os.WriteFile(nonAudioPath, []byte("readme content"), 0644); err != nil {
		t.Fatalf("Failed to create readme: %v", err)
	}

	_, err = db.Exec(`
		INSERT INTO libraryItems (id, ino, libraryId, libraryFolderId, path, relPath, isFile, mtime, ctime, birthtime, createdAt, updatedAt, isMissing, isInvalid, mediaType, mediaId, size, title)
		VALUES ('item-dir', '124', 'lib1', 'folder1', ?, 'dummy-album', 0, '123456', '123456', '123456', '123456', '123456', 0, 0, 'book', 'book-dir', 1000, 'Test Book Dir')
	`, tmpDir)
	if err != nil {
		t.Fatalf("Failed to insert library item: %v", err)
	}

	_, err = db.Exec(`
		INSERT INTO books (id, title, duration, narrators, audioFiles, genres, tags)
		VALUES ('book-dir', 'Test Book Dir', 120.0, '[]', '[]', '[]', '[]')
	`)
	if err != nil {
		t.Fatalf("Failed to insert book: %v", err)
	}

	// Create share links
	ctx := context.Background()

	// Share 1: Single file, no password
	slugFile := "slug-file"
	s1 := &share.ShareLink{
		ID:            slugFile,
		LibraryItemID: "item-file",
		CreatedBy:     "user1",
		ExpiresAt:     time.Now().Add(time.Hour),
	}
	if err := globalShareManager.CreateShare(ctx, s1); err != nil {
		t.Fatalf("Failed to create share: %v", err)
	}

	// Share 2: Directory, no password
	slugDir := "slug-dir"
	s2 := &share.ShareLink{
		ID:            slugDir,
		LibraryItemID: "item-dir",
		CreatedBy:     "user1",
		ExpiresAt:     time.Now().Add(time.Hour),
	}
	if err := globalShareManager.CreateShare(ctx, s2); err != nil {
		t.Fatalf("Failed to create share: %v", err)
	}

	// Share 3: Password protected single file
	slugPass := "slug-pass"
	hash, err := bcrypt.GenerateFromPassword([]byte("secret"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("Failed to hash password: %v", err)
	}
	s3 := &share.ShareLink{
		ID:            slugPass,
		LibraryItemID: "item-file",
		CreatedBy:     "user1",
		ExpiresAt:     time.Now().Add(time.Hour),
		PasswordHash:  string(hash),
	}
	if err := globalShareManager.CreateShare(ctx, s3); err != nil {
		t.Fatalf("Failed to create share: %v", err)
	}

	t.Run("Stream single file successfully", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/s/"+slugFile+"/stream", nil)
		w := httptest.NewRecorder()
		handleGetPublicShareStream(db).ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d. Body: %s", w.Code, w.Body.String())
		}
		if w.Body.String() != "dummy audio content" {
			t.Errorf("Expected 'dummy audio content', got %q", w.Body.String())
		}
	})

	t.Run("Stream directory - auto select first audio file", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/s/"+slugDir+"/stream", nil)
		w := httptest.NewRecorder()
		handleGetPublicShareStream(db).ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d. Body: %s", w.Code, w.Body.String())
		}
		if w.Body.String() != "track1 content" {
			t.Errorf("Expected 'track1 content', got %q", w.Body.String())
		}
	})

	t.Run("Stream directory - specify track", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/s/"+slugDir+"/stream?track=track2.mp3", nil)
		w := httptest.NewRecorder()
		handleGetPublicShareStream(db).ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d. Body: %s", w.Code, w.Body.String())
		}
		if w.Body.String() != "track2 content" {
			t.Errorf("Expected 'track2 content', got %q", w.Body.String())
		}
	})

	t.Run("Stream directory - path traversal blocked", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/s/"+slugDir+"/stream?track=../other", nil)
		w := httptest.NewRecorder()
		handleGetPublicShareStream(db).ServeHTTP(w, req)

		if w.Code != http.StatusForbidden {
			t.Errorf("Expected status 403 (Forbidden) for traversal, got %d. Body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("Stream password protected - unauthorized", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/s/"+slugPass+"/stream", nil)
		w := httptest.NewRecorder()
		handleGetPublicShareStream(db).ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("Expected status 401, got %d", w.Code)
		}
	})

	t.Run("Stream password protected - authorized with query param", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/s/"+slugPass+"/stream?password=secret", nil)
		w := httptest.NewRecorder()
		handleGetPublicShareStream(db).ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", w.Code)
		}
	})

	t.Run("Stream password protected - authorized with header", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/s/"+slugPass+"/stream", nil)
		req.Header.Set("X-Share-Password", "secret")
		w := httptest.NewRecorder()
		handleGetPublicShareStream(db).ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", w.Code)
		}
	})

	t.Run("Stream non-existent slug", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/s/nonexistent/stream", nil)
		w := httptest.NewRecorder()
		handleGetPublicShareStream(db).ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("Expected status 404, got %d", w.Code)
		}
	})
}
