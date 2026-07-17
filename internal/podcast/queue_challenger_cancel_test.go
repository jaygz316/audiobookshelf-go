package podcast

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestChallenger_QueueCancellationCleanup(t *testing.T) {
	blockChan := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/mpeg")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("some-partial-data"))
		<-blockChan
	}))
	defer server.Close()

	db, _, tempDir := setupQueueTest(t, server.URL)
	defer db.Close()

	epID := "ep-cancel"
	_, _ = db.Exec(`INSERT INTO podcastEpisodes (id, podcastId, title, audioFile) VALUES (?, ?, ?, ?)`,
		epID, "pod-1", "Cancel Episode", "{}")

	dest := filepath.Join(tempDir, "ep-cancel.mp3")
	task := &DownloadTask{
		ID:           epID,
		PodcastID:    "pod-1",
		EpisodeTitle: "Cancel Episode",
		EnclosureURL: server.URL,
		DestPath:     dest,
	}

	GlobalQueueManager.Enqueue(task)
	if !waitForStatus(task, StatusDownloading, 200*time.Millisecond) {
		t.Fatalf("expected status to transition to downloading, got %s", task.Status)
	}

	GlobalQueueManager.Cancel(epID)

	GlobalQueueManager.mu.RLock()
	_, exists := GlobalQueueManager.tasksMap[epID]
	var inTasksList bool
	for _, tTask := range GlobalQueueManager.tasks {
		if tTask.ID == epID {
			inTasksList = true
		}
	}
	GlobalQueueManager.mu.RUnlock()

	if exists {
		t.Error("expected task to be removed from tasksMap")
	}
	if inTasksList {
		t.Error("expected task to be removed from tasks slice")
	}

	close(blockChan)
	time.Sleep(20 * time.Millisecond)
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Error("expected partial file to be cleaned up from disk")
	}
}

func TestChallenger_QueueCancelAll(t *testing.T) {
	blockChan := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/mpeg")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("some-partial-data"))
		<-blockChan
	}))
	defer server.Close()

	db, _, tempDir := setupQueueTest(t, server.URL)
	defer db.Close()

	epIDs := []string{"ep-c1", "ep-c2"}
	dests := []string{
		filepath.Join(tempDir, "ep-c1.mp3"),
		filepath.Join(tempDir, "ep-c2.mp3"),
	}

	for i, epID := range epIDs {
		_, _ = db.Exec(`INSERT INTO podcastEpisodes (id, podcastId, title, audioFile) VALUES (?, ?, ?, ?)`,
			epID, "pod-1", epID, "{}")

		task := &DownloadTask{
			ID:           epID,
			PodcastID:    "pod-1",
			EpisodeTitle: epID,
			EnclosureURL: server.URL,
			DestPath:     dests[i],
		}
		GlobalQueueManager.Enqueue(task)
	}

	time.Sleep(50 * time.Millisecond)
	GlobalQueueManager.CancelAll()

	GlobalQueueManager.mu.RLock()
	tasksLen := len(GlobalQueueManager.tasks)
	mapLen := len(GlobalQueueManager.tasksMap)
	GlobalQueueManager.mu.RUnlock()

	if tasksLen != 0 || mapLen != 0 {
		t.Errorf("expected queue to be empty, got tasks=%d, map=%d", tasksLen, mapLen)
	}

	close(blockChan)
	time.Sleep(20 * time.Millisecond)

	for _, dest := range dests {
		if _, err := os.Stat(dest); !os.IsNotExist(err) {
			t.Errorf("expected partial file %s to be cleaned up", dest)
		}
	}
}

func TestChallenger_CancelPendingTask(t *testing.T) {
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

	// Cancel the pending task 3
	GlobalQueueManager.Cancel(ep3ID)

	// Unblock the semaphore by releasing the first two downloads
	close(blockChan)
	time.Sleep(100 * time.Millisecond)

	// Verify if the cancelled pending task was run anyway
	if atomic.LoadInt32(&task3Called) == 1 {
		t.Error("BUG: cancelled pending task was executed and downloaded anyway!")
	}
}
