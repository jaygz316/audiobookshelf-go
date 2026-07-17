package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"encoding/base64"
	"net/http"
	"strings"

	"audiobookshelf/internal/core"
	"audiobookshelf/internal/utils"
)

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
