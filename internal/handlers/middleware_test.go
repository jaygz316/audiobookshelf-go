package handlers

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	ilogger "audiobookshelf/internal/logger"
)

func TestBasePathRewriteMiddleware(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(r.URL.Path))
	})

	mw := BasePathRewriteMiddleware("/audiobookshelf", handler)

	// Case 1: path already starts with RouterBasePath
	req1 := httptest.NewRequest("GET", "/audiobookshelf/ping", nil)
	rr1 := httptest.NewRecorder()
	mw.ServeHTTP(rr1, req1)
	if rr1.Body.String() != "/audiobookshelf/ping" {
		t.Errorf("expected /audiobookshelf/ping, got %s", rr1.Body.String())
	}

	// Case 2: path needs RouterBasePath prefix
	req2 := httptest.NewRequest("GET", "/ping", nil)
	rr2 := httptest.NewRecorder()
	mw.ServeHTTP(rr2, req2)
	if rr2.Body.String() != "/audiobookshelf/ping" {
		t.Errorf("expected /audiobookshelf/ping, got %s", rr2.Body.String())
	}
}

func TestLoggingMiddlewareRedactsHeaders(t *testing.T) {
	var buf bytes.Buffer
	oldOutput := ilogger.Writer()
	ilogger.SetOutput(&buf)
	defer ilogger.SetOutput(oldOutput)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	mw := LoggingMiddleware(handler)

	req := httptest.NewRequest("GET", "/ping", nil)
	req.Header.Set("Authorization", "Bearer my-secret-token")
	req.Header.Set("Cookie", "session=abcdef")
	req.Header.Set("X-Auth-Token", "token123")
	req.Header.Set("X-API-Key", "apikey123")
	req.Header.Set("X-Proxy-Secret", "mysecret")
	req.Header.Set("X-User-Password", "mypassword")
	req.Header.Set("X-Custom-Header", "custom-value")

	rr := httptest.NewRecorder()
	mw.ServeHTTP(rr, req)

	logOutput := buf.String()
	if !strings.Contains(logOutput, "[REDACTED]") {
		t.Errorf("Expected logs to contain [REDACTED], got: %s", logOutput)
	}
	if strings.Contains(logOutput, "Bearer my-secret-token") {
		t.Error("Secret authorization token was leaked in logs!")
	}
	if strings.Contains(logOutput, "session=abcdef") {
		t.Error("Cookie value was leaked in logs!")
	}
	if strings.Contains(logOutput, "token123") {
		t.Error("X-Auth-Token value was leaked in logs!")
	}
	if strings.Contains(logOutput, "apikey123") {
		t.Error("X-API-Key value was leaked in logs!")
	}
	if strings.Contains(logOutput, "mysecret") {
		t.Error("X-Proxy-Secret value was leaked in logs!")
	}
	if strings.Contains(logOutput, "mypassword") {
		t.Error("X-User-Password value was leaked in logs!")
	}
	if !strings.Contains(logOutput, "custom-value") {
		t.Error("Non-sensitive custom header was not logged correctly!")
	}
}

func TestRateLimitMiddleware(t *testing.T) {
	limiter := NewRateLimiter(3, 100*time.Millisecond)
	defer limiter.Close()

	mw := RateLimitMiddleware(limiter)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	testMW := mw(handler)

	// Make 3 requests from IP 1.1.1.1 -> should all succeed
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("GET", "/ping", nil)
		req.RemoteAddr = "1.1.1.1:1234"
		rr := httptest.NewRecorder()
		testMW.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("Request %d from 1.1.1.1 failed with code %d", i, rr.Code)
		}
	}

	// 4th request from 1.1.1.1 -> should fail with 429
	req := httptest.NewRequest("GET", "/ping", nil)
	req.RemoteAddr = "1.1.1.1:1234"
	rr := httptest.NewRecorder()
	testMW.ServeHTTP(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("Expected 4th request from 1.1.1.1 to return 429, got %d", rr.Code)
	}

	// Request from IP 2.2.2.2 -> should succeed (isolated limit)
	req2 := httptest.NewRequest("GET", "/ping", nil)
	req2.RemoteAddr = "2.2.2.2:1234"
	rr2 := httptest.NewRecorder()
	testMW.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Errorf("Request from 2.2.2.2 failed with code %d", rr2.Code)
	}

	// Wait 150ms for the limit window to reset
	time.Sleep(150 * time.Millisecond)

	// Request from 1.1.1.1 again -> should succeed now
	req3 := httptest.NewRequest("GET", "/ping", nil)
	req3.RemoteAddr = "1.1.1.1:1234"
	rr3 := httptest.NewRecorder()
	testMW.ServeHTTP(rr3, req3)
	if rr3.Code != http.StatusOK {
		t.Errorf("Request from 1.1.1.1 after window reset failed with code %d", rr3.Code)
	}
}
