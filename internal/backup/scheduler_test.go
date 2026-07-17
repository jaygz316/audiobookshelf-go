package backup

import (
	"context"
	"database/sql"
	"sync"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestSchedulerLifecycle(t *testing.T) {
	database, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	_, err = database.Exec("CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT, createdAt TEXT, updatedAt TEXT)")
	if err != nil {
		t.Fatalf("failed to create settings table: %v", err)
	}

	settingsJSON := `{"backupsToKeep": 2, "backupPath": "", "backupSchedule": ""}`
	_, err = database.Exec("INSERT INTO settings (key, value) VALUES ('server-settings', ?)", settingsJSON)
	if err != nil {
		t.Fatalf("failed to insert settings: %v", err)
	}

	scheduler := &BackupScheduler{
		db:           database,
		configPath:   t.TempDir(),
		metadataPath: t.TempDir(),
	}

	// Test Start
	scheduler.Start()

	scheduler.mu.Lock()
	if scheduler.cancel == nil {
		scheduler.mu.Unlock()
		t.Error("expected cancel func to be set after Start")
	} else {
		scheduler.mu.Unlock()
	}

	// Test Reload
	scheduler.Reload()

	// Test Stop
	scheduler.Stop()

	scheduler.mu.Lock()
	if scheduler.cancel != nil {
		scheduler.mu.Unlock()
		t.Error("expected cancel func to be nil after Stop")
	} else {
		scheduler.mu.Unlock()
	}
}

func TestSchedulerConcurrency(t *testing.T) {
	database, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	_, err = database.Exec("CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT, createdAt TEXT, updatedAt TEXT)")
	if err != nil {
		t.Fatalf("failed to create settings table: %v", err)
	}

	settingsJSON := `{"backupsToKeep": 2, "backupPath": "", "backupSchedule": ""}`
	_, err = database.Exec("INSERT INTO settings (key, value) VALUES ('server-settings', ?)", settingsJSON)
	if err != nil {
		t.Fatalf("failed to insert settings: %v", err)
	}

	scheduler := &BackupScheduler{
		db:           database,
		configPath:   t.TempDir(),
		metadataPath: t.TempDir(),
	}

	var wg sync.WaitGroup
	numWorkers := 20

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 5; j++ {
				switch (id + j) % 3 {
				case 0:
					scheduler.Start()
				case 1:
					scheduler.Reload()
				case 2:
					scheduler.Stop()
				}
				time.Sleep(1 * time.Millisecond)
			}
		}(i)
	}

	wg.Wait()
	scheduler.Stop()
}

func TestSchedulerCheckAndRun(t *testing.T) {
	database, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	_, err = database.Exec("CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT, createdAt TEXT, updatedAt TEXT)")
	if err != nil {
		t.Fatalf("failed to create settings table: %v", err)
	}

	settingsJSON := `{"backupsToKeep": 2, "backupPath": "", "backupSchedule": "* * * * *"}`
	_, err = database.Exec("INSERT INTO settings (key, value) VALUES ('server-settings', ?)", settingsJSON)
	if err != nil {
		t.Fatalf("failed to insert settings: %v", err)
	}

	configDir := t.TempDir()
	metadataDir := t.TempDir()

	scheduler := &BackupScheduler{
		db:           database,
		configPath:   configDir,
		metadataPath: metadataDir,
	}

	ctx := context.Background()

	// Run once
	scheduler.checkAndRun(ctx)

	// Wait for the background backup goroutine to finish
	time.Sleep(20 * time.Millisecond)
	BackupRestoreMu.Lock()
	BackupRestoreMu.Unlock()

	scheduler.mu.Lock()
	runTime1 := scheduler.lastRunTime
	scheduler.mu.Unlock()

	if runTime1.IsZero() {
		t.Error("expected lastRunTime to be set after checkAndRun")
	}

	// Run again immediately - should not change lastRunTime since now.Truncate(time.Minute) is the same.
	scheduler.checkAndRun(ctx)

	scheduler.mu.Lock()
	runTime2 := scheduler.lastRunTime
	scheduler.mu.Unlock()

	if !runTime1.Equal(runTime2) {
		t.Errorf("expected lastRunTime to remain unchanged, but got %v and %v", runTime1, runTime2)
	}
}
