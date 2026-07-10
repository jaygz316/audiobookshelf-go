package handlers

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"audiobookshelf/internal/core"
	ihls "audiobookshelf/internal/hls"
	isocket "audiobookshelf/internal/socket"
)

func TestRoutingAndEmbeddingDefaultBase(t *testing.T) {
	// Initialize subFS since main() is not called in test
	subFS = os.DirFS("../../frontend")

	// 1. Setup config with default base path (empty string)
	cfg := &core.Config{
		RouterBasePath: "",
		ConfigPath:     t.TempDir(),
		MetadataPath:   t.TempDir(),
	}

	handler := SetupHandler(nil, cfg, false, ".", "2.35.1")

	// Test case A: Request index.html directly
	{
		req := httptest.NewRequest("GET", "/index.html", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected 200 OK for /index.html, got %d", rr.Code)
		}
		if !strings.Contains(rr.Header().Get("Content-Type"), "text/html") {
			t.Errorf("Expected Content-Type text/html, got %q", rr.Header().Get("Content-Type"))
		}
		body := rr.Body.String()
		if !strings.Contains(body, "Audiobookshelf") {
			t.Errorf("Expected body to contain 'Audiobookshelf'")
		}
	}

	// Test case B: Request js/app.js
	{
		req := httptest.NewRequest("GET", "/js/app.js", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected 200 OK for /js/app.js, got %d", rr.Code)
		}
		contentType := rr.Header().Get("Content-Type")
		if !strings.Contains(contentType, "javascript") && !strings.Contains(contentType, "text/plain") {
			t.Errorf("Expected javascript content type, got %q", contentType)
		}
		body := rr.Body.String()
		if !strings.Contains(body, "bootstrapApp") {
			t.Errorf("Expected app.js to contain bootstrapApp")
		}
	}

	// Test case C: Request css/styles.css
	{
		req := httptest.NewRequest("GET", "/css/styles.css", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected 200 OK for /css/styles.css, got %d", rr.Code)
		}
		if !strings.Contains(rr.Header().Get("Content-Type"), "text/css") {
			t.Errorf("Expected Content-Type text/css, got %q", rr.Header().Get("Content-Type"))
		}
	}

	// Test case D: Request assets/images/logo.png
	{
		req := httptest.NewRequest("GET", "/assets/images/logo.png", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected 200 OK for logo, got %d", rr.Code)
		}
		if !strings.Contains(rr.Header().Get("Content-Type"), "image/png") {
			t.Errorf("Expected Content-Type image/png, got %q", rr.Header().Get("Content-Type"))
		}
	}

	// Test case E: Request nonexistent path (SPA fallback check)
	{
		req := httptest.NewRequest("GET", "/some/client/side/route", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected 200 OK for SPA fallback, got %d", rr.Code)
		}
		if !strings.Contains(rr.Header().Get("Content-Type"), "text/html") {
			t.Errorf("Expected Content-Type text/html for fallback, got %q", rr.Header().Get("Content-Type"))
		}
		body := rr.Body.String()
		if !strings.Contains(body, "Audiobookshelf") {
			t.Errorf("Expected fallback body to contain 'Audiobookshelf'")
		}
	}
}

func TestRoutingAndEmbeddingCustomBase(t *testing.T) {
	// Initialize subFS since main() is not called in test
	subFS = os.DirFS("../../frontend")

	// 2. Setup config with a custom RouterBasePath
	cfg := &core.Config{
		RouterBasePath: "/mybase",
		ConfigPath:     t.TempDir(),
		MetadataPath:   t.TempDir(),
	}

	handler := SetupHandler(nil, cfg, false, ".", "2.35.1")

	// Test case A: Request /mybase/index.html
	{
		req := httptest.NewRequest("GET", "/mybase/index.html", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected 200 OK for /mybase/index.html, got %d", rr.Code)
		}
		if !strings.Contains(rr.Header().Get("Content-Type"), "text/html") {
			t.Errorf("Expected Content-Type text/html, got %q", rr.Header().Get("Content-Type"))
		}
		body := rr.Body.String()
		if !strings.Contains(body, "Audiobookshelf") {
			t.Errorf("Expected body to contain 'Audiobookshelf'")
		}
	}

	// Test case B: Request /mybase/js/app.js
	{
		req := httptest.NewRequest("GET", "/mybase/js/app.js", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected 200 OK for /mybase/js/app.js, got %d", rr.Code)
		}
		contentType := rr.Header().Get("Content-Type")
		if !strings.Contains(contentType, "javascript") && !strings.Contains(contentType, "text/plain") {
			t.Errorf("Expected javascript content type, got %q", contentType)
		}
		body := rr.Body.String()
		if !strings.Contains(body, "bootstrapApp") {
			t.Errorf("Expected app.js to contain bootstrapApp")
		}
	}

	// Test case C: Request /mybase/css/styles.css
	{
		req := httptest.NewRequest("GET", "/mybase/css/styles.css", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected 200 OK for /mybase/css/styles.css, got %d", rr.Code)
		}
		if !strings.Contains(rr.Header().Get("Content-Type"), "text/css") {
			t.Errorf("Expected Content-Type text/css, got %q", rr.Header().Get("Content-Type"))
		}
	}

	// Test case D: Request /mybase/some/client/side/route (SPA fallback with custom base)
	{
		req := httptest.NewRequest("GET", "/mybase/some/client/side/route", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected 200 OK for SPA fallback with base, got %d", rr.Code)
		}
		if !strings.Contains(rr.Header().Get("Content-Type"), "text/html") {
			t.Errorf("Expected Content-Type text/html for fallback, got %q", rr.Header().Get("Content-Type"))
		}
		body := rr.Body.String()
		if !strings.Contains(body, "Audiobookshelf") {
			t.Errorf("Expected fallback body to contain 'Audiobookshelf'")
		}
	}

	// Test case E: Request without base prefix /js/app.js
	// Note: The middleware is designed to rewrite this internally to /mybase/js/app.js
	{
		req := httptest.NewRequest("GET", "/js/app.js", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected 200 OK for request without base rewrite, got %d", rr.Code)
		}
		contentType := rr.Header().Get("Content-Type")
		if !strings.Contains(contentType, "javascript") && !strings.Contains(contentType, "text/plain") {
			t.Errorf("Expected javascript content type, got %q", contentType)
		}
		body := rr.Body.String()
		if !strings.Contains(body, "bootstrapApp") {
			t.Errorf("Expected app.js to contain bootstrapApp")
		}
	}

	// Test case F: Request without base prefix /some/client/side/route (rewritten SPA fallback)
	{
		req := httptest.NewRequest("GET", "/some/client/side/route", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected 200 OK for request without base rewritten SPA fallback, got %d", rr.Code)
		}
		if !strings.Contains(rr.Header().Get("Content-Type"), "text/html") {
			t.Errorf("Expected Content-Type text/html for fallback, got %q", rr.Header().Get("Content-Type"))
		}
	}

	// Test case G: Request /mybase/assets/fonts/MaterialSymbolsRounded.woff2
	{
		req := httptest.NewRequest("GET", "/mybase/assets/fonts/MaterialSymbolsRounded.woff2", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected 200 OK for /mybase/assets/fonts/MaterialSymbolsRounded.woff2, got %d. Body: %s", rr.Code, rr.Body.String())
		}
		contentType := rr.Header().Get("Content-Type")
		if !strings.Contains(contentType, "font/woff2") && !strings.Contains(contentType, "application/octet-stream") && !strings.Contains(contentType, "application/x-font-woff") {
			t.Errorf("Expected woff2 content type, got %q", contentType)
		}
	}
}

func TestRoutingMeProgressRoutes(t *testing.T) {
	cfg := &core.Config{
		RouterBasePath: "/audiobookshelf",
		ConfigPath:     t.TempDir(),
		MetadataPath:   t.TempDir(),
	}

	handler := SetupHandler(nil, cfg, false, ".", "2.35.1")

	// 1. GET /audiobookshelf/api/me/progress/some-id/remove-from-continue-listening
	// This should hit AuthMiddlewareWrapper which returns 500 Internal Server Error (Database not connected) since db is nil,
	// rather than 404 (Not Found) or 405 (Method Not Allowed) or being routed to handleGetMeProgress.
	{
		req := httptest.NewRequest("GET", "/audiobookshelf/api/me/progress/some-id/remove-from-continue-listening", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusInternalServerError {
			t.Errorf("Expected 500 Internal Server Error for remove-from-continue-listening, got %d. Body: %s", rr.Code, rr.Body.String())
		}
	}

	// 2. PATCH /audiobookshelf/api/me/progress/some-id/hide-from-continue-listening
	// This should also hit AuthMiddlewareWrapper -> 500
	{
		req := httptest.NewRequest("PATCH", "/audiobookshelf/api/me/progress/some-id/hide-from-continue-listening", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusInternalServerError {
			t.Errorf("Expected 500 Internal Server Error for hide-from-continue-listening, got %d. Body: %s", rr.Code, rr.Body.String())
		}
	}

	// 3. POST /audiobookshelf/api/me/item/some-id/bookmark -> 500
	{
		req := httptest.NewRequest("POST", "/audiobookshelf/api/me/item/some-id/bookmark", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusInternalServerError {
			t.Errorf("Expected 500 Internal Server Error for POST bookmark, got %d", rr.Code)
		}
	}

	// 4. PATCH /audiobookshelf/api/me/item/some-id/bookmark -> 500
	{
		req := httptest.NewRequest("PATCH", "/audiobookshelf/api/me/item/some-id/bookmark", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusInternalServerError {
			t.Errorf("Expected 500 Internal Server Error for PATCH bookmark, got %d", rr.Code)
		}
	}

	// 5. DELETE /audiobookshelf/api/me/item/some-id/bookmark/123.45 -> 500
	{
		req := httptest.NewRequest("DELETE", "/audiobookshelf/api/me/item/some-id/bookmark/123.45", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusInternalServerError {
			t.Errorf("Expected 500 Internal Server Error for DELETE bookmark, got %d. Body: %s", rr.Code, rr.Body.String())
		}
	}
}

func TestRoutingDocs(t *testing.T) {
	// Initialize docsFS since main() is not called in test
	docsFS = os.DirFS("../../docs")

	cfg := &core.Config{
		RouterBasePath: "/audiobookshelf",
		ConfigPath:     t.TempDir(),
		MetadataPath:   t.TempDir(),
	}

	handler := SetupHandler(nil, cfg, false, ".", "2.35.1")

	// 1. GET /audiobookshelf/docs -> Redirect to /audiobookshelf/docs/
	{
		req := httptest.NewRequest("GET", "/audiobookshelf/docs", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusMovedPermanently {
			t.Errorf("Expected 301 Redirect for /audiobookshelf/docs, got %d", rr.Code)
		}
		loc := rr.Header().Get("Location")
		if loc != "/audiobookshelf/docs/" {
			t.Errorf("Expected redirect Location '/audiobookshelf/docs/', got %q", loc)
		}
	}

	// 2. GET /audiobookshelf/docs/ -> Serves index.html
	{
		req := httptest.NewRequest("GET", "/audiobookshelf/docs/", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected 200 OK for /audiobookshelf/docs/, got %d. Body: %s", rr.Code, rr.Body.String())
		}
		body := rr.Body.String()
		if !strings.Contains(body, "Audiobookshelf API") {
			t.Errorf("Expected body to contain 'Audiobookshelf API'")
		}
	}

	// 3. GET /audiobookshelf/docs/openapi.json -> Serves openapi.json
	{
		req := httptest.NewRequest("GET", "/audiobookshelf/docs/openapi.json", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected 200 OK for openapi.json, got %d. Body: %s", rr.Code, rr.Body.String())
		}
		body := rr.Body.String()
		if !strings.Contains(body, `"openapi"`) {
			t.Errorf("Expected openapi.json to contain '\"openapi\"'")
		}
	}
}

func TestMockHLSRequest(t *testing.T) {
	db, err := sql.Open("sqlite", "config/absdatabase.sqlite")
	if err != nil {
		t.Fatalf("Failed to open DB: %v", err)
	}
	defer db.Close()

	globalDB = db
	streamManager = ihls.NewStreamManager()
	metadataPath, _ := filepath.Abs("metadata")

	// Sign token dynamically using database's actual tokenSecret
	secret := getTokenSecret(db)
	claims := &core.AuthClaims{
		UserID:   "743336bf-a4f6-4da2-8a5f-9a1eb3fa74fd",
		Username: "root",
		Type:     "root",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
		},
	}
	tokenObj := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	token, err := tokenObj.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("Failed to sign token: %v", err)
	}

	url := "/audiobookshelf/hls/7c0a8a89-3d4f-4dc3-9d4a-98b11535e538/output-0.ts?token=" + token
	req := httptest.NewRequest("GET", url, nil)
	rr := httptest.NewRecorder()

	handler := AuthMiddlewareWrapper(db, ihls.ServeHLS(db, metadataPath, streamManager, isocket.GlobalAuth))
	handler.ServeHTTP(rr, req)

	t.Logf("Response Code: %d", rr.Code)
	t.Logf("Response Headers: %v", rr.Header())
	t.Logf("Response Body Length: %d", rr.Body.Len())
	if rr.Code != http.StatusOK {
		t.Logf("Response Body: %s", rr.Body.String())
	}
}
