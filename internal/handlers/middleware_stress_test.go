package handlers

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite"

	"audiobookshelf/internal/core"
)

// setupAuthTestDB is a helper to set up a comprehensive in-memory db for testing
func setupAuthTestDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name()))
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}

	tables := []string{
		`CREATE TABLE users (
			id TEXT PRIMARY KEY,
			username TEXT,
			email TEXT,
			pash TEXT,
			type TEXT,
			token TEXT,
			isActive INTEGER,
			isLocked INTEGER,
			lastSeen TEXT,
			permissions TEXT,
			bookmarks TEXT,
			extraData TEXT,
			createdAt TEXT,
			updatedAt TEXT
		)`,
		`CREATE TABLE apiKeys (
			id TEXT PRIMARY KEY,
			isActive INTEGER,
			expiresAt TEXT,
			userId TEXT,
			name TEXT,
			createdAt TEXT
		)`,
		`CREATE TABLE sessions (
			id TEXT PRIMARY KEY,
			userId TEXT,
			ipAddress TEXT,
			userAgent TEXT,
			refreshToken TEXT,
			expiresAt TEXT,
			lastRefreshToken TEXT,
			lastRefreshTokenExpiresAt TEXT,
			createdAt TEXT,
			updatedAt TEXT
		)`,
		`CREATE TABLE settings (
			key TEXT PRIMARY KEY,
			value TEXT,
			createdAt TEXT,
			updatedAt TEXT
		)`,
	}

	for _, q := range tables {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("failed to create table: %v", err)
		}
	}
	return db
}

func createJWT(t *testing.T, secret string, claims *core.AuthClaims) string {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("failed to sign token: %v", err)
	}
	return tokenString
}

func TestAuthMiddleware_BasicAuth(t *testing.T) {
	db := setupAuthTestDB(t)
	defer db.Close()

	hash, _ := bcrypt.GenerateFromPassword([]byte("mypassword"), bcrypt.DefaultCost)

	// Create an active user
	_, err := db.Exec("INSERT INTO users (id, username, pash, type, isActive, permissions) VALUES (?, ?, ?, ?, ?, ?)",
		"user-active", "alice", string(hash), "admin", 1, "{}")
	if err != nil {
		t.Fatal(err)
	}

	// Create an inactive user
	_, err = db.Exec("INSERT INTO users (id, username, pash, type, isActive, permissions) VALUES (?, ?, ?, ?, ?, ?)",
		"user-inactive", "bob", string(hash), "user", 0, "{}")
	if err != nil {
		t.Fatal(err)
	}

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session, ok := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if !ok || session == nil {
			t.Error("Expected UserSession to be present in request context")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(session.Username))
	})

	mw := AuthMiddleware(db, "mysecret", nextHandler)

	t.Run("Valid Basic Auth", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/items", nil)
		req.SetBasicAuth("alice", "mypassword")
		rr := httptest.NewRecorder()
		mw.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d. Body: %s", rr.Code, rr.Body.String())
		}
		if rr.Body.String() != "alice" {
			t.Errorf("Expected body 'alice', got %s", rr.Body.String())
		}
	})

	t.Run("Invalid Password", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/items", nil)
		req.SetBasicAuth("alice", "wrongpassword")
		rr := httptest.NewRecorder()
		mw.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Errorf("Expected status 401, got %d", rr.Code)
		}
	})

	t.Run("Inactive User", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/items", nil)
		req.SetBasicAuth("bob", "mypassword")
		rr := httptest.NewRecorder()
		mw.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Errorf("Expected status 401, got %d", rr.Code)
		}
	})

	t.Run("Non-Existent User", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/items", nil)
		req.SetBasicAuth("charlie", "mypassword")
		rr := httptest.NewRecorder()
		mw.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Errorf("Expected status 401, got %d", rr.Code)
		}
	})
}

func TestAuthMiddleware_JWT(t *testing.T) {
	db := setupAuthTestDB(t)
	defer db.Close()

	secret := "myjwtsecretkey"

	// Create user with non-null extraData to avoid scan error in GetUserByIDOrOldID
	_, err := db.Exec("INSERT INTO users (id, username, type, isActive, permissions, extraData) VALUES (?, ?, ?, ?, ?, ?)",
		"user-1", "alice", "admin", 1, "{}", "{}")
	if err != nil {
		t.Fatal(err)
	}

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session, ok := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if !ok || session == nil {
			t.Error("Expected UserSession to be present in context")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(session.Username))
	})

	mw := AuthMiddleware(db, secret, nextHandler)

	t.Run("Valid JWT without session on DB", func(t *testing.T) {
		claims := &core.AuthClaims{
			UserID:   "user-1",
			Username: "alice",
			Type:     "access",
			RegisteredClaims: jwt.RegisteredClaims{
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
			},
		}
		token := createJWT(t, secret, claims)

		req := httptest.NewRequest("GET", "/api/items", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rr := httptest.NewRecorder()
		mw.ServeHTTP(rr, req)

		// Should fail because no session exists in the sessions table
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("Expected status 401 when no session exists, got %d", rr.Code)
		}
	})

	t.Run("Valid JWT with active session in DB", func(t *testing.T) {
		// Insert active session
		_, err = db.Exec("INSERT INTO sessions (id, userId) VALUES (?, ?)", "session-1", "user-1")
		if err != nil {
			t.Fatal(err)
		}

		claims := &core.AuthClaims{
			UserID:   "user-1",
			Username: "alice",
			Type:     "access",
			RegisteredClaims: jwt.RegisteredClaims{
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
			},
		}
		token := createJWT(t, secret, claims)

		req := httptest.NewRequest("GET", "/api/items", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rr := httptest.NewRecorder()
		mw.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d. Body: %s", rr.Code, rr.Body.String())
		}
		if rr.Body.String() != "alice" {
			t.Errorf("Expected body 'alice', got %s", rr.Body.String())
		}
	})

	t.Run("Expired JWT", func(t *testing.T) {
		claims := &core.AuthClaims{
			UserID:   "user-1",
			Username: "alice",
			Type:     "access",
			RegisteredClaims: jwt.RegisteredClaims{
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(-1 * time.Hour)),
			},
		}
		token := createJWT(t, secret, claims)

		req := httptest.NewRequest("GET", "/api/items", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rr := httptest.NewRecorder()
		mw.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Errorf("Expected status 401 for expired token, got %d", rr.Code)
		}
	})

	t.Run("Refresh Token Rejected", func(t *testing.T) {
		claims := &core.AuthClaims{
			UserID:   "user-1",
			Username: "alice",
			Type:     "refresh",
			RegisteredClaims: jwt.RegisteredClaims{
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
			},
		}
		token := createJWT(t, secret, claims)

		req := httptest.NewRequest("GET", "/api/items", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rr := httptest.NewRecorder()
		mw.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Errorf("Expected status 401 for refresh token, got %d", rr.Code)
		}
	})

	t.Run("Invalid Secret Key", func(t *testing.T) {
		claims := &core.AuthClaims{
			UserID:   "user-1",
			Username: "alice",
			Type:     "access",
			RegisteredClaims: jwt.RegisteredClaims{
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
			},
		}
		token := createJWT(t, "wrongsecretkey", claims)

		req := httptest.NewRequest("GET", "/api/items", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rr := httptest.NewRecorder()
		mw.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Errorf("Expected status 401 for wrong signature, got %d", rr.Code)
		}
	})
}

func TestAuthMiddleware_APIKey(t *testing.T) {
	db := setupAuthTestDB(t)
	defer db.Close()

	// Insert user
	_, err := db.Exec("INSERT INTO users (id, username, type, isActive, permissions) VALUES (?, ?, ?, ?, ?)",
		"user-1", "alice", "admin", 1, "{}")
	if err != nil {
		t.Fatal(err)
	}

	// Insert active API Key
	_, err = db.Exec("INSERT INTO apiKeys (id, isActive, expiresAt, userId, name) VALUES (?, ?, ?, ?, ?)",
		"apikey-valid", 1, "", "user-1", "My Key")
	if err != nil {
		t.Fatal(err)
	}

	// Insert inactive API Key
	_, err = db.Exec("INSERT INTO apiKeys (id, isActive, expiresAt, userId, name) VALUES (?, ?, ?, ?, ?)",
		"apikey-inactive", 0, "", "user-1", "Inactive Key")
	if err != nil {
		t.Fatal(err)
	}

	// Insert expired API Key
	expiredTime := time.Now().Add(-1 * time.Hour).UTC().Format(time.RFC3339)
	_, err = db.Exec("INSERT INTO apiKeys (id, isActive, expiresAt, userId, name) VALUES (?, ?, ?, ?, ?)",
		"apikey-expired", 1, expiredTime, "user-1", "Expired Key")
	if err != nil {
		t.Fatal(err)
	}

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session, ok := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if !ok || session == nil {
			t.Error("Expected UserSession to be present in context")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(session.Username))
	})

	mw := AuthMiddleware(db, "secret", nextHandler)

	t.Run("Direct Valid API Key in Authorization Header", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/items", nil)
		req.Header.Set("Authorization", "Bearer apikey-valid")
		rr := httptest.NewRecorder()
		mw.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d. Body: %s", rr.Code, rr.Body.String())
		}
	})

	t.Run("Direct Valid API Key in Query Param", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/items?token=apikey-valid", nil)
		rr := httptest.NewRecorder()
		mw.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d. Body: %s", rr.Code, rr.Body.String())
		}
	})

	t.Run("JWT with API Key Claim Type", func(t *testing.T) {
		claims := &core.AuthClaims{
			KeyID: "apikey-valid",
			Type:  "api",
			RegisteredClaims: jwt.RegisteredClaims{
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
			},
		}
		token := createJWT(t, "secret", claims)

		req := httptest.NewRequest("GET", "/api/items", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rr := httptest.NewRecorder()
		mw.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d. Body: %s", rr.Code, rr.Body.String())
		}
	})

	t.Run("Inactive API Key", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/items?token=apikey-inactive", nil)
		rr := httptest.NewRecorder()
		mw.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Errorf("Expected status 401 for inactive API key, got %d", rr.Code)
		}
	})

	t.Run("Expired API Key", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/items?token=apikey-expired", nil)
		rr := httptest.NewRecorder()
		mw.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Errorf("Expected status 401 for expired API key, got %d", rr.Code)
		}
	})
}

func TestAuthMiddleware_BypassAndOPDS(t *testing.T) {
	db := setupAuthTestDB(t)
	defer db.Close()

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("passed"))
	})

	mw := AuthMiddleware(db, "secret", nextHandler)

	t.Run("Bypass Auth for Cover Path (GET)", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/items/some-item-id/cover", nil)
		rr := httptest.NewRecorder()
		mw.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected status 200 for cover endpoint bypass, got %d", rr.Code)
		}
		if rr.Body.String() != "passed" {
			t.Errorf("Expected body 'passed', got %s", rr.Body.String())
		}
	})

	t.Run("No Bypass Auth for Cover Path (POST)", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/items/some-item-id/cover", nil)
		rr := httptest.NewRecorder()
		mw.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Errorf("Expected status 401 for POST cover endpoint, got %d", rr.Code)
		}
	})

	t.Run("OPDS Basic Auth Header Injection on Unauthorized", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/opds/books", nil)
		rr := httptest.NewRecorder()
		mw.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Errorf("Expected status 401, got %d", rr.Code)
		}

		wwwAuth := rr.Header().Get("WWW-Authenticate")
		expected := `Basic realm="Audiobookshelf OPDS"`
		if wwwAuth != expected {
			t.Errorf("Expected WWW-Authenticate header to be %q, got %q", expected, wwwAuth)
		}
	})
}

func TestCORSMiddleware(t *testing.T) {
	db := setupAuthTestDB(t)
	defer db.Close()

	// Insert server settings
	_, err := db.Exec(`INSERT INTO settings (key, value) VALUES ('server-settings', '{"allowedCorsOrigins": "http://allowed1.com,http://allowed2.com"}')`)
	if err != nil {
		t.Fatal(err)
	}

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Mock adding some existing CORS headers down the line
		w.Header().Set("Access-Control-Allow-Origin", "http://original.com")
		w.Header().Set("Access-Control-Allow-Methods", "GET")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("cors-ok"))
	})

	mw := CORSMiddleware(db, nextHandler)

	t.Run("Allowed Origin", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/items", nil)
		req.Header.Set("Origin", "http://allowed1.com")
		rr := httptest.NewRecorder()
		mw.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", rr.Code)
		}
		allowOrigin := rr.Header().Get("Access-Control-Allow-Origin")
		if allowOrigin != "http://allowed1.com" {
			t.Errorf("Expected Access-Control-Allow-Origin to be 'http://allowed1.com', got %q", allowOrigin)
		}
	})

	t.Run("Disallowed Origin strips existing headers", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/items", nil)
		req.Header.Set("Origin", "http://disallowed.com")
		rr := httptest.NewRecorder()
		mw.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", rr.Code)
		}
		allowOrigin := rr.Header().Get("Access-Control-Allow-Origin")
		if allowOrigin != "" {
			t.Errorf("Expected Access-Control-Allow-Origin to be stripped, got %q", allowOrigin)
		}
	})

	t.Run("Empty Origin does not modify headers", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/items", nil)
		rr := httptest.NewRecorder()
		mw.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", rr.Code)
		}
		allowOrigin := rr.Header().Get("Access-Control-Allow-Origin")
		if allowOrigin != "http://original.com" {
			t.Errorf("Expected Access-Control-Allow-Origin to be 'http://original.com', got %q", allowOrigin)
		}
		allowMethods := rr.Header().Get("Access-Control-Allow-Methods")
		if allowMethods != "GET" {
			t.Errorf("Expected Access-Control-Allow-Methods to be 'GET', got %q", allowMethods)
		}
	})

	t.Run("Pre-flight OPTIONS Allowed Origin", func(t *testing.T) {
		req := httptest.NewRequest("OPTIONS", "/api/items", nil)
		req.Header.Set("Origin", "http://allowed2.com")
		rr := httptest.NewRecorder()
		mw.ServeHTTP(rr, req)

		if rr.Code != http.StatusNoContent {
			t.Errorf("Expected status 204 (NoContent) for preflight, got %d", rr.Code)
		}
		allowOrigin := rr.Header().Get("Access-Control-Allow-Origin")
		if allowOrigin != "http://allowed2.com" {
			t.Errorf("Expected Access-Control-Allow-Origin to be 'http://allowed2.com', got %q", allowOrigin)
		}
	})

	t.Run("Pre-flight OPTIONS Disallowed Origin passes through", func(t *testing.T) {
		req := httptest.NewRequest("OPTIONS", "/api/items", nil)
		req.Header.Set("Origin", "http://disallowed.com")
		rr := httptest.NewRecorder()
		mw.ServeHTTP(rr, req)

		// Since disallowed, preflight check doesn't succeed at middleware level, passes through to handler
		if rr.Code != http.StatusOK {
			t.Errorf("Expected status 200 from next handler, got %d", rr.Code)
		}
		allowOrigin := rr.Header().Get("Access-Control-Allow-Origin")
		if allowOrigin != "" {
			t.Errorf("Expected Access-Control-Allow-Origin to be stripped, got %q", allowOrigin)
		}
	})
}

func TestRateLimitMiddleware_ConcurrencyAndStress(t *testing.T) {
	// Create a RateLimiter with limit 10 and window 1 second
	limiter := NewRateLimiter(10, 1*time.Second)
	defer limiter.Close()

	mw := RateLimitMiddleware(limiter)

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	testMW := mw(nextHandler)

	t.Run("Stress concurrency same IP", func(t *testing.T) {
		const goroutinesCount = 10
		const requestsPerGoroutine = 3 // Total 30 requests
		var wg sync.WaitGroup
		wg.Add(goroutinesCount)

		var successCount int64
		var rejectedCount int64
		var mu sync.Mutex

		for i := 0; i < goroutinesCount; i++ {
			go func() {
				defer wg.Done()
				for j := 0; j < requestsPerGoroutine; j++ {
					req := httptest.NewRequest("GET", "/api/items", nil)
					req.RemoteAddr = "1.2.3.4:1234"
					rr := httptest.NewRecorder()

					testMW.ServeHTTP(rr, req)

					mu.Lock()
					if rr.Code == http.StatusOK {
						successCount++
					} else if rr.Code == http.StatusTooManyRequests {
						rejectedCount++
					}
					mu.Unlock()
				}
			}()
		}
		wg.Wait()

		// Out of 30 concurrent requests, exactly 10 should be allowed, and 20 should be rejected.
		if successCount != 10 {
			t.Errorf("Expected exactly 10 successful requests, got %d", successCount)
		}
		if rejectedCount != 20 {
			t.Errorf("Expected exactly 20 rejected requests, got %d", rejectedCount)
		}
	})

	t.Run("Stress cleanup loop concurrency", func(t *testing.T) {
		// Run many rate limiters and clean them up concurrently to check for data races
		var wg sync.WaitGroup
		for i := 0; i < 20; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				rl := NewRateLimiter(5, 5*time.Millisecond)
				for k := 0; k < 50; k++ {
					ip := fmt.Sprintf("192.168.1.%d", k%5)
					rl.Allow(ip)
				}
				time.Sleep(12 * time.Millisecond) // Let ticker fire at least once/twice
				rl.Close()
			}(i)
		}
		wg.Wait()
	})

	t.Run("IP Parsing Edge Cases", func(t *testing.T) {
		cases := []struct {
			name       string
			remoteAddr string
			headers    map[string]string
			expectedIP string
		}{
			{
				name:       "IPv6 RemoteAddr with Port",
				remoteAddr: "[::1]:1234",
				expectedIP: "::1",
			},
			{
				name:       "IPv4 RemoteAddr with Port",
				remoteAddr: "127.0.0.1:5678",
				expectedIP: "127.0.0.1",
			},
			{
				name:       "X-Forwarded-For header",
				remoteAddr: "1.1.1.1:1234",
				headers:    map[string]string{"X-Forwarded-For": "10.0.0.1"},
				expectedIP: "10.0.0.1",
			},
			{
				name:       "X-Real-IP header",
				remoteAddr: "1.1.1.1:1234",
				headers:    map[string]string{"X-Real-IP": "10.0.0.2"},
				expectedIP: "10.0.0.2",
			},
			{
				name:       "X-Forwarded-For takes precedence",
				remoteAddr: "1.1.1.1:1234",
				headers:    map[string]string{"X-Forwarded-For": "10.0.0.1", "X-Real-IP": "10.0.0.2"},
				expectedIP: "10.0.0.1",
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				rl := NewRateLimiter(1, time.Second)
				defer rl.Close()

				mw := RateLimitMiddleware(rl)
				var processedIP string
				handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					// Extract how the IP was checked by checking rate limiter requests map keys
					rl.mu.Lock()
					for k := range rl.requests {
						processedIP = k
					}
					rl.mu.Unlock()
					w.WriteHeader(http.StatusOK)
				})

				req := httptest.NewRequest("GET", "/api/items", nil)
				req.RemoteAddr = tc.remoteAddr
				for k, v := range tc.headers {
					req.Header.Set(k, v)
				}

				rr := httptest.NewRecorder()
				mw(handler).ServeHTTP(rr, req)

				if processedIP != tc.expectedIP {
					t.Errorf("Expected processed IP to be %q, got %q", tc.expectedIP, processedIP)
				}
			})
		}
	})
}

func TestBasePathRewriteMiddleware_EdgeCases(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(r.URL.Path))
	})

	t.Run("Empty RouterBasePath", func(t *testing.T) {
		mw := BasePathRewriteMiddleware("", handler)
		req := httptest.NewRequest("GET", "/ping", nil)
		rr := httptest.NewRecorder()
		mw.ServeHTTP(rr, req)
		if rr.Body.String() != "/ping" {
			t.Errorf("Expected /ping, got %s", rr.Body.String())
		}
	})

	t.Run("Single slash RouterBasePath", func(t *testing.T) {
		mw := BasePathRewriteMiddleware("/", handler)
		req := httptest.NewRequest("GET", "/ping", nil)
		rr := httptest.NewRecorder()
		mw.ServeHTTP(rr, req)
		if rr.Body.String() != "/ping" {
			t.Errorf("Expected /ping, got %s", rr.Body.String())
		}
	})

	t.Run("Nested RouterBasePath missing prefix", func(t *testing.T) {
		mw := BasePathRewriteMiddleware("/audio/books", handler)
		req := httptest.NewRequest("GET", "/ping", nil)
		rr := httptest.NewRecorder()
		mw.ServeHTTP(rr, req)
		// Assuming joinPath behaves like path.Join/Clean or simple concatenation
		expectedPath := "/audio/books/ping"
		if rr.Body.String() != expectedPath {
			t.Errorf("Expected %s, got %s", expectedPath, rr.Body.String())
		}
	})
}
