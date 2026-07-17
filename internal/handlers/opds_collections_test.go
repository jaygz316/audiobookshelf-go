package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOPDS_Collections(t *testing.T) {
	db := prepareOPDSTestDB(t)
	defer db.Close()

	handler := AuthMiddleware(db, "mysecret", ServeOPDS(db))

	// Get collections list
	req := httptest.NewRequest("GET", "/opds/v1.2/libraries/lib-1/collections", nil)
	req.SetBasicAuth("admin-user", "mypassword")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "Collection One") || !strings.Contains(body, "/opds/v1.2/libraries/lib-1/collections/coll-1") {
		t.Errorf("Expected collections feed to contain Collection One and link, got: %s", body)
	}

	// Get items in collection
	reqItems := httptest.NewRequest("GET", "/opds/v1.2/libraries/lib-1/collections/coll-1", nil)
	reqItems.SetBasicAuth("admin-user", "mypassword")
	rrItems := httptest.NewRecorder()
	handler.ServeHTTP(rrItems, reqItems)

	if rrItems.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", rrItems.Code)
	}
	bodyItems := rrItems.Body.String()
	if !strings.Contains(bodyItems, "Test Book") {
		t.Errorf("Expected collection items feed to contain Test Book, got: %s", bodyItems)
	}
}
