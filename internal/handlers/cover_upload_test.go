package handlers

import (
	"bytes"
	"context"
	"database/sql"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"audiobookshelf/internal/core"
)

func TestCoverUpload(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	tempDir := t.TempDir()
	cfg := &core.Config{
		RouterBasePath: "",
		ConfigPath:     tempDir,
		MetadataPath:   tempDir,
	}

	// Insert a test user (admin)
	_, err := db.Exec(`INSERT INTO users (id, username, type, isActive, permissions, extraData) VALUES ('admin1', 'adminuser', 'admin', 1, '{}', '{}')`)
	if err != nil {
		t.Fatalf("Failed to insert admin: %v", err)
	}

	// Insert a test user (non-admin)
	_, err = db.Exec(`INSERT INTO users (id, username, type, isActive, permissions, extraData) VALUES ('user1', 'regularuser', 'user', 1, '{}', '{}')`)
	if err != nil {
		t.Fatalf("Failed to insert user: %v", err)
	}

	// Insert a test library item and book
	_, err = db.Exec(`INSERT INTO libraryItems (id, libraryId, mediaType, mediaId, path, isFile) VALUES ('item1', 'lib1', 'book', 'book1', '/fake/path', 0)`)
	if err != nil {
		t.Fatalf("Failed to insert library item: %v", err)
	}
	_, err = db.Exec(`INSERT INTO books (id, title, coverPath) VALUES ('book1', 'Test Book', '')`)
	if err != nil {
		t.Fatalf("Failed to insert book: %v", err)
	}

	// Helper to create multipart request body
	createMultipartBody := func(t *testing.T, fieldName, fileName string, fileContent []byte) (string, *bytes.Buffer) {
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		part, err := writer.CreateFormFile(fieldName, fileName)
		if err != nil {
			t.Fatalf("Failed to create form file: %v", err)
		}
		if _, err := part.Write(fileContent); err != nil {
			t.Fatalf("Failed to write multipart file content: %v", err)
		}
		if err := writer.Close(); err != nil {
			t.Fatalf("Failed to close multipart writer: %v", err)
		}
		return writer.FormDataContentType(), body
	}

	t.Run("Unauthorized request blocked", func(t *testing.T) {
		contentType, body := createMultipartBody(t, "cover", "test.jpg", []byte("fake image data"))
		req := httptest.NewRequest("POST", "/api/items/item1/cover", body)
		req.Header.Set("Content-Type", contentType)

		rr := httptest.NewRecorder()
		handler := handleUploadCover(db, cfg, "item1")
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Errorf("Expected 401 Unauthorized, got %d", rr.Code)
		}
	})

	t.Run("Regular user request blocked", func(t *testing.T) {
		contentType, body := createMultipartBody(t, "cover", "test.jpg", []byte("fake image data"))
		req := httptest.NewRequest("POST", "/api/items/item1/cover", body)
		req.Header.Set("Content-Type", contentType)

		// Set context user session
		userSess := &core.UserSession{
			ID:       "user1",
			Username: "regularuser",
			Type:     "user",
		}
		ctx := context.WithValue(req.Context(), core.UserContextKey, userSess)
		req = req.WithContext(ctx)

		rr := httptest.NewRecorder()
		handler := handleUploadCover(db, cfg, "item1")
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusForbidden {
			t.Errorf("Expected 403 Forbidden, got %d", rr.Code)
		}
	})

	t.Run("Admin upload success", func(t *testing.T) {
		imageData := []byte("fake image jpeg data")
		contentType, body := createMultipartBody(t, "cover", "uploaded_cover.jpg", imageData)
		req := httptest.NewRequest("POST", "/api/items/item1/cover", body)
		req.Header.Set("Content-Type", contentType)

		userSess := &core.UserSession{
			ID:       "admin1",
			Username: "adminuser",
			Type:     "admin",
		}
		ctx := context.WithValue(req.Context(), core.UserContextKey, userSess)
		req = req.WithContext(ctx)

		rr := httptest.NewRecorder()
		handler := handleUploadCover(db, cfg, "item1")
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("Expected 200 OK, got %d. Body: %s", rr.Code, rr.Body.String())
		}

		// Verify database was updated
		var coverPath sql.NullString
		err := db.QueryRow("SELECT coverPath FROM books WHERE id = 'book1'").Scan(&coverPath)
		if err != nil {
			t.Fatalf("Failed to query cover path: %v", err)
		}

		if !coverPath.Valid || coverPath.String == "" {
			t.Fatalf("Expected cover path to be valid and not empty")
		}

		// Verify file exists
		if _, err := os.Stat(coverPath.String); err != nil {
			t.Fatalf("Expected cover file to exist at %s, but got error: %v", coverPath.String, err)
		}

		// Verify contents
		content, err := os.ReadFile(coverPath.String)
		if err != nil {
			t.Fatalf("Failed to read cover file: %v", err)
		}
		if string(content) != string(imageData) {
			t.Errorf("Expected file contents to match %q, got %q", imageData, content)
		}
	})
}
