package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	watcher "audiobookshelf/internal/watcher"
)

func UpdateLibrary(db *sql.DB, libraryID string, payload *UpdateLibraryPayload) (*LibraryJSON, error) {
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var currentMediaType, currentSettingsStr string
	err = tx.QueryRow("SELECT mediaType, settings FROM libraries WHERE id = ?", libraryID).Scan(&currentMediaType, &currentSettingsStr)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("library not found")
	}
	if err != nil {
		return nil, err
	}

	nowStr := time.Now().UTC().Format("2006-01-02 15:04:05.000")

	if payload.Name != nil {
		_, err = tx.Exec("UPDATE libraries SET name = ?, updatedAt = ? WHERE id = ?", *payload.Name, nowStr, libraryID)
		if err != nil {
			return nil, err
		}
	}
	if payload.Provider != nil {
		_, err = tx.Exec("UPDATE libraries SET provider = ?, updatedAt = ? WHERE id = ?", *payload.Provider, nowStr, libraryID)
		if err != nil {
			return nil, err
		}
	}
	if payload.MediaType != nil {
		_, err = tx.Exec("UPDATE libraries SET mediaType = ?, updatedAt = ? WHERE id = ?", *payload.MediaType, nowStr, libraryID)
		if err != nil {
			return nil, err
		}
		currentMediaType = *payload.MediaType
	}
	if payload.Icon != nil {
		_, err = tx.Exec("UPDATE libraries SET icon = ?, updatedAt = ? WHERE id = ?", *payload.Icon, nowStr, libraryID)
		if err != nil {
			return nil, err
		}
	}
	if payload.DisplayOrder != nil {
		_, err = tx.Exec("UPDATE libraries SET displayOrder = ?, updatedAt = ? WHERE id = ?", *payload.DisplayOrder, nowStr, libraryID)
		if err != nil {
			return nil, err
		}
	}

	if payload.Settings != nil {
		var currentSettings map[string]interface{}
		if err := json.Unmarshal([]byte(currentSettingsStr), &currentSettings); err != nil {
			currentSettings = getDefaultSettings(currentMediaType)
		}
		merged, err := mergeSettings(currentMediaType, payload.Settings)
		if err != nil {
			return nil, err
		}
		for k, v := range merged {
			currentSettings[k] = v
		}
		settingsBytes, err := json.Marshal(currentSettings)
		if err != nil {
			return nil, err
		}
		_, err = tx.Exec("UPDATE libraries SET settings = ?, updatedAt = ? WHERE id = ?", settingsBytes, nowStr, libraryID)
		if err != nil {
			return nil, err
		}
	}

	if payload.Folders != nil {
		rows, err := tx.Query("SELECT id, path FROM libraryFolders WHERE libraryId = ?", libraryID)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		existingFolders := make(map[string]string)
		for rows.Next() {
			var fid, fpath string
			if err := rows.Scan(&fid, &fpath); err == nil {
				existingFolders[fid] = fpath
			}
		}
		rows.Close()

		inputFolderIDs := make(map[string]bool)
		for _, folder := range payload.Folders {
			if folder.ID != "" && existingFolders[folder.ID] != "" {
				inputFolderIDs[folder.ID] = true
				if existingFolders[folder.ID] != folder.Path {
					_, err = tx.Exec(`
						UPDATE libraryFolders SET path = ?, updatedAt = ?
						WHERE id = ?`,
						folder.Path, nowStr, folder.ID)
					if err != nil {
						return nil, err
					}
				}
			} else {
				folderID := uuid.New().String()
				_, err = tx.Exec(`
					INSERT INTO libraryFolders (id, path, libraryId, createdAt, updatedAt)
					VALUES (?, ?, ?, ?, ?)`,
					folderID, folder.Path, libraryID, nowStr, nowStr)
				if err != nil {
					return nil, err
				}
			}
		}

		hasMediaProgresses := tableExistsTx(tx, "mediaProgresses")
		hasPlaylistItems := tableExistsTx(tx, "playlistItems")
		for fid, fpath := range existingFolders {
			if !inputFolderIDs[fid] {
				_, err = tx.Exec("DELETE FROM libraryFolders WHERE id = ?", fid)
				if err != nil {
					return nil, err
				}

				if hasMediaProgresses {
					_, _ = tx.Exec("DELETE FROM mediaProgresses WHERE mediaItemId IN (SELECT mediaId FROM libraryItems WHERE libraryFolderId = ?)", fid)
				}
				if hasPlaylistItems {
					_, _ = tx.Exec("DELETE FROM playlistItems WHERE libraryItemId IN (SELECT id FROM libraryItems WHERE libraryFolderId = ?)", fid)
				}
				_, err = tx.Exec("DELETE FROM libraryItems WHERE libraryFolderId = ?", fid)
				if err != nil {
					return nil, err
				}
				_ = fpath
			}
		}

		_, _ = tx.Exec("DELETE FROM books WHERE id NOT IN (SELECT mediaId FROM libraryItems WHERE mediaType = 'book')")
		_, _ = tx.Exec("DELETE FROM podcasts WHERE id NOT IN (SELECT mediaId FROM libraryItems WHERE mediaType = 'podcast')")
		if tableExistsTx(tx, "bookAuthors") {
			_, _ = tx.Exec("DELETE FROM bookAuthors WHERE bookId NOT IN (SELECT id FROM books)")
		}
		if tableExistsTx(tx, "bookSeries") {
			_, _ = tx.Exec("DELETE FROM bookSeries WHERE bookId NOT IN (SELECT id FROM books)")
		}
		if tableExistsTx(tx, "bookAuthors") && tableExistsTx(tx, "authors") {
			_, _ = tx.Exec("DELETE FROM authors WHERE id NOT IN (SELECT authorId FROM bookAuthors) AND (asin IS NULL OR asin = '') AND (description IS NULL OR description = '') AND (imagePath IS NULL OR imagePath = '')")
		}
		if tableExistsTx(tx, "bookSeries") && tableExistsTx(tx, "series") {
			_, _ = tx.Exec("DELETE FROM series WHERE id NOT IN (SELECT seriesId FROM bookSeries)")
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
