package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOPDS_Unauthorized(t *testing.T) {
	db := prepareOPDSTestDB(t)
	defer db.Close()

	handler := AuthMiddleware(db, "mysecret", ServeOPDS(db))

	req := httptest.NewRequest("GET", "/opds", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", rr.Code)
	}

	wwwAuth := rr.Header().Get("WWW-Authenticate")
	if !strings.Contains(wwwAuth, "Basic") {
		t.Errorf("Expected WWW-Authenticate header to contain Basic, got: %q", wwwAuth)
	}
}

func TestOPDS_AuthorizedRoot(t *testing.T) {
	db := prepareOPDSTestDB(t)
	defer db.Close()

	handler := AuthMiddleware(db, "mysecret", ServeOPDS(db))

	req := httptest.NewRequest("GET", "/opds", nil)
	req.SetBasicAuth("admin-user", "mypassword")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d. Body: %s", rr.Code, rr.Body.String())
	}

	body := rr.Body.String()
	if !strings.Contains(body, `Audiobookshelf Go Catalog`) {
		t.Errorf("Expected feed title in response body, got: %s", body)
	}

	if !strings.Contains(body, `Browse library: Audiobooks`) {
		t.Errorf("Expected library entry in response body, got: %s", body)
	}

	if !strings.Contains(body, `/opds/v1.2/libraries/lib-1`) {
		t.Errorf("Expected library link in response body, got: %s", body)
	}
}

func TestOPDS_LibraryDetails(t *testing.T) {
	db := prepareOPDSTestDB(t)
	defer db.Close()

	handler := AuthMiddleware(db, "mysecret", ServeOPDS(db))

	req := httptest.NewRequest("GET", "/opds/v1.2/libraries/lib-1", nil)
	req.SetBasicAuth("admin-user", "mypassword")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d. Body: %s", rr.Code, rr.Body.String())
	}

	body := rr.Body.String()
	if !strings.Contains(body, `<title>Audiobooks</title>`) {
		t.Errorf("Expected library feed title, got: %s", body)
	}

	if !strings.Contains(body, `/opds/v1.2/libraries/lib-1/all`) {
		t.Errorf("Expected subsection All Items link, got: %s", body)
	}

	if !strings.Contains(body, `/opds/v1.2/libraries/lib-1/recent`) {
		t.Errorf("Expected subsection Recent link, got: %s", body)
	}

	if !strings.Contains(body, `/opds/v1.2/libraries/lib-1/search?q={searchTerms}`) {
		t.Errorf("Expected search template, got: %s", body)
	}
}

func TestOPDS_LibraryDetails_NewSubsections(t *testing.T) {
	db := prepareOPDSTestDB(t)
	defer db.Close()

	handler := AuthMiddleware(db, "mysecret", ServeOPDS(db))

	req := httptest.NewRequest("GET", "/opds/v1.2/libraries/lib-1", nil)
	req.SetBasicAuth("admin-user", "mypassword")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d. Body: %s", rr.Code, rr.Body.String())
	}

	body := rr.Body.String()
	subsections := []string{
		`/opds/v1.2/libraries/lib-1/authors`,
		`/opds/v1.2/libraries/lib-1/series`,
		`/opds/v1.2/libraries/lib-1/collections`,
		`/opds/v1.2/libraries/lib-1/playlists`,
	}
	for _, sub := range subsections {
		if !strings.Contains(body, sub) {
			t.Errorf("Expected library details to link to %s", sub)
		}
	}
}
