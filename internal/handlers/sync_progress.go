package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"audiobookshelf/internal/core"
	idb "audiobookshelf/internal/db"
	log "audiobookshelf/internal/logger"
	isocket "audiobookshelf/internal/socket"
)

// handleSyncLocalProgress handles POST /api/me/sync-local-progress
func handleSyncLocalProgress(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Go] POST %s", r.URL.Path)
		userSess, ok := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if !ok {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}

		var payload LocalMediaProgressPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			log.Warnf("[Sync Progress] Decode failed: %v", err)
			http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
			return
		}

		ctx := r.Context()
		now := time.Now()
		progressUpdatedAny := false

		for _, item := range payload.LocalMediaProgress {
			if item.LibraryItemID == "" {
				continue
			}

			updated, err := syncSingleProgress(ctx, db, userSess, item, now)
			if err != nil {
				log.Errorf("[Sync Progress] Error syncing single progress: %v", err)
				continue
			}
			if updated {
				progressUpdatedAny = true
			}
		}

		if progressUpdatedAny {
			user, err := idb.GetUserFullByID(ctx, db, userSess.ID)
			if err == nil && user != nil {
				userJSON := user.ToOldJSONForBrowser(user.Type != "root")
				if isocket.GlobalAuth != nil {
					isocket.GlobalAuth.BroadcastToUser(userSess.ID, "user_updated", userJSON)
				}
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"success":true}`))
	}
}
