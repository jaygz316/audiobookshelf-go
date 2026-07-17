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

// handleRemoveSeriesFromContinueListening handles GET /api/me/series/:id/remove-from-continue-listening
func handleRemoveSeriesFromContinueListening(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Go] GET %s", r.URL.Path)
		userSess, ok := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if !ok {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}

		sub := utils.TrimAPIPath(r.URL.Path, "/api/me/series/")
		seriesID := strings.TrimSuffix(sub, "/remove")
		if seriesID == "" || strings.Contains(seriesID, "/") {
			http.Error(w, `{"error": "Bad Request"}`, http.StatusBadRequest)
			return
		}

		// Check series exists
		var count int
		db.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM series WHERE id = ?", seriesID).Scan(&count)
		if count == 0 {
			http.Error(w, `{"error": "Series not found"}`, http.StatusNotFound)
			return
		}

		user, err := idb.GetUserFullByID(r.Context(), db, userSess.ID)
		if err != nil || user == nil {
			http.Error(w, "idb.User not found", http.StatusNotFound)
			return
		}

		var extra map[string]interface{}
		if len(user.ExtraData) > 0 {
			json.Unmarshal(user.ExtraData, &extra)
		}
		if extra == nil {
			extra = make(map[string]interface{})
		}

		seriesArr, _ := extra["seriesHideFromContinueListening"].([]interface{})
		exists := false
		for _, s := range seriesArr {
			if sStr, ok := s.(string); ok && sStr == seriesID {
				exists = true
				break
			}
		}
		if !exists {
			seriesArr = append(seriesArr, seriesID)
			extra["seriesHideFromContinueListening"] = seriesArr
			extraBytes, _ := json.Marshal(extra)
			_, err = db.ExecContext(r.Context(), "UPDATE users SET extraData = ?, updatedAt = ? WHERE id = ?", string(extraBytes), idb.TimeToDBStr(time.Now()), user.ID)
			if err != nil {
				log.Errorf("[Me Series] DB error: %v", err)
				http.Error(w, "Database error", http.StatusInternalServerError)
				return
			}
			user, _ = idb.GetUserFullByID(r.Context(), db, userSess.ID)
		}

		userJSON := user.ToOldJSONForBrowser(user.Type != "root")
		if isocket.GlobalAuth != nil {
			isocket.GlobalAuth.BroadcastToUser(userSess.ID, "user_updated", userJSON)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(userJSON)
	}
}

// handleReaddSeriesFromContinueListening handles GET /api/me/series/:id/readd-to-continue-listening
func handleReaddSeriesFromContinueListening(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Go] GET %s", r.URL.Path)
		userSess, ok := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if !ok {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}

		sub := utils.TrimAPIPath(r.URL.Path, "/api/me/series/")
		seriesID := strings.TrimSuffix(sub, "/readd")
		if seriesID == "" || strings.Contains(seriesID, "/") {
			http.Error(w, `{"error": "Bad Request"}`, http.StatusBadRequest)
			return
		}

		user, err := idb.GetUserFullByID(r.Context(), db, userSess.ID)
		if err != nil || user == nil {
			http.Error(w, "idb.User not found", http.StatusNotFound)
			return
		}

		var extra map[string]interface{}
		if len(user.ExtraData) > 0 {
			json.Unmarshal(user.ExtraData, &extra)
		}
		if extra == nil {
			extra = make(map[string]interface{})
		}

		seriesArr, _ := extra["seriesHideFromContinueListening"].([]interface{})
		newSeriesArr := []interface{}{}
		changed := false
		for _, s := range seriesArr {
			if sStr, ok := s.(string); ok && sStr == seriesID {
				changed = true
			} else {
				newSeriesArr = append(newSeriesArr, s)
			}
		}

		if changed {
			extra["seriesHideFromContinueListening"] = newSeriesArr
			extraBytes, _ := json.Marshal(extra)
			_, err = db.ExecContext(r.Context(), "UPDATE users SET extraData = ?, updatedAt = ? WHERE id = ?", string(extraBytes), idb.TimeToDBStr(time.Now()), user.ID)
			if err != nil {
				log.Errorf("[Me Series] DB error: %v", err)
				http.Error(w, "Database error", http.StatusInternalServerError)
				return
			}
			user, _ = idb.GetUserFullByID(r.Context(), db, userSess.ID)
		}

		userJSON := user.ToOldJSONForBrowser(user.Type != "root")
		if isocket.GlobalAuth != nil {
			isocket.GlobalAuth.BroadcastToUser(userSess.ID, "user_updated", userJSON)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(userJSON)
	}
}
