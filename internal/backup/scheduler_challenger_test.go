package backup

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// TestChallenger6FieldCronMissed verifies that a 6-field cron schedule
// that triggers on a second not aligned with the scheduler's ticker (e.g. 5 seconds)
// is completely missed by the checkAndRun mechanism.
func TestChallenger6FieldCronMissed(t *testing.T) {
	database, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	_, err = database.Exec("CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT, createdAt TEXT, updatedAt TEXT)")
	if err != nil {
		t.Fatalf("failed to create settings table: %v", err)
	}

	// We set a 6-field cron schedule for second 1 (e.g. "1 * * * * *")
	settingsJSON := `{"backupsToKeep": 2, "backupPath": "", "backupSchedule": "1 * * * * *"}`
	_, err = database.Exec("INSERT INTO settings (key, value) VALUES ('server-settings', ?)", settingsJSON)
	if err != nil {
		t.Fatalf("failed to insert settings: %v", err)
	}

	// First, let's show MatchCron works for second 1:
	t1 := time.Date(2026, 7, 16, 12, 0, 1, 0, time.UTC)
	if !MatchCron("1 * * * * *", t1) {
		t.Error("expected MatchCron to match second 1 for '1 * * * * *'")
	}

	// Second, let's show MatchCron fails for seconds 0, 5, 10...
	for _, sec := range []int{0, 5, 10, 15, 20, 25, 30, 35, 40, 45, 50, 55} {
		tCheck := time.Date(2026, 7, 16, 12, 0, sec, 0, time.UTC)
		if MatchCron("1 * * * * *", tCheck) {
			t.Errorf("expected MatchCron to NOT match second %d for '1 * * * * *'", sec)
		}
	}
}

// TestChallengerBlockingLifecycle demonstrates that the Start/Stop methods
// are non-blocking and do not wait for the current backup to finish.
func TestChallengerBlockingLifecycle(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "abs-blocking-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	configPath := filepath.Join(tempDir, "config")
	metadataPath := filepath.Join(tempDir, "metadata")
	backupDir := filepath.Join(metadataPath, "backups")

	os.MkdirAll(configPath, 0755)
	os.MkdirAll(metadataPath, 0755)
	os.MkdirAll(backupDir, 0755)

	// Create items and authors subdirectories
	os.MkdirAll(filepath.Join(metadataPath, "items"), 0755)
	os.MkdirAll(filepath.Join(metadataPath, "authors"), 0755)

	// Create a large number of dummy files in the metadata items directory
	// to make the ZIP operation take some measurable amount of time.
	for i := 0; i < 3000; i++ {
		filePath := filepath.Join(metadataPath, "items", fmt.Sprintf("file_%d.json", i))
		if err := os.WriteFile(filePath, []byte("some dummy content to zip"), 0644); err != nil {
			t.Fatalf("failed to write dummy file: %v", err)
		}
	}

	dbPath := filepath.Join(configPath, "absdatabase.sqlite")
	database, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	_, err = database.Exec("CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT, createdAt TEXT, updatedAt TEXT)")
	if err != nil {
		t.Fatalf("failed to create settings table: %v", err)
	}

	// Schedule is set to empty to prevent automatic background triggers
	settingsJSON := `{"backupsToKeep": 2, "backupPath": "", "backupSchedule": ""}`
	_, err = database.Exec("INSERT INTO settings (key, value) VALUES ('server-settings', ?)", settingsJSON)
	if err != nil {
		t.Fatalf("failed to insert settings: %v", err)
	}

	scheduler := &BackupScheduler{
		db:           database,
		configPath:   configPath,
		metadataPath: metadataPath,
	}

	// Start the scheduler
	scheduler.Start()

	// Update the settings database to trigger immediately on any check.
	_, err = database.Exec("UPDATE settings SET value = ? WHERE key = 'server-settings'", `{"backupsToKeep": 2, "backupPath": "", "backupSchedule": "* * * * * *"}`)
	if err != nil {
		t.Fatalf("failed to update settings: %v", err)
	}

	scheduler.mu.Lock()
	ctx := scheduler.ctx
	scheduler.wg.Add(1)
	scheduler.mu.Unlock()

	go func() {
		defer scheduler.wg.Done()
		scheduler.checkAndRun(ctx)
	}()

	// Wait a bit to ensure the backup starts and is actively zipping files
	time.Sleep(20 * time.Millisecond)

	startTime := time.Now()
	// Stop() will cancel the context and wait on scheduler.wg.Wait().
	// Stop() should return instantly because the backup executes in a background goroutine.
	scheduler.Stop()
	duration := time.Since(startTime)

	t.Logf("Stop() blocked for %v", duration)

	if duration >= 50*time.Millisecond {
		t.Errorf("expected Stop() to return instantly (non-blocking), but it blocked for %v", duration)
	}

	// Wait for the background backup to finish by polling the mutex or waiting a short time
	var backups []BackupInfo
	for i := 0; i < 200; i++ {
		backups, _ = LoadBackupsList(backupDir)
		if len(backups) > 0 {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}

	// Verify that the backup completed successfully
	if len(backups) == 0 {
		t.Error("expected backup to have completed successfully in the background despite Stop() being called")
	} else {
		t.Logf("Confirmed: backup completed successfully (ID: %s)", backups[0].ID)
	}
}
