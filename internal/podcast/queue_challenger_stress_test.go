package podcast

import (
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestChallenger_QueueStressHighLoad(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/mpeg")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("audio-chunk-data"))
		time.Sleep(5 * time.Millisecond)
	}))
	defer server.Close()

	db, _, tempDir := setupQueueTest(t, server.URL)
	defer db.Close()

	const numTasks = 50
	const numWorkers = 10

	for i := 0; i < numTasks; i++ {
		epID := fmt.Sprintf("ep-stress-%d", i)
		_, _ = db.Exec(`INSERT INTO podcastEpisodes (id, podcastId, title, audioFile) VALUES (?, ?, ?, ?)`,
			epID, "pod-1", fmt.Sprintf("Stress Ep %d", i), "{}")
	}

	var wg sync.WaitGroup
	startSignal := make(chan struct{})

	for i := 0; i < numTasks; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			<-startSignal

			epID := fmt.Sprintf("ep-stress-%d", id)
			task := &DownloadTask{
				ID:           epID,
				PodcastID:    "pod-1",
				EpisodeTitle: fmt.Sprintf("Stress Ep %d", id),
				EnclosureURL: server.URL,
				DestPath:     filepath.Join(tempDir, fmt.Sprintf("stress-%d.mp3", id)),
			}
			GlobalQueueManager.Enqueue(task)
		}(i)
	}

	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			<-startSignal

			rng := rand.New(rand.NewSource(int64(workerID)))
			for step := 0; step < 50; step++ {
				_ = GlobalQueueManager.GetTasks()

				targetID := fmt.Sprintf("ep-stress-%d", rng.Intn(numTasks))
				action := rng.Intn(4)
				switch action {
				case 0:
					GlobalQueueManager.Pause(targetID)
				case 1:
					GlobalQueueManager.Resume(targetID)
				case 2:
					GlobalQueueManager.Cancel(targetID)
				case 3:
					time.Sleep(1 * time.Millisecond)
				}
				time.Sleep(1 * time.Millisecond)
			}
		}(w)
	}

	close(startSignal)
	wg.Wait()

	GlobalQueueManager.CancelAll()

	// Wait for any leaked/pending tasks that run due to the cancellation bug
	// to complete or fail before exiting the test, preventing directory cleanup failures.
	time.Sleep(150 * time.Millisecond)
}
