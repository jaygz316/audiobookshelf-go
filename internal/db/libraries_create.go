package db

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/google/uuid"

	watcher "audiobookshelf/internal/watcher"
)

func CreateLibrary(db *sql.DB, payload *CreateLibraryPayload) (*LibraryJSON, error) {
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if payload.MediaType == "" {
		payload.MediaType = "book"
	}
	if payload.Icon == "" {
		payload.Icon = "database"
	}
	if payload.Provider == "" {
		payload.Provider = "google"
	}

	merged, err := mergeSettings(payload.MediaType, payload.Settings)
	if err != nil {
		return nil, err
	}
	settingsBytes, err := json.Marshal(merged)
	if err != nil {
		return nil, err
	}

	var maxOrder int
	err = tx.QueryRow("SELECT COALESCE(MAX(displayOrder), 0) FROM libraries").Scan(&maxOrder)
	if err != nil {
		return nil, err
	}
	displayOrder := maxOrder + 1

	libraryID := uuid.New().String()
	nowStr := time.Now().UTC().Format("2006-01-02 15:04:05.000")

	_, err = tx.Exec(`
		INSERT INTO libraries (id, name, displayOrder, icon, mediaType, provider, lastScan, lastScanVersion, settings, createdAt, updatedAt)
		VALUES (?, ?, ?, ?, ?, ?, NULL, NULL, ?, ?, ?)`,
		libraryID, payload.Name, displayOrder, payload.Icon, payload.MediaType, payload.Provider, settingsBytes, nowStr, nowStr)
	if err != nil {
		return nil, err
	}

	for _, folder := range payload.Folders {
		folderID := uuid.New().String()
		_, err = tx.Exec(`
			INSERT INTO libraryFolders (id, path, libraryId, createdAt, updatedAt)
			VALUES (?, ?, ?, ?, ?)`,
			folderID, folder.Path, libraryID, nowStr, nowStr)
		if err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	if watcher.GlobalWatcher != nil {
		watcher.GlobalWatcher.Reload()
	}

	return GetLibraryByID(db, libraryID)
}
