package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"audiobookshelf/internal/core"
	idb "audiobookshelf/internal/db"
	log "audiobookshelf/internal/logger"
	isocket "audiobookshelf/internal/socket"
)

// handleSyncLocalSession handles POST /api/session/local
func handleSyncLocalSession(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Go] POST %s", r.URL.Path)
		userSess, ok := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if !ok {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}

		var item LocalSessionItem
		if err := json.NewDecoder(r.Body).Decode(&item); err != nil {
			log.Warnf("[Sync Session] Decode failed: %v", err)
			http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
			return
		}

		ctx := r.Context()
		res, progUpdated := syncSingleSession(ctx, db, userSess, item)

		if progUpdated {
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
		json.NewEncoder(w).Encode(res)
	}
}

// handleSyncLocalSessions handles POST /api/session/local-all
func handleSyncLocalSessions(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Go] POST %s", r.URL.Path)
		userSess, ok := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if !ok {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}

		var payload LocalSessionsPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			log.Warnf("[Sync Sessions] Decode failed: %v", err)
			http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
			return
		}

		ctx := r.Context()
		results := make([]SyncSessionResult, 0, len(payload.Sessions))
		progressUpdatedAny := false

		for _, item := range payload.Sessions {
			res, progUpdated := syncSingleSession(ctx, db, userSess, item)
			if progUpdated {
				progressUpdatedAny = true
			}
			results = append(results, res)
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
		json.NewEncoder(w).Encode(SyncSessionsResponse{
			Results: results,
		})
	}
}
