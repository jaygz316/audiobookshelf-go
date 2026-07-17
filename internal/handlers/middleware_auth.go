package handlers

import (
	log "audiobookshelf/internal/logger"
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"audiobookshelf/internal/core"
	idb "audiobookshelf/internal/db"
)

// AuthMiddlewareWrapper wraps the standard AuthMiddleware from auth.go using the DB-derived token secret.
func AuthMiddlewareWrapper(db *sql.DB, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		actualDB := getDB(db)
		secret := getTokenSecret(actualDB)
		AuthMiddleware(actualDB, secret, next).ServeHTTP(w, r)
	})
}

// AuthMiddleware authenticates incoming requests
func AuthMiddleware(db *sql.DB, tokenSecret string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		db := getDB(db)
		// Public routes/patterns that bypass auth
		if authNotNeeded(r) {
			next.ServeHTTP(w, r)
			return
		}

		if db == nil {
			log.Error("Database is not connected")
			http.Error(w, `{"error": "Database not connected"}`, http.StatusInternalServerError)
			return
		}

		if tokenSecret == "" {
			tokenSecret = getTokenSecret(db)
		}

		// Extract token
		var tokenStr string
		authHeader := r.Header.Get("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") {
			tokenStr = strings.TrimPrefix(authHeader, "Bearer ")
		} else if strings.HasPrefix(authHeader, "Basic ") {
			username, password, ok := r.BasicAuth()
			if ok {
				user, err := idb.GetUserFullByUsername(r.Context(), db, username)
				if err == nil && user != nil && user.IsActive {
					errCompare := bcrypt.CompareHashAndPassword([]byte(user.Pash), []byte(password))
					if errCompare == nil {
						userSession, errSession := idb.GetUserByID(db, user.ID)
						if errSession == nil && userSession != nil {
							ctx := context.WithValue(r.Context(), core.UserContextKey, userSession)
							next.ServeHTTP(w, r.WithContext(ctx))
							return
						}
					}
				}
			}
		} else {
			tokenStr = r.URL.Query().Get("token")
		}

		if tokenStr == "" {
			if strings.HasPrefix(r.URL.Path, "/opds") || strings.Contains(r.URL.Path, "/opds/") {
				w.Header().Set("WWW-Authenticate", `Basic realm="Audiobookshelf OPDS"`)
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte("Unauthorized"))
				return
			}
			log.Warn("Unauthorized: No token found", "method", r.Method, "path", r.URL.Path)
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}

		// Check if tokenStr is a valid API key and authenticate directly
		if !strings.Contains(tokenStr, ".") {
			if userSession, err := idb.CheckAPIKey(db, tokenStr); err == nil && userSession != nil {
				if !userSession.IsActive {
					log.Warn("User is inactive (API key)", "username", userSession.Username)
					http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
					return
				}
				ctx := context.WithValue(r.Context(), core.UserContextKey, userSession)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
		}

		// Parse and validate JWT
		claims := &core.AuthClaims{}
		token, err := jwt.ParseWithClaims(tokenStr, claims, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return []byte(tokenSecret), nil
		})

		if err != nil || !token.Valid {
			log.Warn("Unauthorized: Invalid JWT signature or expired", "method", r.Method, "path", r.URL.Path, "error", err)
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}

		if claims.Type == "refresh" {
			log.Warn("Unauthorized: Cannot use refresh token for standard API access", "method", r.Method, "path", r.URL.Path)
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}

		// Authenticate based on token type
		var userSession *core.UserSession
		var authErr error

		if claims.Type == "api" {
			// API Key based authentication
			userSession, authErr = idb.CheckAPIKey(db, claims.KeyID)
			if authErr != nil {
				log.Warn("API key auth failed", "error", authErr)
				http.Error(w, fmt.Sprintf(`{"error": "%s"}`, authErr.Error()), http.StatusUnauthorized)
				return
			}
			if !userSession.IsActive {
				log.Warn("User is inactive (API key claims)", "username", userSession.Username)
				http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
				return
			}
		} else {
			// Standard JWT authentication
			userSession, authErr = idb.GetUserByIDOrOldID(db, claims.UserID)
			if authErr != nil {
				log.Error("User lookup failed", "userID", claims.UserID, "error", authErr)
				http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
				return
			}
			if !userSession.IsActive {
				log.Warn("User is inactive", "username", userSession.Username)
				http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
				return
			}
			// Verify user has at least one session in sessions table
			var sessionExists int
			err := db.QueryRow("SELECT COUNT(*) FROM sessions WHERE userId = ?", claims.UserID).Scan(&sessionExists)
			if err != nil || sessionExists == 0 {
				log.Warn("Unauthorized: No active sessions for user ID", "userID", claims.UserID)
				http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
				return
			}
		}

		// Inject user info into context
		ctx := context.WithValue(r.Context(), core.UserContextKey, userSession)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
