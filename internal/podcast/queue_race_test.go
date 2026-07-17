package podcast

import (
	"database/sql"
	"sync"
	"testing"
	"time"
)

func TestQueueManager_EnqueueRace(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	// Minimal schema
	_, _ = db.Exec(`CREATE TABLE podcastEpisodes (id TEXT PRIMARY KEY, podcastId TEXT, title TEXT, audioFile TEXT)`)
	_, _ = db.Exec(`CREATE TABLE podcasts (id TEXT PRIMARY KEY, title TEXT, feedURL TEXT, autoDownloadEpisodes INTEGER, maxEpisodesToKeep INTEGER, maxNewEpisodesToDownload INTEGER, autoDeletePlayed INTEGER)`)

	pm := NewPodcastManager(db)
	InitQueueManager(db, pm)

	task := &DownloadTask{
		ID:           "test-task-1",
		PodcastID:    "pod-1",
		EpisodeTitle: "Test Episode",
		EnclosureURL: "http://example.com/audio.mp3",
		Status:       StatusFinished,
	}

	GlobalQueueManager.tasks = append(GlobalQueueManager.tasks, task)
	GlobalQueueManager.tasksMap[task.ID] = task

	// We concurrently run Enqueue and GetTasks
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			// Enqueue should find the task in StatusFinished,
			// reset it, and call processTask.
			// Let's modify status back and forth to keep Enqueue triggerable
			task.mu.Lock()
			task.Status = StatusFinished
			task.mu.Unlock()
			GlobalQueueManager.Enqueue(task)
			time.Sleep(1 * time.Millisecond)
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			_ = GlobalQueueManager.GetTasks()
			time.Sleep(1 * time.Millisecond)
		}
	}()

	wg.Wait()
}
