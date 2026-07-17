package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"audiobookshelf/internal/core"
	idb "audiobookshelf/internal/db"
	isocket "audiobookshelf/internal/socket"
)

// handleUpdateSeries resolves PATCH /api/series/{id}
func handleUpdateSeries(db *sql.DB, seriesID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Go] PATCH /api/series/%s", seriesID)

		userVal := r.Context().Value(core.UserContextKey)
		if userVal == nil {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}
		user := userVal.(*core.UserSession)
		if user.Type != "root" && user.Type != "admin" {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		var payload struct {
			Name             string `json:"name"`
			NameIgnorePrefix string `json:"nameIgnorePrefix"`
			Description      string `json:"description"`
		}

		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
			return
		}

		tx, err := db.Begin()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer tx.Rollback()

		nowStr := time.Now().Format("2006-01-02 15:04:05.000")

		_, err = tx.Exec(`
			UPDATE series
			SET name = ?, nameIgnorePrefix = ?, description = ?, updatedAt = ?
			WHERE id = ?
		`, payload.Name, payload.NameIgnorePrefix, payload.Description, nowStr, seriesID)
		if err != nil {
			http.Error(w, "failed to update series: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Update linked library items sidecar metadata if needed
		rows, err := tx.Query("SELECT bookId FROM bookSeries WHERE seriesId = ?", seriesID)
		if err == nil {
			var bookIDs []string
			for rows.Next() {
				var bid string
				if err := rows.Scan(&bid); err == nil {
					bookIDs = append(bookIDs, bid)
				}
			}
			rows.Close()

			for _, bid := range bookIDs {
				// Emit real-time update
				if isocket.GlobalAuth != nil {
					var itemID string
					_ = tx.QueryRow("SELECT id FROM libraryItems WHERE mediaId = ? AND mediaType = 'book'", bid).Scan(&itemID)
					if itemID != "" {
						if minItem, err := idb.GetLibraryItemMinifiedByID(db, itemID); err == nil {
							EmitLibraryItemEvent("item_updated", minItem)
						}
					}
				}
			}
		}

		err = tx.Commit()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"success": true}`))
	}
}
