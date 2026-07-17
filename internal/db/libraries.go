package db

import (
	"database/sql"
	"encoding/json"
	"time"
)

// GetLibraries returns all libraries in the database
func GetLibraries(db *sql.DB) ([]*LibraryJSON, error) {
	rows, err := db.Query("SELECT id, name, displayOrder, icon, mediaType, provider, lastScan, lastScanVersion, settings, createdAt, updatedAt FROM libraries ORDER BY displayOrder ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var libraries []*LibraryJSON
	for rows.Next() {
		var id, name, mediaType, createdAtStr, updatedAtStr string
		var icon, provider, lastScanVersion sql.NullString
		var displayOrder int
		var lastScanStr sql.NullString
		var settingsBytes []byte

		err = rows.Scan(&id, &name, &displayOrder, &icon, &mediaType, &provider, &lastScanStr, &lastScanVersion, &settingsBytes, &createdAtStr, &updatedAtStr)
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

		libraries = append(libraries, lib)
	}

	// Fetch all library folders and attach them
	folderRows, err := db.Query("SELECT id, path, libraryId, createdAt FROM libraryFolders")
	if err != nil {
		return nil, err
	}
	defer folderRows.Close()

	foldersByLibrary := make(map[string][]*LibraryFolderJSON)
	for folderRows.Next() {
		var id, path, libraryID, createdAtStr string
		err = folderRows.Scan(&id, &path, &libraryID, &createdAtStr)
		if err != nil {
			return nil, err
		}

		createdAtTime, _ := ParseSQLiteTime(createdAtStr)
		folder := &LibraryFolderJSON{
			ID:        id,
			FullPath:  path,
			LibraryID: libraryID,
			AddedAt:   createdAtTime.UnixNano() / int64(time.Millisecond),
		}
		foldersByLibrary[libraryID] = append(foldersByLibrary[libraryID], folder)
	}

	for _, lib := range libraries {
		if folders, ok := foldersByLibrary[lib.ID]; ok {
			lib.Folders = folders
		}
	}

	return libraries, nil
}
