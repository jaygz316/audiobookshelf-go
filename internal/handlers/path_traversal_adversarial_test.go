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
)

func TestPublicShareStream_PathTraversalAdversarial(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	reinitManagers(db)

	tempDir := t.TempDir()
	libraryPath := filepath.Join(tempDir, "library")
	privatePath := filepath.Join(tempDir, "library-private")

	err := os.MkdirAll(libraryPath, 0755)
	if err != nil {
		t.Fatalf("failed to create library directory: %v", err)
	}
	err = os.MkdirAll(privatePath, 0755)
	if err != nil {
		t.Fatalf("failed to create library-private directory: %v", err)
	}

	secretFile := filepath.Join(privatePath, "secret.txt")
	err = os.WriteFile(secretFile, []byte("sensitive info"), 0644)
	if err != nil {
		t.Fatalf("failed to write secret file: %v", err)
	}

	// Insert libraryItem with path pointing to the clean library path (directory, so isFile = 0)
	_, err = db.Exec(`
		INSERT INTO libraryItems (id, ino, libraryId, libraryFolderId, path, relPath, isFile, mtime, ctime, birthtime, createdAt, updatedAt, isMissing, isInvalid, mediaType, mediaId, size, title)
		VALUES ('item-stream-test', '123', 'lib1', 'folder1', ?, 'stream-test', 0, '123456', '123456', '123456', '123456', '123456', 0, 0, 'book', 'book1', 1000, 'Test Book')
	`, libraryPath)
	if err != nil {
		t.Fatalf("Failed to insert library item: %v", err)
	}

	_, err = db.Exec(`
		INSERT INTO books (id, title, duration, narrators, audioFiles, genres, tags)
		VALUES ('book1', 'Test Book', 120.0, '[]', '[]', '[]', '[]')
	`)
	if err != nil {
		t.Fatalf("Failed to insert book: %v", err)
	}

	// Register a public share link
	slug := "stream-slug"
	s := &share.ShareLink{
		ID:             slug,
		LibraryItemID:  "item-stream-test",
		CreatedBy:      "user1",
		ExpiresAt:      time.Now().Add(time.Hour),
		IsDownloadable: true,
	}
	err = globalShareManager.CreateShare(context.Background(), s)
	if err != nil {
		t.Fatalf("failed to create public share: %v", err)
	}

	// 1. Verify standard nested file access works or fails gracefully
	reqOk := httptest.NewRequest("GET", "/api/s/"+slug+"/stream?track=nonexistent.mp3", nil)
	wOk := httptest.NewRecorder()
	handleGetPublicShareStream(db).ServeHTTP(wOk, reqOk)
	// Should be 404 because nonexistent.mp3 doesn't exist, but NOT forbidden (403)
	if wOk.Code != http.StatusNotFound {
		t.Errorf("Expected 404 NotFound for non-existent track, got %d", wOk.Code)
	}

	// 2. Traversal attempt targeting sibling prefix (Vulnerability test)
	// targetPath: libraryPath + "../library-private/secret.txt" -> privatePath/secret.txt
	reqBad := httptest.NewRequest("GET", "/api/s/"+slug+"/stream?track=../library-private/secret.txt", nil)
	wBad := httptest.NewRecorder()
	handleGetPublicShareStream(db).ServeHTTP(wBad, reqBad)

	if wBad.Code != http.StatusForbidden {
		t.Errorf("Expected 403 Forbidden for sibling prefix traversal, got %d. Body: %s", wBad.Code, wBad.Body.String())
	}
	if wBad.Body.String() == "sensitive info" {
		t.Error("VULNERABILITY DETECTED: Sibling prefix traversal successfully bypassed check and read private file!")
	}
}
