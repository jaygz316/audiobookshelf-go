package handlers

import (
	log "audiobookshelf/internal/logger"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"time"

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

type CreateApiKeyRequest struct {
	Name      string `json:"name"`
	UserID    string `json:"userId"`
	ExpiresAt string `json:"expiresAt"`
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

// handlePostApiKey creates a new API key.
func handlePostApiKey(db *sql.DB) http.HandlerFunc {
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

		var req CreateApiKeyRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error": "Invalid request payload"}`, http.StatusBadRequest)
			return
		}

		if strings.TrimSpace(req.Name) == "" {
			http.Error(w, `{"error": "name is required"}`, http.StatusBadRequest)
			return
		}

		if req.UserID == "" {
			http.Error(w, `{"error": "userId is required"}`, http.StatusBadRequest)
			return
		}

		// Check if the user exists and retrieve their username
		var username string
		err := db.QueryRow("SELECT username FROM users WHERE id = ?", req.UserID).Scan(&username)
		if err == sql.ErrNoRows {
			http.Error(w, `{"error": "User does not exist"}`, http.StatusBadRequest)
			return
		} else if err != nil {
			log.Errorf("[API Keys] Failed to query user: %v", err)
			http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
			return
		}

		// Generate a secure random hex API key token (48 hex characters using crypto/rand)
		tokenBytes := make([]byte, 24)
		if _, err := rand.Read(tokenBytes); err != nil {
			log.Errorf("[API Keys] Failed to generate secure token: %v", err)
			http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
			return
		}
		token := hex.EncodeToString(tokenBytes)

		createdAtStr := time.Now().UTC().Format(time.RFC3339)
		isActiveVal := 1

		// Insert into apiKeys table
		_, err = db.Exec(`
			INSERT INTO apiKeys (id, name, userId, expiresAt, createdAt, isActive)
			VALUES (?, ?, ?, ?, ?, ?)
		`, token, req.Name, req.UserID, req.ExpiresAt, createdAtStr, isActiveVal)
		if err != nil {
			log.Errorf("[API Keys] Failed to insert API key: %v", err)
			http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"apiKey": map[string]interface{}{
				"id":        token,
				"name":      req.Name,
				"isActive":  true,
				"expiresAt": req.ExpiresAt,
				"userId":    req.UserID,
				"username":  username,
				"createdAt": createdAtStr,
				"token":     token,
			},
		})
	}
}

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
