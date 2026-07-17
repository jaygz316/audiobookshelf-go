package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"encoding/json"
	"net/http"

	"audiobookshelf/internal/core"
	idb "audiobookshelf/internal/db"
	"audiobookshelf/internal/utils"
)

// handleGetLibrarySeriesByID resolves GET /api/libraries/{id}/series/{seriesId}
func handleGetLibrarySeriesByID(db *sql.DB, libraryID string, seriesID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Go] GET /api/libraries/%s/series/%s", libraryID, seriesID)

		userVal := r.Context().Value(core.UserContextKey)
		if userVal == nil {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}
		user := userVal.(*core.UserSession)
		if !user.CanAccessLibrary(libraryID) {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}
		userID := user.ID

		// Retrieve series metadata
		var id, name string
		var nameIgnorePrefix, description sql.NullString
		var createdAtStr, updatedAtStr string
		err := db.QueryRow(`
			SELECT id, name, nameIgnorePrefix, description, createdAt, updatedAt
			FROM series
			WHERE id = ? AND libraryId = ?
		`, seriesID, libraryID).Scan(&id, &name, &nameIgnorePrefix, &description, &createdAtStr, &updatedAtStr)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		// Query the user's progress for all books in the series
		rows, err := db.Query(`
			SELECT li.id, mp.isFinished
			FROM bookSeries bs
			JOIN libraryItems li ON li.mediaId = bs.bookId AND li.mediaType = 'book'
			LEFT JOIN mediaProgresses mp ON mp.mediaItemId = bs.bookId AND mp.userId = ?
			WHERE bs.seriesId = ? AND li.libraryId = ?
		`, userID, seriesID, libraryID)

		libraryItemIds := []string{}
		libraryItemIdsFinished := []string{}

		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var itemID string
				var isFinished sql.NullInt64
				if err := rows.Scan(&itemID, &isFinished); err == nil {
					libraryItemIds = append(libraryItemIds, itemID)
					if isFinished.Valid && isFinished.Int64 == 1 {
						libraryItemIdsFinished = append(libraryItemIdsFinished, itemID)
					}
				}
			}
		}

		isFinished := len(libraryItemIdsFinished) == len(libraryItemIds) && len(libraryItemIds) > 0

		payload := map[string]interface{}{
			"id":               id,
			"name":             name,
			"nameIgnorePrefix": nameIgnorePrefix.String,
			"description":      utils.NullIfEmpty(description.String),
			"addedAt":          idb.ParseEpochMillis(createdAtStr),
			"updatedAt":        idb.ParseEpochMillis(updatedAtStr),
			"progress": map[string]interface{}{
				"libraryItemIds":         libraryItemIds,
				"libraryItemIdsFinished": libraryItemIdsFinished,
				"isFinished":             isFinished,
			},
			"rssFeed": nil,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(payload)
	}
}
