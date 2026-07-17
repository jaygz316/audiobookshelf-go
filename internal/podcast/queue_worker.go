package podcast

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	log "audiobookshelf/internal/logger"
)

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

	q.mu.RLock()
	task.mu.Lock()
	_, exists := q.tasksMap[task.ID]
	if !exists || task.Status != StatusPending {
		task.mu.Unlock()
		q.mu.RUnlock()
		return
	}
	q.mu.RUnlock()
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
		task.mu.Unlock()
		return
	}

	task.Status = StatusFinished
	task.Progress = 100.0
	task.Speed = 0
	task.mu.Unlock()
	log.Infof("[Queue] Finished download for episode %s: %s", task.ID, task.EpisodeTitle)

	// Save to DB on finish!
	fi, statErr := os.Stat(task.DestPath)
	var sz int64
	if statErr == nil {
		sz = fi.Size()
	}

	var duration float64 = 0.0
	var existingAudioFile sql.NullString
	if err := q.db.QueryRow("SELECT audioFile FROM podcastEpisodes WHERE id = ?", task.ID).Scan(&existingAudioFile); err == nil && existingAudioFile.Valid && existingAudioFile.String != "" {
		var temp struct {
			Duration float64 `json:"duration"`
		}
		if err := json.Unmarshal([]byte(existingAudioFile.String), &temp); err == nil {
			duration = temp.Duration
		}
	}

	audioFileJSON, _ := json.Marshal(map[string]interface{}{
		"duration": duration,
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
