package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"audiobookshelf/internal/core"
)

func generateTestAudioFile(t *testing.T, ext string) string {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test_audio"+ext)

	var codec string
	if ext == ".mp3" {
		codec = "libmp3lame"
	} else if ext == ".m4a" || ext == ".m4b" {
		codec = "aac"
	} else {
		codec = "copy"
	}

	// Generate a 0.5s silent audio file
	cmd := exec.Command("ffmpeg", "-y", "-f", "lavfi", "-i", "anullsrc=r=44100:cl=stereo", "-t", "0.5", "-acodec", codec, path)
	if err := cmd.Run(); err != nil {
		t.Skipf("FFmpeg not available or failed to run: %v", err)
	}
	return path
}

func TestEmbedMetadata(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	audioPath := generateTestAudioFile(t, ".mp3")

	audioFiles := []map[string]interface{}{
		{
			"ino":      "12345",
			"filename": filepath.Base(audioPath),
			"ext":      ".mp3",
			"size":     1000,
			"metadata": map[string]interface{}{
				"path": audioPath,
			},
		},
	}
	bAudioFiles, _ := json.Marshal(audioFiles)

	chapters := []ChapterInfo{
		{ID: 1, Start: 0.0, End: 0.25, Title: "Introduction"},
		{ID: 2, Start: 0.25, End: 0.5, Title: "Conclusion"},
	}
	bChapters, _ := json.Marshal(chapters)

	genres := []string{"Fiction"}
	bGenres, _ := json.Marshal(genres)

	tags := []string{"Best"}
	bTags, _ := json.Marshal(tags)

	narrators := []string{"Narrator One"}
	bNarrators, _ := json.Marshal(narrators)

	// Create a dummy cover art file
	coverPath := filepath.Join(t.TempDir(), "cover.jpg")
	coverCmd := exec.Command("ffmpeg", "-y", "-f", "lavfi", "-i", "color=c=black:s=1x1", "-vframes", "1", coverPath)
	if err := coverCmd.Run(); err != nil {
		t.Skipf("FFmpeg color generation failed: %v", err)
	}

	// Seed database with a book and library item
	_, err := db.Exec(`
		INSERT INTO books (id, title, subtitle, publishedYear, publishedDate, publisher, description, coverPath, narrators, audioFiles, chapters, genres, tags) 
		VALUES ('book-1', 'Test Title', 'Test Subtitle', '2026', '2026-07-10', 'Test Publisher', 'Test Description', ?, ?, ?, ?, ?, ?)`,
		coverPath, bNarrators, bAudioFiles, bChapters, bGenres, bTags)
	if err != nil {
		t.Fatalf("Failed to seed book: %v", err)
	}

	_, err = db.Exec(`
		INSERT INTO libraryItems (id, mediaId, mediaType, authorNamesFirstLast, updatedAt) 
		VALUES ('item-1', 'book-1', 'book', 'Author Name', '2026-06-08 12:00:00.000')`)
	if err != nil {
		t.Fatalf("Failed to seed library item: %v", err)
	}

	userSess := &core.UserSession{
		ID:       "user-1",
		Username: "adminuser",
		Type:     "admin",
		IsActive: true,
	}

	req := httptest.NewRequest("POST", "/api/items/item-1/embed-metadata", nil)
	req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, userSess))
	rr := httptest.NewRecorder()

	cfg := &core.Config{
		MetadataPath: t.TempDir(),
	}

	handler := handleEmbedMetadata(db, cfg, "item-1")
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected status 200 OK, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if resp["success"].(bool) != true {
		t.Errorf("Expected success to be true, got %v", resp["success"])
	}

	updated, ok := resp["updatedFiles"].([]interface{})
	if !ok || len(updated) != 1 {
		t.Fatalf("Expected 1 updated file, got %v", resp["updatedFiles"])
	}

	if updated[0].(string) != filepath.Base(audioPath) {
		t.Errorf("Unexpected updated file: %s", updated[0].(string))
	}

	// Verify the file still exists and was not corrupted
	fi, err := os.Stat(audioPath)
	if err != nil {
		t.Fatalf("File does not exist: %v", err)
	}
	if fi.Size() < 500 {
		t.Errorf("File size is too small: %d", fi.Size())
	}
}

func TestEmbedMetadataForbidden(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	userSess := &core.UserSession{
		ID:       "user-2",
		Username: "regularuser",
		Type:     "user",
		IsActive: true,
	}

	req := httptest.NewRequest("POST", "/api/items/item-1/embed-metadata", nil)
	req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, userSess))
	rr := httptest.NewRecorder()

	cfg := &core.Config{}
	handler := handleEmbedMetadata(db, cfg, "item-1")
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("Expected status 403 Forbidden, got %d", rr.Code)
	}
}
