package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"audiobookshelf/internal/core"
	idb "audiobookshelf/internal/db"
)

// handleGetMe returns the logged-in user details
func handleGetMe(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Go] GET /api/me")
		userSess, ok := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if !ok {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}

		user, err := idb.GetUserFullByID(r.Context(), db, userSess.ID)
		if err != nil || user == nil {
			log.Errorf("[Me] idb.User lookup failed: %v", err)
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(user.ToOldJSONForBrowser(user.Type != "root"))
	}
}

// handleUpdateMePassword allows the user to update their password
func handleUpdateMePassword(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Go] PATCH /api/me/password")
		userSess, ok := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if !ok {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}

		if userSess.Type == "guest" {
			http.Error(w, `{"error": "Guest users cannot change password"}`, http.StatusForbidden)
			return
		}

		var body struct {
			Password    string `json:"password"`
			NewPassword string `json:"newPassword"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
			return
		}

		user, err := idb.GetUserFullByID(r.Context(), db, userSess.ID)
		if err != nil || user == nil {
			http.Error(w, `{"error": "idb.User not found"}`, http.StatusNotFound)
			return
		}

		err = bcrypt.CompareHashAndPassword([]byte(user.Pash), []byte(body.Password))
		if err != nil {
			log.Warnf("[Me] Invalid current password for user %s", user.Username)
			http.Error(w, `{"error": "Invalid current password"}`, http.StatusBadRequest)
			return
		}

		hashed, err := bcrypt.GenerateFromPassword([]byte(body.NewPassword), 8)
		if err != nil {
			http.Error(w, `{"error": "Failed to hash password"}`, http.StatusInternalServerError)
			return
		}

		secret := getTokenSecret(db)
		apiToken := jwt.NewWithClaims(jwt.SigningMethodHS256, &core.AuthClaims{
			UserID:   user.ID,
			Username: user.Username,
			Type:     user.Type,
			RegisteredClaims: jwt.RegisteredClaims{
				Issuer: "audiobookshelf",
			},
		})
		tokenStr, err := apiToken.SignedString([]byte(secret))
		if err != nil {
			log.Errorf("[Me] Failed to sign new token: %v", err)
			http.Error(w, `{"error": "Failed to update password"}`, http.StatusInternalServerError)
			return
		}

		_, err = db.ExecContext(r.Context(), "UPDATE users SET pash = ?, token = ?, updatedAt = ? WHERE id = ?", string(hashed), tokenStr, idb.TimeToDBStr(time.Now()), user.ID)
		if err != nil {
			log.Errorf("[Me] Password update DB error: %v", err)
			http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
			return
		}

		_, err = db.ExecContext(r.Context(), "DELETE FROM sessions WHERE userId = ?", user.ID)
		if err != nil {
			log.Errorf("[Me] Failed to clear user sessions: %v", err)
		}

		w.WriteHeader(http.StatusOK)
	}
}

// trimPathPrefix extracts the part of path after prefix, ignoring any router base path.
func trimPathPrefix(path, prefix string) string {
	if idx := strings.Index(path, prefix); idx != -1 {
		return path[idx+len(prefix):]
	}
	return strings.TrimPrefix(path, prefix)
}

// handleGetSession returns the session user object
func handleGetSession(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Go] GET %s", r.URL.Path)
		userSess, ok := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if !ok {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}

		user, err := idb.GetUserFullByID(r.Context(), db, userSess.ID)
		if err != nil || user == nil {
			log.Errorf("[Session] idb.User lookup failed: %v", err)
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(user.ToOldJSONForBrowser(user.Type != "root"))
	}
}
