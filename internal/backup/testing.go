package backup

import (
	"database/sql"
)

// IsValidBackupIDExported is exported for testing.
func IsValidBackupIDExported(id string) bool {
	return isValidBackupID(id)
}

// IsSafePathExported is exported for testing.
func IsSafePathExported(baseDir, targetPath string) bool {
	return isSafePath(baseDir, targetPath)
}

// RestoreMetadataFilesExported is exported for testing.
func RestoreMetadataFilesExported(metadataPath, zipPath string) error {
	return restoreMetadataFiles(metadataPath, zipPath)
}

// PruneOldBackupsExported is exported for testing.
func PruneOldBackupsExported(db *sql.DB, backupDir string) error {
	return pruneOldBackups(db, backupDir)
}
