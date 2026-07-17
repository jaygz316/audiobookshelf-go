package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"

	idb "audiobookshelf/internal/db"
	"audiobookshelf/internal/utils"
)

// handleGetAuthorByID resolves GET /api/authors/{id}
func handleGetAuthorByID(db *sql.DB, authorID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Go] GET /api/authors/%s", authorID)

		var id, name, lastFirst, createdAtStr, updatedAtStr string
		var asin, description, imagePath sql.NullString
		err := db.QueryRow(`
			SELECT id, name, lastFirst, asin, description, imagePath, createdAt, updatedAt
			FROM authors
			WHERE id = ?
		`, authorID).Scan(&id, &name, &lastFirst, &asin, &description, &imagePath, &createdAtStr, &updatedAtStr)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		includes := r.URL.Query().Get("include")
		includeItems := strings.Contains(includes, "items")
		includeSeries := strings.Contains(includes, "series")

		payload := map[string]interface{}{
			"id":          id,
			"name":        name,
			"lastFirst":   lastFirst,
			"asin":        utils.NullIfEmpty(asin.String),
			"description": utils.NullIfEmpty(description.String),
			"imagePath":   utils.NullIfEmpty(imagePath.String),
			"addedAt":     idb.ParseEpochMillis(createdAtStr),
			"updatedAt":   idb.ParseEpochMillis(updatedAtStr),
		}

		if includeItems {
			payload["libraryItems"] = fetchAuthorLibraryItems(db, authorID)
		}

		if includeSeries {
			payload["series"] = fetchAuthorSeries(db, authorID)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(payload)
	}
}
