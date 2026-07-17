package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"net/http"
	"strings"

	"audiobookshelf/internal/core"
)

// handleDeleteApiKey deletes an API key.
func handleDeleteApiKey(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userVal := r.Context().Value(core.UserContextKey)
		if userVal == nil {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}
		user := userVal.(*core.UserSession)
		if user.Type != "root" && user.Type != "admin" {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		id := trimPathPrefix(r.URL.Path, "/api/api-keys/")
		id = strings.TrimSuffix(id, "/")
		if id == "" {
			http.Error(w, `{"error": "ID is required"}`, http.StatusBadRequest)
			return
		}

		_, err := db.Exec("DELETE FROM apiKeys WHERE id = ?", id)
		if err != nil {
			log.Errorf("[API Keys] Failed to delete API key %s: %v", id, err)
			http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
	}
}
