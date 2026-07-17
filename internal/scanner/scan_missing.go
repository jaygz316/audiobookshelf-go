package scanner

import (
	"database/sql"

	log "audiobookshelf/internal/logger"
	isocket "audiobookshelf/internal/socket"
)

func checkAndMarkMissingItems(db *sql.DB, libraryID string, foundPaths []string, socketAuth *isocket.Authority) error {
	log.Printf("[Scanner] Checking for missing library items...")
	dbItems, err := db.Query("SELECT id, path FROM libraryItems WHERE libraryId = ? AND isMissing = 0", libraryID)
	if err != nil {
		return err
	}
	defer dbItems.Close()
	foundPathsMap := make(map[string]bool)
	for _, p := range foundPaths {
		foundPathsMap[p] = true
	}

	for dbItems.Next() {
		var id, path string
		if err := dbItems.Scan(&id, &path); err != nil {
			return err
		}
		if !foundPathsMap[path] {
			log.Printf("[Scanner] Item %s not found on disk, marking as missing", path)
			_, err = db.Exec("UPDATE libraryItems SET isMissing = 1 WHERE id = ?", id)
			if err != nil {
				return err
			}

			if socketAuth != nil {
				if minItem, err := GetLibraryItemMinifiedByID(db, id); err == nil {
					EmitLibraryItemEvent(socketAuth, "item_updated", minItem)
				}
			}
		}
	}
	if err := dbItems.Err(); err != nil {
		return err
	}

	log.Printf("[Scanner] Scan complete for library ID: %s", libraryID)
	return nil
}
