package podcast

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestChallenger_PausePendingTask(t *testing.T) {
	blockChan := make(chan struct{})
	var task3Called int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/task-3" {
			atomic.StoreInt32(&task3Called, 1)
		}
		w.Header().Set("Content-Type", "audio/mpeg")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data"))
		<-blockChan
	}))
	defer server.Close()

	db, _, tempDir := setupQueueTest(t, server.URL)
	defer db.Close()

	// Enqueue 2 downloads to fill the semaphore (limit is 2)
	for i := 1; i <= 2; i++ {
		epID := fmt.Sprintf("ep-%d", i)
		_, _ = db.Exec(`INSERT INTO podcastEpisodes (id, podcastId, title, audioFile) VALUES (?, ?, ?, ?)`,
			epID, "pod-1", epID, "{}")

		task := &DownloadTask{
			ID:           epID,
			PodcastID:    "pod-1",
			EpisodeTitle: epID,
			EnclosureURL: server.URL + fmt.Sprintf("/task-%d", i),
			DestPath:     filepath.Join(tempDir, epID+".mp3"),
		}
		GlobalQueueManager.Enqueue(task)
	}

	// Wait a bit to ensure both tasks acquired semaphore slots and are downloading
	time.Sleep(50 * time.Millisecond)

	// Enqueue the third task (must remain StatusPending)
	ep3ID := "ep-3"
	_, _ = db.Exec(`INSERT INTO podcastEpisodes (id, podcastId, title, audioFile) VALUES (?, ?, ?, ?)`,
		ep3ID, "pod-1", ep3ID, "{}")

	task3 := &DownloadTask{
		ID:           ep3ID,
		PodcastID:    "pod-1",
		EpisodeTitle: ep3ID,
		EnclosureURL: server.URL + "/task-3",
		DestPath:     filepath.Join(tempDir, "ep-3.mp3"),
	}
	GlobalQueueManager.Enqueue(task3)

	task3.mu.Lock()
	status3 := task3.Status
	task3.mu.Unlock()
	if status3 != StatusPending {
		t.Fatalf("expected task 3 to be pending, got %s", status3)
	}

	// Pause the pending task 3
	GlobalQueueManager.Pause(ep3ID)

	task3.mu.Lock()
	status3AfterPause := task3.Status
	task3.mu.Unlock()
	if status3AfterPause != StatusPaused {
		t.Fatalf("expected task 3 to be paused, got %s", status3AfterPause)
	}

	// Unblock the semaphore by releasing the first two downloads
	close(blockChan)
	time.Sleep(100 * time.Millisecond)

	// Verify if the paused pending task was run anyway
	if atomic.LoadInt32(&task3Called) == 1 {
		t.Error("BUG: paused pending task was executed and downloaded anyway when slot became free!")
	}

	// Double check task 3 status remains StatusPaused
	task3.mu.Lock()
	finalStatus := task3.Status
	task3.mu.Unlock()
	if finalStatus != StatusPaused {
		t.Errorf("expected final status of task 3 to be StatusPaused, got %s", finalStatus)
	}
}
