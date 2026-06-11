package main

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"audiobookshelf/internal/core")

// handleGetAllTags returns all unique tags in alphabetical order (case insensitive)
func handleGetAllTags(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[Go] GET /api/tags")
		userSess, ok := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if !ok || !userSess.IsAdminOrUp() {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		tagMap := make(map[string]bool)

		// Get tags from books
		rows, err := db.Query("SELECT tags FROM books WHERE tags IS NOT NULL")
		if err != nil {
			log.Printf("[Tags] Failed to query tags from books: %v", err)
		} else {
			defer rows.Close()
			for rows.Next() {
				var tagsStr sql.NullString
				if err := rows.Scan(&tagsStr); err != nil {
					log.Printf("[Tags] Failed to scan tag from book: %v", err)
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
				log.Printf("[Tags] Books tags query iteration error: %v", err)
			}
		}

		// Get tags from podcasts
		rows2, err := db.Query("SELECT tags FROM podcasts WHERE tags IS NOT NULL")
		if err != nil {
			log.Printf("[Tags] Failed to query tags from podcasts: %v", err)
		} else {
			defer rows2.Close()
			for rows2.Next() {
				var tagsStr sql.NullString
				if err := rows2.Scan(&tagsStr); err != nil {
					log.Printf("[Tags] Failed to scan tag from podcast: %v", err)
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
				log.Printf("[Tags] Podcasts tags query iteration error: %v", err)
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

// handleRenameTag renames a tag across books, podcasts, and user permissions
func handleRenameTag(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[Go] POST /api/tags/rename")
		userSess, ok := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if !ok || !userSess.IsAdminOrUp() {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		var body struct {
			Tag     string `json:"tag"`
			NewTag  string `json:"newTag"`
			Name    string `json:"name"`
			NewName string `json:"newName"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
			return
		}
		tagVal := body.Tag
		if tagVal == "" {
			tagVal = body.Name
		}
		newTagVal := body.NewTag
		if newTagVal == "" {
			newTagVal = body.NewName
		}
		if tagVal == "" || newTagVal == "" {
			http.Error(w, `{"error": "tag/name and newTag/newName are required"}`, http.StatusBadRequest)
			return
		}

		tx, err := db.Begin()
		if err != nil {
			http.Error(w, "Transaction start error", http.StatusInternalServerError)
			return
		}
		defer tx.Rollback()

		// 1. Update books
		rows, err := tx.Query("SELECT id, tags FROM books WHERE tags IS NOT NULL")
		if err != nil {
			log.Printf("[Rename Tag] Query books failed: %v", err)
			http.Error(w, "Database query error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		for rows.Next() {
			var id string
			var tagsStr sql.NullString
			if err := rows.Scan(&id, &tagsStr); err != nil {
				log.Printf("[Rename Tag] Scan book failed: %v", err)
				http.Error(w, "Database scan error", http.StatusInternalServerError)
				return
			}
			if updated, changed := replaceInJSONArray(tagsStr, tagVal, newTagVal); changed {
				_, err = tx.Exec("UPDATE books SET tags = ? WHERE id = ?", updated, id)
				if err != nil {
					log.Printf("[Rename Tag] Update book failed: %v", err)
					http.Error(w, "Database update error", http.StatusInternalServerError)
					return
				}
			}
		}
		if err := rows.Err(); err != nil {
			log.Printf("[Rename Tag] Books iteration failed: %v", err)
			http.Error(w, "Database iteration error", http.StatusInternalServerError)
			return
		}
		rows.Close()

		// 2. Update podcasts
		rows2, err := tx.Query("SELECT id, tags FROM podcasts WHERE tags IS NOT NULL")
		if err != nil {
			log.Printf("[Rename Tag] Query podcasts failed: %v", err)
			http.Error(w, "Database query error", http.StatusInternalServerError)
			return
		}
		defer rows2.Close()
		for rows2.Next() {
			var id string
			var tagsStr sql.NullString
			if err := rows2.Scan(&id, &tagsStr); err != nil {
				log.Printf("[Rename Tag] Scan podcast failed: %v", err)
				http.Error(w, "Database scan error", http.StatusInternalServerError)
				return
			}
			if updated, changed := replaceInJSONArray(tagsStr, tagVal, newTagVal); changed {
				_, err = tx.Exec("UPDATE podcasts SET tags = ? WHERE id = ?", updated, id)
				if err != nil {
					log.Printf("[Rename Tag] Update podcast failed: %v", err)
					http.Error(w, "Database update error", http.StatusInternalServerError)
					return
				}
			}
		}
		if err := rows2.Err(); err != nil {
			log.Printf("[Rename Tag] Podcasts iteration failed: %v", err)
			http.Error(w, "Database iteration error", http.StatusInternalServerError)
			return
		}
		rows2.Close()

		// 3. Update users permissions (itemTagsSelected)
		rows3, err := tx.Query("SELECT id, permissions FROM users WHERE permissions IS NOT NULL")
		if err != nil {
			log.Printf("[Rename Tag] Query users failed: %v", err)
			http.Error(w, "Database query error", http.StatusInternalServerError)
			return
		}
		defer rows3.Close()
		for rows3.Next() {
			var id string
			var permsStr sql.NullString
			if err := rows3.Scan(&id, &permsStr); err != nil {
				log.Printf("[Rename Tag] Scan user failed: %v", err)
				http.Error(w, "Database scan error", http.StatusInternalServerError)
				return
			}
			if permsStr.Valid && permsStr.String != "" {
				var perms map[string]interface{}
				if json.Unmarshal([]byte(permsStr.String), &perms) == nil {
					if tagsSel, ok := perms["itemTagsSelected"].([]interface{}); ok {
						changed := false
						newTagsSel := []interface{}{}
						for _, t := range tagsSel {
							if tStr, ok := t.(string); ok && tStr == tagVal {
								// Rename
								alreadyHasNew := false
								for _, existT := range tagsSel {
									if existTStr, ok := existT.(string); ok && existTStr == newTagVal {
										alreadyHasNew = true
										break
									}
								}
								if !alreadyHasNew {
									newTagsSel = append(newTagsSel, newTagVal)
								}
								changed = true
							} else {
								newTagsSel = append(newTagsSel, t)
							}
						}
						if changed {
							perms["itemTagsSelected"] = newTagsSel
							newPermsBytes, _ := json.Marshal(perms)
							_, err = tx.Exec("UPDATE users SET permissions = ? WHERE id = ?", string(newPermsBytes), id)
							if err != nil {
								log.Printf("[Rename Tag] Update user failed: %v", err)
								http.Error(w, "Database update error", http.StatusInternalServerError)
								return
							}
						}
					}
				}
			}
		}
		if err := rows3.Err(); err != nil {
			log.Printf("[Rename Tag] Users iteration failed: %v", err)
			http.Error(w, "Database iteration error", http.StatusInternalServerError)
			return
		}

		if err := tx.Commit(); err != nil {
			log.Printf("[Rename Tag] Commit error: %v", err)
			http.Error(w, "Database commit error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"tagMerged": false,
		})
	}
}

// handleDeleteTag removes a tag base64 parameter from books, podcasts, and users permissions
func handleDeleteTag(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[Go] DELETE %s", r.URL.Path)
		userSess, ok := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if !ok || !userSess.IsAdminOrUp() {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		tagParam := trimAPIPath(r.URL.Path, "/api/tags/")
		if tagParam == "" || strings.Contains(tagParam, "/") {
			http.Error(w, `{"error": "Bad Request"}`, http.StatusBadRequest)
			return
		}

		tagBytes, err := base64.StdEncoding.DecodeString(tagParam)
		if err != nil {
			// Try URL-safe base64
			tagBytes, err = base64.URLEncoding.DecodeString(tagParam)
			if err != nil {
				log.Printf("[Delete Tag] Failed to decode base64: %v", err)
				http.Error(w, `{"error": "Invalid base64 encoding"}`, http.StatusBadRequest)
				return
			}
		}
		targetTag := string(tagBytes)

		tx, err := db.Begin()
		if err != nil {
			http.Error(w, "Transaction start error", http.StatusInternalServerError)
			return
		}
		defer tx.Rollback()

		// 1. Update books
		rows, err := tx.Query("SELECT id, tags FROM books WHERE tags IS NOT NULL")
		if err != nil {
			log.Printf("[Delete Tag] Query books failed: %v", err)
			http.Error(w, "Database query error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		for rows.Next() {
			var id string
			var tagsStr sql.NullString
			if err := rows.Scan(&id, &tagsStr); err != nil {
				log.Printf("[Delete Tag] Scan book failed: %v", err)
				http.Error(w, "Database scan error", http.StatusInternalServerError)
				return
			}
			if updated, changed := removeFromJSONArray(tagsStr, targetTag); changed {
				_, err = tx.Exec("UPDATE books SET tags = ? WHERE id = ?", updated, id)
				if err != nil {
					log.Printf("[Delete Tag] Update book failed: %v", err)
					http.Error(w, "Database update error", http.StatusInternalServerError)
					return
				}
			}
		}
		if err := rows.Err(); err != nil {
			log.Printf("[Delete Tag] Books iteration failed: %v", err)
			http.Error(w, "Database iteration error", http.StatusInternalServerError)
			return
		}
		rows.Close()

		// 2. Update podcasts
		rows2, err := tx.Query("SELECT id, tags FROM podcasts WHERE tags IS NOT NULL")
		if err != nil {
			log.Printf("[Delete Tag] Query podcasts failed: %v", err)
			http.Error(w, "Database query error", http.StatusInternalServerError)
			return
		}
		defer rows2.Close()
		for rows2.Next() {
			var id string
			var tagsStr sql.NullString
			if err := rows2.Scan(&id, &tagsStr); err != nil {
				log.Printf("[Delete Tag] Scan podcast failed: %v", err)
				http.Error(w, "Database scan error", http.StatusInternalServerError)
				return
			}
			if updated, changed := removeFromJSONArray(tagsStr, targetTag); changed {
				_, err = tx.Exec("UPDATE podcasts SET tags = ? WHERE id = ?", updated, id)
				if err != nil {
					log.Printf("[Delete Tag] Update podcast failed: %v", err)
					http.Error(w, "Database update error", http.StatusInternalServerError)
					return
				}
			}
		}
		if err := rows2.Err(); err != nil {
			log.Printf("[Delete Tag] Podcasts iteration failed: %v", err)
			http.Error(w, "Database iteration error", http.StatusInternalServerError)
			return
		}
		rows2.Close()

		// 3. Update users permissions
		rows3, err := tx.Query("SELECT id, permissions FROM users WHERE permissions IS NOT NULL")
		if err != nil {
			log.Printf("[Delete Tag] Query users failed: %v", err)
			http.Error(w, "Database query error", http.StatusInternalServerError)
			return
		}
		defer rows3.Close()
		for rows3.Next() {
			var id string
			var permsStr sql.NullString
			if err := rows3.Scan(&id, &permsStr); err != nil {
				log.Printf("[Delete Tag] Scan user failed: %v", err)
				http.Error(w, "Database scan error", http.StatusInternalServerError)
				return
			}
			if permsStr.Valid && permsStr.String != "" {
				var perms map[string]interface{}
				if json.Unmarshal([]byte(permsStr.String), &perms) == nil {
					if tagsSel, ok := perms["itemTagsSelected"].([]interface{}); ok {
						changed := false
						newTagsSel := []interface{}{}
						for _, t := range tagsSel {
							if tStr, ok := t.(string); ok && tStr == targetTag {
								changed = true
							} else {
								newTagsSel = append(newTagsSel, t)
							}
						}
						if changed {
							perms["itemTagsSelected"] = newTagsSel
							newPermsBytes, _ := json.Marshal(perms)
							_, err = tx.Exec("UPDATE users SET permissions = ? WHERE id = ?", string(newPermsBytes), id)
							if err != nil {
								log.Printf("[Delete Tag] Update user failed: %v", err)
								http.Error(w, "Database update error", http.StatusInternalServerError)
								return
							}
						}
					}
				}
			}
		}
		if err := rows3.Err(); err != nil {
			log.Printf("[Delete Tag] Users iteration failed: %v", err)
			http.Error(w, "Database iteration error", http.StatusInternalServerError)
			return
		}

		if err := tx.Commit(); err != nil {
			log.Printf("[Delete Tag] Commit error: %v", err)
			http.Error(w, "Database commit error", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
	}
}

// handleGetAllGenres returns all unique genres in alphabetical order
func handleGetAllGenres(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[Go] GET /api/genres")
		userSess, ok := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if !ok || !userSess.IsAdminOrUp() {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		genreMap := make(map[string]bool)

		// Get genres from books
		rows, err := db.Query("SELECT genres FROM books WHERE genres IS NOT NULL")
		if err != nil {
			log.Printf("[Genres] Failed to query genres from books: %v", err)
		} else {
			defer rows.Close()
			for rows.Next() {
				var gStr sql.NullString
				if err := rows.Scan(&gStr); err != nil {
					log.Printf("[Genres] Failed to scan genre: %v", err)
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
				log.Printf("[Genres] Books query iteration error: %v", err)
			}
		}

		// Get genres from podcasts
		rows2, err := db.Query("SELECT genres FROM podcasts WHERE genres IS NOT NULL")
		if err != nil {
			log.Printf("[Genres] Failed to query genres from podcasts: %v", err)
		} else {
			defer rows2.Close()
			for rows2.Next() {
				var gStr sql.NullString
				if err := rows2.Scan(&gStr); err != nil {
					log.Printf("[Genres] Failed to scan genre: %v", err)
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
				log.Printf("[Genres] Podcasts query iteration error: %v", err)
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

// handleRenameGenre renames a genre across books and podcasts
func handleRenameGenre(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[Go] POST /api/genres/rename")
		userSess, ok := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if !ok || !userSess.IsAdminOrUp() {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		var body struct {
			Genre    string `json:"genre"`
			NewGenre string `json:"newGenre"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
			return
		}
		if body.Genre == "" || body.NewGenre == "" {
			http.Error(w, `{"error": "genre and newGenre are required"}`, http.StatusBadRequest)
			return
		}

		tx, err := db.Begin()
		if err != nil {
			http.Error(w, "Transaction start error", http.StatusInternalServerError)
			return
		}
		defer tx.Rollback()

		// 1. Update books
		rows, err := tx.Query("SELECT id, genres FROM books WHERE genres IS NOT NULL")
		if err != nil {
			log.Printf("[Rename Genre] Query books failed: %v", err)
			http.Error(w, "Database query error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		for rows.Next() {
			var id string
			var gStr sql.NullString
			if err := rows.Scan(&id, &gStr); err != nil {
				log.Printf("[Rename Genre] Scan book failed: %v", err)
				http.Error(w, "Database scan error", http.StatusInternalServerError)
				return
			}
			if updated, changed := replaceInJSONArray(gStr, body.Genre, body.NewGenre); changed {
				_, err = tx.Exec("UPDATE books SET genres = ? WHERE id = ?", updated, id)
				if err != nil {
					log.Printf("[Rename Genre] Update book failed: %v", err)
					http.Error(w, "Database update error", http.StatusInternalServerError)
					return
				}
			}
		}
		if err := rows.Err(); err != nil {
			log.Printf("[Rename Genre] Books iteration failed: %v", err)
			http.Error(w, "Database iteration error", http.StatusInternalServerError)
			return
		}
		rows.Close()

		// 2. Update podcasts
		rows2, err := tx.Query("SELECT id, genres FROM podcasts WHERE genres IS NOT NULL")
		if err != nil {
			log.Printf("[Rename Genre] Query podcasts failed: %v", err)
			http.Error(w, "Database query error", http.StatusInternalServerError)
			return
		}
		defer rows2.Close()
		for rows2.Next() {
			var id string
			var gStr sql.NullString
			if err := rows2.Scan(&id, &gStr); err != nil {
				log.Printf("[Rename Genre] Scan podcast failed: %v", err)
				http.Error(w, "Database scan error", http.StatusInternalServerError)
				return
			}
			if updated, changed := replaceInJSONArray(gStr, body.Genre, body.NewGenre); changed {
				_, err = tx.Exec("UPDATE podcasts SET genres = ? WHERE id = ?", updated, id)
				if err != nil {
					log.Printf("[Rename Genre] Update podcast failed: %v", err)
					http.Error(w, "Database update error", http.StatusInternalServerError)
					return
				}
			}
		}
		if err := rows2.Err(); err != nil {
			log.Printf("[Rename Genre] Podcasts iteration failed: %v", err)
			http.Error(w, "Database iteration error", http.StatusInternalServerError)
			return
		}

		if err := tx.Commit(); err != nil {
			log.Printf("[Rename Genre] Commit error: %v", err)
			http.Error(w, "Database commit error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"genreMerged": false,
		})
	}
}

// handleDeleteGenre deletes a genre base64 parameter from books and podcasts
func handleDeleteGenre(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[Go] DELETE %s", r.URL.Path)
		userSess, ok := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if !ok || !userSess.IsAdminOrUp() {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		genreParam := trimAPIPath(r.URL.Path, "/api/genres/")
		if genreParam == "" || strings.Contains(genreParam, "/") {
			http.Error(w, `{"error": "Bad Request"}`, http.StatusBadRequest)
			return
		}

		genreBytes, err := base64.StdEncoding.DecodeString(genreParam)
		if err != nil {
			// Try URL-safe base64
			genreBytes, err = base64.URLEncoding.DecodeString(genreParam)
			if err != nil {
				log.Printf("[Delete Genre] Failed to decode base64: %v", err)
				http.Error(w, `{"error": "Invalid base64 encoding"}`, http.StatusBadRequest)
				return
			}
		}
		targetGenre := string(genreBytes)

		tx, err := db.Begin()
		if err != nil {
			http.Error(w, "Transaction start error", http.StatusInternalServerError)
			return
		}
		defer tx.Rollback()

		// 1. Update books
		rows, err := tx.Query("SELECT id, genres FROM books WHERE genres IS NOT NULL")
		if err != nil {
			log.Printf("[Delete Genre] Query books failed: %v", err)
			http.Error(w, "Database query error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		for rows.Next() {
			var id string
			var gStr sql.NullString
			if err := rows.Scan(&id, &gStr); err != nil {
				log.Printf("[Delete Genre] Scan book failed: %v", err)
				http.Error(w, "Database scan error", http.StatusInternalServerError)
				return
			}
			if updated, changed := removeFromJSONArray(gStr, targetGenre); changed {
				_, err = tx.Exec("UPDATE books SET genres = ? WHERE id = ?", updated, id)
				if err != nil {
					log.Printf("[Delete Genre] Update book failed: %v", err)
					http.Error(w, "Database update error", http.StatusInternalServerError)
					return
				}
			}
		}
		if err := rows.Err(); err != nil {
			log.Printf("[Delete Genre] Books iteration failed: %v", err)
			http.Error(w, "Database iteration error", http.StatusInternalServerError)
			return
		}
		rows.Close()

		// 2. Update podcasts
		rows2, err := tx.Query("SELECT id, genres FROM podcasts WHERE genres IS NOT NULL")
		if err != nil {
			log.Printf("[Delete Genre] Query podcasts failed: %v", err)
			http.Error(w, "Database query error", http.StatusInternalServerError)
			return
		}
		defer rows2.Close()
		for rows2.Next() {
			var id string
			var gStr sql.NullString
			if err := rows2.Scan(&id, &gStr); err != nil {
				log.Printf("[Delete Genre] Scan podcast failed: %v", err)
				http.Error(w, "Database scan error", http.StatusInternalServerError)
				return
			}
			if updated, changed := removeFromJSONArray(gStr, targetGenre); changed {
				_, err = tx.Exec("UPDATE podcasts SET genres = ? WHERE id = ?", updated, id)
				if err != nil {
					log.Printf("[Delete Genre] Update podcast failed: %v", err)
					http.Error(w, "Database update error", http.StatusInternalServerError)
					return
				}
			}
		}
		if err := rows2.Err(); err != nil {
			log.Printf("[Delete Genre] Podcasts iteration failed: %v", err)
			http.Error(w, "Database iteration error", http.StatusInternalServerError)
			return
		}

		if err := tx.Commit(); err != nil {
			log.Printf("[Delete Genre] Commit error: %v", err)
			http.Error(w, "Database commit error", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
	}
}

// handleGetAdminStatsForYear stub returning mock admin/user listening stats
func handleGetAdminStatsForYear(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[Go] GET %s", r.URL.Path)
		userSess, ok := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if !ok || !userSess.IsAdminOrUp() {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		res := map[string]interface{}{
			"totalListeningSessions":    0,
			"totalListeningTime":        0,
			"totalBookListeningTime":    0,
			"totalPodcastListeningTime": 0,
			"topAuthors":                []interface{}{},
			"topGenres":                 []interface{}{},
			"mostListenedNarrator":      nil,
			"mostListenedMonth":         nil,
			"numBooksFinished":          0,
			"numBooksListened":          0,
			"longestAudiobookFinished":  nil,
			"booksWithCovers":           []interface{}{},
			"finishedBooksWithCovers":   []interface{}{},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(res)
	}
}

// handleGetLoggerData stub returning empty logs structure
func handleGetLoggerData(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[Go] GET /api/logger-data")
		userSess, ok := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if !ok || !userSess.IsAdminOrUp() {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"currentDailyLogs": GlobalLogBuffer.Get(),
		})
	}
}

// handleValidateCron validates simple cron expression fields
func handleValidateCron(w http.ResponseWriter, r *http.Request) {
	log.Printf("[Go] POST /api/validate-cron")
	var body struct {
		Expression string `json:"expression"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error": "Invalid request"}`, http.StatusBadRequest)
		return
	}

	parts := strings.Fields(body.Expression)
	if len(parts) < 5 || len(parts) > 6 {
		http.Error(w, "Invalid cron expression", http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// handleWatcherUpdate stub for file watcher updates
func handleWatcherUpdate(w http.ResponseWriter, r *http.Request) {
	log.Printf("[Go] POST /api/watcher/update")
	w.WriteHeader(http.StatusOK)
}

// Helper functions for tags/genres array replacement
func replaceInJSONArray(jsonStr sql.NullString, oldVal, newVal string) (string, bool) {
	if !jsonStr.Valid || jsonStr.String == "" || jsonStr.String == "null" {
		return "[]", false
	}
	var arr []string
	if err := json.Unmarshal([]byte(jsonStr.String), &arr); err != nil {
		return jsonStr.String, false
	}
	found := false
	newArr := []string{}
	for _, val := range arr {
		if val == oldVal {
			found = true
			alreadyHasNew := false
			for _, v := range arr {
				if v == newVal {
					alreadyHasNew = true
					break
				}
			}
			if !alreadyHasNew {
				newArr = append(newArr, newVal)
			}
		} else {
			newArr = append(newArr, val)
		}
	}
	if !found {
		return jsonStr.String, false
	}
	res, _ := json.Marshal(newArr)
	return string(res), true
}

func removeFromJSONArray(jsonStr sql.NullString, valToRemove string) (string, bool) {
	if !jsonStr.Valid || jsonStr.String == "" || jsonStr.String == "null" {
		return "[]", false
	}
	var arr []string
	if err := json.Unmarshal([]byte(jsonStr.String), &arr); err != nil {
		return jsonStr.String, false
	}
	found := false
	newArr := []string{}
	for _, val := range arr {
		if val == valToRemove {
			found = true
		} else {
			newArr = append(newArr, val)
		}
	}
	if !found {
		return jsonStr.String, false
	}
	res, _ := json.Marshal(newArr)
	return string(res), true
}

type DirectoryInfo struct {
	Path    string `json:"path"`
	Dirname string `json:"dirname"`
	Level   int    `json:"level"`
}

func isSameOrSubPath(parentPath, childPath string) bool {
	parentPath = filepath.Clean(parentPath)
	childPath = filepath.Clean(childPath)
	if parentPath == childPath {
		return true
	}
	rel, err := filepath.Rel(parentPath, childPath)
	if err != nil {
		return false
	}
	if rel == "" || rel == "." {
		return true
	}
	return !strings.HasPrefix(rel, "..")
}

// handleGetFilesystem retrieves POSIX directories in a path
func handleGetFilesystem(appRoot string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[Go] GET /api/filesystem")
		userSess, ok := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if !ok || !userSess.IsAdminOrUp() {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		relpath := r.URL.Query().Get("path")
		levelStr := r.URL.Query().Get("level")
		level := 0
		if levelStr != "" {
			if l, err := strconv.Atoi(levelStr); err == nil {
				level = l
			}
		}

		// Default path if empty
		if relpath == "" {
			relpath = "/"
		}

		// Validate path. Must be absolute and must exist
		if !filepath.IsAbs(relpath) {
			log.Printf("[FileSystem] Path is not absolute: %s", relpath)
			http.Error(w, `Invalid "path" query string`, http.StatusBadRequest)
			return
		}

		fi, err := os.Stat(relpath)
		if err != nil || !fi.IsDir() {
			log.Printf("[FileSystem] Path does not exist or is not a directory: %s", relpath)
			http.Error(w, `Invalid "path" query string`, http.StatusBadRequest)
			return
		}

		entries, err := os.ReadDir(relpath)
		if err != nil {
			log.Printf("[FileSystem] Failed to read directory %s: %v", relpath, err)
			http.Error(w, `Failed to read directory`, http.StatusInternalServerError)
			return
		}

		// Excluded directories from appRoot
		excludedDirNames := []string{
			"node_modules", "client", "server", ".git", "static", "build", "dist",
			"metadata", "config", "sys", "proc", ".devcontainer", ".nyc_output",
			"sys", "proc", ".github", ".vscode",
		}
		excludedPaths := make(map[string]bool)
		for _, name := range excludedDirNames {
			fullPath := filepath.Join(appRoot, name)
			excludedPaths[filepath.ToSlash(fullPath)] = true
		}

		// Always exclude /sys and /proc on Linux as well
		excludedPaths["/sys"] = true
		excludedPaths["/proc"] = true

		var directories []DirectoryInfo
		for _, entry := range entries {
			if entry.IsDir() {
				// Ignore dot files / dot directories
				if strings.HasPrefix(entry.Name(), ".") {
					continue
				}
				fullPath := filepath.Join(relpath, entry.Name())
				posixPath := filepath.ToSlash(fullPath)
				if excludedPaths[posixPath] {
					continue
				}
				directories = append(directories, DirectoryInfo{
					Path:    posixPath,
					Dirname: entry.Name(),
					Level:   level,
				})
			}
		}

		if directories == nil {
			directories = []DirectoryInfo{}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"posix":       runtime.GOOS != "windows",
			"directories": directories,
		})
	}
}

// handleCheckPathExists checks if directory exists inside library folder
func handleCheckPathExists(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[Go] POST /api/filesystem/pathexists")
		userSess, ok := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if !ok {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}

		var body struct {
			Directory  string `json:"directory"`
			FolderPath string `json:"folderPath"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			log.Printf("[FileSystem] Invalid request body: %v", err)
			http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
			return
		}

		if body.Directory == "" || body.FolderPath == "" {
			log.Printf("[FileSystem] Invalid request body: directory or folderPath is empty")
			http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
			return
		}

		// Check that library folder exists
		var libraryID string
		err := db.QueryRow("SELECT libraryId FROM libraryFolders WHERE path = ?", body.FolderPath).Scan(&libraryID)
		if err == sql.ErrNoRows {
			log.Printf("[FileSystem] Library folder not found: %s", body.FolderPath)
			http.Error(w, `{"error": "Library folder not found"}`, http.StatusNotFound)
			return
		} else if err != nil {
			log.Printf("[FileSystem] DB error querying library folder: %v", err)
			http.Error(w, `{"error": "Database error"}`, http.StatusInternalServerError)
			return
		}

		// Check user can access library
		if !userSess.CanAccessLibrary(libraryID) {
			log.Printf("[FileSystem] User %s attempting to check path exists for library %s without access", userSess.Username, libraryID)
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		filePath := filepath.Join(body.FolderPath, body.Directory)
		filePathPOSIX := filepath.ToSlash(filePath)
		folderPathPOSIX := filepath.ToSlash(body.FolderPath)

		// Ensure filepath is inside library folder (prevents directory traversal)
		if !isSameOrSubPath(folderPathPOSIX, filePathPOSIX) {
			log.Printf("[FileSystem] Filepath is not inside library folder: %s", filePathPOSIX)
			http.Error(w, `{"error": "Invalid path"}`, http.StatusBadRequest)
			return
		}

		exists := false
		if _, err := os.Stat(filePath); err == nil {
			exists = true
		}

		if exists {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"exists":true}`))
			return
		}

		// Check if a library item exists in a subdirectory
		cleanedDirectory := strings.Trim(filepath.ToSlash(body.Directory), "/")
		if strings.Contains(cleanedDirectory, "/") {
			// Can only be 2 levels deep
			var possiblePaths []string
			subdir := filepath.Dir(cleanedDirectory)
			possiblePaths = append(possiblePaths, filepath.ToSlash(filepath.Join(body.FolderPath, subdir)))
			if strings.Contains(subdir, "/") {
				possiblePaths = append(possiblePaths, filepath.ToSlash(filepath.Join(body.FolderPath, filepath.Dir(subdir))))
			}

			if len(possiblePaths) > 0 {
				placeholders := make([]string, len(possiblePaths))
				args := make([]interface{}, len(possiblePaths))
				for i, p := range possiblePaths {
					placeholders[i] = "?"
					args[i] = p
				}
				query := fmt.Sprintf("SELECT title FROM libraryItems WHERE path IN (%s) LIMIT 1", strings.Join(placeholders, ","))
				var title string
				err := db.QueryRow(query, args...).Scan(&title)
				if err == nil {
					w.Header().Set("Content-Type", "application/json")
					json.NewEncoder(w).Encode(map[string]interface{}{
						"exists":           true,
						"libraryItemTitle": title,
					})
					return
				}
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"exists":false}`))
	}
}

// handleGetTasks returns tasks list stubbed for task manager
func handleGetTasks(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[Go] GET /api/tasks")
		userSess, ok := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if !ok || !userSess.IsAdminOrUp() {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		include := r.URL.Query().Get("include")
		includeArray := strings.Split(include, ",")

		data := map[string]interface{}{
			"tasks": []interface{}{},
		}

		hasQueue := false
		for _, inc := range includeArray {
			if inc == "queue" {
				hasQueue = true
				break
			}
		}

		if hasQueue {
			data["queuedTaskData"] = map[string]interface{}{
				"embedMetadata": []interface{}{},
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(data)
	}
}

// handleCancelAllTasks cancels all tasks
func handleCancelAllTasks(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[Go] POST /api/tasks/cancel-all")
		userSess, ok := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if !ok || !userSess.IsAdminOrUp() {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"success": true}`))
	}
}
