package handlers

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"audiobookshelf/internal/core"
	_ "modernc.org/sqlite"
)

func setupUploadTestDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite", "file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS libraries (
			id TEXT PRIMARY KEY,
			name TEXT,
			mediaType TEXT,
			provider TEXT,
			settings TEXT,
			createdAt TEXT,
			updatedAt TEXT
		);
		CREATE TABLE IF NOT EXISTS libraryFolders (
			id TEXT PRIMARY KEY,
			path TEXT,
			libraryId TEXT,
			createdAt TEXT,
			updatedAt TEXT
		);
	`)
	if err != nil {
		t.Fatalf("failed to setup upload tables: %v", err)
	}

	return db
}

func createFormFileWithPath(w *multipart.Writer, fieldname, filename string) (io.Writer, error) {
	h := make(map[string][]string)
	h["Content-Disposition"] = []string{
		fmt.Sprintf(`form-data; name="%s"; filename="%s"`, fieldname, filename),
	}
	h["Content-Type"] = []string{"application/octet-stream"}
	return w.CreatePart(h)
}

func TestHandleUpload_Auth(t *testing.T) {
	db := setupUploadTestDB(t)
	defer db.Close()

	// Non-admin user session
	userSess := &core.UserSession{
		ID:       "user-1",
		Username: "normal_user",
		Type:     "user",
	}

	req := httptest.NewRequest(http.MethodPost, "/api/upload", nil)
	req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, userSess))

	rr := httptest.NewRecorder()
	handler := handleUpload(db)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected status Forbidden (403), got %d", rr.Code)
	}
}

func TestHandleUpload_SuccessAndTraversal(t *testing.T) {
	db := setupUploadTestDB(t)
	defer db.Close()

	// Create a temp folder for library destination
	tempDir, err := os.MkdirTemp("", "abs-upload-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Seed library and folders in DB
	_, err = db.Exec(`
		INSERT INTO libraries (id, name, mediaType) VALUES ('lib-1', 'Test Library', 'book');
		INSERT INTO libraryFolders (id, path, libraryId) VALUES ('folder-1', ?, 'lib-1');
	`, tempDir)
	if err != nil {
		t.Fatalf("failed to seed db: %v", err)
	}

	adminSess := &core.UserSession{
		ID:       "admin-1",
		Username: "admin_user",
		Type:     "admin",
	}

	// 1. Test Successful File Upload (single & nested files)
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Add fields
	_ = writer.WriteField("library", "lib-1")
	_ = writer.WriteField("folder", "folder-1")

	// Add a normal file
	part, _ := createFormFileWithPath(writer, "files", "test_book.epub")
	_, _ = part.Write([]byte("fake epub content"))

	// Add a nested file (recreating directory structure)
	partNested, _ := createFormFileWithPath(writer, "files", "Author Name/Book Title/audio.mp3")
	_, _ = partNested.Write([]byte("fake audio content"))

	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, adminSess))

	rr := httptest.NewRecorder()
	handler := handleUpload(db)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status OK (200), got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if resp["success"] != true {
		t.Errorf("expected success to be true, got %v", resp["success"])
	}

	// Check on-disk files
	normalFile := filepath.Join(tempDir, "test_book.epub")
	if _, err := os.Stat(normalFile); os.IsNotExist(err) {
		t.Error("expected test_book.epub to exist on disk")
	}

	nestedFile := filepath.Join(tempDir, "Author Name", "Book Title", "audio.mp3")
	if _, err := os.Stat(nestedFile); os.IsNotExist(err) {
		t.Error("expected nested file to exist on disk")
	}

	// 2. Test Path Traversal Protection
	bodyTraversal := &bytes.Buffer{}
	writerTraversal := multipart.NewWriter(bodyTraversal)
	_ = writerTraversal.WriteField("library", "lib-1")
	_ = writerTraversal.WriteField("folder", "folder-1")

	partTraversal, _ := createFormFileWithPath(writerTraversal, "files", "../../../etc/passwd")
	_, _ = partTraversal.Write([]byte("attempted exploit"))
	writerTraversal.Close()

	reqTraversal := httptest.NewRequest(http.MethodPost, "/api/upload", bodyTraversal)
	reqTraversal.Header.Set("Content-Type", writerTraversal.FormDataContentType())
	reqTraversal = reqTraversal.WithContext(context.WithValue(reqTraversal.Context(), core.UserContextKey, adminSess))

	rrTraversal := httptest.NewRecorder()
	handler.ServeHTTP(rrTraversal, reqTraversal)

	if rrTraversal.Code != http.StatusBadRequest {
		t.Errorf("expected Bad Request for path traversal attempt, got %d", rrTraversal.Code)
	}

	// 3. Test Partial Prefix Sibling Path Traversal Protection (Adversarial)
	bodyAdversarial := &bytes.Buffer{}
	writerAdversarial := multipart.NewWriter(bodyAdversarial)
	_ = writerAdversarial.WriteField("library", "lib-1")
	_ = writerAdversarial.WriteField("folder", "folder-1")

	baseDirName := filepath.Base(tempDir)
	siblingRelPath := "../" + baseDirName + "-private/exploit.txt"

	partAdversarial, _ := createFormFileWithPath(writerAdversarial, "files", siblingRelPath)
	_, _ = partAdversarial.Write([]byte("attempted sibling traversal exploit"))
	writerAdversarial.Close()

	reqAdversarial := httptest.NewRequest(http.MethodPost, "/api/upload", bodyAdversarial)
	reqAdversarial.Header.Set("Content-Type", writerAdversarial.FormDataContentType())
	reqAdversarial = reqAdversarial.WithContext(context.WithValue(reqAdversarial.Context(), core.UserContextKey, adminSess))

	rrAdversarial := httptest.NewRecorder()
	handler.ServeHTTP(rrAdversarial, reqAdversarial)

	if rrAdversarial.Code != http.StatusBadRequest {
		t.Errorf("expected Bad Request for partial-prefix traversal attempt, got %d. Body: %s", rrAdversarial.Code, rrAdversarial.Body.String())
	}

	// 4. Test Successful File Upload with Library ID in Path
	bodyPathLib := &bytes.Buffer{}
	writerPathLib := multipart.NewWriter(bodyPathLib)
	_ = writerPathLib.WriteField("folder", "folder-1")
	partPathLib, _ := createFormFileWithPath(writerPathLib, "files", "path_lib_test.epub")
	_, _ = partPathLib.Write([]byte("path library file content"))
	writerPathLib.Close()

	reqPathLib := httptest.NewRequest(http.MethodPost, "/api/libraries/lib-1/items", bodyPathLib)
	reqPathLib.Header.Set("Content-Type", writerPathLib.FormDataContentType())
	reqPathLib = reqPathLib.WithContext(context.WithValue(reqPathLib.Context(), core.UserContextKey, adminSess))

	rrPathLib := httptest.NewRecorder()
	handler.ServeHTTP(rrPathLib, reqPathLib)

	if rrPathLib.Code != http.StatusOK {
		t.Errorf("expected status OK (200) for path library upload, got %d: %s", rrPathLib.Code, rrPathLib.Body.String())
	}

	pathLibFile := filepath.Join(tempDir, "path_lib_test.epub")
	if _, err := os.Stat(pathLibFile); os.IsNotExist(err) {
		t.Error("expected path_lib_test.epub to exist on disk")
	}
}
