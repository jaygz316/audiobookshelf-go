package backup

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	log "audiobookshelf/internal/logger"
	inotification "audiobookshelf/internal/notification"
)

// CreateBackup orchestrates generating database snapshot and metadata archives.
func CreateBackup(ctx context.Context, db *sql.DB, configPath, metadataPath, backupDir string) ([]BackupInfo, error) {
	if !BackupRestoreMu.TryLock() {
		return nil, fmt.Errorf("backup or restore already in progress")
	}
	defer BackupRestoreMu.Unlock()

	if err := os.MkdirAll(backupDir, 0755); err != nil {
		inotification.TriggerEvent(ctx, db, "onBackupFailed", nil, "Backup Failed", fmt.Sprintf("Failed to create backup directory: %v", err), nil)
		return nil, err
	}

	now := time.Now()
	id := now.Format("2006-01-02T1504")
	fullPath := filepath.Join(backupDir, id+".audiobookshelf")

	tempDBPath, err := createDBSnapshot(ctx, db, configPath, id)
	if err != nil {
		inotification.TriggerEvent(ctx, db, "onBackupFailed", nil, "Backup Failed", fmt.Sprintf("Failed to create DB snapshot: %v", err), nil)
		return nil, err
	}
	defer os.Remove(tempDBPath)

	if err := zipBackupContents(fullPath, tempDBPath, metadataPath, id, now); err != nil {
		inotification.TriggerEvent(ctx, db, "onBackupFailed", nil, "Backup Failed", fmt.Sprintf("Failed to zip backup contents: %v", err), nil)
		return nil, err
	}

	if err := pruneOldBackups(db, backupDir); err != nil {
		log.Printf("[Backups] Pruning old backups failed: %v", err)
	}

	// Trigger onBackupCompleted!
	inotification.TriggerEvent(ctx, db, "onBackupCompleted", nil, "Backup Completed", "Metadata backup completed successfully.", map[string]string{
		"backupId": id,
		"filename": id + ".audiobookshelf",
	})

	return LoadBackupsList(backupDir)
}

func createDBSnapshot(ctx context.Context, db *sql.DB, configPath, id string) (string, error) {
	tempDBPath := filepath.Join(os.TempDir(), fmt.Sprintf("absdatabase-%s-%d.sqlite", id, time.Now().UnixNano()))

	_, err := db.ExecContext(ctx, fmt.Sprintf("VACUUM INTO '%s'", tempDBPath))
	if err == nil {
		return tempDBPath, nil
	}

	log.Printf("[Backups] VACUUM INTO failed: %v. Falling back to simple file copy", err)
	srcFile, err := os.Open(filepath.Join(configPath, "absdatabase.sqlite"))
	if err != nil {
		return "", err
	}
	defer srcFile.Close()

	destFile, err := os.Create(tempDBPath)
	if err != nil {
		return "", err
	}
	defer destFile.Close()

	if _, err = io.Copy(destFile, srcFile); err != nil {
		os.Remove(tempDBPath)
		return "", err
	}

	return tempDBPath, nil
}

func pruneOldBackups(db *sql.DB, backupDir string) error {
	settings, err := getServerSettings(db)
	backupsToKeep := 2
	if err == nil && settings != nil && settings.BackupsToKeep > 0 {
		backupsToKeep = settings.BackupsToKeep
	}

	backups, err := LoadBackupsList(backupDir)
	if err != nil {
		return err
	}

	if len(backups) > backupsToKeep {
		for i := backupsToKeep; i < len(backups); i++ {
			log.Printf("[Backups] Pruning old backup: %s", backups[i].FullPath)
			os.Remove(backups[i].FullPath)
		}
	}
	return nil
}
