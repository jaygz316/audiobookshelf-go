package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOPDS_LibraryAllItems(t *testing.T) {
	db := prepareOPDSTestDB(t)
	defer db.Close()

	handler := AuthMiddleware(db, "mysecret", ServeOPDS(db))

	req := httptest.NewRequest("GET", "/opds/v1.2/libraries/lib-1/all", nil)
	req.SetBasicAuth("admin-user", "mypassword")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d. Body: %s", rr.Code, rr.Body.String())
	}

	body := rr.Body.String()
	if !strings.Contains(body, `<title>Test Book - A Cool Test</title>`) {
		t.Errorf("Expected book title in item list, got: %s", body)
	}

	if !strings.Contains(body, `/api/items/item-1/download`) {
		t.Errorf("Expected download link in item list, got: %s", body)
	}

	if !strings.Contains(body, `type="application/epub+zip"`) {
		t.Errorf("Expected EPUB acquisition mimetype, got: %s", body)
	}

	if !strings.Contains(body, `/api/items/item-1/cover`) {
		t.Errorf("Expected cover link in item list, got: %s", body)
	}
}

func TestOPDS_Search(t *testing.T) {
	db := prepareOPDSTestDB(t)
	defer db.Close()

	handler := AuthMiddleware(db, "mysecret", ServeOPDS(db))

	// Match query
	req := httptest.NewRequest("GET", "/opds/v1.2/libraries/lib-1/search?q=Test", nil)
	req.SetBasicAuth("admin-user", "mypassword")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d. Body: %s", rr.Code, rr.Body.String())
	}

	body := rr.Body.String()
	if !strings.Contains(body, `<title>Test Book - A Cool Test</title>`) {
		t.Errorf("Expected matching search result, got: %s", body)
	}

	// Non-matching query
	reqNoMatch := httptest.NewRequest("GET", "/opds/v1.2/libraries/lib-1/search?q=Nonexistent", nil)
	reqNoMatch.SetBasicAuth("admin-user", "mypassword")
	rrNoMatch := httptest.NewRecorder()

	handler.ServeHTTP(rrNoMatch, reqNoMatch)

	if rrNoMatch.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", rrNoMatch.Code)
	}

	bodyNoMatch := rrNoMatch.Body.String()
	if strings.Contains(bodyNoMatch, `<title>Test Book - A Cool Test</title>`) {
		t.Errorf("Expected zero search results, but got item in feed: %s", bodyNoMatch)
	}
}
