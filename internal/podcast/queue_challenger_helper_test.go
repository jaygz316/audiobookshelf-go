package podcast

import (
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func setupQueueTest(t *testing.T, serverURL string) (*sql.DB, *PodcastManager, string) {
	db := setupTestDB(t, false)
	m := NewPodcastManager(db)
	configureTestClient(t, m, serverURL)
	InitQueueManager(db, m)

	_, _ = db.Exec(`INSERT INTO podcasts (id, title, feedURL, autoDownloadEpisodes, maxEpisodesToKeep, maxNewEpisodesToDownload) VALUES (?, ?, ?, ?, ?, ?)`,
		"pod-1", "Test Podcast", serverURL, 0, 0, 0)

	tempDir := t.TempDir()
	return db, m, tempDir
}

func waitForStatus(task *DownloadTask, expected DownloadStatus, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		task.mu.Lock()
		status := task.Status
		task.mu.Unlock()
		if status == expected {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return false
}
