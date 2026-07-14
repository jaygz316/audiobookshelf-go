package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"audiobookshelf/internal/core"
)

func createTestAudioFile(t *testing.T, path string, durationSeconds int) {
	// Create a silent audio file using ffmpeg with standard aac codec
	cmd := exec.Command("ffmpeg", "-y", "-f", "lavfi", "-i", "anullsrc=r=8000:cl=mono", "-t", fmt.Sprintf("%d", durationSeconds), "-c:a", "aac", path)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to create silent test audio file %s: %v, stderr: %s", path, err, stderr.String())
	}
}

func TestMergeAudioFiles(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	tempDir := t.TempDir()
	file1 := filepath.Join(tempDir, "track1.m4a")
	file2 := filepath.Join(tempDir, "track2.m4a")

	createTestAudioFile(t, file1, 2)
	createTestAudioFile(t, file2, 3)

	// Verify they exist
	if _, err := os.Stat(file1); err != nil {
		t.Fatalf("track1.m4a was not created: %v", err)
	}
	if _, err := os.Stat(file2); err != nil {
		t.Fatalf("track2.m4a was not created: %v", err)
	}

	// Prepare data to seed database
	audioFiles := []MergeAudioFile{
		{
			Index:    0,
			Exclude:  false,
			Duration: 2.0,
			Codec:    "aac",
			MimeType: "audio/mp4",
			Title:    "Track 1",
		},
		{
			Index:    1,
			Exclude:  false,
			Duration: 3.0,
			Codec:    "aac",
			MimeType: "audio/mp4",
			Title:    "Track 2",
		},
	}
	audioFiles[0].Metadata.Path = file1
	audioFiles[0].Metadata.Filename = "track1.m4a"
	audioFiles[0].Metadata.Size = 1000 // dummy size
	audioFiles[1].Metadata.Path = file2
	audioFiles[1].Metadata.Filename = "track2.m4a"
	audioFiles[1].Metadata.Size = 1500 // dummy size

	bAudioFiles, err := json.Marshal(audioFiles)
	if err != nil {
		t.Fatalf("Failed to marshal audioFiles: %v", err)
	}

	_, err = db.Exec(`INSERT INTO books (id, title, audioFiles, chapters) VALUES ('book-123', 'My Test Audiobook', ?, '[]')`, bAudioFiles)
	if err != nil {
		t.Fatalf("Failed to seed book: %v", err)
	}

	_, err = db.Exec(`INSERT INTO libraryItems (id, mediaId, mediaType, path, size, updatedAt) VALUES ('item-123', 'book-123', 'book', ?, 2500, '2026-06-08 12:00:00.000')`, tempDir)
	if err != nil {
		t.Fatalf("Failed to seed library item: %v", err)
	}

	_, err = db.Exec(`INSERT INTO libraryFolders (id, path, libraryId) VALUES ('folder-123', ?, 'lib-123')`, tempDir)
	if err != nil {
		t.Fatalf("Failed to seed libraryFolders: %v", err)
	}

	// Admin user session for authentication
	userSess := &core.UserSession{
		ID:       "user-admin",
		Username: "admin",
		Type:     "admin",
		IsActive: true,
	}

	// Make the merge request
	req := httptest.NewRequest("POST", "/api/items/item-123/merge", nil)
	req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, userSess))
	rr := httptest.NewRecorder()

	handler := handleMergeAudioFiles(db)
	handler.ServeHTTP(rr, req)

	// Check response status
	if rr.Code != http.StatusOK {
		t.Fatalf("Expected status 200 OK, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if resp["success"] != true {
		t.Errorf("Expected success to be true, got: %v", resp["success"])
	}

	// Verify that the original files were deleted
	if _, err := os.Stat(file1); !os.IsNotExist(err) {
		t.Errorf("track1.m4a was not deleted")
	}
	if _, err := os.Stat(file2); !os.IsNotExist(err) {
		t.Errorf("track2.m4a was not deleted")
	}

	// Verify the merged file exists
	expectedMergedName := "My Test Audiobook_merged.m4b"
	expectedMergedPath := filepath.Join(tempDir, expectedMergedName)
	mergedStat, err := os.Stat(expectedMergedPath)
	if err != nil {
		t.Fatalf("Merged file does not exist: %v", err)
	}
	if mergedStat.Size() == 0 {
		t.Errorf("Merged file is empty")
	}

	// Check database records
	var dbAudioFiles, dbChapters string
	var dbDuration float64
	err = db.QueryRow("SELECT audioFiles, chapters, duration FROM books WHERE id = 'book-123'").Scan(&dbAudioFiles, &dbChapters, &dbDuration)
	if err != nil {
		t.Fatalf("Failed to query books table: %v", err)
	}

	var updatedAudioFiles []MergeAudioFile
	if err := json.Unmarshal([]byte(dbAudioFiles), &updatedAudioFiles); err != nil {
		t.Fatalf("Failed to unmarshal updated audioFiles: %v", err)
	}

	if len(updatedAudioFiles) != 1 {
		t.Fatalf("Expected exactly 1 merged audio file, got %d", len(updatedAudioFiles))
	}

	mTrack := updatedAudioFiles[0]
	if mTrack.Metadata.Filename != expectedMergedName {
		t.Errorf("Expected merged filename %s, got %s", expectedMergedName, mTrack.Metadata.Filename)
	}
	if mTrack.Metadata.Path != expectedMergedPath {
		t.Errorf("Expected merged path %s, got %s", expectedMergedPath, mTrack.Metadata.Path)
	}
	if mTrack.Duration != 5.0 {
		t.Errorf("Expected total duration 5.0, got %f", mTrack.Duration)
	}
	if dbDuration != 5.0 {
		t.Errorf("Expected book duration to be 5.0, got %f", dbDuration)
	}

	var updatedChapters []MergeChapter
	if err := json.Unmarshal([]byte(dbChapters), &updatedChapters); err != nil {
		t.Fatalf("Failed to unmarshal updated chapters: %v", err)
	}

	if len(updatedChapters) != 2 {
		t.Fatalf("Expected 2 chapters, got %d", len(updatedChapters))
	}

	c1 := updatedChapters[0]
	if c1.ID != 1 || c1.Start != 0.0 || c1.End != 2.0 || c1.Title != "Track 1" {
		t.Errorf("Unexpected chapter 1: %+v", c1)
	}

	c2 := updatedChapters[1]
	if c2.ID != 2 || c2.Start != 2.0 || c2.End != 5.0 || c2.Title != "Track 2" {
		t.Errorf("Unexpected chapter 2: %+v", c2)
	}

	// Verify libraryItems updates
	var dbItemSize int64
	err = db.QueryRow("SELECT size FROM libraryItems WHERE id = 'item-123'").Scan(&dbItemSize)
	if err != nil {
		t.Fatalf("Failed to query libraryItems table: %v", err)
	}

	if dbItemSize != mergedStat.Size() {
		t.Errorf("Expected library item size in DB to be %d, got %d", mergedStat.Size(), dbItemSize)
	}
}
