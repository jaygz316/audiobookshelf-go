package podcast

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSyncAllFeeds_StandardSchema(t *testing.T) {
	db := setupTestDB(t, false)
	defer db.Close()
	m := NewPodcastManager(db)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(feedUTF8))
	}))
	defer server.Close()

	configureTestClient(t, m, server.URL)

	_, err := db.Exec(`
		INSERT INTO podcasts (id, title, feedURL, autoDownloadEpisodes, maxEpisodesToKeep, maxNewEpisodesToDownload)
		VALUES (?, ?, ?, ?, ?, ?)
	`, "pod-std", "Std Podcast", server.URL, 0, 0, 0)
	if err != nil {
		t.Fatalf("failed to insert podcast: %v", err)
	}

	ctx := context.Background()
	err = m.SyncAllFeeds(ctx)
	if err != nil {
		t.Fatalf("SyncAllFeeds failed: %v", err)
	}

	rows, err := db.Query("SELECT id, podcastId, title, audioFile FROM podcastEpisodes WHERE podcastId = ?", "pod-std")
	if err != nil {
		t.Fatalf("query podcastEpisodes failed: %v", err)
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var id, podId, title, audioFile string
		if err := rows.Scan(&id, &podId, &title, &audioFile); err != nil {
			t.Fatalf("scan podcast episode failed: %v", err)
		}
		count++
	}
	if count != 2 {
		t.Errorf("expected 2 episodes inserted, got %d", count)
	}
}

func TestSyncAllFeeds_FullSchema(t *testing.T) {
	db := setupTestDB(t, true)
	defer db.Close()
	m := NewPodcastManager(db)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(feedUTF8))
	}))
	defer server.Close()

	configureTestClient(t, m, server.URL)

	_, err := db.Exec(`
		INSERT INTO podcasts (id, title, feedURL, autoDownloadEpisodes, maxEpisodesToKeep, maxNewEpisodesToDownload)
		VALUES (?, ?, ?, ?, ?, ?)
	`, "pod-full", "Full Podcast", server.URL, 0, 0, 0)
	if err != nil {
		t.Fatalf("failed to insert podcast: %v", err)
	}

	ctx := context.Background()
	err = m.SyncAllFeeds(ctx)
	if err != nil {
		t.Fatalf("SyncAllFeeds failed: %v", err)
	}

	rows, err := db.Query("SELECT id, podcastId, title, pubDate, description, enclosureURL FROM podcastEpisodes WHERE podcastId = ?", "pod-full")
	if err != nil {
		t.Fatalf("query podcastEpisodes failed: %v", err)
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var id, podId, title, pubDate, description, encURL string
		if err := rows.Scan(&id, &podId, &title, &pubDate, &description, &encURL); err != nil {
			t.Fatalf("scan podcast episode failed: %v", err)
		}
		count++
		if title == "Episode 1" {
			if !strings.Contains(pubDate, "2026-06-08T12:00:00") {
				t.Errorf("expected parsed pubDate, got %q", pubDate)
			}
			if description != "<p>Content of Episode 1</p>" {
				t.Errorf("expected description, got %q", description)
			}
			if encURL != "http://example.com/ep1.mp3" {
				t.Errorf("expected enclosureURL, got %q", encURL)
			}
		}
	}
	if count != 2 {
		t.Errorf("expected 2 episodes inserted, got %d", count)
	}
}

func TestSyncAllFeeds_LimitNewEpisodes(t *testing.T) {
	db := setupTestDB(t, false)
	defer db.Close()
	m := NewPodcastManager(db)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(feedUTF8))
	}))
	defer server.Close()

	configureTestClient(t, m, server.URL)

	_, err := db.Exec(`
		INSERT INTO podcasts (id, title, feedURL, autoDownloadEpisodes, maxEpisodesToKeep, maxNewEpisodesToDownload)
		VALUES (?, ?, ?, ?, ?, ?)
	`, "pod-limit", "Limit Podcast", server.URL, 0, 0, 1)
	if err != nil {
		t.Fatalf("failed to insert podcast: %v", err)
	}

	ctx := context.Background()
	err = m.SyncAllFeeds(ctx)
	if err != nil {
		t.Fatalf("SyncAllFeeds failed: %v", err)
	}

	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM podcastEpisodes WHERE podcastId = ?", "pod-limit").Scan(&count)
	if err != nil {
		t.Fatalf("query count failed: %v", err)
	}

	if count != 1 {
		t.Errorf("expected exactly 1 episode to be inserted due to limit, got %d", count)
	}
}
