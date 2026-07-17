package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"encoding/json"
	"net/http"
	"sort"
	"strings"

	"audiobookshelf/internal/core"
)

// handleGetAllTags returns all unique tags in alphabetical order (case insensitive)
func handleGetAllTags(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Go] GET /api/tags")
		userSess, ok := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if !ok || !userSess.IsAdminOrUp() {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		tagMap := make(map[string]bool)

		// Get tags from books
		rows, err := db.Query("SELECT tags FROM books WHERE tags IS NOT NULL")
		if err != nil {
			log.Errorf("[Tags] Failed to query tags from books: %v", err)
		} else {
			defer rows.Close()
			for rows.Next() {
				var tagsStr sql.NullString
				if err := rows.Scan(&tagsStr); err != nil {
					log.Errorf("[Tags] Failed to scan tag from book: %v", err)
					continue
				}
				if tagsStr.Valid && tagsStr.String != "" {
					var arr []string
					if json.Unmarshal([]byte(tagsStr.String), &arr) == nil {
						for _, t := range arr {
							if t != "" {
								tagMap[t] = true
							}
						}
					}
				}
			}
			if err := rows.Err(); err != nil {
				log.Errorf("[Tags] Books tags query iteration error: %v", err)
			}
		}

		// Get tags from podcasts
		rows2, err := db.Query("SELECT tags FROM podcasts WHERE tags IS NOT NULL")
		if err != nil {
			log.Errorf("[Tags] Failed to query tags from podcasts: %v", err)
		} else {
			defer rows2.Close()
			for rows2.Next() {
				var tagsStr sql.NullString
				if err := rows2.Scan(&tagsStr); err != nil {
					log.Errorf("[Tags] Failed to scan tag from podcast: %v", err)
					continue
				}
				if tagsStr.Valid && tagsStr.String != "" {
					var arr []string
					if json.Unmarshal([]byte(tagsStr.String), &arr) == nil {
						for _, t := range arr {
							if t != "" {
								tagMap[t] = true
							}
						}
					}
				}
			}
			if err := rows2.Err(); err != nil {
				log.Errorf("[Tags] Podcasts tags query iteration error: %v", err)
			}
		}

		tagsList := []string{}
		for t := range tagMap {
			tagsList = append(tagsList, t)
		}

		// Sort case-insensitively
		sort.Slice(tagsList, func(i, j int) bool {
			return strings.ToLower(tagsList[i]) < strings.ToLower(tagsList[j])
		})

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"tags": tagsList,
		})
	}
}
