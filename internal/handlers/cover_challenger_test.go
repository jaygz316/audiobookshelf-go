package handlers

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"audiobookshelf/internal/core"
)

func TestCoverChallenger_InvalidItemIDs(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	tempDir := t.TempDir()

	handler := serveCover(db, tempDir)

	tests := []struct {
		name     string
		path     string
		wantCode int
		wantBody string
	}{
		{
			name:     "Empty item ID (double slash)",
			path:     "/api/items//cover",
			wantCode: http.StatusBadRequest,
			wantBody: `{"error": "Invalid Item ID"}`,
		},
		{
			name:     "Path traversal in item ID (double dot)",
			path:     "/api/items/../cover",
			wantCode: http.StatusBadRequest,
			wantBody: `{"error": "Invalid Item ID"}`,
		},
		{
			name:     "Path traversal in item ID (backslash)",
			path:     `/api/items/item1\..\cover`,
			wantCode: http.StatusBadRequest,
			wantBody: `{"error": "Invalid Item ID"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.path, nil)
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != tt.wantCode {
				t.Errorf("Expected status %d, got %d", tt.wantCode, rr.Code)
			}
			if strings.TrimSpace(rr.Body.String()) != tt.wantBody {
				t.Errorf("Expected body %q, got %q", tt.wantBody, rr.Body.String())
			}
		})
	}
}

func TestCoverChallenger_MissingFilesAndIDs(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	tempDir := t.TempDir()

	handler := serveCover(db, tempDir)

	// 1. Non-existent item
	t.Run("Non-existent item ID", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/items/nonexistent/cover", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusNotFound {
			t.Errorf("Expected 404, got %d", rr.Code)
		}
	})

	// 2. Existing item with missing cover file on disk
	_, err := db.Exec(`INSERT INTO libraryItems (id, libraryId, mediaType, mediaId, path, isFile) VALUES ('item_missing', 'lib1', 'book', 'book_missing', '/fake/path', 0)`)
	if err != nil {
		t.Fatalf("Failed to insert library item: %v", err)
	}
	nonExistentFile := filepath.Join(tempDir, "does-not-exist.jpg")
	_, err = db.Exec(`INSERT INTO books (id, title, coverPath) VALUES ('book_missing', 'Missing Book', ?)`, nonExistentFile)
	if err != nil {
		t.Fatalf("Failed to insert book: %v", err)
	}

	t.Run("Missing cover file - raw", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/items/item_missing/cover?raw=1", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusNotFound {
			t.Errorf("Expected 404, got %d", rr.Code)
		}
	})

	t.Run("Missing cover file - resized", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/items/item_missing/cover?width=200", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusNotFound {
			t.Errorf("Expected 404, got %d", rr.Code)
		}
	})
}

func TestCoverChallenger_PathTraversalPrevention(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	tempDir := t.TempDir()
	metadataDir := filepath.Join(tempDir, "metadata")
	outsideDir := filepath.Join(tempDir, "outside")

	if err := os.MkdirAll(metadataDir, 0755); err != nil {
		t.Fatalf("Failed to create metadata dir: %v", err)
	}
	if err := os.MkdirAll(outsideDir, 0755); err != nil {
		t.Fatalf("Failed to create outside dir: %v", err)
	}

	sensitiveFile := filepath.Join(outsideDir, "sensitive.txt")
	if err := os.WriteFile(sensitiveFile, []byte("super secret data"), 0644); err != nil {
		t.Fatalf("Failed to write sensitive file: %v", err)
	}

	handler := serveCover(db, metadataDir)

	// Insert item pointing to outside file
	_, err := db.Exec(`INSERT INTO libraryItems (id, libraryId, mediaType, mediaId, path, isFile) VALUES ('item_traversal', 'lib1', 'book', 'book_traversal', '/fake/path', 0)`)
	if err != nil {
		t.Fatalf("Failed to insert library item: %v", err)
	}
	_, err = db.Exec(`INSERT INTO books (id, title, coverPath) VALUES ('book_traversal', 'Traversal Book', ?)`, sensitiveFile)
	if err != nil {
		t.Fatalf("Failed to insert book: %v", err)
	}

	// 1. Raw cover request -> should return 403 Forbidden
	t.Run("Raw cover traversal block", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/items/item_traversal/cover?raw=1", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusForbidden {
			t.Errorf("Expected 403 Forbidden, got %d", rr.Code)
		}
	})

	// 2. Resized cover request -> should return 403 Forbidden
	t.Run("Resized cover traversal block", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/items/item_traversal/cover?width=200", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusForbidden {
			t.Errorf("Expected 403 Forbidden, got %d", rr.Code)
		}
	})
}

func TestCoverChallenger_InvalidFormatsAndDimensions(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	tempDir := t.TempDir()

	handler := serveCover(db, tempDir)

	// Insert a valid item and book with a valid cover file
	_, err := db.Exec(`INSERT INTO libraryItems (id, libraryId, mediaType, mediaId, path, isFile) VALUES ('item_ok', 'lib1', 'book', 'book_ok', '/fake/path', 0)`)
	if err != nil {
		t.Fatalf("Failed to insert library item: %v", err)
	}
	coverPath := filepath.Join(tempDir, "cover.jpg")
	if err := os.WriteFile(coverPath, []byte("fake image content"), 0644); err != nil {
		t.Fatalf("Failed to write cover: %v", err)
	}
	_, err = db.Exec(`INSERT INTO books (id, title, coverPath) VALUES ('book_ok', 'Ok Book', ?)`, coverPath)
	if err != nil {
		t.Fatalf("Failed to insert book: %v", err)
	}

	// 1. Invalid formats
	invalidFormats := []string{"gif", "bmp", "tiff", "svg", "xyz"}
	for _, f := range invalidFormats {
		t.Run("Format "+f, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/items/item_ok/cover?format="+f, nil)
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)
			if rr.Code != http.StatusBadRequest {
				t.Errorf("Expected 400 Bad Request for format %q, got %d", f, rr.Code)
			}
		})
	}

	// 2. Non-numeric dimensions
	invalidDimensions := []struct {
		width  string
		height string
	}{
		{width: "abc", height: ""},
		{width: "", height: "xyz"},
		{width: "100a", height: ""},
		{width: "", height: "200-"},
	}
	for _, d := range invalidDimensions {
		t.Run(fmt.Sprintf("Dimensions w=%s h=%s", d.width, d.height), func(t *testing.T) {
			req := httptest.NewRequest("GET", fmt.Sprintf("/api/items/item_ok/cover?width=%s&height=%s", d.width, d.height), nil)
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)
			if rr.Code != http.StatusBadRequest {
				t.Errorf("Expected 400 Bad Request for width=%q height=%q, got %d", d.width, d.height, rr.Code)
			}
		})
	}
}

func TestCoverChallenger_BoundaryDimensionsGracefulFallback(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	tempDir := t.TempDir()

	handler := serveCover(db, tempDir)

	// Insert a valid item and book with a valid cover file
	_, err := db.Exec(`INSERT INTO libraryItems (id, libraryId, mediaType, mediaId, path, isFile) VALUES ('item_ok', 'lib1', 'book', 'book_ok', '/fake/path', 0)`)
	if err != nil {
		t.Fatalf("Failed to insert library item: %v", err)
	}
	coverPath := filepath.Join(tempDir, "cover.jpg")
	dummyImageBytes := []byte("fake image content")
	if err := os.WriteFile(coverPath, dummyImageBytes, 0644); err != nil {
		t.Fatalf("Failed to write cover: %v", err)
	}
	_, err = db.Exec(`INSERT INTO books (id, title, coverPath) VALUES ('book_ok', 'Ok Book', ?)`, coverPath)
	if err != nil {
		t.Fatalf("Failed to insert book: %v", err)
	}

	boundaryDims := []struct {
		width  string
		height string
	}{
		{width: "0", height: ""},
		{width: "999999999999999999", height: ""},
	}

	for _, d := range boundaryDims {
		t.Run(fmt.Sprintf("Boundary w=%s h=%s", d.width, d.height), func(t *testing.T) {
			req := httptest.NewRequest("GET", fmt.Sprintf("/api/items/item_ok/cover?width=%s&height=%s", d.width, d.height), nil)
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)
			if rr.Code != http.StatusOK {
				t.Errorf("Expected 200 OK (graceful fallback to raw), got %d", rr.Code)
			}
			if !bytes.Equal(rr.Body.Bytes(), dummyImageBytes) {
				t.Errorf("Expected response body to match raw image bytes")
			}
		})
	}
}

func TestCoverChallenger_UpdateCoverFromURL_SSRF(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	tempDir := t.TempDir()
	cfg := &core.Config{
		RouterBasePath: "",
		ConfigPath:     tempDir,
		MetadataPath:   tempDir,
	}

	// Insert item
	_, err := db.Exec(`INSERT INTO libraryItems (id, libraryId, mediaType, mediaId, path, isFile) VALUES ('item_ok', 'lib1', 'book', 'book_ok', '/fake/path', 0)`)
	if err != nil {
		t.Fatalf("Failed to insert library item: %v", err)
	}
	_, err = db.Exec(`INSERT INTO books (id, title, coverPath) VALUES ('book_ok', 'Ok Book', '')`)
	if err != nil {
		t.Fatalf("Failed to insert book: %v", err)
	}

	userSess := &core.UserSession{
		ID:       "admin1",
		Username: "adminuser",
		Type:     "admin",
	}

	handler := handleUpdateCoverFromURL(db, cfg, "item_ok")

	t.Run("Blocked local IP (default safeurl)", func(t *testing.T) {
		reqBody := `{"coverUrl": "http://127.0.0.1:54321/cover.jpg"}`
		req := httptest.NewRequest("POST", "/api/items/item_ok/cover-from-url", strings.NewReader(reqBody))
		ctx := context.WithValue(req.Context(), core.UserContextKey, userSess)
		req = req.WithContext(ctx)

		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		// It should fail because safeurl blocks loopback addresses by default.
		if rr.Code != http.StatusInternalServerError {
			t.Errorf("Expected 500 Internal Server Error, got %d", rr.Code)
		}
	})
}
