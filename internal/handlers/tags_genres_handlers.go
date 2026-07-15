package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"sort"
	"strings"

	"audiobookshelf/internal/core"
	"audiobookshelf/internal/utils"
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

// handleRenameTag renames a tag across books, podcasts, and user permissions
func handleRenameTag(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Go] POST /api/tags/rename")
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
			log.Errorf("[Rename Tag] Query books failed: %v", err)
			http.Error(w, "Database query error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		var bookUpdateIds []string
		var bookUpdateArgs []interface{}
		var bookUpdateCases []string
		for rows.Next() {
			var id string
			var tagsStr sql.NullString
			if err := rows.Scan(&id, &tagsStr); err != nil {
				log.Errorf("[Rename Tag] Scan book failed: %v", err)
				http.Error(w, "Database scan error", http.StatusInternalServerError)
				return
			}
			if updated, changed := utils.ReplaceInJSONArray(tagsStr, tagVal, newTagVal); changed {
				bookUpdateIds = append(bookUpdateIds, id)
				bookUpdateArgs = append(bookUpdateArgs, id, updated)
				bookUpdateCases = append(bookUpdateCases, "WHEN ? THEN ?")
			}
		}
		if err := rows.Err(); err != nil {
			log.Errorf("[Rename Tag] Books iteration failed: %v", err)
			http.Error(w, "Database iteration error", http.StatusInternalServerError)
			return
		}
		rows.Close() // Explicitly close before exec
		if len(bookUpdateIds) > 0 {
			chunkSize := 1000
			for i := 0; i < len(bookUpdateIds); i += chunkSize {
				end := i + chunkSize
				if end > len(bookUpdateIds) {
					end = len(bookUpdateIds)
				}
				chunkIds := bookUpdateIds[i:end]
				chunkArgs := bookUpdateArgs[i*2 : end*2]
				chunkCases := bookUpdateCases[i:end]

				query := "UPDATE books SET tags = CASE id " + strings.Join(chunkCases, " ") + " END WHERE id IN (?" + strings.Repeat(",?", len(chunkIds)-1) + ")"
				args := append(chunkArgs, make([]interface{}, len(chunkIds))...)
				for j, id := range chunkIds {
					args[len(chunkArgs)+j] = id
				}
				_, err = tx.Exec(query, args...)
				if err != nil {
					log.Errorf("[Rename Tag] Update book failed: %v", err)
					http.Error(w, "Database update error", http.StatusInternalServerError)
					return
				}
			}
		}

		// 2. Update podcasts
		rows2, err := tx.Query("SELECT id, tags FROM podcasts WHERE tags IS NOT NULL")
		if err != nil {
			log.Errorf("[Rename Tag] Query podcasts failed: %v", err)
			http.Error(w, "Database query error", http.StatusInternalServerError)
			return
		}
		defer rows2.Close()
		var podcastUpdateIds []string
		var podcastUpdateArgs []interface{}
		var podcastUpdateCases []string
		for rows2.Next() {
			var id string
			var tagsStr sql.NullString
			if err := rows2.Scan(&id, &tagsStr); err != nil {
				log.Errorf("[Rename Tag] Scan podcast failed: %v", err)
				http.Error(w, "Database scan error", http.StatusInternalServerError)
				return
			}
			if updated, changed := utils.ReplaceInJSONArray(tagsStr, tagVal, newTagVal); changed {
				podcastUpdateIds = append(podcastUpdateIds, id)
				podcastUpdateArgs = append(podcastUpdateArgs, id, updated)
				podcastUpdateCases = append(podcastUpdateCases, "WHEN ? THEN ?")
			}
		}
		if err := rows2.Err(); err != nil {
			log.Errorf("[Rename Tag] Podcasts iteration failed: %v", err)
			http.Error(w, "Database iteration error", http.StatusInternalServerError)
			return
		}
		rows2.Close() // Explicitly close before exec
		if len(podcastUpdateIds) > 0 {
			chunkSize := 1000
			for i := 0; i < len(podcastUpdateIds); i += chunkSize {
				end := i + chunkSize
				if end > len(podcastUpdateIds) {
					end = len(podcastUpdateIds)
				}
				chunkIds := podcastUpdateIds[i:end]
				chunkArgs := podcastUpdateArgs[i*2 : end*2]
				chunkCases := podcastUpdateCases[i:end]

				query := "UPDATE podcasts SET tags = CASE id " + strings.Join(chunkCases, " ") + " END WHERE id IN (?" + strings.Repeat(",?", len(chunkIds)-1) + ")"
				args := append(chunkArgs, make([]interface{}, len(chunkIds))...)
				for j, id := range chunkIds {
					args[len(chunkArgs)+j] = id
				}
				_, err = tx.Exec(query, args...)
				if err != nil {
					log.Errorf("[Rename Tag] Update podcast failed: %v", err)
					http.Error(w, "Database update error", http.StatusInternalServerError)
					return
				}
			}
		}

		// 3. Update users permissions (itemTagsSelected)
		rows3, err := tx.Query("SELECT id, permissions FROM users WHERE permissions IS NOT NULL")
		if err != nil {
			log.Errorf("[Rename Tag] Query users failed: %v", err)
			http.Error(w, "Database query error", http.StatusInternalServerError)
			return
		}
		defer rows3.Close()
		var userUpdateIds []string
		var userUpdateArgs []interface{}
		var userUpdateCases []string
		for rows3.Next() {
			var id string
			var permsStr sql.NullString
			if err := rows3.Scan(&id, &permsStr); err != nil {
				log.Errorf("[Rename Tag] Scan user failed: %v", err)
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
							userUpdateIds = append(userUpdateIds, id)
							userUpdateArgs = append(userUpdateArgs, id, string(newPermsBytes))
							userUpdateCases = append(userUpdateCases, "WHEN ? THEN ?")
						}
					}
				}
			}
		}
		if err := rows3.Err(); err != nil {
			log.Errorf("[Rename Tag] Users iteration failed: %v", err)
			http.Error(w, "Database iteration error", http.StatusInternalServerError)
			return
		}
		rows3.Close() // Explicitly close before exec
		if len(userUpdateIds) > 0 {
			chunkSize := 1000
			for i := 0; i < len(userUpdateIds); i += chunkSize {
				end := i + chunkSize
				if end > len(userUpdateIds) {
					end = len(userUpdateIds)
				}
				chunkIds := userUpdateIds[i:end]
				chunkArgs := userUpdateArgs[i*2 : end*2]
				chunkCases := userUpdateCases[i:end]

				query := "UPDATE users SET permissions = CASE id " + strings.Join(chunkCases, " ") + " END WHERE id IN (?" + strings.Repeat(",?", len(chunkIds)-1) + ")"
				args := append(chunkArgs, make([]interface{}, len(chunkIds))...)
				for j, id := range chunkIds {
					args[len(chunkArgs)+j] = id
				}
				_, err = tx.Exec(query, args...)
				if err != nil {
					log.Errorf("[Rename Tag] Update user failed: %v", err)
					http.Error(w, "Database update error", http.StatusInternalServerError)
					return
				}
			}
		}

		if err := tx.Commit(); err != nil {
			log.Errorf("[Rename Tag] Commit error: %v", err)
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
		log.Infof("[Go] DELETE %s", r.URL.Path)
		userSess, ok := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if !ok || !userSess.IsAdminOrUp() {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		tagParam := utils.TrimAPIPath(r.URL.Path, "/api/tags/")
		if tagParam == "" || strings.Contains(tagParam, "/") {
			http.Error(w, `{"error": "Bad Request"}`, http.StatusBadRequest)
			return
		}

		tagBytes, err := base64.StdEncoding.DecodeString(tagParam)
		if err != nil {
			// Try URL-safe base64
			tagBytes, err = base64.URLEncoding.DecodeString(tagParam)
			if err != nil {
				log.Errorf("[Delete Tag] Failed to decode base64: %v", err)
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
		_, err = tx.Exec(`
			UPDATE books
			SET tags = IFNULL(
				(
					SELECT json_group_array(value)
					FROM json_each(tags)
					WHERE value != ?
				),
				json_array()
			)
			WHERE tags IS NOT NULL
			AND EXISTS (
				SELECT 1 FROM json_each(tags)
				WHERE value = ?
			)
		`, targetTag, targetTag)
		if err != nil {
			log.Errorf("[Delete Tag] Update books failed: %v", err)
			http.Error(w, "Database update error", http.StatusInternalServerError)
			return
		}

		// 2. Update podcasts
		_, err = tx.Exec(`
			UPDATE podcasts
			SET tags = IFNULL(
				(
					SELECT json_group_array(value)
					FROM json_each(tags)
					WHERE value != ?
				),
				json_array()
			)
			WHERE tags IS NOT NULL
			AND EXISTS (
				SELECT 1 FROM json_each(tags)
				WHERE value = ?
			)
		`, targetTag, targetTag)
		if err != nil {
			log.Errorf("[Delete Tag] Update podcasts failed: %v", err)
			http.Error(w, "Database update error", http.StatusInternalServerError)
			return
		}

		// 3. Update users permissions
		_, err = tx.Exec(`
			UPDATE users
			SET permissions = json_set(
				permissions,
				'$.itemTagsSelected',
				IFNULL(
					(
						SELECT json_group_array(value)
						FROM json_each(permissions, '$.itemTagsSelected')
						WHERE value != ?
					),
					json_array()
				)
			)
			WHERE permissions IS NOT NULL
			AND json_extract(permissions, '$.itemTagsSelected') IS NOT NULL
			AND EXISTS (
				SELECT 1 FROM json_each(permissions, '$.itemTagsSelected')
				WHERE value = ?
			)
		`, targetTag, targetTag)
		if err != nil {
			log.Errorf("[Delete Tag] Update users failed: %v", err)
			http.Error(w, "Database update error", http.StatusInternalServerError)
			return
		}

		if err := tx.Commit(); err != nil {
			log.Errorf("[Delete Tag] Commit error: %v", err)
			http.Error(w, "Database commit error", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
	}
}

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

// handleRenameGenre renames a genre across books and podcasts
func handleRenameGenre(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Go] POST /api/genres/rename")
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
		_, err = tx.Exec(`
			UPDATE books
			SET genres = IFNULL(
				(
					SELECT json_group_array(
						CASE
							WHEN value = ? THEN ?
							ELSE value
						END
					)
					FROM json_each(genres)
				),
				json_array()
			)
			WHERE genres IS NOT NULL
			AND EXISTS (
				SELECT 1 FROM json_each(genres)
				WHERE value = ?
			)
		`, body.Genre, body.NewGenre, body.Genre)
		if err != nil {
			log.Errorf("[Rename Genre] Update books failed: %v", err)
			http.Error(w, "Database update error", http.StatusInternalServerError)
			return
		}

		// 2. Update podcasts
		_, err = tx.Exec(`
			UPDATE podcasts
			SET genres = IFNULL(
				(
					SELECT json_group_array(
						CASE
							WHEN value = ? THEN ?
							ELSE value
						END
					)
					FROM json_each(genres)
				),
				json_array()
			)
			WHERE genres IS NOT NULL
			AND EXISTS (
				SELECT 1 FROM json_each(genres)
				WHERE value = ?
			)
		`, body.Genre, body.NewGenre, body.Genre)
		if err != nil {
			log.Errorf("[Rename Genre] Update podcasts failed: %v", err)
			http.Error(w, "Database update error", http.StatusInternalServerError)
			return
		}

		if err := tx.Commit(); err != nil {
			log.Errorf("[Rename Genre] Commit error: %v", err)
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
		log.Infof("[Go] DELETE %s", r.URL.Path)
		userSess, ok := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if !ok || !userSess.IsAdminOrUp() {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		genreParam := utils.TrimAPIPath(r.URL.Path, "/api/genres/")
		if genreParam == "" || strings.Contains(genreParam, "/") {
			http.Error(w, `{"error": "Bad Request"}`, http.StatusBadRequest)
			return
		}

		genreBytes, err := base64.StdEncoding.DecodeString(genreParam)
		if err != nil {
			// Try URL-safe base64
			genreBytes, err = base64.URLEncoding.DecodeString(genreParam)
			if err != nil {
				log.Errorf("[Delete Genre] Failed to decode base64: %v", err)
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
		_, err = tx.Exec(`
			UPDATE books
			SET genres = IFNULL(
				(
					SELECT json_group_array(value)
					FROM json_each(genres)
					WHERE value != ?
				),
				json_array()
			)
			WHERE genres IS NOT NULL
			AND EXISTS (
				SELECT 1 FROM json_each(genres)
				WHERE value = ?
			)
		`, targetGenre, targetGenre)
		if err != nil {
			log.Errorf("[Delete Genre] Update books failed: %v", err)
			http.Error(w, "Database update error", http.StatusInternalServerError)
			return
		}

		// 2. Update podcasts
		_, err = tx.Exec(`
			UPDATE podcasts
			SET genres = IFNULL(
				(
					SELECT json_group_array(value)
					FROM json_each(genres)
					WHERE value != ?
				),
				json_array()
			)
			WHERE genres IS NOT NULL
			AND EXISTS (
				SELECT 1 FROM json_each(genres)
				WHERE value = ?
			)
		`, targetGenre, targetGenre)
		if err != nil {
			log.Errorf("[Delete Genre] Update podcasts failed: %v", err)
			http.Error(w, "Database update error", http.StatusInternalServerError)
			return
		}

		if err := tx.Commit(); err != nil {
			log.Errorf("[Delete Genre] Commit error: %v", err)
			http.Error(w, "Database commit error", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
	}
}
