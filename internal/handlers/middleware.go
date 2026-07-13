package handlers

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"audiobookshelf/internal/core"
	idb "audiobookshelf/internal/db"
)

// BasePathRewriteMiddleware ensures the request path starts with RouterBasePath.
func BasePathRewriteMiddleware(routerBasePath string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, routerBasePath) {
			r.URL.Path = joinPath(routerBasePath, r.URL.Path)
		}
		next.ServeHTTP(w, r)
	})
}

// AuthMiddlewareWrapper wraps the standard AuthMiddleware from auth.go using the DB-derived token secret.
func AuthMiddlewareWrapper(db *sql.DB, next http.Handler) http.Handler {
	return AuthMiddleware(db, getTokenSecret(db), next)
}

var (
	coverRegex  = regexp.MustCompile(`^/audiobookshelf/api/items/[^/]+/cover$`)
	authorRegex = regexp.MustCompile(`^/audiobookshelf/api/authors/[^/]+/image$`)

	// LoginRateLimiter limits login and initialization attempts (5 requests per minute per IP)
	LoginRateLimiter = NewRateLimiter(5, time.Minute)
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
			log.Printf("[Auth] Unauthorized: No token found for %s %s", r.Method, r.URL.Path)
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}

		// Check if tokenStr is a valid API key and authenticate directly
		if !strings.Contains(tokenStr, ".") {
			if userSession, err := idb.CheckAPIKey(db, tokenStr); err == nil && userSession != nil {
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
			log.Printf("[Auth] Unauthorized: Invalid JWT signature or expired for %s %s: %v", r.Method, r.URL.Path, err)
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
				log.Printf("[Auth] API key auth failed: %v", authErr)
				http.Error(w, fmt.Sprintf(`{"error": "%s"}`, authErr.Error()), http.StatusUnauthorized)
				return
			}
		} else {
			// Standard JWT authentication
			userSession, authErr = idb.GetUserByIDOrOldID(db, claims.UserID)
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
			// Verify user has at least one session in sessions table
			var sessionExists int
			err := db.QueryRow("SELECT COUNT(*) FROM sessions WHERE userId = ?", claims.UserID).Scan(&sessionExists)
			if err != nil || sessionExists == 0 {
				log.Printf("[Auth] Unauthorized: No active sessions for user ID %s", claims.UserID)
				http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
				return
			}
		}

		// Inject user info into context
		ctx := context.WithValue(r.Context(), core.UserContextKey, userSession)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

var tokenSecretCache string
var tokenSecretCacheMu sync.RWMutex

func getTokenSecret(db *sql.DB) string {
	if envSecret := os.Getenv("JWT_SECRET_KEY"); envSecret != "" {
		return envSecret
	}
	tokenSecretCacheMu.RLock()
	cached := tokenSecretCache
	tokenSecretCacheMu.RUnlock()
	if cached != "" {
		return cached
	}
	if db == nil {
		return ""
	}
	settings, err := idb.GetServerSettings(db)
	if err == nil && settings != nil && settings.TokenSecret != "" {
		tokenSecretCacheMu.Lock()
		tokenSecretCache = settings.TokenSecret
		tokenSecretCacheMu.Unlock()
		return settings.TokenSecret
	}
	return ""
}

// RateLimiter tracks request rate per IP.
type RateLimiter struct {
	mu       sync.Mutex
	requests map[string][]time.Time
	limit    int           // max requests
	window   time.Duration // time window
	stopChan chan struct{}
}

// NewRateLimiter creates a new RateLimiter.
func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	limiter := &RateLimiter{
		requests: make(map[string][]time.Time),
		limit:    limit,
		window:   window,
		stopChan: make(chan struct{}),
	}
	go limiter.cleanupLoop()
	return limiter
}

// Close stops the cleanup loop.
func (rl *RateLimiter) Close() {
	close(rl.stopChan)
}

func (rl *RateLimiter) cleanupLoop() {
	ticker := time.NewTicker(rl.window * 2)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			rl.mu.Lock()
			now := time.Now()
			for ip, times := range rl.requests {
				var valid []time.Time
				for _, t := range times {
					if now.Sub(t) < rl.window {
						valid = append(valid, t)
					}
				}
				if len(valid) == 0 {
					delete(rl.requests, ip)
				} else {
					rl.requests[ip] = valid
				}
			}
			rl.mu.Unlock()
		case <-rl.stopChan:
			return
		}
	}
}

// Allow checks if the request is allowed for the given IP.
func (rl *RateLimiter) Allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	times, exists := rl.requests[ip]
	if !exists {
		rl.requests[ip] = []time.Time{now}
		return true
	}

	var active []time.Time
	for _, t := range times {
		if now.Sub(t) < rl.window {
			active = append(active, t)
		}
	}

	if len(active) >= rl.limit {
		rl.requests[ip] = active
		return false
	}

	rl.requests[ip] = append(active, now)
	return true
}

// RateLimitMiddleware returns a middleware that rate limits requests.
func RateLimitMiddleware(limiter *RateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := r.Header.Get("X-Forwarded-For")
			if ip == "" {
				ip = r.Header.Get("X-Real-IP")
			}
			if ip == "" {
				idx := strings.LastIndex(r.RemoteAddr, ":")
				if idx != -1 {
					ip = r.RemoteAddr[:idx]
				} else {
					ip = r.RemoteAddr
				}
			}

			// Clean IPv6 brackets if RemoteAddr has them
			ip = strings.TrimPrefix(ip, "[")
			ip = strings.TrimSuffix(ip, "]")

			if !limiter.Allow(ip) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte(`{"error": "Too many requests. Please try again later."}`))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
