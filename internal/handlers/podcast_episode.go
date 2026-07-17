package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"

	"audiobookshelf/internal/core"
)

func handleClearEpisodeQueue(db *sql.DB, id string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true}`))
	}
}

func handleGetEpisodeDownloads(db *sql.DB, id string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userVal := r.Context().Value(core.UserContextKey)
		if userVal == nil {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}
		user := userVal.(*core.UserSession)

		var podcastID, libraryID string
		err := db.QueryRow(`
			SELECT p.id, li.libraryId
			FROM podcasts p
			JOIN libraryItems li ON li.mediaId = p.id AND li.mediaType = 'podcast'
			WHERE p.id = ? OR li.id = ?
		`, id, id).Scan(&podcastID, &libraryID)
		if err != nil {
			http.Error(w, `{"error": "Podcast not found"}`, http.StatusNotFound)
			return
		}

		if !user.CanAccessLibrary(libraryID) {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		episodes, err := fetchPodcastEpisodesList(r.Context(), db, podcastID)
		if err != nil {
			http.Error(w, `{"error": "Failed to fetch downloads"}`, http.StatusInternalServerError)
			return
		}

		var downloads []map[string]interface{}
		for _, ep := range episodes {
			if af, ok := ep["audioFile"].(map[string]interface{}); ok && af != nil && len(af) > 0 {
				if meta, ok := af["metadata"].(map[string]interface{}); ok && meta != nil {
					if path, ok := meta["path"].(string); ok && path != "" {
						downloads = append(downloads, ep)
					}
				}
			}
		}

		if downloads == nil {
			downloads = []map[string]interface{}{}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"downloads": downloads,
		})
	}
}

func handleSearchEpisode(db *sql.DB, id string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userVal := r.Context().Value(core.UserContextKey)
		if userVal == nil {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}
		user := userVal.(*core.UserSession)

		var podcastID, libraryID string
		err := db.QueryRow(`
			SELECT p.id, li.libraryId
			FROM podcasts p
			JOIN libraryItems li ON li.mediaId = p.id AND li.mediaType = 'podcast'
			WHERE p.id = ? OR li.id = ?
		`, id, id).Scan(&podcastID, &libraryID)
		if err != nil {
			http.Error(w, `{"error": "Podcast not found"}`, http.StatusNotFound)
			return
		}

		if !user.CanAccessLibrary(libraryID) {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		titleQuery := r.URL.Query().Get("title")
		if titleQuery == "" {
			http.Error(w, `{"error": "title parameter is required"}`, http.StatusBadRequest)
			return
		}

		episodes, err := fetchPodcastEpisodesList(r.Context(), db, podcastID)
		if err != nil {
			http.Error(w, `{"error": "Failed to fetch episodes"}`, http.StatusInternalServerError)
			return
		}

		var filtered []map[string]interface{}
		for _, ep := range episodes {
			if title, ok := ep["title"].(string); ok && strings.Contains(strings.ToLower(title), strings.ToLower(titleQuery)) {
				filtered = append(filtered, ep)
			}
		}

		if filtered == nil {
			filtered = []map[string]interface{}{}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"episodes": filtered,
		})
	}
}

func handleGetEpisode(db *sql.DB, id, episodeId string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userVal := r.Context().Value(core.UserContextKey)
		if userVal == nil {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}
		user := userVal.(*core.UserSession)

		var podcastID, libraryID string
		err := db.QueryRow(`
			SELECT p.id, li.libraryId
			FROM podcasts p
			JOIN libraryItems li ON li.mediaId = p.id AND li.mediaType = 'podcast'
			WHERE p.id = ? OR li.id = ?
		`, id, id).Scan(&podcastID, &libraryID)
		if err != nil {
			http.Error(w, `{"error": "Podcast not found"}`, http.StatusNotFound)
			return
		}

		if !user.CanAccessLibrary(libraryID) {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		episodes, err := fetchPodcastEpisodesList(r.Context(), db, podcastID)
		if err != nil {
			http.Error(w, `{"error": "Failed to fetch episodes"}`, http.StatusInternalServerError)
			return
		}

		for _, ep := range episodes {
			if ep["id"].(string) == episodeId {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(ep)
				return
			}
		}

		http.Error(w, `{"error": "Episode not found"}`, http.StatusNotFound)
	}
}
