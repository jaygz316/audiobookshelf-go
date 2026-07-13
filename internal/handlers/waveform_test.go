package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"audiobookshelf/internal/core"
)

func TestGetWaveform_Book(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Seed database with a book and library item
	audioFilesJSON := `[
		{
			"exclude": false,
			"duration": 120.0,
			"metadata": {
				"path": "/nonexistent/test1.mp3"
			}
		},
		{
			"exclude": false,
			"duration": 80.0,
			"metadata": {
				"path": "/nonexistent/test2.mp3"
			}
		}
	]`
	_, err := db.Exec(`INSERT INTO books (id, title, audioFiles) VALUES ('book-1', 'Test Book', ?)`, audioFilesJSON)
	if err != nil {
		t.Fatalf("Failed to seed book: %v", err)
	}

	_, err = db.Exec(`INSERT INTO libraryItems (id, mediaId, mediaType) VALUES ('item-1', 'book-1', 'book')`)
	if err != nil {
		t.Fatalf("Failed to seed library item: %v", err)
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

	// 1. Initial request: generates peaks (will fall back to 0s since files don't exist)
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

	if resp["itemId"].(string) != "item-1" {
		t.Errorf("Expected itemId 'item-1', got %v", resp["itemId"])
	}

	peaks, ok := resp["peaks"].([]interface{})
	if !ok || len(peaks) != 200 {
		t.Fatalf("Expected 200 peaks, got %v", resp["peaks"])
	}

	// Verify cached file was written
	cachedFile := filepath.Join(tempDir, "items", "item-1", "waveform.json")
	if _, err := os.Stat(cachedFile); os.IsNotExist(err) {
		t.Fatalf("Expected cached file at %s, but it does not exist", cachedFile)
	}

	// 2. Modify the cached file manually to verify it serves from cache next time
	mockCachedData := `{"itemId":"item-1","peaks":[1,2,3]}`
	err = os.WriteFile(cachedFile, []byte(mockCachedData), 0644)
	if err != nil {
		t.Fatalf("Failed to write mock cached file: %v", err)
	}

	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req)

	if rr2.Code != http.StatusOK {
		t.Fatalf("Expected status 200 OK on cached hit, got %d", rr2.Code)
	}

	var resp2 map[string]interface{}
	if err := json.Unmarshal(rr2.Body.Bytes(), &resp2); err != nil {
		t.Fatalf("Failed to parse cached response: %v", err)
	}

	peaks2 := resp2["peaks"].([]interface{})
	if len(peaks2) != 3 || peaks2[0].(float64) != 1 || peaks2[2].(float64) != 3 {
		t.Errorf("Expected mocked cached peaks [1,2,3], got %v", peaks2)
	}
}

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
