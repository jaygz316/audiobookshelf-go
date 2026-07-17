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

type CreateApiKeyRequest struct {
	Name      string `json:"name"`
	UserID    string `json:"userId"`
	ExpiresAt string `json:"expiresAt"`
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
