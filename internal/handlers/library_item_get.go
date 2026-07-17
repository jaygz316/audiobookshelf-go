package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"encoding/json"
	"net/http"

	"audiobookshelf/internal/core"
	idb "audiobookshelf/internal/db"
)

// handleGetLibraryItemByID resolves GET /api/items/{id}
func handleGetLibraryItemByID(db *sql.DB, itemID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Go] GET /api/items/%s", itemID)

		userVal := r.Context().Value(core.UserContextKey)
		if userVal == nil {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}
		user := userVal.(*core.UserSession)

		var id, ino, libraryID, folderID, path, relPath, mediaType, mediaID, mtimeStr, ctimeStr, birthtimeStr, createdAtStr, updatedAtStr string
		var isFileVal, isMissingVal, isInvalidVal int
		var size int64

		query := `
			SELECT id, ino, libraryId, libraryFolderId, path, relPath, isFile, mtime, ctime, birthtime, createdAt, updatedAt, isMissing, isInvalid, mediaType, mediaId, size
			FROM libraryItems
			WHERE id = ?
		`
		err := db.QueryRow(query, itemID).Scan(
			&id, &ino, &libraryID, &folderID, &path, &relPath, &isFileVal, &mtimeStr, &ctimeStr, &birthtimeStr, &createdAtStr, &updatedAtStr, &isMissingVal, &isInvalidVal, &mediaType, &mediaID, &size,
		)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		if !user.CanAccessLibrary(libraryID) {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		payload := map[string]interface{}{
			"id":           id,
			"ino":          ino,
			"libraryId":    libraryID,
			"folderId":     folderID,
			"path":         path,
			"relPath":      relPath,
			"isFile":       isFileVal != 0,
			"mtimeMs":      idb.ParseEpochMillis(mtimeStr),
			"ctimeMs":      idb.ParseEpochMillis(ctimeStr),
			"birthtimeMs":  idb.ParseEpochMillis(birthtimeStr),
			"addedAt":      idb.ParseEpochMillis(createdAtStr),
			"updatedAt":    idb.ParseEpochMillis(updatedAtStr),
			"isMissing":    isMissingVal != 0,
			"isInvalid":    isInvalidVal != 0,
			"mediaType":    mediaType,
			"size":         size,
			"libraryFiles": []interface{}{},
		}

		if mediaType == "book" {
			if !getBookMediaDetails(w, r, db, mediaID, libraryID, itemID, ino, size, user, payload) {
				return
			}
		} else if mediaType == "podcast" {
			if !getPodcastMediaDetails(w, r, db, mediaID, user, payload) {
				return
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(payload)
	}
}
