package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"encoding/json"
	"net/http"
	"os"

	"audiobookshelf/internal/core"
	idb "audiobookshelf/internal/db"
	"audiobookshelf/internal/utils"
)

func handleDeleteEpisode(db *sql.DB, id, episodeId string) http.HandlerFunc {
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

		hardDelete := r.URL.Query().Get("hard") == "1"

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

		var audioFileStr string
		err = db.QueryRow("SELECT audioFile FROM podcastEpisodes WHERE id = ? AND podcastId = ?", episodeId, podcastID).Scan(&audioFileStr)
		if err == nil && audioFileStr != "" {
			var af map[string]interface{}
			if json.Unmarshal([]byte(audioFileStr), &af) == nil && af != nil {
				if meta, ok := af["metadata"].(map[string]interface{}); ok && meta != nil {
					if path, ok := meta["path"].(string); ok && path != "" {
						if utils.IsSafeFilePath(db, MetadataPath, path) {
							if err := os.Remove(path); err != nil {
								log.Errorf("[DeleteEpisode] Failed to remove file %s: %v", path, err)
							}
						} else {
							log.Warnf("[DeleteEpisode] Deletion of unsafe path blocked: %s", path)
						}
					}
				}
			}
		}

		if hardDelete {
			_, err = db.Exec("DELETE FROM podcastEpisodes WHERE id = ? AND podcastId = ?", episodeId, podcastID)
		} else {
			_, err = db.Exec("UPDATE podcastEpisodes SET audioFile = '{}' WHERE id = ? AND podcastId = ?", episodeId, podcastID)
		}

		if err != nil {
			log.Errorf("[DeleteEpisode] Delete failed: %v", err)
			http.Error(w, `{"error": "Delete failed"}`, http.StatusInternalServerError)
			return
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

func handleDeleteEpisodes(db *sql.DB, id string) http.HandlerFunc {
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

		var episodeIDs []string
		if err := json.NewDecoder(r.Body).Decode(&episodeIDs); err != nil {
			http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
			return
		}

		hardDelete := r.URL.Query().Get("hard") == "1"

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

		for _, episodeId := range episodeIDs {
			var audioFileStr string
			err = db.QueryRow("SELECT audioFile FROM podcastEpisodes WHERE id = ? AND podcastId = ?", episodeId, podcastID).Scan(&audioFileStr)
			if err == nil && audioFileStr != "" {
				var af map[string]interface{}
				if json.Unmarshal([]byte(audioFileStr), &af) == nil && af != nil {
					if meta, ok := af["metadata"].(map[string]interface{}); ok && meta != nil {
						if path, ok := meta["path"].(string); ok && path != "" {
							if utils.IsSafeFilePath(db, MetadataPath, path) {
								if err := os.Remove(path); err != nil {
									log.Errorf("[DeleteEpisode] Failed to remove file %s: %v", path, err)
								}
							} else {
								log.Warnf("[DeleteEpisode] Deletion of unsafe path blocked: %s", path)
							}
						}
					}
				}
			}

			if hardDelete {
				_, err = db.Exec("DELETE FROM podcastEpisodes WHERE id = ? AND podcastId = ?", episodeId, podcastID)
			} else {
				_, err = db.Exec("UPDATE podcastEpisodes SET audioFile = '{}' WHERE id = ? AND podcastId = ?", episodeId, podcastID)
			}
			if err != nil {
				log.Errorf("[DeleteEpisode] Delete/Update failed for episode %s: %v", episodeId, err)
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
