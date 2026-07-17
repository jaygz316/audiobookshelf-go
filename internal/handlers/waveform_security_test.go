package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"audiobookshelf/internal/core"
)

// TestWaveform_PathTraversalAdversarialExtended tests various invalid itemID formats
// and verifies that the path traversal checks work correctly.
func TestWaveform_PathTraversalAdversarialExtended(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	userSess := &core.UserSession{
		ID:       "user-1",
		Username: "testuser",
		Type:     "admin",
		IsActive: true,
	}

	tempDir := t.TempDir()
	cfg := &core.Config{
		MetadataPath: tempDir,
	}

	traversalIDs := []string{
		"../escaped",
		"item/nested",
		"item\\backslash",
		"..",
		"/absolute/path",
	}

	for _, badID := range traversalIDs {
		t.Run("ID_"+badID, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/items/"+badID+"/waveform", nil)
			req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, userSess))
			rr := httptest.NewRecorder()

			handler := handleGetWaveform(db, cfg, badID)
			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusBadRequest {
				t.Errorf("Expected 400 Bad Request for itemID %q, got %d. Body: %s", badID, rr.Code, rr.Body.String())
			}
		})
	}
}

// TestWaveform_UnsafeAudioPaths tests that if the database returns files outside the library folders
// or metadata directory, they are correctly rejected with 403 Forbidden.
func TestWaveform_UnsafeAudioPaths(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	userSess := &core.UserSession{
		ID:       "user-1",
		Username: "testuser",
		Type:     "admin",
		IsActive: true,
	}

	tempDir := t.TempDir()
	cfg := &core.Config{
		MetadataPath: tempDir,
	}

	// Seed database with a book whose audio file path is in a restricted area
	audioFilesJSON := `[
		{
			"exclude": false,
			"duration": 60.0,
			"metadata": {
				"path": "/etc/shadow"
			}
		}
	]`
	_, err := db.Exec(`INSERT INTO books (id, title, audioFiles) VALUES ('book-unsafe', 'Secret Book', ?)`, audioFilesJSON)
	if err != nil {
		t.Fatalf("Failed to seed book: %v", err)
	}

	_, err = db.Exec(`INSERT INTO libraryItems (id, mediaId, mediaType) VALUES ('item-unsafe', 'book-unsafe', 'book')`)
	if err != nil {
		t.Fatalf("Failed to seed library item: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/items/item-unsafe/waveform", nil)
	req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, userSess))
	rr := httptest.NewRecorder()

	handler := handleGetWaveform(db, cfg, "item-unsafe")
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("Expected 403 Forbidden for unsafe audio file path, got %d. Body: %s", rr.Code, rr.Body.String())
	}
}
