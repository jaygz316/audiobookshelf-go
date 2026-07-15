package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"audiobookshelf/internal/core"
	idb "audiobookshelf/internal/db"
	"audiobookshelf/internal/utils"
)

// Auth Handlers (Native Go)

func handleInit(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Info("[Go] POST /init")
		db = getDB(db)
		if db == nil {
			http.Error(w, `{"error": "Database not connected"}`, http.StatusInternalServerError)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
			return
		}

		hasRoot, err := idb.HasRootUser(db)
		if err != nil {
			log.Errorf("[Init] Error checking root user: %v", err)
			http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
			return
		}
		if hasRoot {
			log.Warnf("[Init] Attempt to init server when root user already exists")
			http.Error(w, `{"error": "Root user already exists"}`, http.StatusForbidden)
			return
		}

		var reqBody struct {
			NewRoot struct {
				Username string `json:"username"`
				Password string `json:"password"`
			} `json:"newRoot"`
		}
		r.Body = http.MaxBytesReader(w, r.Body, 1048576)
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			http.Error(w, `{"error": "Invalid request"}`, http.StatusBadRequest)
			return
		}

		username := reqBody.NewRoot.Username
		if username == "" {
			username = "root"
		}
		password := reqBody.NewRoot.Password

		hashed, err := bcrypt.GenerateFromPassword([]byte(password), 8)
		if err != nil {
			log.Errorf("[Init] Hashing failed: %v", err)
			http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
			return
		}

		userID := uuid.New().String()
		apiToken := jwt.NewWithClaims(jwt.SigningMethodHS256, &core.AuthClaims{
			UserID:   userID,
			Username: username,
			Type:     "root",
			RegisteredClaims: jwt.RegisteredClaims{
				Issuer: "audiobookshelf",
			},
		})
		tokenStr, err := apiToken.SignedString([]byte(getTokenSecret(db)))
		if err != nil {
			log.Errorf("[Init] Token signing failed: %v", err)
			http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
			return
		}

		nowStr := idb.TimeToDBStr(time.Now())
		defaultPerms := idb.GetDefaultPermissionsForUserType("root")

		_, err = db.ExecContext(r.Context(), `INSERT INTO users (id, username, type, pash, token, isActive, permissions, extraData, bookmarks, createdAt, updatedAt) 
			VALUES (?, ?, 'root', ?, ?, 1, ?, '{}', '[]', ?, ?)`,
			userID, username, string(hashed), tokenStr, defaultPerms, nowStr, nowStr)
		if err != nil {
			log.Errorf("[Init] Failed to create root user: %v", err)
			http.Error(w, `{"error": "Failed to create root user"}`, http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}
}

func handleLogin(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Info("[Go] POST /login")
		db = getDB(db)
		if db == nil {
			http.Error(w, `{"error": "Database not connected"}`, http.StatusInternalServerError)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
			return
		}

		if db == nil {
			log.Warnf("[Login] Database not available")
			w.Header().Set("Content-Type", "application/json")
			http.Error(w, `{"error": "Server is not initialized yet. Please wait for the database to be ready."}`, http.StatusServiceUnavailable)
			return
		}

		var credentials struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		r.Body = http.MaxBytesReader(w, r.Body, 1048576)
		if err := json.NewDecoder(r.Body).Decode(&credentials); err != nil {
			http.Error(w, `{"error": "Invalid JSON body"}`, http.StatusBadRequest)
			return
		}

		user, err := idb.GetUserFullByUsername(r.Context(), db, credentials.Username)
		if err != nil {
			log.Errorf("[Login] DB lookup failed: %v", err)
			http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
			return
		}

		if user == nil {
			log.Warnf("[Login] idb.User not found: %s", credentials.Username)
			http.Error(w, `{"error": "Invalid username or password"}`, http.StatusUnauthorized)
			return
		}

		if !user.IsActive {
			log.Warnf("[Login] idb.User %s is inactive", user.Username)
			http.Error(w, `{"error": "idb.User is inactive"}`, http.StatusUnauthorized)
			return
		}

		err = bcrypt.CompareHashAndPassword([]byte(user.Pash), []byte(credentials.Password))
		if err != nil {
			log.Warnf("[Login] Invalid password for user %s", user.Username)
			http.Error(w, `{"error": "Invalid username or password"}`, http.StatusUnauthorized)
			return
		}

		// Generate access token (expiring)
		secret := getTokenSecret(db)
		claims := &core.AuthClaims{
			UserID:   user.ID,
			Username: user.Username,
			Type:     "access",
			RegisteredClaims: jwt.RegisteredClaims{
				ID:        uuid.New().String(),
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(2 * time.Hour)),
				Issuer:    "audiobookshelf",
			},
		}
		accessToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
		if err != nil {
			log.Errorf("[Login] Failed to sign access token: %v", err)
			http.Error(w, `{"error": "Failed to login"}`, http.StatusInternalServerError)
			return
		}

		// Generate refresh token
		refreshClaims := &core.AuthClaims{
			UserID:   user.ID,
			Username: user.Username,
			Type:     "refresh",
			RegisteredClaims: jwt.RegisteredClaims{
				ID:        uuid.New().String(),
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(30 * 24 * time.Hour)),
				Issuer:    "audiobookshelf",
			},
		}
		refreshToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims).SignedString([]byte(secret))
		if err != nil {
			log.Errorf("[Login] Failed to sign refresh token: %v", err)
			http.Error(w, `{"error": "Failed to login"}`, http.StatusInternalServerError)
			return
		}

		// Save session
		ipAddress := utils.GetClientIP(r)
		userAgent := r.Header.Get("User-Agent")
		expiresAt := time.Now().Add(30 * 24 * time.Hour)

		if err := idb.CreateSession(r.Context(), db, user.ID, ipAddress, userAgent, refreshToken, expiresAt); err != nil {
			log.Errorf("[Login] Failed to create session: %v", err)
			http.Error(w, `{"error": "Failed to login"}`, http.StatusInternalServerError)
			return
		}

		// Set Cookie
		http.SetCookie(w, &http.Cookie{
			Name:     "refresh_token",
			Value:    refreshToken,
			Path:     "/",
			MaxAge:   30 * 24 * 60 * 60,
			HttpOnly: true,
		})

		// Return login response payload
		payload, err := idb.GetUserLoginPayload(r.Context(), db, user)
		if err != nil {
			log.Errorf("[Login] Failed to build response payload: %v", err)
			http.Error(w, `{"error": "Failed to login"}`, http.StatusInternalServerError)
			return
		}

		// Include access token in response user object or payload
		userJSON := user.ToOldJSONForBrowser(false)
		userJSON["accessToken"] = accessToken
		payload["user"] = userJSON

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(payload)
	}
}

func handleAuthorize(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Info("[Go] /api/authorize")
		db = getDB(db)
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

func handleLogout(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Info("[Go] POST /logout")
		db = getDB(db)
		if db == nil {
			http.Error(w, `{"error": "Database not connected"}`, http.StatusInternalServerError)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
			return
		}

		cookie, err := r.Cookie("refresh_token")
		if err == nil && cookie.Value != "" {
			_, _ = idb.DeleteSessionByRefreshToken(r.Context(), db, cookie.Value)
		}

		// Clear Cookie
		http.SetCookie(w, &http.Cookie{
			Name:     "refresh_token",
			Value:    "",
			Path:     "/",
			MaxAge:   -1,
			HttpOnly: true,
		})

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"success":true}`))
	}
}

func handleRefresh(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Info("[Go] POST /auth/refresh")
		db = getDB(db)
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
