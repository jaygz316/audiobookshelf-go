package db

import (
	"database/sql"
	"encoding/json"
	"time"
)

// GetLibraryByID fetches a single library by ID
func GetLibraryByID(db *sql.DB, libraryID string) (*LibraryJSON, error) {
	var id, name, mediaType, createdAtStr, updatedAtStr string
	var icon, provider, lastScanVersion sql.NullString
	var displayOrder int
	var lastScanStr sql.NullString
	var settingsBytes []byte

	err := db.QueryRow("SELECT id, name, displayOrder, icon, mediaType, provider, lastScan, lastScanVersion, settings, createdAt, updatedAt FROM libraries WHERE id = ?", libraryID).
		Scan(&id, &name, &displayOrder, &icon, &mediaType, &provider, &lastScanStr, &lastScanVersion, &settingsBytes, &createdAtStr, &updatedAtStr)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var lastScan *int64
	if lastScanStr.Valid && lastScanStr.String != "" {
		t, err := ParseSQLiteTime(lastScanStr.String)
		if err == nil {
			val := t.UnixNano() / int64(time.Millisecond)
			lastScan = &val
		}
	}

	createdAtTime, _ := ParseSQLiteTime(createdAtStr)
	updatedAtTime, _ := ParseSQLiteTime(updatedAtStr)

	var iconVal string
	if icon.Valid {
		iconVal = icon.String
	}
	var providerVal string
	if provider.Valid {
		providerVal = provider.String
	}
	var lastScanVersionVal string
	if lastScanVersion.Valid {
		lastScanVersionVal = lastScanVersion.String
	}

	lib := &LibraryJSON{
		ID:              id,
		Name:            name,
		DisplayOrder:    displayOrder,
		Icon:            iconVal,
		MediaType:       mediaType,
		Provider:        providerVal,
		LastScan:        lastScan,
		LastScanVersion: lastScanVersionVal,
		Settings:        json.RawMessage(settingsBytes),
		CreatedAt:       createdAtTime.UnixNano() / int64(time.Millisecond),
		LastUpdate:      updatedAtTime.UnixNano() / int64(time.Millisecond),
		Folders:         []*LibraryFolderJSON{},
	}
	if len(lib.Settings) == 0 {
		lib.Settings = json.RawMessage(`{}`)
	}

	// Fetch folders
	rows, err := db.Query("SELECT id, path, libraryId, createdAt FROM libraryFolders WHERE libraryId = ?", libraryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var folderID, path, folderLibID, folderCreatedAtStr string
		err = rows.Scan(&folderID, &path, &folderLibID, &folderCreatedAtStr)
		if err != nil {
			return nil, err
		}
		fCreatedAt, _ := ParseSQLiteTime(folderCreatedAtStr)
		lib.Folders = append(lib.Folders, &LibraryFolderJSON{
			ID:        folderID,
			FullPath:  path,
			LibraryID: folderLibID,
			AddedAt:   fCreatedAt.UnixNano() / int64(time.Millisecond),
		})
	}

	return lib, nil
}
