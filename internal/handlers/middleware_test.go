package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"
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

	// Case 2: path does not start with RouterBasePath
	req2 := httptest.NewRequest("GET", "/ping", nil)
	rr2 := httptest.NewRecorder()
	mw.ServeHTTP(rr2, req2)
	if rr2.Body.String() != "/audiobookshelf/ping" {
		t.Errorf("expected /audiobookshelf/ping, got %s", rr2.Body.String())
	}
}
