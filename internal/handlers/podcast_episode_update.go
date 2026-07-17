package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"audiobookshelf/internal/core"
	idb "audiobookshelf/internal/db"
)

func handleUpdateEpisode(db *sql.DB, id, episodeId string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userVal := r.Context().Value(core.UserContextKey)
		if userVal == nil {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}
		user := userVal.(*core.UserSession)
		if !user.IsAdminOrUp() {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		var req map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
			return
		}

		var podcastID, libraryItemID string
		err := db.QueryRow(`
			SELECT p.id, li.id
			FROM podcasts p
			JOIN libraryItems li ON li.mediaId = p.id AND li.mediaType = 'podcast'
			WHERE p.id = ? OR li.id = ?
		`, id, id).Scan(&podcastID, &libraryItemID)
		if err != nil {
			http.Error(w, `{"error": "Podcast not found"}`, http.StatusNotFound)
			return
		}

		cols := getTableColumns(db, "podcastEpisodes")
		var setParts []string
		var args []interface{}

		for k, v := range req {
			if cols[k] && k != "id" && k != "podcastId" {
				setParts = append(setParts, fmt.Sprintf("%s = ?", k))
				args = append(args, v)
			}
		}

		if len(setParts) > 0 {
			args = append(args, episodeId, podcastID)
			query := fmt.Sprintf("UPDATE podcastEpisodes SET %s WHERE id = ? AND podcastId = ?", strings.Join(setParts, ", "))
			_, err = db.Exec(query, args...)
			if err != nil {
				log.Errorf("[UpdateEpisode] Update failed: %v", err)
				http.Error(w, `{"error": "Update failed"}`, http.StatusInternalServerError)
				return
			}
		}

		itemMin, err := idb.GetLibraryItemMinifiedByID(db, libraryItemID)
		if err == nil && itemMin != nil {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(itemMin)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true}`))
	}
}
