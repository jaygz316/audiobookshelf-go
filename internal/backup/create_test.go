package backup

import (
	"archive/zip"
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func createMockBackupZip(t *testing.T, backupDir, id string, createdAtMs int64) {
	filename := id + ".audiobookshelf"
	fullPath := filepath.Join(backupDir, filename)
	zipFile, err := os.Create(fullPath)
	if err != nil {
		t.Fatalf("failed to create mock zip file: %v", err)
	}
	defer zipFile.Close()

	zw := zip.NewWriter(zipFile)
	defer zw.Close()

	detailsString := fmt.Sprintf("%s\nsqlite\n%d\n2.8.0\n", id, createdAtMs)
	writer, err := zw.Create("details")
	if err != nil {
		t.Fatalf("failed to create details entry: %v", err)
	}
	if _, err := writer.Write([]byte(detailsString)); err != nil {
		t.Fatalf("failed to write details: %v", err)
	}
}

func TestBackupAndPruneFlow(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "abs-backup-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	configPath := filepath.Join(tempDir, "config")
	metadataPath := filepath.Join(tempDir, "metadata")
	backupDir := filepath.Join(tempDir, "backups")

	os.MkdirAll(configPath, 0755)
	os.MkdirAll(metadataPath, 0755)
	os.MkdirAll(backupDir, 0755)

	dbPath := filepath.Join(configPath, "absdatabase.sqlite")
	if err := os.WriteFile(dbPath, []byte("sqlite file content"), 0644); err != nil {
		t.Fatalf("failed to create dummy db file: %v", err)
	}

	os.MkdirAll(filepath.Join(metadataPath, "items"), 0755)
	os.WriteFile(filepath.Join(metadataPath, "items", "item1.json"), []byte("item content"), 0644)

	os.MkdirAll(filepath.Join(metadataPath, "authors"), 0755)
	os.WriteFile(filepath.Join(metadataPath, "authors", "author1.json"), []byte("author content"), 0644)

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	_, err = db.Exec("CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT, createdAt TEXT, updatedAt TEXT)")
	if err != nil {
		t.Logf("Skipping sqlite DB actions if driver is not loaded: %v", err)
		return
	}

	settingsJSON := `{"backupsToKeep": 2, "backupPath": ""}`
	_, err = db.Exec("INSERT INTO settings (key, value) VALUES ('server-settings', ?)", settingsJSON)
	if err != nil {
		t.Fatalf("failed to insert settings: %v", err)
	}

	ctx := context.Background()
	backups, err := CreateBackup(ctx, db, configPath, metadataPath, backupDir)
	if err != nil {
		t.Fatalf("failed to CreateBackup: %v", err)
	}

	if len(backups) != 1 {
		t.Fatalf("expected 1 backup, got %d", len(backups))
	}

	zipPath := backups[0].FullPath
	if _, err := os.Stat(zipPath); err != nil {
		t.Errorf("backup zip file does not exist: %v", err)
	}

	// Now manually create 2 older mock backup zips
	nowMs := time.Now().UnixNano() / int64(time.Millisecond)
	createMockBackupZip(t, backupDir, "2006-01-02T1501", nowMs-20000)
	createMockBackupZip(t, backupDir, "2006-01-02T1502", nowMs-10000)

	// Prune backups
	if err := pruneOldBackups(db, backupDir); err != nil {
		t.Fatalf("pruneOldBackups failed: %v", err)
	}

	backups, err = LoadBackupsList(backupDir)
	if err != nil {
		t.Fatalf("LoadBackupsList failed: %v", err)
	}

	if len(backups) != 2 {
		t.Errorf("expected backups list length to be 2 after pruning, got %d", len(backups))
	}

	// Verify that the oldest backup (2006-01-02T1501) was removed
	for _, b := range backups {
		if b.ID == "2006-01-02T1501" {
			t.Errorf("expected oldest backup 2006-01-02T1501 to be pruned, but it is still present")
		}
	}
}
