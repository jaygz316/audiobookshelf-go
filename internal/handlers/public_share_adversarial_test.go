package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"audiobookshelf/internal/core"
	"audiobookshelf/internal/share"

	"golang.org/x/crypto/bcrypt"
)

func TestPublicShareAdversarial(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	testTempDir := t.TempDir()
	MetadataPath = testTempDir

	// Initialize managers
	reinitManagers(db)

	// Create dummy library item, folder, and book
	_, err := db.Exec(`INSERT INTO libraryFolders (id, path, libraryId) VALUES ('folder1', ?, 'lib1')`, testTempDir)
	if err != nil {
		t.Fatalf("Failed to insert library folder: %v", err)
	}

	_, err = db.Exec(`
		INSERT INTO libraryItems (id, ino, libraryId, libraryFolderId, path, relPath, isFile, mtime, ctime, birthtime, createdAt, updatedAt, isMissing, isInvalid, mediaType, mediaId, size, title)
		VALUES ('item-adv', '123', 'lib1', 'folder1', '/books/item-adv.m4b', 'item-adv.m4b', 1, '123456', '123456', '123456', '123456', '123456', 0, 0, 'book', 'book-adv', 1000, 'Adv Book')
	`)
	if err != nil {
		t.Fatalf("Failed to insert library item: %v", err)
	}

	_, err = db.Exec(`
		INSERT INTO books (id, title, duration, narrators, audioFiles, genres, tags)
		VALUES ('book-adv', 'Adv Book', 120.0, '[]', '[]', '[]', '[]')
	`)
	if err != nil {
		t.Fatalf("Failed to insert book: %v", err)
	}

	// Create dummy media file to stream/download
	tmpFile, err := os.CreateTemp(testTempDir, "dummy-*.m4b")
	if err != nil {
		t.Fatalf("Failed to create dummy audio file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	_, _ = tmpFile.WriteString("adversarial dummy content")
	tmpFile.Close()

	// Update library item path to point to temp file
	_, err = db.Exec("UPDATE libraryItems SET path = ? WHERE id = 'item-adv'", tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to update path: %v", err)
	}

	// Create dummy cover file
	tmpCover, err := os.CreateTemp(testTempDir, "cover-*.jpg")
	if err != nil {
		t.Fatalf("Failed to create dummy cover: %v", err)
	}
	defer os.Remove(tmpCover.Name())
	_, _ = tmpCover.WriteString("adversarial cover data")
	tmpCover.Close()

	_, err = db.Exec("UPDATE books SET coverPath = ? WHERE id = 'book-adv'", tmpCover.Name())
	if err != nil {
		t.Fatalf("Failed to update coverPath: %v", err)
	}

	ctx := context.Background()

	t.Run("Max downloads constraint and increments", func(t *testing.T) {
		slug := "slug-max-downloads"
		s := &share.ShareLink{
			ID:             slug,
			LibraryItemID:  "item-adv",
			CreatedBy:      "user1",
			ExpiresAt:      time.Now().Add(time.Hour),
			IsDownloadable: true,
			MaxDownloads:   2,
			DownloadsCount: 0,
		}
		err = globalShareManager.CreateShare(ctx, s)
		if err != nil {
			t.Fatalf("Failed to create share: %v", err)
		}

		// Download 1
		req := httptest.NewRequest(http.MethodGet, "/api/s/"+slug+"/download", nil)
		w := httptest.NewRecorder()
		handleGetPublicShareDownload(db).ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("Expected 200, got %d. Body: %s", w.Code, w.Body.String())
		}

		// Check downloadsCount is incremented in DB
		sCheck, err := globalShareManager.GetShare(ctx, slug)
		if err != nil || sCheck == nil {
			t.Fatalf("Failed to fetch share: %v", err)
		}
		if sCheck.DownloadsCount != 1 {
			t.Errorf("Expected downloads count to be 1, got %d", sCheck.DownloadsCount)
		}

		// Download 2
		w = httptest.NewRecorder()
		handleGetPublicShareDownload(db).ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("Expected 200, got %d. Body: %s", w.Code, w.Body.String())
		}

		// Download 3 - should fail
		w = httptest.NewRecorder()
		handleGetPublicShareDownload(db).ServeHTTP(w, req)
		if w.Code != http.StatusForbidden {
			t.Errorf("Expected 403 Forbidden, got %d. Body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("Expired share link gets deleted on fetch", func(t *testing.T) {
		slug := "slug-expired"
		s := &share.ShareLink{
			ID:             slug,
			LibraryItemID:  "item-adv",
			CreatedBy:      "user1",
			ExpiresAt:      time.Now().Add(-time.Hour), // expired 1 hour ago
			IsDownloadable: true,
		}
		err = globalShareManager.CreateShare(ctx, s)
		if err != nil {
			t.Fatalf("Failed to create share: %v", err)
		}

		// Try to fetch it via API
		req := httptest.NewRequest(http.MethodGet, "/api/s/"+slug, nil)
		w := httptest.NewRecorder()
		handleGetPublicShare(db).ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Errorf("Expected 404 for expired share, got %d. Body: %s", w.Code, w.Body.String())
		}

		// Verify it was deleted from DB
		var count int
		err = db.QueryRow("SELECT COUNT(*) FROM shares WHERE id = ?", slug).Scan(&count)
		if err != nil {
			t.Fatalf("Failed to check db count: %v", err)
		}
		if count != 0 {
			t.Errorf("Expected expired share link to be deleted, but it is still in DB")
		}
	})

	t.Run("Password protection checks on cover, download, and stream", func(t *testing.T) {
		slug := "slug-pass-protected"
		hash, err := bcrypt.GenerateFromPassword([]byte("secret"), bcrypt.DefaultCost)
		if err != nil {
			t.Fatalf("Failed to hash password: %v", err)
		}
		s := &share.ShareLink{
			ID:             slug,
			LibraryItemID:  "item-adv",
			CreatedBy:      "user1",
			ExpiresAt:      time.Now().Add(time.Hour),
			IsDownloadable: true,
			PasswordHash:   string(hash),
		}
		err = globalShareManager.CreateShare(ctx, s)
		if err != nil {
			t.Fatalf("Failed to create share: %v", err)
		}

		// Cover endpoint - no/bad password -> 401, good password -> 200
		reqCoverBad := httptest.NewRequest(http.MethodGet, "/api/s/"+slug+"/cover?raw=1", nil)
		w := httptest.NewRecorder()
		handleGetPublicShareCover(db, testTempDir).ServeHTTP(w, reqCoverBad)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("Cover: Expected 401, got %d", w.Code)
		}

		reqCoverGood := httptest.NewRequest(http.MethodGet, "/api/s/"+slug+"/cover?raw=1&password=secret", nil)
		w = httptest.NewRecorder()
		handleGetPublicShareCover(db, testTempDir).ServeHTTP(w, reqCoverGood)
		if w.Code != http.StatusOK {
			t.Errorf("Cover: Expected 200, got %d. Body: %s", w.Code, w.Body.String())
		}

		// Download endpoint - no/bad password -> 401, good password -> 200
		reqDownloadBad := httptest.NewRequest(http.MethodGet, "/api/s/"+slug+"/download", nil)
		w = httptest.NewRecorder()
		handleGetPublicShareDownload(db).ServeHTTP(w, reqDownloadBad)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("Download: Expected 401, got %d", w.Code)
		}

		reqDownloadGood := httptest.NewRequest(http.MethodGet, "/api/s/"+slug+"/download?password=secret", nil)
		w = httptest.NewRecorder()
		handleGetPublicShareDownload(db).ServeHTTP(w, reqDownloadGood)
		if w.Code != http.StatusOK {
			t.Errorf("Download: Expected 200, got %d. Body: %s", w.Code, w.Body.String())
		}

		// Stream endpoint - no/bad password -> 401, good password -> 200
		reqStreamBad := httptest.NewRequest(http.MethodGet, "/api/s/"+slug+"/stream", nil)
		w = httptest.NewRecorder()
		handleGetPublicShareStream(db).ServeHTTP(w, reqStreamBad)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("Stream: Expected 401, got %d", w.Code)
		}

		reqStreamGood := httptest.NewRequest(http.MethodGet, "/api/s/"+slug+"/stream?password=secret", nil)
		w = httptest.NewRecorder()
		handleGetPublicShareStream(db).ServeHTTP(w, reqStreamGood)
		if w.Code != http.StatusOK {
			t.Errorf("Stream: Expected 200, got %d. Body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("Invalid path/slug request format handling", func(t *testing.T) {
		// Mock routes_shares behavior:
		routerBasePath := "/"

		// Helper router matching routes_shares.go exactly
		testMux := http.NewServeMux()
		registerShareRoutes(testMux, &core.Config{RouterBasePath: routerBasePath, MetadataPath: testTempDir}, db)

		// Request for invalid method on /api/s/ -> should be 404 per the fallthrough or NotFound
		reqPost := httptest.NewRequest(http.MethodPost, "/api/s/slug-nopass", nil)
		w := httptest.NewRecorder()
		testMux.ServeHTTP(w, reqPost)
		if w.Code != http.StatusNotFound {
			t.Errorf("Expected 404 NotFound for invalid method on public route, got %d", w.Code)
		}

		// Request with extra path levels -> should be 404
		reqExtra := httptest.NewRequest(http.MethodGet, "/api/s/slug-nopass/cover/extra", nil)
		w = httptest.NewRecorder()
		testMux.ServeHTTP(w, reqExtra)
		if w.Code != http.StatusNotFound {
			t.Errorf("Expected 404 NotFound for extra path levels, got %d", w.Code)
		}
	})
}
