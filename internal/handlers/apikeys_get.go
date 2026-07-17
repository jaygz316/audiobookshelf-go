package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"encoding/json"
	"net/http"

	"audiobookshelf/internal/core"
)

type ApiKeyResponse struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	UserID    string `json:"userId"`
	Username  string `json:"username"`
	ExpiresAt string `json:"expiresAt"`
	CreatedAt string `json:"createdAt"`
	IsActive  bool   `json:"isActive"`
}

// handleGetApiKeys returns a list of API keys left joined with users.
func handleGetApiKeys(db *sql.DB) http.HandlerFunc {
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

		rows, err := db.Query(`
			SELECT a.id, a.name, a.userId, u.username, a.expiresAt, a.createdAt, a.isActive
			FROM apiKeys a
			LEFT JOIN users u ON a.userId = u.id
		`)
		if err != nil {
			log.Errorf("[API Keys] Failed to query API keys: %v", err)
			http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		keys := make([]ApiKeyResponse, 0)
		for rows.Next() {
			var id string
			var name sql.NullString
			var userId string
			var username sql.NullString
			var expiresAt sql.NullString
			var createdAt sql.NullString
			var isActiveInt int

			if err := rows.Scan(&id, &name, &userId, &username, &expiresAt, &createdAt, &isActiveInt); err != nil {
				log.Errorf("[API Keys] Failed to scan API key row: %v", err)
				http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
				return
			}

			keys = append(keys, ApiKeyResponse{
				ID:        id,
				Name:      name.String,
				UserID:    userId,
				Username:  username.String,
				ExpiresAt: expiresAt.String,
				CreatedAt: createdAt.String,
				IsActive:  isActiveInt != 0,
			})
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"apiKeys": keys,
		})
	}
}
