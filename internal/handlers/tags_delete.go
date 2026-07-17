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
