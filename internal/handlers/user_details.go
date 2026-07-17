package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"audiobookshelf/internal/core"
	idb "audiobookshelf/internal/db"
	isocket "audiobookshelf/internal/socket"
)

func handleUserListeningStats(db *sql.DB, w http.ResponseWriter, r *http.Request, userSess *core.UserSession, targetUserID string) {
	log.Infof("[Go] GET /api/users/%s/listening-stats", targetUserID)
	if userSess.Type != "root" && userSess.Type != "admin" && userSess.ID != targetUserID {
		http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
		return
	}
	stats, err := getUserListeningStats(db, targetUserID)
	if err != nil {
		log.Errorf("[Listening Stats] Failed to query stats: %v", err)
		http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

func handleUserListeningSessionsRoute(db *sql.DB, w http.ResponseWriter, r *http.Request, userSess *core.UserSession, targetUserID string) {
	log.Infof("[Go] GET /api/users/%s/listening-sessions", targetUserID)
	if userSess.Type != "root" && userSess.Type != "admin" && userSess.ID != targetUserID {
		http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
		return
	}
	page := 0
	if pVal := r.URL.Query().Get("page"); pVal != "" {
		if p, err := strconv.Atoi(pVal); err == nil {
			page = p
		}
	}
	itemsPerPage := 10
	if limitVal := r.URL.Query().Get("itemsPerPage"); limitVal != "" {
		if limit, err := strconv.Atoi(limitVal); err == nil {
			itemsPerPage = limit
		}
	}
	sessions, err := handleGetUserListeningSessions(db, targetUserID, page, itemsPerPage)
	if err != nil {
		log.Errorf("[Listening Sessions] Failed to query sessions: %v", err)
		http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sessions)
}

func handleUserOpenIDUnlink(db *sql.DB, w http.ResponseWriter, r *http.Request, userSess *core.UserSession, targetUserID string) {
	log.Infof("[Go] PATCH /api/users/%s/openid-unlink", targetUserID)
	if userSess.Type != "root" && userSess.Type != "admin" && userSess.ID != targetUserID {
		http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
		return
	}

	targetUser, err := idb.GetUserFullByID(r.Context(), db, targetUserID)
	if err != nil || targetUser == nil {
		http.Error(w, `{"error": "User not found"}`, http.StatusNotFound)
		return
	}

	var extra map[string]interface{}
	if len(targetUser.ExtraData) > 0 {
		json.Unmarshal(targetUser.ExtraData, &extra)
	}
	if extra == nil {
		extra = make(map[string]interface{})
	}
	delete(extra, "authOpenIDSub")
	newExtraBytes, _ := json.Marshal(extra)

	_, err = db.ExecContext(r.Context(), "UPDATE users SET extraData = ?, updatedAt = ? WHERE id = ?", string(newExtraBytes), idb.TimeToDBStr(time.Now()), targetUserID)
	if err != nil {
		http.Error(w, `{"error": "Failed to update user"}`, http.StatusInternalServerError)
		return
	}

	targetUser.ExtraData = newExtraBytes
	userJSON := targetUser.ToOldJSONForBrowser(userSess.Type != "root")
	if isocket.GlobalAuth != nil {
		isocket.GlobalAuth.BroadcastToUser(targetUserID, "user_updated", userJSON)
	}

	w.WriteHeader(http.StatusOK)
}

func handleUserDetails(db *sql.DB, w http.ResponseWriter, r *http.Request, userSess *core.UserSession, targetUserID string) {
	log.Infof("[Go] GET /api/users/%s", targetUserID)
	if userSess.Type != "root" && userSess.Type != "admin" && userSess.ID != targetUserID {
		http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
		return
	}

	targetUser, err := idb.GetUserFullByID(r.Context(), db, targetUserID)
	if err != nil || targetUser == nil {
		http.Error(w, `{"error": "User not found"}`, http.StatusNotFound)
		return
	}

	userJSON := targetUser.ToOldJSONForBrowser(userSess.Type != "root")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(userJSON)
}
