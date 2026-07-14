package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"audiobookshelf/internal/core"
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

	// Insert library folder to satisfy path safety check
	_, err = db.Exec(`INSERT INTO libraryFolders (id, path, libraryId) VALUES ('folder1', ?, 'lib1')`, libraryPath)
	if err != nil {
		t.Fatalf("Failed to insert library folder: %v", err)
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

func TestWaveform_PathTraversalAdversarial(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	userSess := &core.UserSession{
		ID:       "user1",
		Username: "testuser",
		Type:     "admin",
		IsActive: true,
	}

	tempDir := t.TempDir()
	cfg := &core.Config{
		MetadataPath: tempDir,
	}

	// 1. Check itemID containing .. is blocked with 400 Bad Request
	req := httptest.NewRequest("GET", "/api/items/../waveform", nil)
	req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, userSess))
	rr := httptest.NewRecorder()

	handler := handleGetWaveform(db, cfg, "../items")
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400 Bad Request for traverse itemID, got %d", rr.Code)
	}

	// 2. Check unsafe audio file path traversal is blocked with 403 Forbidden
	audioFilesJSON := `[
		{
			"exclude": false,
			"duration": 120.0,
			"metadata": {
				"path": "/etc/passwd"
			}
		}
	]`
	_, err := db.Exec(`INSERT INTO books (id, title, audioFiles) VALUES ('book-unsafe', 'Unsafe Book', ?)`, audioFilesJSON)
	if err != nil {
		t.Fatalf("Failed to seed book: %v", err)
	}

	_, err = db.Exec(`INSERT INTO libraryItems (id, mediaId, mediaType) VALUES ('item-unsafe', 'book-unsafe', 'book')`)
	if err != nil {
		t.Fatalf("Failed to seed library item: %v", err)
	}

	req2 := httptest.NewRequest("GET", "/api/items/item-unsafe/waveform", nil)
	req2 = req2.WithContext(context.WithValue(req2.Context(), core.UserContextKey, userSess))
	rr2 := httptest.NewRecorder()

	handler2 := handleGetWaveform(db, cfg, "item-unsafe")
	handler2.ServeHTTP(rr2, req2)

	if rr2.Code != http.StatusForbidden {
		t.Errorf("Expected status 403 Forbidden for unsafe audio file path, got %d. Body: %s", rr2.Code, rr2.Body.String())
	}
}

func TestAuthorImage_PathTraversalAdversarial(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	userSess := &core.UserSession{
		ID:       "user1",
		Username: "testuser",
		Type:     "admin",
		IsActive: true,
	}

	tempDir := t.TempDir()
	metadataPath := filepath.Join(tempDir, "metadata")
	privatePath := filepath.Join(tempDir, "private")
	_ = os.MkdirAll(metadataPath, 0755)
	_ = os.MkdirAll(privatePath, 0755)

	cfg := &core.Config{
		MetadataPath: metadataPath,
	}

	// Write a secret file outside metadata path that we want to protect
	secretFile := filepath.Join(privatePath, "secret.txt")
	err := os.WriteFile(secretFile, []byte("secret"), 0644)
	if err != nil {
		t.Fatalf("failed to write secret file: %v", err)
	}

	// Create author whose imagePath is absolute path pointing to the secret file (outside metadataPath)
	_, err = db.Exec(`INSERT INTO authors (id, name, imagePath) VALUES ('author-unsafe', 'Unsafe Author', ?)`, secretFile)
	if err != nil {
		t.Fatalf("failed to seed author: %v", err)
	}

	req := httptest.NewRequest("DELETE", "/api/authors/author-unsafe/image", nil)
	req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, userSess))
	rr := httptest.NewRecorder()

	handler := handleDeleteAuthorImage(db, cfg, "author-unsafe")
	handler.ServeHTTP(rr, req)

	// Since fullPath is secretFile which is outside MetadataPath, IsSafeFilePath should fail and prevent deletion.
	if _, err := os.Stat(secretFile); os.IsNotExist(err) {
		t.Error("VULNERABILITY: Secret file outside metadata directory was deleted!")
	}
}

func TestUpdateAuthor_PathTraversalAdversarial(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	userSess := &core.UserSession{
		ID:       "user1",
		Username: "testuser",
		Type:     "admin",
		IsActive: true,
	}

	tempDir := t.TempDir()
	metadataPath := filepath.Join(tempDir, "metadata")
	privatePath := filepath.Join(tempDir, "private")
	_ = os.MkdirAll(metadataPath, 0755)
	_ = os.MkdirAll(privatePath, 0755)

	MetadataPath = metadataPath // set package-scoped variable

	// Write a secret file outside metadata path that we want to protect
	targetDir := filepath.Join(privatePath, "secret_dir")
	_ = os.MkdirAll(targetDir, 0755)

	// Seed database:
	// 1. Author
	_, err := db.Exec(`INSERT INTO authors (id, name, lastFirst) VALUES ('author1', 'Author One', 'One, Author')`)
	if err != nil {
		t.Fatalf("failed to seed author: %v", err)
	}
	// 2. Book
	_, err = db.Exec(`INSERT INTO books (id, title) VALUES ('book1', 'Test Book')`)
	if err != nil {
		t.Fatalf("failed to seed book: %v", err)
	}
	// 3. bookAuthors link
	_, err = db.Exec(`INSERT INTO bookAuthors (bookId, authorId) VALUES ('book1', 'author1')`)
	if err != nil {
		t.Fatalf("failed to seed bookAuthors: %v", err)
	}
	// 4. libraryItems with a traversal ID: itemID = "../../private/secret_dir"
	_, err = db.Exec(`
		INSERT INTO libraryItems (id, libraryId, mediaId, mediaType, path)
		VALUES ('../../private/secret_dir', 'lib1', 'book1', 'book', '/fake/path')
	`)
	if err != nil {
		t.Fatalf("failed to seed libraryItems: %v", err)
	}

	traversalMetaFile := filepath.Join(targetDir, "metadata.json")
	err = os.WriteFile(traversalMetaFile, []byte(`{"title":"Original"}`), 0644)
	if err != nil {
		t.Fatalf("failed to write mock metadata: %v", err)
	}

	// Make request to handleUpdateAuthor
	reqBody := `{"name":"Author Updated","lastFirst":"Updated, Author","asin":"","description":""}`
	req := httptest.NewRequest("PATCH", "/api/authors/author1", strings.NewReader(reqBody))
	req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, userSess))
	rr := httptest.NewRecorder()

	handler := handleUpdateAuthor(db, "author1")
	handler.ServeHTTP(rr, req)

	// Verify that the file at traversalMetaFile was NOT modified/updated!
	mBytes, err := os.ReadFile(traversalMetaFile)
	if err != nil {
		t.Fatalf("failed to read metadata file: %v", err)
	}
	var meta map[string]interface{}
	if err := json.Unmarshal(mBytes, &meta); err != nil {
		t.Fatalf("failed to unmarshal metadata: %v", err)
	}
	if meta["authors"] != nil {
		t.Error("VULNERABILITY: Metadata file outside metadata path was successfully updated/modified!")
	}
}

