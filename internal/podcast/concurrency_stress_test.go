package podcast

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestSyncFeed_Concurrency(t *testing.T) {
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

	podID := "pod-concurrent"
	_, err := db.Exec(`
		INSERT INTO podcasts (id, title, feedURL, autoDownloadEpisodes, maxEpisodesToKeep, maxNewEpisodesToDownload)
		VALUES (?, ?, ?, ?, ?, ?)
	`, podID, "Concurrent Podcast", server.URL, 0, 0, 0)
	if err != nil {
		t.Fatalf("failed to insert podcast: %v", err)
	}

	const concurrencyCount = 10
	var wg sync.WaitGroup
	wg.Add(concurrencyCount)

	for i := 0; i < concurrencyCount; i++ {
		go func() {
			defer wg.Done()
			ctx := context.Background()
			_ = m.SyncFeed(ctx, podID)
		}()
	}

	wg.Wait()

	// Check for duplicate episodes
	rows, err := db.Query("SELECT title, COUNT(*) FROM podcastEpisodes WHERE podcastId = ? GROUP BY title", podID)
	if err != nil {
		t.Fatalf("query podcastEpisodes failed: %v", err)
	}
	defer rows.Close()

	hasDuplicates := false
	for rows.Next() {
		var title string
		var count int
		if err := rows.Scan(&title, &count); err != nil {
			t.Fatalf("scan failed: %v", err)
		}
		if count > 1 {
			hasDuplicates = true
			t.Logf("Duplicate found: Episode %q was inserted %d times", title, count)
		}
	}

	if hasDuplicates {
		t.Errorf("SyncFeed is not concurrent-safe; duplicate episodes were inserted")
	}
}

func TestSyncFeed_AutoDownload_Concurrency(t *testing.T) {
	db := setupTestDB(t, false)
	defer db.Close()
	m := NewPodcastManager(db)

	tmpDir, err := os.MkdirTemp("", "podcast_download_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	var audioRequestCount int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/feed.xml") {
			customFeed := strings.ReplaceAll(feedUTF8, "http://example.com/ep1.mp3", "http://"+r.Host+"/ep1.mp3")
			customFeed = strings.ReplaceAll(customFeed, "http://example.com/ep2.mp3", "http://"+r.Host+"/ep2.mp3")
			w.Header().Set("Content-Type", "application/rss+xml")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(customFeed))
		} else if strings.HasSuffix(r.URL.Path, ".mp3") {
			atomic.AddInt64(&audioRequestCount, 1)
			w.Header().Set("Content-Type", "audio/mpeg")
			w.WriteHeader(http.StatusOK)
			w.Write(make([]byte, 1024*1024)) // 1MB of zeroes
		}
	}))
	defer server.Close()

	configureTestClient(t, m, server.URL+"/feed.xml", server.URL+"/ep1.mp3", server.URL+"/ep2.mp3")

	podID := "pod-download-concurrent"
	_, err = db.Exec(`
		INSERT INTO podcasts (id, title, feedURL, autoDownloadEpisodes, maxEpisodesToKeep, maxNewEpisodesToDownload)
		VALUES (?, ?, ?, ?, ?, ?)
	`, podID, "Concurrent Download Podcast", server.URL+"/feed.xml", 1, 0, 0)
	if err != nil {
		t.Fatalf("failed to insert podcast: %v", err)
	}

	_, err = db.Exec(`
		INSERT INTO libraryItems (id, path, mediaId, mediaType)
		VALUES (?, ?, ?, ?)
	`, "lib-concurrent", tmpDir, podID, "podcast")
	if err != nil {
		t.Fatalf("failed to insert libraryItem: %v", err)
	}

	const concurrencyCount = 5
	var wg sync.WaitGroup
	wg.Add(concurrencyCount)

	for i := 0; i < concurrencyCount; i++ {
		go func() {
			defer wg.Done()
			ctx := context.Background()
			_ = m.SyncFeed(ctx, podID)
		}()
	}

	wg.Wait()
	time.Sleep(100 * time.Millisecond)

	files, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("failed to read temp dir: %v", err)
	}

	t.Logf("Number of files in temp dir: %d", len(files))
	for _, f := range files {
		info, err := f.Info()
		if err != nil {
			continue
		}
		t.Logf("File name: %s, size: %d", f.Name(), info.Size())
		// If concurrent download succeeded with no lock, we might see multiple duplicate files,
		// or one corrupted/partially written file. We log this as evidence.
	}
}
