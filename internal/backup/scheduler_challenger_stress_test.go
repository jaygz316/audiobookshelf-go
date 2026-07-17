package backup

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// TestChallengerStressConcurrentOperations stress tests the scheduler under concurrent Start/Stop/Reload operations
// while database settings are being updated and backups are being triggered.
func TestChallengerStressConcurrentOperations(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "abs-stress-test")
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

	os.MkdirAll(filepath.Join(metadataPath, "items"), 0755)
	os.MkdirAll(filepath.Join(metadataPath, "authors"), 0755)

	// Create a few dummy files
	for i := 0; i < 10; i++ {
		filePath := filepath.Join(metadataPath, "items", fmt.Sprintf("file_%d.json", i))
		_ = os.WriteFile(filePath, []byte("dummy data"), 0644)
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

	settingsJSON := `{"backupsToKeep": 2, "backupPath": "", "backupSchedule": "* * * * * *"}`
	_, err = database.Exec("INSERT INTO settings (key, value) VALUES ('server-settings', ?)", settingsJSON)
	if err != nil {
		t.Fatalf("failed to insert settings: %v", err)
	}

	scheduler := &BackupScheduler{
		db:           database,
		configPath:   configPath,
		metadataPath: metadataPath,
	}

	scheduler.Start()
	defer scheduler.Stop()

	// Spawn goroutines to perform concurrent operations
	var wg sync.WaitGroup
	stopChan := make(chan struct{})

	// 1. Worker goroutines calling Start/Stop/Reload
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			ticker := time.NewTicker(5 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-stopChan:
					return
				case <-ticker.C:
					switch id % 3 {
					case 0:
						scheduler.Start()
					case 1:
						scheduler.Reload()
					case 2:
						scheduler.Stop()
					}
				}
			}
		}(i)
	}

	// 2. Worker goroutines mutating settings schedule
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			ticker := time.NewTicker(10 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-stopChan:
					return
				case <-ticker.C:
					schedule := "* * * * * *"
					if id%2 == 0 {
						schedule = ""
					}
					settings := fmt.Sprintf(`{"backupsToKeep": 2, "backupPath": "", "backupSchedule": "%s"}`, schedule)
					_, _ = database.Exec("UPDATE settings SET value = ? WHERE key = 'server-settings'", settings)
				}
			}
		}(i)
	}

	// Let the stress run for a short duration
	time.Sleep(300 * time.Millisecond)
	close(stopChan)
	wg.Wait()
}
