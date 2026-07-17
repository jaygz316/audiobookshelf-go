package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"

	"audiobookshelf/internal/core"
)

func handleGetPodcastFeed(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if globalPodcastManager == nil {
			initManagers(db)
		}
		var req struct {
			RSSFeed string `json:"rssFeed"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
			return
		}
		if req.RSSFeed == "" {
			http.Error(w, `{"error": "rssFeed parameter is required"}`, http.StatusBadRequest)
			return
		}

		feed, err := globalPodcastManager.FetchFeed(r.Context(), req.RSSFeed)
		if err != nil {
			log.Errorf("[GetPodcastFeed] FetchFeed failed: %v", err)
			http.Error(w, fmt.Sprintf(`{"error": "Failed to fetch podcast feed: %s"}`, err.Error()), http.StatusBadRequest)
			return
		}

		type FeedResponseEpisode struct {
			Title        string  `json:"title"`
			Description  string  `json:"description"`
			PubDate      string  `json:"pubDate"`
			PublishedAt  string  `json:"publishedAt"`
			Duration     float64 `json:"duration"`
			EnclosureURL string  `json:"enclosureUrl"`
		}

		var episodes []*FeedResponseEpisode
		for _, ep := range feed.Episodes {
			episodes = append(episodes, &FeedResponseEpisode{
				Title:        ep.Title,
				Description:  ep.Description,
				PubDate:      ep.PublishedAt,
				PublishedAt:  ep.PublishedAt,
				Duration:     ep.Duration,
				EnclosureURL: ep.EnclosureURL,
			})
		}

		response := map[string]interface{}{
			"podcast": map[string]interface{}{
				"metadata": map[string]interface{}{
					"title":       feed.Title,
					"author":      feed.Author,
					"description": feed.Description,
					"feedUrl":     req.RSSFeed,
				},
				"episodes": episodes,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}
}

func handleCheckNewEpisodes(db *sql.DB, id string) http.HandlerFunc {
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

		err = globalPodcastManager.SyncFeed(r.Context(), podcastID)
		if err != nil {
			log.Errorf("[CheckNewEpisodes] SyncFeed failed: %v", err)
			http.Error(w, fmt.Sprintf(`{"error": "Sync failed: %s"}`, err.Error()), http.StatusInternalServerError)
			return
		}

		episodes, err := fetchPodcastEpisodesList(r.Context(), db, podcastID)
		if err != nil {
			http.Error(w, `{"error": "Failed to fetch episodes"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"episodes": episodes,
		})
	}
}
