package backup

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// TestChallengerFreshInstallBackup verifies that CreateBackup silently succeeds even when
// metadata directories (items, authors) do not exist (e.g., on a fresh install).
// This is because addDirToZip uses filepath.Walk which returns nil when the callback returns nil on error.
func TestChallengerFreshInstallBackup(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "abs-fresh-install-test")
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

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	_, err = CreateBackup(ctx, db, configPath, metadataPath, backupDir)
	if err == nil {
		t.Log("CONFIRMED: CreateBackup silently succeeds when metadata subdirectories are missing because filepath.Walk errors are ignored.")
	} else {
		t.Errorf("CreateBackup failed unexpectedly: %v", err)
	}
}

// TestChallengerActiveConnectionRestore verifies that the old connection keeps reading from
// the unlinked old database file, completely missing the restored database updates until reconnected.
func TestChallengerActiveConnectionRestore(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "abs-active-conn-test")
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
	os.MkdirAll(filepath.Join(metadataPath, "items"), 0755)
	os.MkdirAll(filepath.Join(metadataPath, "authors"), 0755)

	dbPath := filepath.Join(configPath, "absdatabase.sqlite")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}

	_, err = db.Exec("CREATE TABLE test_data (val TEXT); INSERT INTO test_data VALUES ('initial');")
	if err != nil {
		t.Fatalf("failed to setup DB: %v", err)
	}

	// Create backup (contains 'initial')
	ctx := context.Background()
	backups, err := CreateBackup(ctx, db, configPath, metadataPath, backupDir)
	if err != nil {
		t.Fatalf("failed to create backup: %v", err)
	}
	backupID := backups[0].ID

	// Update the database to have 'changed'
	_, err = db.Exec("UPDATE test_data SET val = 'changed'")
	if err != nil {
		t.Fatalf("failed to update DB: %v", err)
	}

	var val string
	err = db.QueryRow("SELECT val FROM test_data").Scan(&val)
	if err != nil {
		t.Fatalf("failed to read: %v", err)
	}
	t.Logf("Before restore, value in DB = %s", val)

	// Now apply the backup while `db` is still open!
	err = ApplyBackup(ctx, configPath, metadataPath, backupDir, backupID, nil)
	if err != nil {
		t.Fatalf("Restore failed: %v", err)
	}

	// Try to query using the old `db` connection after the restore
	var postVal string
	err = db.QueryRow("SELECT val FROM test_data").Scan(&postVal)
	if err != nil {
		t.Errorf("Querying database AFTER restore on old open connection failed: %v", err)
	} else {
		t.Logf("Querying database AFTER restore on old open connection returned value = %s", postVal)
		if postVal == "changed" {
			t.Log("CONFIRMED: The old connection is still talking to the old unlinked database file, NOT the restored one!")
		} else {
			t.Log("The old connection somehow read from the new database file.")
		}
	}
	db.Close()
}
