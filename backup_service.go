package main

// backup_service.go — thin wrapper re-exporting backup types and functions from internal/backup.

import (
	ibackup "audiobookshelf/internal/backup"
	"context"
	"database/sql"
	"log"
)

// BackupInfo is an alias for the internal backup BackupInfo type.
type BackupInfo = ibackup.BackupInfo

// GetBackupDirPath returns the backup directory path.
func GetBackupDirPath(ctx context.Context, db *sql.DB, metadataPath string) string {
	return ibackup.GetBackupDirPath(ctx, db, metadataPath)
}

// LoadBackupsList returns the list of backups sorted by newest first.
func LoadBackupsList(backupDir string) ([]BackupInfo, error) {
	return ibackup.LoadBackupsList(backupDir)
}

// ReadBackupDetailsFromZip extracts the backup details from the details file in the ZIP.
func ReadBackupDetailsFromZip(zipPath string, fileSize int64) (BackupInfo, error) {
	return ibackup.ReadBackupDetailsFromZip(zipPath, fileSize)
}

// CreateBackup orchestrates generating database snapshot and metadata archives.
func CreateBackup(ctx context.Context, db *sql.DB, configPath, metadataPath, backupDir string) ([]BackupInfo, error) {
	return ibackup.CreateBackup(ctx, db, configPath, metadataPath, backupDir)
}

// ApplyBackup performs low-level backup restore operations on db and metadata directories.
func ApplyBackup(ctx context.Context, configPath, metadataPath, backupDir, id string) error {
	return ibackup.ApplyBackup(ctx, configPath, metadataPath, backupDir, id, func(dbPath string) error {
		return reconnectDB(dbPath)
	})
}

func reconnectDB(dbPath string) error {
	dbFile, err := initDB(dbPath)
	if err != nil {
		log.Printf("[Apply Backup] Failed to reconnect to restored DB: %v", err)
		return err
	}
	globalDB = dbFile
	if SocketAuth != nil {
		SocketAuth.SetDB(dbFile)
	}
	reinitManagers(dbFile)
	return nil
}

// isValidBackupID is exposed for testing.
func isValidBackupID(id string) bool {
	return ibackup.IsValidBackupIDExported(id)
}

// isSafePath is exposed for testing.
func isSafePath(baseDir, targetPath string) bool {
	return ibackup.IsSafePathExported(baseDir, targetPath)
}

// restoreMetadataFiles is exposed for testing.
func restoreMetadataFiles(metadataPath, zipPath string) error {
	return ibackup.RestoreMetadataFilesExported(metadataPath, zipPath)
}

// pruneOldBackups is exposed for testing.
func pruneOldBackups(db *sql.DB, backupDir string) error {
	return ibackup.PruneOldBackupsExported(db, backupDir)
}
