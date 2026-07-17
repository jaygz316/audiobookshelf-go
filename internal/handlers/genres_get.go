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

// handleGetAllGenres returns all unique genres in alphabetical order
func handleGetAllGenres(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Go] GET /api/genres")
		userSess, ok := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if !ok || !userSess.IsAdminOrUp() {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		genreMap := make(map[string]bool)

		// Get genres from books
		rows, err := db.Query("SELECT genres FROM books WHERE genres IS NOT NULL")
		if err != nil {
			log.Errorf("[Genres] Failed to query genres from books: %v", err)
		} else {
			defer rows.Close()
			for rows.Next() {
				var gStr sql.NullString
				if err := rows.Scan(&gStr); err != nil {
					log.Errorf("[Genres] Failed to scan genre: %v", err)
					continue
				}
				if gStr.Valid && gStr.String != "" {
					var arr []string
					if json.Unmarshal([]byte(gStr.String), &arr) == nil {
						for _, g := range arr {
							if g != "" {
								genreMap[g] = true
							}
						}
					}
				}
			}
			if err := rows.Err(); err != nil {
				log.Errorf("[Genres] Books query iteration error: %v", err)
			}
		}

		// Get genres from podcasts
		rows2, err := db.Query("SELECT genres FROM podcasts WHERE genres IS NOT NULL")
		if err != nil {
			log.Errorf("[Genres] Failed to query genres from podcasts: %v", err)
		} else {
			defer rows2.Close()
			for rows2.Next() {
				var gStr sql.NullString
				if err := rows2.Scan(&gStr); err != nil {
					log.Errorf("[Genres] Failed to scan genre: %v", err)
					continue
				}
				if gStr.Valid && gStr.String != "" {
					var arr []string
					if json.Unmarshal([]byte(gStr.String), &arr) == nil {
						for _, g := range arr {
							if g != "" {
								genreMap[g] = true
							}
						}
					}
				}
			}
			if err := rows2.Err(); err != nil {
				log.Errorf("[Genres] Podcasts query iteration error: %v", err)
			}
		}

		genresList := []string{}
		for g := range genreMap {
			genresList = append(genresList, g)
		}

		// Sort case-insensitively
		sort.Slice(genresList, func(i, j int) bool {
			return strings.ToLower(genresList[i]) < strings.ToLower(genresList[j])
		})

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"genres": genresList,
		})
	}
}
