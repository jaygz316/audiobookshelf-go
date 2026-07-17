package podcast

import (
	"context"
	"database/sql"
	"sync"
	"time"
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
