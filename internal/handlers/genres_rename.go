package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"encoding/json"
	"net/http"

	"audiobookshelf/internal/core"
)

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
