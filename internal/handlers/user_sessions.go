package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"encoding/json"
	"net/http"

	"audiobookshelf/internal/core"
	idb "audiobookshelf/internal/db"
)

type UserSessionResponse struct {
	ID        string `json:"id"`
	UserID    string `json:"userId"`
	IPAddress string `json:"ipAddress"`
	UserAgent string `json:"userAgent"`
	CreatedAt int64  `json:"createdAt"`
	UpdatedAt int64  `json:"updatedAt"`
	IsCurrent bool   `json:"isCurrent"`
}

func mapUserSessionToResponse(r *http.Request, s idb.UserSessionDB) UserSessionResponse {
	isCurrent := false
	cookie, err := r.Cookie("refresh_token")
	if err == nil && cookie.Value != "" {
		if s.RefreshToken == cookie.Value || s.LastRefreshToken == cookie.Value {
			isCurrent = true
		}
	}
	return UserSessionResponse{
		ID:        s.ID,
		UserID:    s.UserID,
		IPAddress: s.IPAddress,
		UserAgent: s.UserAgent,
		CreatedAt: s.CreatedAt,
		UpdatedAt: s.UpdatedAt,
		IsCurrent: isCurrent,
	}
}

// handleGetUserLoginSessions returns active login sessions for a user
func handleGetUserLoginSessions(db *sql.DB, targetUserID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userSess := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if userSess.Type != "root" && userSess.Type != "admin" && userSess.ID != targetUserID {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		sessions, err := idb.GetUserSessions(r.Context(), db, targetUserID)
		if err != nil {
			log.Errorf("[Login Sessions] Failed to get sessions: %v", err)
			http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
			return
		}

		resp := make([]UserSessionResponse, len(sessions))
		for i, s := range sessions {
			resp[i] = mapUserSessionToResponse(r, s)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}

// handleDeleteUserLoginSession deletes/revokes a specific login session
func handleDeleteUserLoginSession(db *sql.DB, targetUserID string, sessionID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userSess := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if userSess.Type != "root" && userSess.Type != "admin" && userSess.ID != targetUserID {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		// Verify session exists and belongs to this user
		var sessionUser string
		var sessionRefreshToken string
		err := db.QueryRowContext(r.Context(), "SELECT userId, refreshToken FROM sessions WHERE id = ?", sessionID).Scan(&sessionUser, &sessionRefreshToken)
		if err == sql.ErrNoRows {
			http.Error(w, `{"error": "Session not found"}`, http.StatusNotFound)
			return
		} else if err != nil {
			log.Errorf("[Delete Session] DB error: %v", err)
			http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
			return
		}

		if sessionUser != targetUserID {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		// Delete session
		err = idb.DeleteSessionByID(r.Context(), db, sessionID)
		if err != nil {
			log.Errorf("[Delete Session] Failed to delete session: %v", err)
			http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
			return
		}

		// If they deleted their own current session, clear the cookie
		cookie, err := r.Cookie("refresh_token")
		if err == nil && cookie.Value != "" && cookie.Value == sessionRefreshToken {
			http.SetCookie(w, &http.Cookie{
				Name:     "refresh_token",
				Value:    "",
				Path:     "/",
				MaxAge:   -1,
				HttpOnly: true,
			})
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
	}
}
