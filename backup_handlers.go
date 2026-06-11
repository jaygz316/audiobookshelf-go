package main

// backup_handlers.go — thin wrapper re-exporting backup HTTP handlers from internal/backup.

import (
	ibackup "audiobookshelf/internal/backup"
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
		if SocketAuth != nil {
			SocketAuth.BroadcastToAll("backup_applied", nil)
		}
	})
}
