package podcast

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

func TestChallenger_QueueStateTransitions(t *testing.T) {
	blockChan := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/mpeg")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("start-chunk"))
		<-blockChan
		_, _ = w.Write([]byte("end-chunk"))
	}))
	defer server.Close()

	db, _, tempDir := setupQueueTest(t, server.URL)
	defer db.Close()

	epID := "ep-transition"
	_, _ = db.Exec(`INSERT INTO podcastEpisodes (id, podcastId, title, audioFile) VALUES (?, ?, ?, ?)`,
		epID, "pod-1", "Transition Episode", "{}")

	dest := filepath.Join(tempDir, "ep-transition.mp3")
	task := &DownloadTask{
		ID:           epID,
		PodcastID:    "pod-1",
		EpisodeTitle: "Transition Episode",
		EnclosureURL: server.URL,
		DestPath:     dest,
	}

	GlobalQueueManager.Enqueue(task)
	if !waitForStatus(task, StatusDownloading, 200*time.Millisecond) {
		t.Fatalf("expected status to transition to downloading, got %s", task.Status)
	}

	GlobalQueueManager.Pause(epID)
	if !waitForStatus(task, StatusPaused, 200*time.Millisecond) {
		t.Fatalf("expected status to transition to paused, got %s", task.Status)
	}

	GlobalQueueManager.Resume(epID)
	if !waitForStatus(task, StatusDownloading, 200*time.Millisecond) {
		t.Fatalf("expected status to transition to downloading after resume, got %s", task.Status)
	}

	close(blockChan)
	if !waitForStatus(task, StatusFinished, 200*time.Millisecond) {
		t.Fatalf("expected status to transition to finished, got %s", task.Status)
	}

	var audioFile string
	err := db.QueryRow("SELECT audioFile FROM podcastEpisodes WHERE id = ?", epID).Scan(&audioFile)
	if err != nil {
		t.Fatalf("failed to query DB: %v", err)
	}

	var meta map[string]interface{}
	if err := json.Unmarshal([]byte(audioFile), &meta); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if meta["metadata"] == nil {
		t.Fatalf("expected metadata in audioFile, got %s", audioFile)
	}
}

func TestChallenger_QueueFailedState(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	db, _, tempDir := setupQueueTest(t, server.URL)
	defer db.Close()

	epID := "ep-failed"
	_, _ = db.Exec(`INSERT INTO podcastEpisodes (id, podcastId, title, audioFile) VALUES (?, ?, ?, ?)`,
		epID, "pod-1", "Failed Episode", "{}")

	dest := filepath.Join(tempDir, "ep-failed.mp3")
	task := &DownloadTask{
		ID:           epID,
		PodcastID:    "pod-1",
		EpisodeTitle: "Failed Episode",
		EnclosureURL: server.URL,
		DestPath:     dest,
	}

	GlobalQueueManager.Enqueue(task)
	if !waitForStatus(task, StatusFailed, 200*time.Millisecond) {
		t.Fatalf("expected status to transition to failed, got %s", task.Status)
	}

	task.mu.Lock()
	errStr := task.Error
	task.mu.Unlock()
	if errStr == "" {
		t.Error("expected error message to be set on task")
	}
}
