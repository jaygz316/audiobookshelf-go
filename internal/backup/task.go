package backup

import (
	"context"
	"strings"
	"time"

	"audiobookshelf/internal/db"
	log "audiobookshelf/internal/logger"
)

func (s *BackupScheduler) loop(ctx context.Context) {
	// Retrieve initial settings to determine dynamic ticker duration.
	settings, err := db.GetServerSettings(s.db)
	var schedule string
	if err == nil {
		schedule = strings.TrimSpace(string(settings.BackupSchedule))
	}

	tickerDuration := 5 * time.Second
	parts := strings.Fields(schedule)
	if len(parts) == 6 {
		tickerDuration = 1 * time.Second
	}

	ticker := time.NewTicker(tickerDuration)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.checkAndRun(ctx)
		}
	}
}

func (s *BackupScheduler) checkAndRun(ctx context.Context) {
	settings, err := db.GetServerSettings(s.db)
	if err != nil {
		return
	}

	schedule := strings.TrimSpace(string(settings.BackupSchedule))
	if schedule == "" {
		return
	}

	now := time.Now()
	parts := strings.Fields(schedule)
	var checkTime time.Time
	var R time.Duration

	if len(parts) == 5 {
		checkTime = now.Truncate(time.Minute)
		R = time.Minute
	} else if len(parts) == 6 {
		checkTime = now.Truncate(time.Second)
		R = time.Second
	} else {
		return // Invalid schedule format
	}

	s.mu.Lock()
	if s.lastRunTime.IsZero() {
		s.lastRunTime = checkTime.Add(-R)
	}

	limit := 15 * time.Minute
	if len(parts) == 5 {
		limit = 24 * time.Hour
	}
	if checkTime.Sub(s.lastRunTime) > limit {
		s.lastRunTime = checkTime.Add(-limit)
	}

	var triggerTime time.Time
	matches := false

	// Check each unit of resolution R from lastRunTime+R to checkTime
	for t := s.lastRunTime.Add(R); !t.After(checkTime); t = t.Add(R) {
		if MatchCron(schedule, t) {
			matches = true
			triggerTime = t
			break
		}
	}

	s.lastRunTime = checkTime
	s.mu.Unlock()

	if !matches {
		return
	}

	log.Printf("[Backup Scheduler] Scheduled backup triggered by schedule: %s (matching time: %v)", schedule, triggerTime)

	// Spawn the backup operation in a separate goroutine to prevent blocking the scheduler loop
	go func() {
		backupDir := GetBackupDirPath(context.Background(), s.db, s.metadataPath)
		_, err := CreateBackup(context.Background(), s.db, s.configPath, s.metadataPath, backupDir)
		if err != nil {
			log.Printf("[Backup Scheduler] Scheduled backup failed: %v", err)
		} else {
			log.Printf("[Backup Scheduler] Scheduled backup completed successfully")
		}
	}()
}
