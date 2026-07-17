package podcast

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestScheduleRefresh(t *testing.T) {
	db := setupTestDB(t, false)
	defer db.Close()
	m := NewPodcastManager(db)

	var reqCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&reqCount, 1)
		w.Header().Set("Content-Type", "application/rss+xml")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(feedUTF8))
	}))
	defer server.Close()

	configureTestClient(t, m, server.URL)

	_, err := db.Exec(`
		INSERT INTO podcasts (id, title, feedURL, autoDownloadEpisodes, maxEpisodesToKeep, maxNewEpisodesToDownload)
		VALUES (?, ?, ?, ?, ?, ?)
	`, "pod-refresh", "Refresh Podcast", server.URL, 0, 0, 0)
	if err != nil {
		t.Fatalf("failed to insert podcast: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = m.ScheduleRefresh(ctx, "* * * * *")
	if err != nil {
		t.Fatalf("ScheduleRefresh failed: %v", err)
	}

	start := time.Now()
	for {
		if atomic.LoadInt32(&reqCount) > 0 {
			break
		}
		if time.Since(start) > 1500*time.Millisecond {
			t.Fatal("timed out waiting for background sync to trigger")
		}
		time.Sleep(10 * time.Millisecond)
	}

	cancel()
	time.Sleep(50 * time.Millisecond)
}

func TestParseCronToDuration(t *testing.T) {
	tests := []struct {
		expr     string
		expected time.Duration
	}{
		{"* * * * *", 1 * time.Minute},
		{"*/5 * * * *", 5 * time.Minute},
		{"5 * * * *", 1 * time.Hour},
		{"5 */3 * * *", 3 * time.Hour},
		{"5 2 * * *", 24 * time.Hour},
		{"invalid", 1 * time.Hour},
		{"*", 1 * time.Hour},
	}

	for _, tc := range tests {
		t.Run(tc.expr, func(t *testing.T) {
			res := parseCronToDuration(tc.expr)
			if res != tc.expected {
				t.Errorf("expected %v for expr %q, got %v", tc.expected, tc.expr, res)
			}
		})
	}
}
