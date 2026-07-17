package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"audiobookshelf/internal/core"
	idb "audiobookshelf/internal/db"
)

// handleRefresh handles rotation and validation of access and refresh tokens.
func handleRefresh(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Info("[Go] POST /auth/refresh")
		db := getDB(db)
		if db == nil {
			http.Error(w, `{"error": "Database not connected"}`, http.StatusInternalServerError)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
			return
		}

		cookie, err := r.Cookie("refresh_token")
		if err != nil || cookie.Value == "" {
			log.Warnf("[Refresh] No refresh token cookie")
			http.Error(w, `{"error": "No refresh token"}`, http.StatusBadRequest)
			return
		}

		refreshToken := cookie.Value
		secret := getTokenSecret(db)

		// Verify refresh token
		claims := &core.AuthClaims{}
		token, err := jwt.ParseWithClaims(refreshToken, claims, func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method")
			}
			return []byte(secret), nil
		})

		if err != nil || !token.Valid || claims.Type != "refresh" {
			log.Warnf("[Refresh] Invalid refresh token: %v", err)
			http.Error(w, `{"error": "Invalid refresh token"}`, http.StatusBadRequest)
			return
		}

		// Find session in DB
		var session idb.UserSessionDB
		var expiresAtStr string
		var lastExpiresAtStr sql.NullString
		var lastRefreshToken sql.NullString
		err = db.QueryRowContext(r.Context(), "SELECT id, userId, refreshToken, expiresAt, lastRefreshToken, lastRefreshTokenExpiresAt FROM sessions WHERE refreshToken = ? OR lastRefreshToken = ?", refreshToken, refreshToken).
			Scan(&session.ID, &session.UserID, &session.RefreshToken, &expiresAtStr, &lastRefreshToken, &lastExpiresAtStr)

		if err == sql.ErrNoRows {
			log.Warnf("[Refresh] Session not found in DB")
			http.Error(w, `{"error": "Invalid refresh token"}`, http.StatusBadRequest)
			return
		} else if err != nil {
			log.Errorf("[Refresh] DB error: %v", err)
			http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
			return
		}

		session.ExpiresAt = idb.ParseTimeStr(expiresAtStr)
		if lastRefreshToken.Valid {
			session.LastRefreshToken = lastRefreshToken.String
		}
		if lastExpiresAtStr.Valid {
			session.LastRefreshTokenExpiresAt = idb.ParseTimeStr(lastExpiresAtStr.String)
		}

		user, err := idb.GetUserFullByID(r.Context(), db, session.UserID)
		if err != nil || user == nil || !user.IsActive {
			log.Warnf("[Refresh] idb.User inactive or not found")
			http.Error(w, `{"error": "idb.User inactive"}`, http.StatusUnauthorized)
			return
		}

		isGracePeriod := false
		if session.RefreshToken != refreshToken {
			// Matched lastRefreshToken
			if session.LastRefreshTokenExpiresAt > time.Now().UnixNano()/int64(time.Millisecond) {
				isGracePeriod = true
				log.Infof("[Refresh] Grace period hit for user %s", user.Username)
			} else {
				log.Warnf("[Refresh] Grace period expired")
				http.Error(w, `{"error": "Invalid refresh token"}`, http.StatusBadRequest)
				return
			}
		} else {
			// Matched current refreshToken, check DB expiration
			if session.ExpiresAt < time.Now().UnixNano()/int64(time.Millisecond) {
				log.Warnf("[Refresh] Session expired in DB")
				db.ExecContext(r.Context(), "DELETE FROM sessions WHERE id = ?", session.ID)
				http.Error(w, `{"error": "Refresh token expired"}`, http.StatusUnauthorized)
				return
			}
		}

		newAccessTokenClaims := &core.AuthClaims{
			UserID:   user.ID,
			Username: user.Username,
			Type:     "access",
			RegisteredClaims: jwt.RegisteredClaims{
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(2 * time.Hour)),
				Issuer:    "audiobookshelf",
			},
		}
		newAccessToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, newAccessTokenClaims).SignedString([]byte(secret))
		if err != nil {
			log.Errorf("[Refresh] Failed to sign new access token: %v", err)
			http.Error(w, `{"error": "Refresh failed"}`, http.StatusInternalServerError)
			return
		}

		newRefreshToken := refreshToken
		if !isGracePeriod {
			newRefreshClaims := &core.AuthClaims{
				UserID:   user.ID,
				Username: user.Username,
				Type:     "refresh",
				RegisteredClaims: jwt.RegisteredClaims{
					ExpiresAt: jwt.NewNumericDate(time.Now().Add(30 * 24 * time.Hour)),
					Issuer:    "audiobookshelf",
				},
			}
			newRefreshToken, err = jwt.NewWithClaims(jwt.SigningMethodHS256, newRefreshClaims).SignedString([]byte(secret))
			if err != nil {
				log.Errorf("[Refresh] Failed to sign new refresh token: %v", err)
				http.Error(w, `{"error": "Refresh failed"}`, http.StatusInternalServerError)
				return
			}

			// Rotate tokens in DB
			nowStr := idb.TimeToDBStr(time.Now())
			expiresStr := idb.TimeToDBStr(time.Now().Add(30 * 24 * time.Hour))
			graceExpiresStr := idb.TimeToDBStr(time.Now().Add(60 * time.Second))

			_, err = db.ExecContext(r.Context(), "UPDATE sessions SET refreshToken = ?, expiresAt = ?, lastRefreshToken = ?, lastRefreshTokenExpiresAt = ?, updatedAt = ? WHERE id = ? AND refreshToken = ?",
				newRefreshToken, expiresStr, refreshToken, graceExpiresStr, nowStr, session.ID, refreshToken)
			if err != nil {
				log.Errorf("[Refresh] Failed to update session in DB: %v", err)
				http.Error(w, `{"error": "Refresh failed"}`, http.StatusInternalServerError)
				return
			}

			// Update cookie
			http.SetCookie(w, &http.Cookie{
				Name:     "refresh_token",
				Value:    newRefreshToken,
				Path:     "/",
				MaxAge:   30 * 24 * 60 * 60,
				HttpOnly: true,
			})
		}

		payload, err := idb.GetUserLoginPayload(r.Context(), db, user)
		if err != nil {
			log.Errorf("[Refresh] Failed to get response payload: %v", err)
			http.Error(w, `{"error": "Refresh failed"}`, http.StatusInternalServerError)
			return
		}

		userJSON := user.ToOldJSONForBrowser(false)
		userJSON["accessToken"] = newAccessToken
		payload["user"] = userJSON

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(payload)
	}
}
