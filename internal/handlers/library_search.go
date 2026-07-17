package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"

	"audiobookshelf/internal/core"
)

// HandleSearchLibrary handles GET /api/libraries/{libraryID}/search
func HandleSearchLibrary(db *sql.DB, libraryID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Go] GET /api/libraries/%s/search", libraryID)

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

		q := r.URL.Query().Get("q")
		if q == "" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"book":      []interface{}{},
				"podcast":   []interface{}{},
				"episodes":  []interface{}{},
				"authors":   []interface{}{},
				"series":    []interface{}{},
				"tags":      []interface{}{},
				"genres":    []interface{}{},
				"narrators": []interface{}{},
			})
			return
		}

		limitVal := r.URL.Query().Get("limit")
		limit := 3
		if limitVal != "" {
			if l, err := strconv.Atoi(limitVal); err == nil && l > 0 {
				limit = l
			}
		}

		bookResults := searchBooks(db, libraryID, user, q, limit)
		podcastResults := searchPodcasts(db, libraryID, user, q, limit)
		episodeResults := searchEpisodes(db, libraryID, q, limit)
		authorResults := searchAuthors(db, libraryID, q, limit)
		seriesResults := searchSeries(db, libraryID, q, limit)
		matchedTags, matchedGenres, matchedNarrators := searchTaxonomy(db, libraryID, q, limit)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"book":      bookResults,
			"podcast":   podcastResults,
			"episodes":  episodeResults,
			"authors":   authorResults,
			"series":    seriesResults,
			"tags":      matchedTags,
			"genres":    matchedGenres,
			"narrators": matchedNarrators,
		})
	}
}
