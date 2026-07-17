package handlers

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"

	_ "modernc.org/sqlite"

	idb "audiobookshelf/internal/db"
)

func prepareStressTestDB(t testing.TB) (*sql.DB, string) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "stress_test.db")
	db, err := idb.InitDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to initialize test DB: %v", err)
	}
	return db, tmpDir
}

func TestStressInit(t *testing.T) {
	db, _ := prepareStressTestDB(t)
	defer db.Close()

	const numGoroutines = 20
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	results := make(chan int, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(workerID int) {
			defer wg.Done()
			payload := fmt.Sprintf(`{"newRoot": {"username": "admin-root", "password": "rootpassword-%d"}}`, workerID)
			req := httptest.NewRequest("POST", "/init", bytes.NewBufferString(payload))
			rr := httptest.NewRecorder()

			handler := handleInit(db)
			handler.ServeHTTP(rr, req)

			results <- rr.Code
		}(i)
	}

	wg.Wait()
	close(results)

	successCount := 0
	forbiddenCount := 0
	otherCount := 0

	for code := range results {
		switch code {
		case http.StatusOK:
			successCount++
		case http.StatusForbidden:
			forbiddenCount++
		default:
			otherCount++
		}
	}

	t.Logf("Init stress test results: 200 OK=%d, 403 Forbidden=%d, Other=%d", successCount, forbiddenCount, otherCount)

	// In a fully serialized/atomic world, exactly 1 should succeed.
	// Let's verify how many actually succeeded and how many users are in the DB.
	var count int
	err := db.QueryRow("SELECT count(*) FROM users WHERE type = 'root'").Scan(&count)
	if err != nil {
		t.Fatalf("Failed to check user count: %v", err)
	}

	t.Logf("Total root users in DB: %d", count)

	if count == 0 {
		t.Error("Expected at least one root user to be created")
	}
}

func TestStressLogin(t *testing.T) {
	db, _ := prepareStressTestDB(t)
	defer db.Close()

	// Initialize the root user first
	initPayload := `{"newRoot": {"username": "stressroot", "password": "stresspassword"}}`
	initReq := httptest.NewRequest("POST", "/init", bytes.NewBufferString(initPayload))
	initRR := httptest.NewRecorder()
	handleInit(db).ServeHTTP(initRR, initReq)
	if initRR.Code != http.StatusOK {
		t.Fatalf("Failed to initialize database: %s", initRR.Body.String())
	}

	const numGoroutines = 50
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	results := make(chan int, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			loginPayload := `{"username": "stressroot", "password": "stresspassword"}`
			req := httptest.NewRequest("POST", "/login", bytes.NewBufferString(loginPayload))
			rr := httptest.NewRecorder()

			handler := handleLogin(db)
			handler.ServeHTTP(rr, req)

			results <- rr.Code
		}()
	}

	wg.Wait()
	close(results)

	successCount := 0
	for code := range results {
		if code == http.StatusOK {
			successCount++
		}
	}

	t.Logf("Login stress test: %d/%d requests succeeded", successCount, numGoroutines)
	if successCount != numGoroutines {
		t.Errorf("Expected all %d logins to succeed, but only %d did", numGoroutines, successCount)
	}

	// Verify number of sessions in DB
	var sessionCount int
	err := db.QueryRow("SELECT count(*) FROM sessions").Scan(&sessionCount)
	if err != nil {
		t.Fatalf("Failed to check sessions count: %v", err)
	}
	t.Logf("Total sessions created in DB: %d", sessionCount)
	if sessionCount != numGoroutines {
		t.Errorf("Expected %d sessions in DB, found %d", numGoroutines, sessionCount)
	}
}

func TestStressAuthorize(t *testing.T) {
	db, _ := prepareStressTestDB(t)
	defer db.Close()

	// Initialize database
	initPayload := `{"newRoot": {"username": "stressroot", "password": "stresspassword"}}`
	initReq := httptest.NewRequest("POST", "/init", bytes.NewBufferString(initPayload))
	initRR := httptest.NewRecorder()
	handleInit(db).ServeHTTP(initRR, initReq)

	// Login once to get access token
	loginPayload := `{"username": "stressroot", "password": "stresspassword"}`
	loginReq := httptest.NewRequest("POST", "/login", bytes.NewBufferString(loginPayload))
	loginRR := httptest.NewRecorder()
	handleLogin(db).ServeHTTP(loginRR, loginReq)

	var loginResp map[string]interface{}
	if err := json.Unmarshal(loginRR.Body.Bytes(), &loginResp); err != nil {
		t.Fatalf("Failed to decode login response: %v", err)
	}

	userObj, ok := loginResp["user"].(map[string]interface{})
	if !ok {
		t.Fatalf("No user object in login response: %v", loginResp)
	}
	accessToken, ok := userObj["accessToken"].(string)
	if !ok || accessToken == "" {
		t.Fatalf("No access token in login response user: %v", userObj)
	}

	// Access /api/authorize concurrently
	const numGoroutines = 100
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	results := make(chan int, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			req := httptest.NewRequest("GET", "/api/authorize", nil)
			req.Header.Set("Authorization", "Bearer "+accessToken)
			rr := httptest.NewRecorder()

			handler := AuthMiddlewareWrapper(db, http.HandlerFunc(handleAuthorize(db)))
			handler.ServeHTTP(rr, req)

			results <- rr.Code
		}()
	}

	wg.Wait()
	close(results)

	successCount := 0
	for code := range results {
		if code == http.StatusOK {
			successCount++
		}
	}

	t.Logf("Authorize stress test: %d/%d requests succeeded", successCount, numGoroutines)
	if successCount != numGoroutines {
		t.Errorf("Expected all %d authorizations to succeed, but only %d did", numGoroutines, successCount)
	}
}

func TestStressRefresh(t *testing.T) {
	db, _ := prepareStressTestDB(t)
	defer db.Close()

	// Initialize database
	initPayload := `{"newRoot": {"username": "stressroot", "password": "stresspassword"}}`
	initReq := httptest.NewRequest("POST", "/init", bytes.NewBufferString(initPayload))
	initRR := httptest.NewRecorder()
	handleInit(db).ServeHTTP(initRR, initReq)

	// Login to get refresh token cookie
	loginPayload := `{"username": "stressroot", "password": "stresspassword"}`
	loginReq := httptest.NewRequest("POST", "/login", bytes.NewBufferString(loginPayload))
	loginRR := httptest.NewRecorder()
	handleLogin(db).ServeHTTP(loginRR, loginReq)

	cookies := loginRR.Result().Cookies()
	var refreshCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "refresh_token" {
			refreshCookie = c
			break
		}
	}
	if refreshCookie == nil || refreshCookie.Value == "" {
		t.Fatalf("No refresh token cookie returned from login")
	}

	// Make concurrent calls to /auth/refresh with the same refresh token cookie
	const numGoroutines = 10
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	results := make(chan int, numGoroutines)
	cookiesChan := make(chan *http.Cookie, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			req := httptest.NewRequest("POST", "/auth/refresh", nil)
			req.AddCookie(refreshCookie)
			rr := httptest.NewRecorder()

			handler := handleRefresh(db)
			handler.ServeHTTP(rr, req)

			results <- rr.Code

			// Extract returned cookie
			for _, c := range rr.Result().Cookies() {
				if c.Name == "refresh_token" {
					cookiesChan <- c
				}
			}
		}()
	}

	wg.Wait()
	close(results)
	close(cookiesChan)

	statusCounts := make(map[int]int)
	for code := range results {
		statusCounts[code]++
	}

	t.Logf("Refresh token concurrent test results: %+v", statusCounts)

	// Collect all unique cookies returned
	var returnedCookies []*http.Cookie
	for c := range cookiesChan {
		returnedCookies = append(returnedCookies, c)
	}

	t.Logf("Returned %d cookies from concurrent refreshes", len(returnedCookies))

	// Now try to refresh with each returned cookie. They should all be valid!
	// If any returned cookie is invalid (HTTP 400), it indicates a race condition where a token was given to the client but not registered/saved in the DB.
	invalidCount := 0
	for idx, c := range returnedCookies {
		req := httptest.NewRequest("POST", "/auth/refresh", nil)
		req.AddCookie(c)
		rr := httptest.NewRecorder()

		handleRefresh(db).ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			invalidCount++
			t.Logf("Returned Cookie %d (value: %s...) is INVALID: %d %s", idx, c.Value[:15], rr.Code, rr.Body.String())
		} else {
			t.Logf("Returned Cookie %d is VALID", idx)
		}
	}

	if invalidCount > 0 {
		t.Errorf("Race condition detected: %d out of %d returned refresh tokens are invalid!", invalidCount, len(returnedCookies))
	}
}

func TestStressLogout(t *testing.T) {
	db, _ := prepareStressTestDB(t)
	defer db.Close()

	// Initialize database
	initPayload := `{"newRoot": {"username": "stressroot", "password": "stresspassword"}}`
	initReq := httptest.NewRequest("POST", "/init", bytes.NewBufferString(initPayload))
	initRR := httptest.NewRecorder()
	handleInit(db).ServeHTTP(initRR, initReq)

	// Generate 50 sessions by logging in 50 times
	cookies := make([]*http.Cookie, 50)
	for i := 0; i < 50; i++ {
		loginPayload := `{"username": "stressroot", "password": "stresspassword"}`
		loginReq := httptest.NewRequest("POST", "/login", bytes.NewBufferString(loginPayload))
		loginRR := httptest.NewRecorder()
		handleLogin(db).ServeHTTP(loginRR, loginReq)

		for _, c := range loginRR.Result().Cookies() {
			if c.Name == "refresh_token" {
				cookies[i] = c
				break
			}
		}
	}

	// Logout concurrently
	var wg sync.WaitGroup
	wg.Add(50)

	results := make(chan int, 50)

	for i := 0; i < 50; i++ {
		go func(cookie *http.Cookie) {
			defer wg.Done()
			req := httptest.NewRequest("POST", "/logout", nil)
			if cookie != nil {
				req.AddCookie(cookie)
			}
			rr := httptest.NewRecorder()

			handler := handleLogout(db)
			handler.ServeHTTP(rr, req)

			results <- rr.Code
		}(cookies[i])
	}

	wg.Wait()
	close(results)

	successCount := 0
	for code := range results {
		if code == http.StatusOK {
			successCount++
		}
	}

	t.Logf("Logout stress test: %d/50 requests succeeded", successCount)
	if successCount != 50 {
		t.Errorf("Expected all 50 logouts to succeed, but only %d did", successCount)
	}

	// Verify database is clean (no sessions left)
	var sessionCount int
	err := db.QueryRow("SELECT count(*) FROM sessions").Scan(&sessionCount)
	if err != nil {
		t.Fatalf("Failed to check sessions count: %v", err)
	}
	t.Logf("Total sessions remaining in DB: %d", sessionCount)
	if sessionCount != 0 {
		t.Errorf("Expected 0 sessions remaining in DB, found %d", sessionCount)
	}
}
