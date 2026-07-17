package podcast

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestChallenger_EnforceRetentionPolicy_EdgeCases(t *testing.T) {
	db := setupRetentionTestDB(t)
	defer db.Close()
	m := NewPodcastManager(db)
	tempDir := t.TempDir()

	pValid := filepath.Join(tempDir, "valid.mp3")
	_ = os.WriteFile(pValid, []byte("valid"), 0644)

	pMissing := filepath.Join(tempDir, "missing.mp3") // not created on disk

	jsonValid := fmt.Sprintf(`{"metadata":{"path":"%s"}}`, pValid)
	jsonMissing := fmt.Sprintf(`{"metadata":{"path":"%s"}}`, pMissing)
	jsonInvalid := `{"metadata":` // bad json

	podID := "pod-edge"
	_, _ = db.Exec(`INSERT INTO podcastEpisodes (id, podcastId, title, audioFile, pubDate) VALUES (?, ?, ?, ?, ?)`,
		"ep-valid", podID, "Valid Ep", jsonValid, "2026-07-01")
	_, _ = db.Exec(`INSERT INTO podcastEpisodes (id, podcastId, title, audioFile, pubDate) VALUES (?, ?, ?, ?, ?)`,
		"ep-missing", podID, "Missing Ep", jsonMissing, "2026-07-02")
	_, _ = db.Exec(`INSERT INTO podcastEpisodes (id, podcastId, title, audioFile, pubDate) VALUES (?, ?, ?, ?, ?)`,
		"ep-invalid", podID, "Invalid Ep", jsonInvalid, "2026-07-03")

	err := m.EnforceRetentionPolicy(context.Background(), podID, 1)
	if err != nil {
		t.Fatalf("EnforceRetentionPolicy failed: %v", err)
	}

	if _, err := os.Stat(pValid); !os.IsNotExist(err) {
		t.Error("expected valid.mp3 to be deleted")
	}

	var audioFile string
	_ = db.QueryRow("SELECT audioFile FROM podcastEpisodes WHERE id = ?", "ep-valid").Scan(&audioFile)
	if audioFile != "{}" {
		t.Errorf("expected ep-valid to have audioFile cleared to '{}', got %s", audioFile)
	}

	_ = db.QueryRow("SELECT audioFile FROM podcastEpisodes WHERE id = ?", "ep-missing").Scan(&audioFile)
	if audioFile == "{}" {
		t.Error("expected ep-missing to keep its audioFile metadata in DB")
	}
}

func TestChallenger_QueueManager_LimitAndCancel(t *testing.T) {
	db := setupTestDB(t, false)
	defer db.Close()
	m := NewPodcastManager(db)

	var activeDownloads int32
	var maxActiveDownloads int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		active := atomic.AddInt32(&activeDownloads, 1)
		defer atomic.AddInt32(&activeDownloads, -1)

		for {
			currentMax := atomic.LoadInt32(&maxActiveDownloads)
			if active > currentMax {
				if atomic.CompareAndSwapInt32(&maxActiveDownloads, currentMax, active) {
					break
				}
			} else {
				break
			}
		}

		time.Sleep(50 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("audio-data"))
	}))
	defer server.Close()

	configureTestClient(t, m, server.URL)
	InitQueueManager(db, m)

	_, _ = db.Exec(`INSERT INTO podcasts (id, title, feedURL, autoDownloadEpisodes, maxEpisodesToKeep, maxNewEpisodesToDownload) VALUES (?, ?, ?, ?, ?, ?)`,
		"pod-q", "Q Pod", server.URL, 0, 0, 0)

	tempDir := t.TempDir()

	tasks := make([]*DownloadTask, 5)
	for i := 0; i < 5; i++ {
		epID := fmt.Sprintf("ep-q-%d", i)
		_, _ = db.Exec(`INSERT INTO podcastEpisodes (id, podcastId, title, audioFile) VALUES (?, ?, ?, ?)`,
			epID, "pod-q", fmt.Sprintf("Ep %d", i), "{}")

		tasks[i] = &DownloadTask{
			ID:           epID,
			PodcastID:    "pod-q",
			EpisodeTitle: fmt.Sprintf("Ep %d", i),
			EnclosureURL: server.URL,
			DestPath:     filepath.Join(tempDir, fmt.Sprintf("ep-%d.mp3", i)),
		}
		GlobalQueueManager.Enqueue(tasks[i])
	}

	time.Sleep(300 * time.Millisecond)

	maxActive := atomic.LoadInt32(&maxActiveDownloads)
	if maxActive > 2 {
		t.Errorf("expected max active downloads <= 2, got %d", maxActive)
	}

	for i := 0; i < 5; i++ {
		tasks[i].mu.Lock()
		status := tasks[i].Status
		tasks[i].mu.Unlock()
		if status != StatusFinished {
			t.Errorf("task %d status is %s, expected finished", i, status)
		}
	}
}
