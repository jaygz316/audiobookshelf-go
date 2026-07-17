package podcast

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestDownloadEpisode(t *testing.T) {
	db := setupTestDB(t, false)
	defer db.Close()
	m := NewPodcastManager(db)

	audioContent := []byte("fake-mp3-stream-data")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/mpeg")
		w.WriteHeader(http.StatusOK)
		w.Write(audioContent)
	}))
	defer server.Close()

	configureTestClient(t, m, server.URL)

	tempDir := t.TempDir()
	destPath := filepath.Join(tempDir, "ep1.mp3")

	ctx := context.Background()
	err := m.DownloadEpisode(ctx, server.URL, destPath)
	if err != nil {
		t.Fatalf("DownloadEpisode failed: %v", err)
	}

	data, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("failed to read downloaded file: %v", err)
	}

	if string(data) != string(audioContent) {
		t.Errorf("downloaded content mismatch, expected %q, got %q", string(audioContent), string(data))
	}
}

func TestSyncAllFeeds_AutoDownload(t *testing.T) {
	db := setupTestDB(t, true)
	defer db.Close()
	m := NewPodcastManager(db)

	audioContent := []byte("audio-file-data-stream")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/mpeg")
		w.WriteHeader(http.StatusOK)
		w.Write(audioContent)
	}))
	defer server.Close()

	feedWithEnclosure := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
<channel>
	<title>Download Feed</title>
	<item>
		<title>Downloadable Episode</title>
		<enclosure url="%s" length="123" type="audio/mpeg" />
		<pubDate>Mon, 08 Jun 2026 12:00:00 +0000</pubDate>
	</item>
</channel>
</rss>`, server.URL)

	feedServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(feedWithEnclosure))
	}))
	defer feedServer.Close()

	configureTestClient(t, m, server.URL, feedServer.URL)

	podID := "pod-dl"
	_, err := db.Exec(`
		INSERT INTO podcasts (id, title, feedURL, autoDownloadEpisodes, maxEpisodesToKeep, maxNewEpisodesToDownload)
		VALUES (?, ?, ?, ?, ?, ?)
	`, podID, "Download Podcast", feedServer.URL, 1, 0, 0)
	if err != nil {
		t.Fatalf("failed to insert podcast: %v", err)
	}

	tempLibDir := t.TempDir()

	_, err = db.Exec(`
		INSERT INTO libraryItems (id, path, mediaId, mediaType)
		VALUES (?, ?, ?, ?)
	`, "lib-item-1", tempLibDir, podID, "podcast")
	if err != nil {
		t.Fatalf("failed to insert library item: %v", err)
	}

	expectedDestName := sanitizeFilename("Downloadable Episode") + ".mp3"
	duplicateCheckPath := filepath.Join(tempLibDir, expectedDestName)
	err = os.WriteFile(duplicateCheckPath, []byte("pre-existing-content"), 0644)
	if err != nil {
		t.Fatalf("failed to write pre-existing file: %v", err)
	}

	ctx := context.Background()
	err = m.SyncAllFeeds(ctx)
	if err != nil {
		t.Fatalf("SyncAllFeeds failed: %v", err)
	}

	files, err := os.ReadDir(tempLibDir)
	if err != nil {
		t.Fatalf("failed to read library dir: %v", err)
	}

	var downloadedFile string
	for _, f := range files {
		if f.Name() != expectedDestName {
			downloadedFile = filepath.Join(tempLibDir, f.Name())
		}
	}

	if downloadedFile == "" {
		t.Fatal("expected duplicate-resolved file to be downloaded, but found none")
	}

	dlContent, err := os.ReadFile(downloadedFile)
	if err != nil {
		t.Fatalf("failed to read downloaded file: %v", err)
	}
	if string(dlContent) != string(audioContent) {
		t.Errorf("downloaded file content mismatch, expected %q, got %q", string(audioContent), string(dlContent))
	}

	var audioFileJSON string
	err = db.QueryRow("SELECT audioFile FROM podcastEpisodes WHERE podcastId = ?", podID).Scan(&audioFileJSON)
	if err != nil {
		t.Fatalf("failed to query episode audioFile: %v", err)
	}

	var audioFileMap map[string]interface{}
	err = json.Unmarshal([]byte(audioFileJSON), &audioFileMap)
	if err != nil {
		t.Fatalf("failed to unmarshal audioFile JSON %q: %v", audioFileJSON, err)
	}

	meta, ok := audioFileMap["metadata"].(map[string]interface{})
	if !ok {
		t.Fatalf("invalid audioFile JSON structure, missing metadata: %s", audioFileJSON)
	}

	if meta["path"] != downloadedFile {
		t.Errorf("expected path %q in DB, got %q", downloadedFile, meta["path"])
	}
	if int64(meta["size"].(float64)) != int64(len(audioContent)) {
		t.Errorf("expected size %d, got %v", len(audioContent), meta["size"])
	}
}
