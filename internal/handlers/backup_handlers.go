package handlers

// backup_handlers.go — thin wrapper re-exporting backup HTTP handlers from internal/backup.

import (
	log "audiobookshelf/internal/logger"

	ibackup "audiobookshelf/internal/backup"
	idb "audiobookshelf/internal/db"
	isocket "audiobookshelf/internal/socket"
	"database/sql"
	"net/http"
)

// handleGetBackups returns an HTTP handler for GET /api/backups.
func handleGetBackups(db *sql.DB, metadataPath string) http.HandlerFunc {
	return ibackup.HandleGetBackups(db, metadataPath)
}

// handleCreateBackup returns an HTTP handler for POST /api/backups.
func handleCreateBackup(db *sql.DB, configPath string, metadataPath string) http.HandlerFunc {
	return ibackup.HandleCreateBackup(db, configPath, metadataPath)
}

// handleDeleteBackup returns an HTTP handler for DELETE /api/backups/:id.
func handleDeleteBackup(db *sql.DB, metadataPath string) http.HandlerFunc {
	return ibackup.HandleDeleteBackup(db, metadataPath)
}

// handleDownloadBackup returns an HTTP handler for GET /api/backups/:id/download.
func handleDownloadBackup(db *sql.DB, metadataPath string) http.HandlerFunc {
	return ibackup.HandleDownloadBackup(db, metadataPath)
}

// handleUpdateBackupPath returns an HTTP handler for PATCH /api/backups/path.
func handleUpdateBackupPath(db *sql.DB, metadataPath string) http.HandlerFunc {
	return ibackup.HandleUpdateBackupPath(db, metadataPath)
}

// handleUploadBackup returns an HTTP handler for POST /api/backups/upload.
func handleUploadBackup(db *sql.DB, metadataPath string) http.HandlerFunc {
	return ibackup.HandleUploadBackup(db, metadataPath)
}

// handleApplyBackup returns an HTTP handler for POST /api/backups/:id/apply.
func handleApplyBackup(db *sql.DB, configPath string, metadataPath string, triggerReload func()) http.HandlerFunc {
	return ibackup.HandleApplyBackup(db, configPath, metadataPath, func(dbPath string) error {
		return reconnectDB(dbPath)
	}, triggerReload, func() {
		if isocket.GlobalAuth != nil {
			isocket.GlobalAuth.BroadcastToAll("backup_applied", nil)
		}
	})
}

func reconnectDB(dbPath string) error {
	dbFile, err := idb.InitDB(dbPath)
	if err != nil {
		log.Printf("[Apply Backup] Failed to reconnect to restored DB: %v", err)
		return err
	}
	globalDB = dbFile
	if isocket.GlobalAuth != nil {
		isocket.GlobalAuth.SetDB(dbFile)
	}
	reinitManagers(dbFile)
	return nil
}
