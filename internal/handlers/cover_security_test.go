package handlers

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"audiobookshelf/internal/core"
)

func TestCoverAndAuthorImageSecurity(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Ensure authors table is present
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS authors (id TEXT PRIMARY KEY, name TEXT, imagePath TEXT)`)
	if err != nil {
		t.Fatalf("Failed to create authors table: %v", err)
	}

	// Ensure sessions table is present and insert a session for user1
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS sessions (id TEXT PRIMARY KEY, userId TEXT)`)
	if err != nil {
		t.Fatalf("Failed to create sessions table: %v", err)
	}
	_, err = db.Exec(`INSERT INTO sessions (id, userId) VALUES ('session1', 'user1')`)
	if err != nil {
		t.Fatalf("Failed to insert session: %v", err)
	}

	// Insert a test user
	_, err = db.Exec(`INSERT INTO users (id, username, type, isActive, permissions, extraData) VALUES ('user1', 'testuser', 'admin', 1, '{}', '{}')`)
	if err != nil {
		t.Fatalf("Failed to insert test user: %v", err)
	}

	// Set token secret in environment so that it is picked up by getTokenSecret
	t.Setenv("JWT_SECRET_KEY", "my-test-token-secret-1234567890")

	// Insert a test library item and book
	_, err = db.Exec(`INSERT INTO libraryItems (id, libraryId, mediaType, mediaId, path) VALUES ('item1', 'lib1', 'book', 'book1', '/fake/path')`)
	if err != nil {
		t.Fatalf("Failed to insert library item: %v", err)
	}

	// Create a dummy cover image file
	tempDir := t.TempDir()
	dummyCover := filepath.Join(tempDir, "cover.jpg")
	if err := os.WriteFile(dummyCover, []byte("fake image data"), 0644); err != nil {
		t.Fatalf("Failed to create dummy cover file: %v", err)
	}
	_, err = db.Exec(`INSERT INTO books (id, title, coverPath) VALUES ('book1', 'Test Book', ?)`, dummyCover)
	if err != nil {
		t.Fatalf("Failed to insert book: %v", err)
	}

	// Insert test author
	dummyAuthorImg := filepath.Join(tempDir, "author1.jpg")
	if err := os.WriteFile(dummyAuthorImg, []byte("fake author data"), 0644); err != nil {
		t.Fatalf("Failed to create dummy author image: %v", err)
	}
	_, err = db.Exec(`INSERT INTO authors (id, name, imagePath) VALUES ('author1', 'Test Author', ?)`, dummyAuthorImg)
	if err != nil {
		t.Fatalf("Failed to insert author: %v", err)
	}

	// Create dynamic JWT token
	secret := "my-test-token-secret-1234567890"
	claims := &core.AuthClaims{
		UserID:   "user1",
		Username: "testuser",
		Type:     "admin",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
		},
	}
	tokenObj := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	validToken, err := tokenObj.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("Failed to sign token: %v", err)
	}

	// Set up router using SetupHandler
	cfg := &core.Config{
		RouterBasePath: "",
		ConfigPath:     tempDir,
		MetadataPath:   tempDir,
	}
	subFS = os.DirFS("../../frontend")
	router := SetupHandler(db, cfg, true, ".", "2.35.1")

	// 1. GET /api/items/item1/cover without token -> Should pass (200 OK) because cover bypasses auth
	t.Run("Cover without token", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/items/item1/cover?raw=1", nil)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("Expected cover request without token to return 200, got %d. Body: %s", rr.Code, rr.Body.String())
		}
	})

	// 2. GET /api/authors/author1/image without token -> Should fail (401 Unauthorized)
	t.Run("Author image without token", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/authors/author1/image", nil)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("Expected author image without token to return 401, got %d", rr.Code)
		}
	})

	// 3. GET /api/authors/author1/image with valid token -> Should pass (200 OK)
	t.Run("Author image with valid token", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/authors/author1/image?token="+validToken, nil)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("Expected author image with valid token to return 200, got %d. Body: %s", rr.Code, rr.Body.String())
		}
	})
}
