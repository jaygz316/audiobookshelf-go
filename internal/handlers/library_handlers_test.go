package handlers

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestServeStaticOrSPA_PathTraversal(t *testing.T) {
	tempDir := t.TempDir()

	// Create a dummy frontend directory
	frontendDir := filepath.Join(tempDir, "frontend")
	os.MkdirAll(frontendDir, 0755)
	os.WriteFile(filepath.Join(frontendDir, "index.html"), []byte("index"), 0644)
	os.WriteFile(filepath.Join(frontendDir, "app.js"), []byte("app"), 0644)

	// Create a secret file outside the frontend directory
	secretFile := filepath.Join(tempDir, "secret.txt")
	os.WriteFile(secretFile, []byte("super secret"), 0644)

	fSys := os.DirFS(frontendDir)
	handler := serveStaticOrSPA(fSys, "")

	tests := []struct {
		name         string
		path         string
		expectedCode int
		expectedBody string
	}{
		{
			name:         "Normal access to index",
			path:         "/index.html",
			expectedCode: http.StatusOK,
			expectedBody: "index",
		},
		{
			name:         "Normal access to app.js",
			path:         "/app.js",
			expectedCode: http.StatusOK,
			expectedBody: "app",
		},
		{
			name:         "Path traversal attempt",
			path:         "/../secret.txt",
			expectedCode: http.StatusOK,
			expectedBody: "index", // Should fallback to index.html and not return secret
		},
		{
			name:         "Path traversal attempt encoded",
			path:         "/%2e%2e/secret.txt",
			expectedCode: http.StatusOK,
			expectedBody: "index", // Should fallback to index.html and not return secret
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tc.path, nil)
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != tc.expectedCode {
				t.Errorf("expected status %v, got %v", tc.expectedCode, rr.Code)
			}

			if tc.expectedBody != "" && rr.Body.String() != tc.expectedBody {
				t.Errorf("expected body %q, got %q", tc.expectedBody, rr.Body.String())
			}

			if rr.Body.String() == "super secret" {
				t.Errorf("VULNERABILITY DETECTED: path %s accessed secret file!", tc.path)
			}
		})
	}
}
