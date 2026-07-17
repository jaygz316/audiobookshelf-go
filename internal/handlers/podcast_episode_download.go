package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"encoding/json"
	"net/http"
	"path/filepath"

	"audiobookshelf/internal/core"
	"audiobookshelf/internal/podcast"
	"audiobookshelf/internal/utils"
)

func handleDownloadEpisodes(db *sql.DB, id string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if globalPodcastManager == nil {
			initManagers(db)
		}
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

		var podcastID, libraryItemID, podcastPath, podcastTitle string
		err := db.QueryRow(`
			SELECT p.id, p.title, li.id, li.path
			FROM podcasts p
			JOIN libraryItems li ON li.mediaId = p.id AND li.mediaType = 'podcast'
			WHERE p.id = ? OR li.id = ?
		`, id, id).Scan(&podcastID, &podcastTitle, &libraryItemID, &podcastPath)
		if err != nil {
			http.Error(w, `{"error": "Podcast not found"}`, http.StatusNotFound)
			return
		}

		for _, epID := range episodeIDs {
			var epTitle, enclosureURL string
			err := db.QueryRow(`
				SELECT title, enclosureURL
				FROM podcastEpisodes
				WHERE id = ? AND podcastId = ?
			`, epID, podcastID).Scan(&epTitle, &enclosureURL)
			if err != nil || enclosureURL == "" {
				continue
			}

			destFile := filepath.Join(podcastPath, sanitizeFilename(epTitle)+".mp3")
			if !utils.IsSafeFilePath(db, MetadataPath, destFile) {
				log.Errorf("[DownloadEpisode] Traversal/Unauthorized path attempt blocked: %s", destFile)
				continue
			}

			if podcast.GlobalQueueManager == nil {
				podcast.InitQueueManager(db, globalPodcastManager)
			}

			podcast.GlobalQueueManager.Enqueue(&podcast.DownloadTask{
				ID:           epID,
				PodcastID:    podcastID,
				PodcastTitle: podcastTitle,
				EpisodeTitle: epTitle,
				EnclosureURL: enclosureURL,
				DestPath:     destFile,
			})
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true}`))
	}
}

func handleMatchEpisodes(db *sql.DB, id string) http.HandlerFunc {
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

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"numEpisodesUpdated":0}`))
	}
}
