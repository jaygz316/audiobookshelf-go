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

func init() {
	ibackup.OnBeforeRestore = func() {
		if GetGlobalDB() != nil {
			log.Infof("[Apply Backup] Closing active database connection before restore")
			if err := GetGlobalDB().Close(); err != nil {
				log.Errorf("[Apply Backup] Failed to close old database connection: %v", err)
			}
			SetGlobalDB(nil)
		}
	}
}

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
	if GetGlobalDB() != nil {
		if err := GetGlobalDB().Close(); err != nil {
			log.Errorf("[Apply Backup] Failed to close old database connection: %v", err)
		}
		SetGlobalDB(nil)
	}
	dbFile, err := idb.InitDB(dbPath)
	if err != nil {
		log.Errorf("[Apply Backup] Failed to reconnect to restored DB: %v", err)
		return err
	}
	SetGlobalDB(dbFile)
	if ibackup.GlobalScheduler != nil && globalCfg != nil {
		ibackup.GlobalScheduler.Stop()
		ibackup.InitScheduler(dbFile, globalCfg.ConfigPath, globalCfg.MetadataPath)
	}
	if isocket.GlobalAuth != nil {
		isocket.GlobalAuth.SetDB(dbFile)
	}
	reinitManagers(dbFile)
	if ActiveHandler != nil && globalCfg != nil {
		SetupHandler(dbFile, globalCfg, true, globalAppRoot, globalVersion)
	}
	return nil
}
