package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"audiobookshelf/internal/share"

	"golang.org/x/crypto/bcrypt"
)

func TestPublicShareHandlers(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	testTempDir := t.TempDir()
	MetadataPath = testTempDir

	// Initialize managers
	reinitManagers(db)

	// Create a dummy library item and book
	_, err := db.Exec(`
		INSERT INTO libraryItems (id, ino, libraryId, libraryFolderId, path, relPath, isFile, mtime, ctime, birthtime, createdAt, updatedAt, isMissing, isInvalid, mediaType, mediaId, size, title)
		VALUES ('item1', '123', 'lib1', 'folder1', '/books/item1.m4b', 'item1.m4b', 1, '123456', '123456', '123456', '123456', '123456', 0, 0, 'book', 'book1', 1000, 'Test Book')
	`)
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

	// Create share links
	ctx := context.Background()

	// 1. Valid non-password public share
	slug1 := "slug-nopass"
	s1 := &share.ShareLink{
		ID:             slug1,
		LibraryItemID:  "item1",
		CreatedBy:      "user1",
		ExpiresAt:      time.Now().Add(time.Hour),
		IsDownloadable: true,
	}
	err = globalShareManager.CreateShare(ctx, s1)
	if err != nil {
		t.Fatalf("Failed to create share 1: %v", err)
	}

	// 2. Password protected public share
	slug2 := "slug-pass"
	hash, err := bcrypt.GenerateFromPassword([]byte("secret"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("Failed to hash password: %v", err)
	}
	s2 := &share.ShareLink{
		ID:             slug2,
		LibraryItemID:  "item1",
		CreatedBy:      "user1",
		ExpiresAt:      time.Now().Add(time.Hour),
		IsDownloadable: false,
		PasswordHash:   string(hash),
	}
	err = globalShareManager.CreateShare(ctx, s2)
	if err != nil {
		t.Fatalf("Failed to create share 2: %v", err)
	}

	t.Run("Get public share info - no password", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/s/"+slug1, nil)
		w := httptest.NewRecorder()
		handleGetPublicShare(db).ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d. Body: %s", w.Code, w.Body.String())
		}

		var resp map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to unmarshal response: %v", err)
		}

		if resp["id"] != slug1 {
			t.Errorf("Expected slug %s, got %v", slug1, resp["id"])
		}
		if resp["hasPassword"] != false {
			t.Errorf("Expected hasPassword to be false")
		}
		if resp["isDownloadable"] != true {
			t.Errorf("Expected isDownloadable to be true")
		}
	})

	t.Run("Get public share info - password protected unauthorized", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/s/"+slug2, nil)
		w := httptest.NewRecorder()
		handleGetPublicShare(db).ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("Expected status 401, got %d", w.Code)
		}

		var resp map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to unmarshal response: %v", err)
		}

		if resp["hasPassword"] != true {
			t.Errorf("Expected hasPassword to be true")
		}
	})

	t.Run("Get public share info - password protected authorized", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/s/"+slug2+"?password=secret", nil)
		w := httptest.NewRecorder()
		handleGetPublicShare(db).ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d. Body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("Get public share download - forbidden if disabled", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/s/"+slug2+"?password=secret", nil)
		w := httptest.NewRecorder()
		handleGetPublicShareDownload(db).ServeHTTP(w, req)

		if w.Code != http.StatusForbidden {
			t.Errorf("Expected status 403, got %d", w.Code)
		}
	})

	t.Run("Get public share download - serves file if allowed", func(t *testing.T) {
		// Create a temporary file to mock download
		tmpFile, err := os.CreateTemp(testTempDir, "dummy-audio-*.m4b")
		if err != nil {
			t.Fatalf("Failed to create temp file: %v", err)
		}
		defer os.Remove(tmpFile.Name())
		_, _ = tmpFile.WriteString("dummy audio content")
		tmpFile.Close()

		// Update library item path to point to temp file
		_, err = db.Exec("UPDATE libraryItems SET path = ? WHERE id = 'item1'", tmpFile.Name())
		if err != nil {
			t.Fatalf("Failed to update path: %v", err)
		}

		req := httptest.NewRequest(http.MethodGet, "/api/s/"+slug1+"/download", nil)
		w := httptest.NewRecorder()
		handleGetPublicShareDownload(db).ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d. Body: %s", w.Code, w.Body.String())
		}

		if w.Body.String() != "dummy audio content" {
			t.Errorf("Expected body 'dummy audio content', got %q", w.Body.String())
		}
	})

	t.Run("List all share links", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/shares", nil)
		w := httptest.NewRecorder()
		handleGetShares(db).ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d. Body: %s", w.Code, w.Body.String())
		}

		var resp []map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to unmarshal response: %v", err)
		}

		if len(resp) != 2 {
			t.Errorf("Expected 2 share links, got %d", len(resp))
		}

		// Check one of them
		first := resp[0]
		if first["mediaTitle"] != "Test Book" {
			t.Errorf("Expected mediaTitle 'Test Book', got %v", first["mediaTitle"])
		}
	})

	// Create a dummy cover image file for tests
	tmpCover, err := os.CreateTemp(testTempDir, "test-cover-*.jpg")
	if err != nil {
		t.Fatalf("Failed to create temp cover file: %v", err)
	}
	defer os.Remove(tmpCover.Name())
	_, _ = tmpCover.WriteString("dummy cover image data")
	tmpCover.Close()

	_, err = db.Exec("UPDATE books SET coverPath = ? WHERE id = 'book1'", tmpCover.Name())
	if err != nil {
		t.Fatalf("Failed to update book coverPath: %v", err)
	}

	t.Run("Get public share cover - invalid parameters", func(t *testing.T) {
		// Test invalid width
		req := httptest.NewRequest(http.MethodGet, "/api/s/"+slug1+"/cover?width=123a", nil)
		w := httptest.NewRecorder()
		handleGetPublicShareCover(db, testTempDir).ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected status 400 for invalid width, got %d", w.Code)
		}

		// Test invalid height
		req = httptest.NewRequest(http.MethodGet, "/api/s/"+slug1+"/cover?height=abc", nil)
		w = httptest.NewRecorder()
		handleGetPublicShareCover(db, testTempDir).ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected status 400 for invalid height, got %d", w.Code)
		}

		// Test invalid format
		req = httptest.NewRequest(http.MethodGet, "/api/s/"+slug1+"/cover?format=gif", nil)
		w = httptest.NewRecorder()
		handleGetPublicShareCover(db, testTempDir).ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected status 400 for invalid format, got %d", w.Code)
		}

		// Test path traversal in width
		req = httptest.NewRequest(http.MethodGet, "/api/s/"+slug1+"/cover?width=../../", nil)
		w = httptest.NewRecorder()
		handleGetPublicShareCover(db, testTempDir).ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected status 400 for path traversal in width, got %d", w.Code)
		}
	})

	t.Run("Get public share cover - serves raw cover", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/s/"+slug1+"/cover?raw=1", nil)
		w := httptest.NewRecorder()
		handleGetPublicShareCover(db, testTempDir).ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d. Body: %s", w.Code, w.Body.String())
		}
		if w.Body.String() != "dummy cover image data" {
			t.Errorf("Expected body 'dummy cover image data', got %q", w.Body.String())
		}
	})
}
