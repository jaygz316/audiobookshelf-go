package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"encoding/json"
	"net/http"

	"audiobookshelf/internal/core"
)

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
		if err := renameTagInBooks(tx, tagVal, newTagVal); err != nil {
			http.Error(w, "Failed to update tags in books", http.StatusInternalServerError)
			return
		}

		// 2. Update podcasts
		if err := renameTagInPodcasts(tx, tagVal, newTagVal); err != nil {
			http.Error(w, "Failed to update tags in podcasts", http.StatusInternalServerError)
			return
		}

		// 3. Update users permissions (itemTagsSelected)
		if err := renameTagInUsers(tx, tagVal, newTagVal); err != nil {
			http.Error(w, "Failed to update tags in user permissions", http.StatusInternalServerError)
			return
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
