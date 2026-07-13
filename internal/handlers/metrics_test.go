package handlers

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMetricsEndpointAndMiddleware(t *testing.T) {
	// Reset atomic counters first so test is deterministic
	metricHTTPRequestsTotal = 0
	metricHTTPActiveRequests = 0

	// 1. Check direct metrics handler output
	handler := handleMetrics(nil) // Pass nil db for isolated runtime/middleware test
	req := httptest.NewRequest("GET", "/metrics", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected 200 OK for metrics, got %d", rr.Code)
	}

	contentType := rr.Header().Get("Content-Type")
	if !strings.Contains(contentType, "text/plain") {
		t.Errorf("Expected Content-Type text/plain, got %s", contentType)
	}

	body, err := io.ReadAll(rr.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	bodyStr := string(body)
	if !strings.Contains(bodyStr, "go_goroutines") {
		t.Error("Metrics output missing go_goroutines gauge")
	}
	if !strings.Contains(bodyStr, "audiobookshelf_http_requests_total") {
		t.Error("Metrics output missing audiobookshelf_http_requests_total counter")
	}

	// 2. Test metrics middleware tracks requests
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mw := MetricsMiddleware(testHandler)

	for i := 0; i < 3; i++ {
		r := httptest.NewRequest("GET", "/any-endpoint", nil)
		w := httptest.NewRecorder()
		mw.ServeHTTP(w, r)
	}

	// Check metrics handler output again to verify counters incremented
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req)

	body2, _ := io.ReadAll(rr2.Body)
	body2Str := string(body2)

	if !strings.Contains(body2Str, "audiobookshelf_http_requests_total 3") {
		t.Errorf("Expected total requests to be 3 in metrics output, body was:\n%s", body2Str)
	}
}
