package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

type contextKey string

const UserContextKey contextKey = "user"

// AuthClaims represents the structure of Audiobookshelf JWT claims
type AuthClaims struct {
	UserID   string `json:"userId,omitempty"`
	Username string `json:"username,omitempty"`
	KeyID    string `json:"keyId,omitempty"`
	Type     string `json:"type,omitempty"`
	jwt.RegisteredClaims
}

var (
	coverRegex  = regexp.MustCompile(`^/audiobookshelf/api/items/[^/]+/cover$`)
	authorRegex = regexp.MustCompile(`^/audiobookshelf/api/authors/[^/]+/image$`)
)

// authNotNeeded checks if a request does not require authentication
func authNotNeeded(r *http.Request) bool {
	if r.Method != http.MethodGet {
		return false
	}
	path := r.URL.Path
	return coverRegex.MatchString(path) || authorRegex.MatchString(path)
}

// AuthMiddleware authenticates incoming requests
func AuthMiddleware(db *sql.DB, tokenSecret string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Public routes/patterns that bypass auth
		if authNotNeeded(r) {
			next.ServeHTTP(w, r)
			return
		}

		if db == nil {
			log.Printf("[Auth] Database is not connected")
			http.Error(w, `{"error": "Database not connected"}`, http.StatusInternalServerError)
			return
		}

		// Extract token
		var tokenStr string
		authHeader := r.Header.Get("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") {
			tokenStr = strings.TrimPrefix(authHeader, "Bearer ")
		} else {
			tokenStr = r.URL.Query().Get("token")
		}

		if tokenStr == "" {
			// Check cookie (refresh token or session token, though Audiobookshelf relies on Bearer/Query)
			log.Printf("[Auth] Unauthorized: No token found")
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}

		// Parse and validate JWT
		claims := &AuthClaims{}
		token, err := jwt.ParseWithClaims(tokenStr, claims, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return []byte(tokenSecret), nil
		})

		if err != nil || !token.Valid {
			log.Printf("[Auth] Unauthorized: Invalid JWT signature or expired: %v", err)
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}

		// Authenticate based on token type
		var userSession *UserSession
		var authErr error

		if claims.Type == "api" {
			// API Key based authentication
			userSession, authErr = CheckAPIKey(db, claims.KeyID)
			if authErr != nil {
				log.Printf("[Auth] API key auth failed: %v", authErr)
				http.Error(w, fmt.Sprintf(`{"error": "%s"}`, authErr.Error()), http.StatusUnauthorized)
				return
			}
		} else {
			// Standard JWT authentication
			userSession, authErr = GetUserByIDOrOldID(db, claims.UserID)
			if authErr != nil {
				log.Printf("[Auth] User lookup failed for ID %s: %v", claims.UserID, authErr)
				http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
				return
			}
			if !userSession.IsActive {
				log.Printf("[Auth] User %s is inactive", userSession.Username)
				http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
				return
			}
		}

		// Inject user info into context
		ctx := context.WithValue(r.Context(), UserContextKey, userSession)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
