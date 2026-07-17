package podcast

import (
	"database/sql"
	"fmt"
)

func InitQueueManager(db *sql.DB, pm *PodcastManager) {
	GlobalQueueManager = &QueueManager{
		tasksMap: make(map[string]*DownloadTask),
		db:       db,
		pm:       pm,
		sem:      make(chan struct{}, 2), // max 2 concurrent downloads
	}
}

func (q *QueueManager) Enqueue(task *DownloadTask) {
	q.mu.Lock()
	if existing, exists := q.tasksMap[task.ID]; exists {
		existing.mu.Lock()
		// If it's already in the queue, only reset if it failed/finished/paused
		if existing.Status == StatusFailed || existing.Status == StatusFinished || existing.Status == StatusPaused {
			existing.Status = StatusPending
			existing.Progress = 0
			existing.Speed = 0
			existing.BytesDownloaded = 0
			existing.BytesTotal = 0
			existing.Error = ""
			existing.mu.Unlock()
			q.mu.Unlock()
			go q.processTask(existing)
			return
		}
		existing.mu.Unlock()
		q.mu.Unlock()
		return
	}

	task.Status = StatusPending
	q.tasks = append(q.tasks, task)
	q.tasksMap[task.ID] = task
	q.mu.Unlock()

	go q.processTask(task)
}

func (q *QueueManager) GetTasks() []map[string]interface{} {
	q.mu.RLock()
	defer q.mu.RUnlock()

	var result []map[string]interface{}
	for _, t := range q.tasks {
		t.mu.Lock()
		taskType := "download" // original type
		desc := fmt.Sprintf("Downloading podcast episode: %s", t.EpisodeTitle)
		taskMap := map[string]interface{}{
			"id":              t.ID,
			"type":            taskType,
			"name":            taskType, // original name field
			"description":     desc,
			"status":          string(t.Status),
			"progress":        t.Progress,
			"speed":           t.Speed,
			"bytesDownloaded": t.BytesDownloaded,
			"bytesTotal":      t.BytesTotal,
			"podcastId":       t.PodcastID,
			"podcastTitle":    t.PodcastTitle,
			"episodeTitle":    t.EpisodeTitle,
			"error":           t.Error,
		}
		t.mu.Unlock()
		result = append(result, taskMap)
	}
	return result
}
