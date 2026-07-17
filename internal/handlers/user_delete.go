package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"encoding/json"
	"net/http"

	"audiobookshelf/internal/core"
	idb "audiobookshelf/internal/db"
	isocket "audiobookshelf/internal/socket"
)

func handleUserDelete(db *sql.DB, w http.ResponseWriter, r *http.Request, userSess *core.UserSession, targetUserID string) {
	log.Infof("[Go] DELETE /api/users/%s", targetUserID)
	if userSess.Type != "root" && userSess.Type != "admin" {
		http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
		return
	}
	if targetUserID == "root" {
		http.Error(w, `{"error": "Cannot delete root user"}`, http.StatusBadRequest)
		return
	}
	if targetUserID == userSess.ID {
		http.Error(w, `{"error": "Cannot delete self"}`, http.StatusBadRequest)
		return
	}

	targetUser, err := idb.GetUserFullByID(r.Context(), db, targetUserID)
	if err != nil || targetUser == nil {
		http.Error(w, `{"error": "User not found"}`, http.StatusNotFound)
		return
	}

	if targetUser.Type == "root" {
		http.Error(w, `{"error": "Cannot delete root user"}`, http.StatusForbidden)
		return
	}

	userJSON := targetUser.ToOldJSONForBrowser(userSess.Type != "root")

	tx, err := db.BeginTx(r.Context(), nil)
	if err != nil {
		http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	_, _ = tx.ExecContext(r.Context(), "DELETE FROM playlists WHERE userId = ?", targetUserID)
	_, _ = tx.ExecContext(r.Context(), "DELETE FROM sessions WHERE userId = ?", targetUserID)
	_, _ = tx.ExecContext(r.Context(), "DELETE FROM mediaProgress WHERE userId = ?", targetUserID)
	_, _ = tx.ExecContext(r.Context(), "DELETE FROM users WHERE id = ?", targetUserID)

	if err := tx.Commit(); err != nil {
		http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
		return
	}

	if isocket.GlobalAuth != nil {
		isocket.GlobalAuth.BroadcastToAdmins("user_removed", userJSON)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
	})
}
