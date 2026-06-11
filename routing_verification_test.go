package main

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRoutingAndEmbeddingDefaultBase(t *testing.T) {
	// Initialize subFS since main() is not called in test
	var err error
	subFS, err = fs.Sub(frontendFS, "frontend")
	if err != nil {
		t.Fatalf("Failed to initialize subFS: %v", err)
	}

	// 1. Setup config with default base path (empty string)
	cfg := &Config{
		RouterBasePath: "",
		ConfigPath:     t.TempDir(),
		MetadataPath:   t.TempDir(),
	}

	handler := setupHandler(nil, cfg, false, ".", "2.35.1")

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
		if !strings.Contains(body, "Audiobookshelf Mockup") {
			t.Errorf("Expected body to contain 'Audiobookshelf Mockup'")
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
		if !strings.Contains(body, "Audiobookshelf Mockup") {
			t.Errorf("Expected fallback body to contain 'Audiobookshelf Mockup'")
		}
	}
}

func TestRoutingAndEmbeddingCustomBase(t *testing.T) {
	// Initialize subFS since main() is not called in test
	var err error
	subFS, err = fs.Sub(frontendFS, "frontend")
	if err != nil {
		t.Fatalf("Failed to initialize subFS: %v", err)
	}

	// 2. Setup config with a custom RouterBasePath
	cfg := &Config{
		RouterBasePath: "/mybase",
		ConfigPath:     t.TempDir(),
		MetadataPath:   t.TempDir(),
	}

	handler := setupHandler(nil, cfg, false, ".", "2.35.1")

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
		if !strings.Contains(body, "Audiobookshelf Mockup") {
			t.Errorf("Expected body to contain 'Audiobookshelf Mockup'")
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
		if !strings.Contains(body, "Audiobookshelf Mockup") {
			t.Errorf("Expected fallback body to contain 'Audiobookshelf Mockup'")
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
		body := rr.Body.String()
		if !strings.Contains(body, "Audiobookshelf Mockup") {
			t.Errorf("Expected fallback body to contain 'Audiobookshelf Mockup'")
		}
	}
}
