package backup

import (
	"context"
	"database/sql"
	"strconv"
	"strings"
	"sync"
	"time"

	"audiobookshelf/internal/db"
	log "audiobookshelf/internal/logger"
)

// GlobalScheduler is the singleton instance of the backup scheduler.
var GlobalScheduler *BackupScheduler

// BackupScheduler manages background scheduled backups.
type BackupScheduler struct {
	db           *sql.DB
	configPath   string
	metadataPath string

	mu           sync.Mutex
	ctx          context.Context
	cancel       context.CancelFunc
	lastRunTime  time.Time
	currentSched string
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
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cancel != nil {
		s.cancel()
	}

	s.ctx, s.cancel = context.WithCancel(context.Background())
	go s.loop(s.ctx)
	log.Printf("[Backup Scheduler] Started background scheduler")
}

// Stop stops the background scheduler loop.
func (s *BackupScheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	log.Printf("[Backup Scheduler] Stopped background scheduler")
}

// Reload restarts the scheduler and re-evaluates settings.
func (s *BackupScheduler) Reload() {
	s.Start()
}

func (s *BackupScheduler) loop(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
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

	if len(parts) == 5 {
		// Truncate to minute
		checkTime = now.Truncate(time.Minute)
	} else if len(parts) == 6 {
		// Truncate to second
		checkTime = now.Truncate(time.Second)
	} else {
		return // Invalid schedule format
	}

	s.mu.Lock()
	alreadyRan := !checkTime.After(s.lastRunTime)
	s.mu.Unlock()

	if alreadyRan {
		return
	}

	if MatchCron(schedule, checkTime) {
		s.mu.Lock()
		s.lastRunTime = checkTime
		s.mu.Unlock()

		log.Printf("[Backup Scheduler] Scheduled backup triggered by schedule: %s", schedule)
		backupDir := GetBackupDirPath(ctx, s.db, s.metadataPath)
		_, err := CreateBackup(ctx, s.db, s.configPath, s.metadataPath, backupDir)
		if err != nil {
			log.Printf("[Backup Scheduler] Scheduled backup failed: %v", err)
		} else {
			log.Printf("[Backup Scheduler] Scheduled backup completed successfully")
		}
	}
}

// MatchCron evaluates if the given time matches the cron expression.
func MatchCron(expression string, t time.Time) bool {
	parts := strings.Fields(expression)
	if len(parts) != 5 && len(parts) != 6 {
		return false
	}

	var secPart, minPart, hourPart, domPart, monthPart, dowPart string
	if len(parts) == 5 {
		secPart = "0"
		minPart = parts[0]
		hourPart = parts[1]
		domPart = parts[2]
		monthPart = parts[3]
		dowPart = parts[4]
	} else {
		secPart = parts[0]
		minPart = parts[1]
		hourPart = parts[2]
		domPart = parts[3]
		monthPart = parts[4]
		dowPart = parts[5]
	}

	if !matchCronField(secPart, t.Second()) {
		return false
	}
	if !matchCronField(minPart, t.Minute()) {
		return false
	}
	if !matchCronField(hourPart, t.Hour()) {
		return false
	}
	if !matchCronField(domPart, t.Day()) {
		return false
	}
	if !matchCronField(monthPart, int(t.Month())) {
		return false
	}

	dowVal := int(t.Weekday())
	if dowPart != "*" {
		dowPartNormalized := strings.ReplaceAll(dowPart, "7", "0")
		if !matchCronField(dowPartNormalized, dowVal) {
			return false
		}
	}

	return true
}

func matchCronField(field string, value int) bool {
	if field == "*" {
		return true
	}
	if strings.Contains(field, ",") {
		parts := strings.Split(field, ",")
		for _, part := range parts {
			if matchCronField(part, value) {
				return true
			}
		}
		return false
	}
	if strings.Contains(field, "/") {
		parts := strings.Split(field, "/")
		if len(parts) != 2 {
			return false
		}
		step, err := strconv.Atoi(parts[1])
		if err != nil {
			return false
		}
		rangePart := parts[0]
		if rangePart == "*" {
			return value%step == 0
		}
		start, end, ok := parseCronRange(rangePart)
		if !ok {
			return false
		}
		return value >= start && value <= end && (value-start)%step == 0
	}
	if strings.Contains(field, "-") {
		start, end, ok := parseCronRange(field)
		if !ok {
			return false
		}
		return value >= start && value <= end
	}
	val, err := strconv.Atoi(field)
	if err != nil {
		return false
	}
	return val == value
}

func parseCronRange(field string) (int, int, bool) {
	parts := strings.Split(field, "-")
	if len(parts) != 2 {
		return 0, 0, false
	}
	start, err1 := strconv.Atoi(parts[0])
	end, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	return start, end, true
}
