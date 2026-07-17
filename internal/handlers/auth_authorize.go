package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"encoding/json"
	"net/http"

	"audiobookshelf/internal/core"
	idb "audiobookshelf/internal/db"
)

// handleAuthorize handles token authorization requests and returns the authorized user payload.
func handleAuthorize(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Info("[Go] /api/authorize")
		db := getDB(db)
		if db == nil {
			http.Error(w, `{"error": "Database not connected"}`, http.StatusInternalServerError)
			return
		}
		if r.Method != http.MethodPost && r.Method != http.MethodGet {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
			return
		}

		userVal := r.Context().Value(core.UserContextKey)
		if userVal == nil {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}
		userSess := userVal.(*core.UserSession)

		user, err := idb.GetUserFullByID(r.Context(), db, userSess.ID)
		if err != nil {
			log.Errorf("[Authorize] DB lookup failed for user ID %s: %v", userSess.ID, err)
			http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
			return
		}
		if user == nil {
			log.Warnf("[Authorize] idb.User not found: %s", userSess.ID)
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}

		payload, err := idb.GetUserLoginPayload(r.Context(), db, user)
		if err != nil {
			log.Errorf("[Authorize] Failed to build response payload: %v", err)
			http.Error(w, `{"error": "Failed to authorize"}`, http.StatusInternalServerError)
			return
		}

		userJSON := user.ToOldJSONForBrowser(false)
		payload["user"] = userJSON

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(payload)
	}
}
