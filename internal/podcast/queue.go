package podcast

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	log "audiobookshelf/internal/logger"
)

type DownloadStatus string

const (
	StatusPending     DownloadStatus = "pending"
	StatusDownloading DownloadStatus = "downloading"
	StatusPaused      DownloadStatus = "paused"
	StatusFinished    DownloadStatus = "finished"
	StatusFailed      DownloadStatus = "failed"
)

type DownloadTask struct {
	ID              string         `json:"id"` // Episode ID
	PodcastID       string         `json:"podcastId"`
	PodcastTitle    string         `json:"podcastTitle"`
	EpisodeTitle    string         `json:"episodeTitle"`
	EnclosureURL    string         `json:"enclosureUrl"`
	DestPath        string         `json:"destPath"`
	Status          DownloadStatus `json:"status"`
	Progress        float64        `json:"progress"`
	Speed           float64        `json:"speed"`
	BytesDownloaded int64          `json:"bytesDownloaded"`
	BytesTotal      int64          `json:"bytesTotal"`
	Error           string         `json:"error,omitempty"`

	cancelFunc context.CancelFunc
	mu         sync.Mutex
	startTime  time.Time
}

type QueueManager struct {
	tasks    []*DownloadTask
	tasksMap map[string]*DownloadTask
	mu       sync.RWMutex
	db       *sql.DB
	pm       *PodcastManager
	sem      chan struct{} // worker limit semaphore
}

var GlobalQueueManager *QueueManager

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
		// If it's already in the queue, only reset if it failed/finished/paused
		if existing.Status == StatusFailed || existing.Status == StatusFinished || existing.Status == StatusPaused {
			existing.Status = StatusPending
			existing.Progress = 0
			existing.Speed = 0
			existing.BytesDownloaded = 0
			existing.BytesTotal = 0
			existing.Error = ""
			q.mu.Unlock()
			go q.processTask(existing)
			return
		}
		q.mu.Unlock()
		return
	}

	task.Status = StatusPending
	q.tasks = append(q.tasks, task)
	q.tasksMap[task.ID] = task
	q.mu.Unlock()

	go q.processTask(task)
}

func (q *QueueManager) processTask(task *DownloadTask) {
	// Acquire semaphore slot
	select {
	case q.sem <- struct{}{}:
	default:
		// If we can't acquire immediately, keep status as pending
		// We'll block until slot is available
		q.sem <- struct{}{}
	}
	defer func() {
		<-q.sem
	}()

	task.mu.Lock()
	if task.Status != StatusPending {
		task.mu.Unlock()
		return
	}
	task.Status = StatusDownloading
	task.startTime = time.Now()
	ctx, cancel := context.WithCancel(context.Background())
	task.cancelFunc = cancel
	task.mu.Unlock()

	log.Infof("[Queue] Starting download for episode %s: %s", task.ID, task.EpisodeTitle)

	var lastTime time.Time = time.Now()
	var lastBytes int64 = 0

	err := q.pm.DownloadEpisodeWithProgress(ctx, task.EnclosureURL, task.DestPath, func(downloaded, total int64) {
		task.mu.Lock()
		defer task.mu.Unlock()
		if task.Status != StatusDownloading {
			return
		}
		task.BytesDownloaded = downloaded
		task.BytesTotal = total
		if total > 0 {
			task.Progress = float64(downloaded) * 100.0 / float64(total)
		}

		now := time.Now()
		elapsed := now.Sub(lastTime).Seconds()
		if elapsed >= 0.5 { // Update speed at most every 0.5s
			diffBytes := downloaded - lastBytes
			task.Speed = float64(diffBytes) / elapsed
			lastTime = now
			lastBytes = downloaded
		}
	})

	task.mu.Lock()
	defer task.mu.Unlock()

	if err != nil {
		if ctx.Err() == context.Canceled {
			task.Status = StatusPaused
			task.Speed = 0
			log.Infof("[Queue] Paused download for episode %s: %s", task.ID, task.EpisodeTitle)
		} else {
			task.Status = StatusFailed
			task.Speed = 0
			task.Error = err.Error()
			log.Errorf("[Queue] Failed download for episode %s: %v", task.ID, err)
		}
		return
	}

	task.Status = StatusFinished
	task.Progress = 100.0
	task.Speed = 0
	log.Infof("[Queue] Finished download for episode %s: %s", task.ID, task.EpisodeTitle)

	// Save to DB on finish!
	fi, statErr := os.Stat(task.DestPath)
	var sz int64
	if statErr == nil {
		sz = fi.Size()
	}

	audioFileJSON, _ := json.Marshal(map[string]interface{}{
		"duration": 0,
		"mimeType": "audio/mpeg",
		"metadata": map[string]interface{}{
			"path":     task.DestPath,
			"filename": filepath.Base(task.DestPath),
			"size":     sz,
		},
	})

	_, dbErr := q.db.Exec(`
		UPDATE podcastEpisodes
		SET audioFile = ?
		WHERE id = ?
	`, string(audioFileJSON), task.ID)
	if dbErr != nil {
		log.Errorf("[Queue] Failed to update episode in DB: %v", dbErr)
	} else {
		var maxKeep int
		_ = q.db.QueryRow("SELECT maxEpisodesToKeep FROM podcasts WHERE id = ?", task.PodcastID).Scan(&maxKeep)
		if maxKeep > 0 {
			if err := q.pm.EnforceRetentionPolicy(context.Background(), task.PodcastID, maxKeep); err != nil {
				log.Errorf("[Queue] Failed to enforce retention policy: %v", err)
			}
		}
	}
}

func (q *QueueManager) Pause(taskID string) {
	q.mu.RLock()
	task, exists := q.tasksMap[taskID]
	q.mu.RUnlock()

	if exists {
		task.mu.Lock()
		if task.Status == StatusDownloading {
			if task.cancelFunc != nil {
				task.cancelFunc()
			}
		}
		task.mu.Unlock()
	}
}

func (q *QueueManager) Resume(taskID string) {
	q.mu.RLock()
	task, exists := q.tasksMap[taskID]
	q.mu.RUnlock()

	if exists {
		task.mu.Lock()
		if task.Status == StatusPaused || task.Status == StatusFailed {
			task.Status = StatusPending
			task.mu.Unlock()
			go q.processTask(task)
			return
		}
		task.mu.Unlock()
	}
}

func (q *QueueManager) Cancel(taskID string) {
	q.mu.Lock()
	task, exists := q.tasksMap[taskID]
	if exists {
		// Remove from slice
		for i, t := range q.tasks {
			if t.ID == taskID {
				q.tasks = append(q.tasks[:i], q.tasks[i+1:]...)
				break
			}
		}
		delete(q.tasksMap, taskID)
	}
	q.mu.Unlock()

	if exists {
		task.mu.Lock()
		if task.Status == StatusDownloading && task.cancelFunc != nil {
			task.cancelFunc()
		}
		task.mu.Unlock()
		// Clean up partial file
		_ = os.Remove(task.DestPath)
	}
}

func (q *QueueManager) CancelAll() {
	q.mu.Lock()
	tasksToCancel := make([]*DownloadTask, len(q.tasks))
	copy(tasksToCancel, q.tasks)
	q.tasks = nil
	q.tasksMap = make(map[string]*DownloadTask)
	q.mu.Unlock()

	for _, task := range tasksToCancel {
		task.mu.Lock()
		if task.Status == StatusDownloading && task.cancelFunc != nil {
			task.cancelFunc()
		}
		task.mu.Unlock()
		_ = os.Remove(task.DestPath)
	}
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
