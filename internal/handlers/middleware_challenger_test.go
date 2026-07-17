package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"audiobookshelf/internal/core"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// setupChallengerDB drops and recreates tables with full schema needed for Auth and CORS testing.
func setupChallengerDB(t *testing.T) *sql.DB {
	db := setupTestDB(t)

	// Drop simple users table created by setupTestDB to define the full schema
	_, err := db.Exec(`DROP TABLE users`)
	if err != nil {
		t.Fatalf("failed to drop users table: %v", err)
	}

	// Create full schema tables
	_, err = db.Exec(`CREATE TABLE users (
		id TEXT PRIMARY KEY, 
		username TEXT, 
		email TEXT, 
		pash TEXT, 
		type TEXT, 
		token TEXT, 
		isActive INTEGER, 
		isLocked INTEGER, 
		lastSeen TEXT, 
		permissions TEXT NOT NULL DEFAULT '{}', 
		bookmarks TEXT, 
		extraData TEXT NOT NULL DEFAULT '{}', 
		createdAt TEXT, 
		updatedAt TEXT
	)`)
	if err != nil {
		t.Fatalf("failed to create full users table: %v", err)
	}

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS sessions (
		id TEXT PRIMARY KEY, 
		userId TEXT
	)`)
	if err != nil {
		t.Fatalf("failed to create sessions table: %v", err)
	}

	return db
}

func TestChallenger_AuthMiddleware_BasicAuth(t *testing.T) {
	db := setupChallengerDB(t)
	defer db.Close()

	os.Setenv("JWT_SECRET_KEY", "my-test-secret-123")
	defer os.Unsetenv("JWT_SECRET_KEY")

	// Insert test user
	passHash, _ := bcrypt.GenerateFromPassword([]byte("correct-password"), bcrypt.DefaultCost)
	_, err := db.Exec(`INSERT INTO users (id, username, pash, isActive, type, permissions) VALUES (?, ?, ?, 1, 'admin', '{}')`,
		"user-1", "john_doe", string(passHash))
	if err != nil {
		t.Fatalf("failed to insert user: %v", err)
	}

	// Active session in sessions table (needed for some flows, though basic auth might bypass checking session table depending on code paths, but let's see)
	_, _ = db.Exec(`INSERT INTO sessions (id, userId) VALUES ('sess-1', 'user-1')`)

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, ok := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if !ok || u == nil {
			http.Error(w, "no user in context", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(u.Username))
	})

	mw := AuthMiddleware(db, "my-test-secret-123", nextHandler)

	t.Run("Valid Basic Auth", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/libraries", nil)
		req.SetBasicAuth("john_doe", "correct-password")
		rr := httptest.NewRecorder()

		mw.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected 200 OK, got %d. Body: %s", rr.Code, rr.Body.String())
		}
		if rr.Body.String() != "john_doe" {
			t.Errorf("expected body 'john_doe', got %q", rr.Body.String())
		}
	})

	t.Run("Invalid Password", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/libraries", nil)
		req.SetBasicAuth("john_doe", "wrong-password")
		rr := httptest.NewRecorder()

		mw.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Errorf("expected 401 Unauthorized, got %d", rr.Code)
		}
	})

	t.Run("Nonexistent User", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/libraries", nil)
		req.SetBasicAuth("unknown_user", "some-password")
		rr := httptest.NewRecorder()

		mw.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Errorf("expected 401 Unauthorized, got %d", rr.Code)
		}
	})

	t.Run("Inactive User Basic Auth", func(t *testing.T) {
		// Set john_doe to inactive
		_, _ = db.Exec(`UPDATE users SET isActive = 0 WHERE id = 'user-1'`)
		defer func() {
			_, _ = db.Exec(`UPDATE users SET isActive = 1 WHERE id = 'user-1'`)
		}()

		req := httptest.NewRequest("GET", "/api/libraries", nil)
		req.SetBasicAuth("john_doe", "correct-password")
		rr := httptest.NewRecorder()

		mw.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Errorf("expected 401 Unauthorized, got %d", rr.Code)
		}
	})

	t.Run("OPDS Basic Auth Prompt", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/opds", nil)
		rr := httptest.NewRecorder()

		mw.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Errorf("expected 401 Unauthorized, got %d", rr.Code)
		}
		authHeader := rr.Header().Get("WWW-Authenticate")
		if !strings.Contains(authHeader, `Basic realm="Audiobookshelf OPDS"`) {
			t.Errorf("expected WWW-Authenticate header, got %q", authHeader)
		}
	})
}

func TestChallenger_AuthMiddleware_JWT(t *testing.T) {
	db := setupChallengerDB(t)
	defer db.Close()

	secret := "my-secret-key-456"
	os.Setenv("JWT_SECRET_KEY", secret)
	defer os.Unsetenv("JWT_SECRET_KEY")

	// Insert test user
	_, err := db.Exec(`INSERT INTO users (id, username, isActive, type, permissions) VALUES (?, ?, 1, 'user', '{}')`,
		"user-jwt", "jwt_user")
	if err != nil {
		t.Fatalf("failed to insert user: %v", err)
	}

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, ok := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if !ok || u == nil {
			http.Error(w, "no user in context", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(u.Username))
	})

	mw := AuthMiddleware(db, secret, nextHandler)

	generateJWT := func(userID, username, tokenType string, exp time.Time, key string) string {
		claims := &core.AuthClaims{
			UserID:   userID,
			Username: username,
			Type:     tokenType,
			RegisteredClaims: jwt.RegisteredClaims{
				ExpiresAt: jwt.NewNumericDate(exp),
			},
		}
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		str, _ := token.SignedString([]byte(key))
		return str
	}

	t.Run("Valid JWT", func(t *testing.T) {
		// Standard JWT requires at least one active session in the sessions table
		_, _ = db.Exec(`INSERT INTO sessions (id, userId) VALUES ('sess-jwt', 'user-jwt')`)
		defer func() {
			_, _ = db.Exec(`DELETE FROM sessions WHERE id = 'sess-jwt'`)
		}()

		tokenStr := generateJWT("user-jwt", "jwt_user", "access", time.Now().Add(time.Hour), secret)

		req := httptest.NewRequest("GET", "/api/libraries", nil)
		req.Header.Set("Authorization", "Bearer "+tokenStr)
		rr := httptest.NewRecorder()

		mw.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected 200 OK, got %d. Body: %s", rr.Code, rr.Body.String())
		}
	})

	t.Run("JWT Expired", func(t *testing.T) {
		_, _ = db.Exec(`INSERT INTO sessions (id, userId) VALUES ('sess-jwt', 'user-jwt')`)
		defer func() {
			_, _ = db.Exec(`DELETE FROM sessions WHERE id = 'sess-jwt'`)
		}()

		tokenStr := generateJWT("user-jwt", "jwt_user", "access", time.Now().Add(-time.Hour), secret)

		req := httptest.NewRequest("GET", "/api/libraries", nil)
		req.Header.Set("Authorization", "Bearer "+tokenStr)
		rr := httptest.NewRecorder()

		mw.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Errorf("expected 401 Unauthorized, got %d", rr.Code)
		}
	})

	t.Run("JWT Wrong Secret", func(t *testing.T) {
		_, _ = db.Exec(`INSERT INTO sessions (id, userId) VALUES ('sess-jwt', 'user-jwt')`)
		defer func() {
			_, _ = db.Exec(`DELETE FROM sessions WHERE id = 'sess-jwt'`)
		}()

		tokenStr := generateJWT("user-jwt", "jwt_user", "access", time.Now().Add(time.Hour), "wrong-secret")

		req := httptest.NewRequest("GET", "/api/libraries", nil)
		req.Header.Set("Authorization", "Bearer "+tokenStr)
		rr := httptest.NewRecorder()

		mw.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Errorf("expected 401 Unauthorized, got %d", rr.Code)
		}
	})

	t.Run("JWT Refresh Token Rejected", func(t *testing.T) {
		_, _ = db.Exec(`INSERT INTO sessions (id, userId) VALUES ('sess-jwt', 'user-jwt')`)
		defer func() {
			_, _ = db.Exec(`DELETE FROM sessions WHERE id = 'sess-jwt'`)
		}()

		tokenStr := generateJWT("user-jwt", "jwt_user", "refresh", time.Now().Add(time.Hour), secret)

		req := httptest.NewRequest("GET", "/api/libraries", nil)
		req.Header.Set("Authorization", "Bearer "+tokenStr)
		rr := httptest.NewRecorder()

		mw.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Errorf("expected 401 Unauthorized, got %d", rr.Code)
		}
	})

	t.Run("JWT Missing Session in DB", func(t *testing.T) {
		// No session in sessions table
		_, _ = db.Exec(`DELETE FROM sessions`)

		tokenStr := generateJWT("user-jwt", "jwt_user", "access", time.Now().Add(time.Hour), secret)

		req := httptest.NewRequest("GET", "/api/libraries", nil)
		req.Header.Set("Authorization", "Bearer "+tokenStr)
		rr := httptest.NewRecorder()

		mw.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Errorf("expected 401 Unauthorized, got %d. Body: %s", rr.Code, rr.Body.String())
		}
	})
}

func TestChallenger_AuthMiddleware_APIKey(t *testing.T) {
	db := setupChallengerDB(t)
	defer db.Close()

	os.Setenv("JWT_SECRET_KEY", "my-test-secret-123")
	defer os.Unsetenv("JWT_SECRET_KEY")

	// Insert user
	_, err := db.Exec(`INSERT INTO users (id, username, isActive, type, permissions) VALUES (?, ?, 1, 'user', '{}')`,
		"user-api", "api_user")
	if err != nil {
		t.Fatalf("failed to insert user: %v", err)
	}

	// Insert API Key
	_, err = db.Exec(`INSERT INTO apiKeys (id, isActive, expiresAt, userId, name, createdAt) VALUES (?, 1, '', ?, 'Test Key', '')`,
		"api-key-xyz", "user-api")
	if err != nil {
		t.Fatalf("failed to insert api key: %v", err)
	}

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, ok := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if !ok || u == nil {
			http.Error(w, "no user in context", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(u.Username))
	})

	mw := AuthMiddleware(db, "my-test-secret-123", nextHandler)

	t.Run("Valid API Key via Bearer Header", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/libraries", nil)
		req.Header.Set("Authorization", "Bearer api-key-xyz")
		rr := httptest.NewRecorder()

		mw.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected 200 OK, got %d. Body: %s", rr.Code, rr.Body.String())
		}
		if rr.Body.String() != "api_user" {
			t.Errorf("expected body 'api_user', got %q", rr.Body.String())
		}
	})

	t.Run("Valid API Key via Query Parameter", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/libraries?token=api-key-xyz", nil)
		rr := httptest.NewRecorder()

		mw.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected 200 OK, got %d. Body: %s", rr.Code, rr.Body.String())
		}
	})

	t.Run("Invalid API Key format (claims token type 'api')", func(t *testing.T) {
		// If key claims to be api in a JWT structure, verify it behaves as expected
		claims := &core.AuthClaims{
			KeyID: "api-key-xyz",
			Type:  "api",
			RegisteredClaims: jwt.RegisteredClaims{
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			},
		}
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		tokenStr, _ := token.SignedString([]byte("my-test-secret-123"))

		req := httptest.NewRequest("GET", "/api/libraries", nil)
		req.Header.Set("Authorization", "Bearer "+tokenStr)
		rr := httptest.NewRecorder()

		mw.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected 200 OK, got %d. Body: %s", rr.Code, rr.Body.String())
		}
		if rr.Body.String() != "api_user" {
			t.Errorf("expected body 'api_user', got %q", rr.Body.String())
		}
	})

	t.Run("Invalid/Expired/Inactive API Key", func(t *testing.T) {
		// Set key to inactive
		_, _ = db.Exec(`UPDATE apiKeys SET isActive = 0 WHERE id = 'api-key-xyz'`)
		defer func() {
			_, _ = db.Exec(`UPDATE apiKeys SET isActive = 1 WHERE id = 'api-key-xyz'`)
		}()

		req := httptest.NewRequest("GET", "/api/libraries", nil)
		req.Header.Set("Authorization", "Bearer api-key-xyz")
		rr := httptest.NewRecorder()

		mw.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Errorf("expected 401 Unauthorized, got %d", rr.Code)
		}
	})
}

func TestChallenger_CORSMiddleware(t *testing.T) {
	db := setupChallengerDB(t)
	defer db.Close()

	// Configure allowed origins in server-settings
	settingsJSON, _ := json.Marshal(map[string]interface{}{
		"allowedCorsOrigins": "http://allowed.example.com,https://another.example.com",
	})
	_, err := db.Exec(`UPDATE settings SET value = ? WHERE key = 'server-settings'`, string(settingsJSON))
	if err != nil {
		t.Fatalf("failed to update server settings: %v", err)
	}

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	mw := CORSMiddleware(db, nextHandler)

	t.Run("CORS Allowed Origin Standard Request", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/libraries", nil)
		req.Header.Set("Origin", "http://allowed.example.com")
		rr := httptest.NewRecorder()

		mw.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected 200 OK, got %d", rr.Code)
		}
		if rr.Header().Get("Access-Control-Allow-Origin") != "http://allowed.example.com" {
			t.Errorf("expected Access-Control-Allow-Origin 'http://allowed.example.com', got %q", rr.Header().Get("Access-Control-Allow-Origin"))
		}
		if rr.Header().Get("Access-Control-Allow-Credentials") != "true" {
			t.Errorf("expected Access-Control-Allow-Credentials 'true', got %q", rr.Header().Get("Access-Control-Allow-Credentials"))
		}
	})

	t.Run("CORS Disallowed Origin Standard Request", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/libraries", nil)
		req.Header.Set("Origin", "http://disallowed.example.com")
		rr := httptest.NewRecorder()

		mw.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected 200 OK, got %d", rr.Code)
		}
		if rr.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Errorf("expected no CORS headers, got %q", rr.Header().Get("Access-Control-Allow-Origin"))
		}
	})

	t.Run("CORS Preflight OPTIONS Allowed Origin", func(t *testing.T) {
		req := httptest.NewRequest("OPTIONS", "/api/libraries", nil)
		req.Header.Set("Origin", "http://allowed.example.com")
		rr := httptest.NewRecorder()

		mw.ServeHTTP(rr, req)

		if rr.Code != http.StatusNoContent {
			t.Errorf("expected 204 No Content, got %d", rr.Code)
		}
		if rr.Header().Get("Access-Control-Allow-Origin") != "http://allowed.example.com" {
			t.Errorf("expected allowed origin header, got %q", rr.Header().Get("Access-Control-Allow-Origin"))
		}
	})

	t.Run("CORS Preflight OPTIONS Disallowed Origin", func(t *testing.T) {
		req := httptest.NewRequest("OPTIONS", "/api/libraries", nil)
		req.Header.Set("Origin", "http://disallowed.example.com")
		rr := httptest.NewRecorder()

		mw.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected 200 OK from fallback, got %d", rr.Code)
		}
		if rr.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Errorf("expected no CORS headers for disallowed origin preflight, got %q", rr.Header().Get("Access-Control-Allow-Origin"))
		}
	})

	t.Run("CORS Preflight OPTIONS No Origin Header", func(t *testing.T) {
		req := httptest.NewRequest("OPTIONS", "/api/libraries", nil)
		rr := httptest.NewRecorder()

		mw.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected 200 OK from fallback, got %d", rr.Code)
		}
		if rr.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Errorf("expected no CORS headers, got %q", rr.Header().Get("Access-Control-Allow-Origin"))
		}
	})
}

func TestChallenger_RateLimitMiddleware_Stress(t *testing.T) {
	// 5 allowed requests in a small window
	window := 150 * time.Millisecond
	limiter := NewRateLimiter(5, window)
	defer limiter.Close()

	mw := RateLimitMiddleware(limiter)

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	testMW := mw(nextHandler)

	t.Run("Concurrent Requests Same IP Stress", func(t *testing.T) {
		var wg sync.WaitGroup
		concurrentReqs := 10
		responses := make([]int, concurrentReqs)

		for i := 0; i < concurrentReqs; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				req := httptest.NewRequest("GET", "/ping", nil)
				req.RemoteAddr = "1.2.3.4:4321"
				rr := httptest.NewRecorder()
				testMW.ServeHTTP(rr, req)
				responses[idx] = rr.Code
			}(i)
		}
		wg.Wait()

		successCount := 0
		tooManyRequestsCount := 0
		for _, code := range responses {
			if code == http.StatusOK {
				successCount++
			} else if code == http.StatusTooManyRequests {
				tooManyRequestsCount++
			}
		}

		if successCount != 5 {
			t.Errorf("expected exactly 5 successful requests under concurrent access, got %d", successCount)
		}
		if tooManyRequestsCount != 5 {
			t.Errorf("expected exactly 5 requests to be rate-limited, got %d", tooManyRequestsCount)
		}
	})

	t.Run("Isolation between different IPs under concurrency", func(t *testing.T) {
		// Run concurrent requests from multiple distinct IPs
		var wg sync.WaitGroup
		ips := []string{"11.22.33.44", "55.66.77.88", "99.100.111.122"}
		successesPerIP := make(map[string]int)
		var mu sync.Mutex

		for _, ip := range ips {
			for i := 0; i < 4; i++ { // 4 requests per IP (under the limit of 5)
				wg.Add(1)
				go func(testIP string) {
					defer wg.Done()
					req := httptest.NewRequest("GET", "/ping", nil)
					req.RemoteAddr = testIP + ":1234"
					rr := httptest.NewRecorder()
					testMW.ServeHTTP(rr, req)

					mu.Lock()
					if rr.Code == http.StatusOK {
						successesPerIP[testIP]++
					}
					mu.Unlock()
				}(ip)
			}
		}
		wg.Wait()

		for _, ip := range ips {
			if successesPerIP[ip] != 4 {
				t.Errorf("expected 4 successes for IP %s, got %d", ip, successesPerIP[ip])
			}
		}
	})

	t.Run("Window Reset Behavior", func(t *testing.T) {
		// Hit limit for IP 5.5.5.5
		for i := 0; i < 5; i++ {
			req := httptest.NewRequest("GET", "/ping", nil)
			req.RemoteAddr = "5.5.5.5:80"
			rr := httptest.NewRecorder()
			testMW.ServeHTTP(rr, req)
			if rr.Code != http.StatusOK {
				t.Fatalf("expected initial request %d to succeed", i)
			}
		}

		// 6th request should fail
		req := httptest.NewRequest("GET", "/ping", nil)
		req.RemoteAddr = "5.5.5.5:80"
		rr := httptest.NewRecorder()
		testMW.ServeHTTP(rr, req)
		if rr.Code != http.StatusTooManyRequests {
			t.Errorf("expected 6th request to be throttled, got %d", rr.Code)
		}

		// Wait for window to reset (150ms window, let's wait 200ms)
		time.Sleep(200 * time.Millisecond)

		// Request should succeed now
		rrAfter := httptest.NewRecorder()
		testMW.ServeHTTP(rrAfter, req)
		if rrAfter.Code != http.StatusOK {
			t.Errorf("expected request to succeed after window reset, got %d", rrAfter.Code)
		}
	})

	t.Run("Limiter cleanupLoop stops memory leak", func(t *testing.T) {
		cleanupLimiter := NewRateLimiter(5, 50*time.Millisecond)
		defer cleanupLimiter.Close()

		cleanupLimiter.Allow("9.9.9.9")

		cleanupLimiter.mu.Lock()
		reqsLenBefore := len(cleanupLimiter.requests)
		cleanupLimiter.mu.Unlock()

		if reqsLenBefore != 1 {
			t.Errorf("expected 1 entry in limiter requests, got %d", reqsLenBefore)
		}

		// Wait for cleanup ticker (which runs every window * 2, i.e. 100ms)
		time.Sleep(150 * time.Millisecond)

		cleanupLimiter.mu.Lock()
		reqsLenAfter := len(cleanupLimiter.requests)
		cleanupLimiter.mu.Unlock()

		if reqsLenAfter != 0 {
			t.Errorf("expected 0 entries in limiter requests after cleanup, got %d", reqsLenAfter)
		}
	})
}

func TestChallenger_BasePathRewriteMiddleware(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(r.URL.Path))
	})

	t.Run("Empty Base Path", func(t *testing.T) {
		mw := BasePathRewriteMiddleware("", handler)
		req := httptest.NewRequest("GET", "/ping", nil)
		rr := httptest.NewRecorder()
		mw.ServeHTTP(rr, req)

		if rr.Body.String() != "/ping" {
			t.Errorf("expected '/ping', got %q", rr.Body.String())
		}
	})

	t.Run("Slash Base Path", func(t *testing.T) {
		mw := BasePathRewriteMiddleware("/", handler)
		req := httptest.NewRequest("GET", "/ping", nil)
		rr := httptest.NewRecorder()
		mw.ServeHTTP(rr, req)

		if rr.Body.String() != "/ping" {
			t.Errorf("expected '/ping', got %q", rr.Body.String())
		}
	})

	t.Run("Prefix rewrite segment", func(t *testing.T) {
		mw := BasePathRewriteMiddleware("/abs", handler)
		req := httptest.NewRequest("GET", "/books", nil)
		rr := httptest.NewRecorder()
		mw.ServeHTTP(rr, req)

		if rr.Body.String() != "/abs/books" {
			t.Errorf("expected '/abs/books', got %q", rr.Body.String())
		}
	})

	t.Run("Already contains prefix", func(t *testing.T) {
		mw := BasePathRewriteMiddleware("/abs", handler)
		req := httptest.NewRequest("GET", "/abs/books", nil)
		rr := httptest.NewRecorder()
		mw.ServeHTTP(rr, req)

		if rr.Body.String() != "/abs/books" {
			t.Errorf("expected '/abs/books', got %q", rr.Body.String())
		}
	})

	t.Run("Contains prefix as substring but not segment", func(t *testing.T) {
		// If base path is "/api", and request is "/api-items", HasPrefix checks character by character.
		// strings.HasPrefix("/api-items", "/api") will return true!
		// Therefore it does NOT rewrite it to "/api/api-items". Let's verify this behavior.
		mw := BasePathRewriteMiddleware("/api", handler)
		req := httptest.NewRequest("GET", "/api-items", nil)
		rr := httptest.NewRecorder()
		mw.ServeHTTP(rr, req)

		if rr.Body.String() != "/api-items" {
			t.Errorf("expected '/api-items' (no rewrite due to character prefix matching), got %q", rr.Body.String())
		}
	})
}
