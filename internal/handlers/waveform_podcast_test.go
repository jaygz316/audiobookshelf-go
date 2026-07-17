package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"audiobookshelf/internal/core"
)

func TestGetWaveform_Podcast(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Seed database with a podcast, a podcast episode, and a library item
	audioFileJSON := `{
		"duration": 300.0,
		"metadata": {
			"path": "/nonexistent/episode.mp3"
		}
	}`
	_, err := db.Exec(`INSERT INTO podcasts (id, title) VALUES ('podcast-1', 'Test Podcast')`)
	if err != nil {
		t.Fatalf("Failed to seed podcast: %v", err)
	}

	_, err = db.Exec(`INSERT INTO podcastEpisodes (id, podcastId, title, audioFile) VALUES ('episode-1', 'podcast-1', 'Episode 1', ?)`, audioFileJSON)
	if err != nil {
		t.Fatalf("Failed to seed podcast episode: %v", err)
	}

	_, err = db.Exec(`INSERT INTO libraryItems (id, mediaId, mediaType) VALUES ('item-1', 'podcast-1', 'podcast')`)
	if err != nil {
		t.Fatalf("Failed to seed library item: %v", err)
	}

	_, err = db.Exec(`INSERT INTO libraryFolders (id, path, libraryId) VALUES ('folder-1', '/nonexistent', 'lib-1')`)
	if err != nil {
		t.Fatalf("Failed to seed library folder: %v", err)
	}

	userSess := &core.UserSession{
		ID:       "user-1",
		Username: "testuser",
		Type:     "user",
		IsActive: true,
	}

	tempDir := t.TempDir()
	cfg := &core.Config{
		MetadataPath: tempDir,
	}

	// Request with library item ID (will fall back to the first episode)
	req := httptest.NewRequest("GET", "/api/items/item-1/waveform", nil)
	req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, userSess))
	rr := httptest.NewRecorder()

	handler := handleGetWaveform(db, cfg, "item-1")
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected status 200 OK, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	peaks, ok := resp["peaks"].([]interface{})
	if !ok || len(peaks) != 200 {
		t.Fatalf("Expected 200 peaks, got %v", resp["peaks"])
	}

	// Request directly with episode ID
	req2 := httptest.NewRequest("GET", "/api/items/episode-1/waveform", nil)
	req2 = req2.WithContext(context.WithValue(req2.Context(), core.UserContextKey, userSess))
	rr2 := httptest.NewRecorder()

	handler2 := handleGetWaveform(db, cfg, "episode-1")
	handler2.ServeHTTP(rr2, req2)

	if rr2.Code != http.StatusOK {
		t.Fatalf("Expected status 200 OK, got %d: %s", rr2.Code, rr2.Body.String())
	}
}
