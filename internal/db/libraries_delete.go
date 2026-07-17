package db

import (
	"database/sql"
	"fmt"

	watcher "audiobookshelf/internal/watcher"
)

func DeleteLibrary(db *sql.DB, libraryID string) (*LibraryJSON, error) {
	lib, err := GetLibraryByID(db, libraryID)
	if err != nil {
		return nil, err
	}
	if lib == nil {
		return nil, fmt.Errorf("library not found")
	}

	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if tableExistsTx(tx, "collections") {
		_, err = tx.Exec("DELETE FROM collections WHERE libraryId = ?", libraryID)
		if err != nil {
			return nil, err
		}
	}

	if tableExistsTx(tx, "playbackSessions") {
		_, err = tx.Exec("UPDATE playbackSessions SET libraryId = NULL WHERE libraryId = ?", libraryID)
		if err != nil {
			return nil, err
		}
	}

	hasMediaProgresses := tableExistsTx(tx, "mediaProgresses")
	hasPlaylistItems := tableExistsTx(tx, "playlistItems")
	if hasMediaProgresses {
		_, _ = tx.Exec("DELETE FROM mediaProgresses WHERE mediaItemId IN (SELECT mediaId FROM libraryItems WHERE libraryId = ?)", libraryID)
	}
	if hasPlaylistItems {
		_, _ = tx.Exec("DELETE FROM playlistItems WHERE libraryItemId IN (SELECT id FROM libraryItems WHERE libraryId = ?)", libraryID)
	}
	_, err = tx.Exec("DELETE FROM libraryItems WHERE libraryId = ?", libraryID)
	if err != nil {
		return nil, err
	}

	_, err = tx.Exec("DELETE FROM libraryFolders WHERE libraryId = ?", libraryID)
	if err != nil {
		return nil, err
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

	_, err = tx.Exec("DELETE FROM libraries WHERE id = ?", libraryID)
	if err != nil {
		return nil, err
	}

	rows, err := tx.Query("SELECT id FROM libraries ORDER BY displayOrder ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var remainingIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			remainingIDs = append(remainingIDs, id)
		}
	}
	rows.Close()

	for i, id := range remainingIDs {
		_, err = tx.Exec("UPDATE libraries SET displayOrder = ? WHERE id = ?", i+1, id)
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

	return lib, nil
}
