package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"audiobookshelf/internal/core"
	idb "audiobookshelf/internal/db"
	"audiobookshelf/internal/utils"
)

// handleLogin handles user login by validating credentials and returning authentication tokens.
func handleLogin(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Info("[Go] POST /login")
		db := getDB(db)
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
