package handlers

import (
	log "audiobookshelf/internal/logger"
	"context"
	"database/sql"
	"fmt"
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
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		actualDB := getDB(db)
		secret := getTokenSecret(actualDB)
		AuthMiddleware(actualDB, secret, next).ServeHTTP(w, r)
	})
}

var (
	coverRegex = regexp.MustCompile(`(?i)/api/items/[^/]+/cover/?$`)

	// LoginRateLimiter limits login and initialization attempts (5 requests per minute per IP)
	LoginRateLimiter = NewRateLimiter(5, time.Minute)

	// ShareRateLimiter limits public share requests (30 requests per minute per IP)
	ShareRateLimiter = NewRateLimiter(30, time.Minute)
)

// authNotNeeded checks if a request does not require authentication
func authNotNeeded(r *http.Request) bool {
	if r.Method != http.MethodGet {
		return false
	}
	return coverRegex.MatchString(r.URL.Path)
}

// AuthMiddleware authenticates incoming requests
func AuthMiddleware(db *sql.DB, tokenSecret string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		db = getDB(db)
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
	secret := idb.GetTokenSecret(db)
	if secret != "" {
		tokenSecretCacheMu.Lock()
		tokenSecretCache = secret
		tokenSecretCacheMu.Unlock()
	}
	return secret
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

// getDB returns the passed db if non-nil, otherwise falls back to the package-level globalDB.
func getDB(db *sql.DB) *sql.DB {
	if db != nil {
		return db
	}
	return globalDB
}

// LoggingMiddleware logs incoming HTTP requests with sanitized headers.
func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Info("[HTTP] Request",
			"method", r.Method,
			"path", r.URL.Path,
			"remote", r.RemoteAddr,
			"userAgent", r.UserAgent(),
		)

		sanitizedHeaders := make(http.Header)
		for k, v := range r.Header {
			lowerK := strings.ToLower(k)
			if strings.Contains(lowerK, "token") ||
				strings.Contains(lowerK, "key") ||
				strings.Contains(lowerK, "secret") ||
				strings.Contains(lowerK, "auth") ||
				strings.Contains(lowerK, "cookie") ||
				strings.Contains(lowerK, "password") {
				sanitizedHeaders[k] = []string{"[REDACTED]"}
			} else {
				sanitizedHeaders[k] = v
			}
		}
		log.Info("[HTTP] Request Headers", "headers", sanitizedHeaders)
		next.ServeHTTP(w, r)
	})
}

type corsResponseWriter struct {
	http.ResponseWriter
	allowedOrigin string
	headersSet    bool
}

func (w *corsResponseWriter) setCORSHeaders() {
	if w.headersSet {
		return
	}
	h := w.ResponseWriter.Header()
	if w.allowedOrigin != "" {
		h.Set("Access-Control-Allow-Origin", w.allowedOrigin)
		h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		h.Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With, Origin, Accept")
		h.Set("Access-Control-Allow-Credentials", "true")
	} else {
		h.Del("Access-Control-Allow-Origin")
		h.Del("Access-Control-Allow-Methods")
		h.Del("Access-Control-Allow-Headers")
		h.Del("Access-Control-Allow-Credentials")
	}
	w.headersSet = true
}

func (w *corsResponseWriter) WriteHeader(statusCode int) {
	w.setCORSHeaders()
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *corsResponseWriter) Write(b []byte) (int, error) {
	w.setCORSHeaders()
	return w.ResponseWriter.Write(b)
}

// CORSMiddleware handles CORS requests and pre-flight OPTIONS requests.
func CORSMiddleware(db *sql.DB, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		var allowedOrigin string

		if origin != "" {
			settings, err := idb.GetServerSettings(db)
			if err == nil && settings != nil && settings.AllowedCorsOrigins != "" {
				origins := strings.Split(settings.AllowedCorsOrigins, ",")
				for _, o := range origins {
					if strings.TrimSpace(o) == origin {
						allowedOrigin = origin
						break
					}
				}
			}
		}

		if r.Method == "OPTIONS" {
			if allowedOrigin != "" {
				w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With, Origin, Accept")
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}

		writerWrapper := &corsResponseWriter{
			ResponseWriter: w,
			allowedOrigin:  allowedOrigin,
		}

		next.ServeHTTP(writerWrapper, r)
	})
}
