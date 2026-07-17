package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"audiobookshelf/internal/core"
	idb "audiobookshelf/internal/db"
	log "audiobookshelf/internal/logger"
	isocket "audiobookshelf/internal/socket"
	"audiobookshelf/internal/utils"
)

// handleRemoveMeProgress handles DELETE /api/me/progress/:id
func handleRemoveMeProgress(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Go] DELETE %s", r.URL.Path)
		userSess, ok := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if !ok {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}
		subPath := utils.TrimAPIPath(r.URL.Path, "/api/me/progress/")
		parts := strings.Split(subPath, "/")
		if len(parts) == 0 || parts[0] == "" {
			http.Error(w, `{"error": "Bad Request"}`, http.StatusBadRequest)
			return
		}

		idParam := parts[0]
		var episodeID string
		if len(parts) > 1 {
			episodeID = parts[1]
		}

		var progressID string
		if episodeID != "" {
			_ = db.QueryRowContext(r.Context(), "SELECT id FROM mediaProgresses WHERE userId = ? AND mediaItemId = ?", userSess.ID, episodeID).Scan(&progressID)
		} else {
			var count int
			_ = db.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM mediaProgresses WHERE id = ? AND userId = ?", idParam, userSess.ID).Scan(&count)
			if count > 0 {
				progressID = idParam
			} else {
				_ = db.QueryRowContext(r.Context(), `SELECT id FROM mediaProgresses WHERE userId = ? AND 
					(mediaItemId = ? OR json_extract(extraData, '$.libraryItemId') = ?) LIMIT 1`, userSess.ID, idParam, idParam).Scan(&progressID)
			}
		}

		if progressID == "" {
			http.Error(w, `{"error": "Progress not found"}`, http.StatusNotFound)
			return
		}

		_, err := db.ExecContext(r.Context(), "DELETE FROM mediaProgresses WHERE id = ?", progressID)
		if err != nil {
			log.Errorf("[Me Progress] Delete error: %v", err)
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}

		// Broadcast update
		user, err := idb.GetUserFullByID(r.Context(), db, userSess.ID)
		if err == nil && user != nil {
			userJSON := user.ToOldJSONForBrowser(user.Type != "root")
			if isocket.GlobalAuth != nil {
				isocket.GlobalAuth.BroadcastToUser(userSess.ID, "user_updated", userJSON)
			}
		}

		w.WriteHeader(http.StatusOK)
	}
}

// handleHideMeProgressFromContinueListening handles GET /api/me/progress/:id/remove-from-continue-listening
func handleHideMeProgressFromContinueListening(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Go] GET %s", r.URL.Path)
		userSess, ok := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if !ok {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}

		sub := utils.TrimAPIPath(r.URL.Path, "/api/me/progress/")
		progressID := strings.TrimSuffix(sub, "/remove-from-continue-listening")
		progressID = strings.TrimSuffix(progressID, "/hide-from-continue-listening")
		if progressID == "" || strings.Contains(progressID, "/") {
			http.Error(w, `{"error": "Bad Request"}`, http.StatusBadRequest)
			return
		}

		_, err := db.ExecContext(r.Context(), "UPDATE mediaProgresses SET hideFromContinueListening = 1, updatedAt = ? WHERE id = ? AND userId = ?",
			idb.TimeToDBStr(time.Now()), progressID, userSess.ID)
		if err != nil {
			log.Errorf("[Me Progress] Hide progress error: %v", err)
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}

		user, err := idb.GetUserFullByID(r.Context(), db, userSess.ID)
		if err == nil && user != nil {
			userJSON := user.ToOldJSONForBrowser(user.Type != "root")
			if isocket.GlobalAuth != nil {
				isocket.GlobalAuth.BroadcastToUser(userSess.ID, "user_updated", userJSON)
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(userJSON)
			return
		}

		http.Error(w, "idb.User not found", http.StatusNotFound)
	}
}
