package backup

import (
	"context"
	"database/sql"
	"sync"
	"time"

	log "audiobookshelf/internal/logger"
)

// GlobalScheduler is the singleton instance of the backup scheduler.
var GlobalScheduler *BackupScheduler

// BackupScheduler manages background scheduled backups.
type BackupScheduler struct {
	db           *sql.DB
	configPath   string
	metadataPath string

	mu          sync.Mutex
	lifecycleMu sync.Mutex
	ctx         context.Context
	cancel      context.CancelFunc
	lastRunTime time.Time
	wg          sync.WaitGroup
}

// InitScheduler initializes and starts the backup scheduler.
func InitScheduler(database *sql.DB, configPath, metadataPath string) {
	GlobalScheduler = &BackupScheduler{
		db:           database,
		configPath:   configPath,
		metadataPath: metadataPath,
	}
	GlobalScheduler.Start()
}

// Start starts the background scheduler loop.
func (s *BackupScheduler) Start() {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()

	s.mu.Lock()
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
		s.mu.Unlock()
		s.wg.Wait()
	} else {
		s.mu.Unlock()
	}

	s.mu.Lock()
	s.ctx, s.cancel = context.WithCancel(context.Background())
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.loop(s.ctx)
	}()
	s.mu.Unlock()
	log.Printf("[Backup Scheduler] Started background scheduler")
}

// Stop stops the background scheduler loop.
func (s *BackupScheduler) Stop() {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()

	s.mu.Lock()
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
		s.mu.Unlock()
		s.wg.Wait()
	} else {
		s.mu.Unlock()
	}
	log.Printf("[Backup Scheduler] Stopped background scheduler")
}

// Reload restarts the scheduler and re-evaluates settings.
func (s *BackupScheduler) Reload() {
	s.Start()
}
