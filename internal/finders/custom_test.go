package finders

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"

	_ "modernc.org/sqlite"
)

func TestFinderWithCustomProvider(t *testing.T) {
	// 1. Setup mock HTTP server for custom provider
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"matches": [
				{
					"title": "Custom Book",
					"author": "Custom Author"
				}
			]
		}`))
	}))
	defer server.Close()

	// 2. Setup in-memory SQLite database
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	// Create table and insert a custom provider
	_, err = db.Exec(`CREATE TABLE customMetadataProviders (
		id TEXT PRIMARY KEY,
		name TEXT,
		mediaType TEXT,
		url TEXT,
		authHeaderValue TEXT,
		extraData TEXT,
		createdAt INTEGER,
		updatedAt INTEGER
	)`)
	if err != nil {
		t.Fatalf("failed to create table: %v", err)
	}

	_, err = db.Exec(`INSERT INTO customMetadataProviders (id, name, mediaType, url, authHeaderValue, extraData, createdAt, updatedAt)
		VALUES ('test-prov-id', 'Mock Custom', 'book', ?, 'Bearer tok', '{}', 12345, 12345)`, server.URL)
	if err != nil {
		t.Fatalf("failed to insert custom provider: %v", err)
	}

	// 3. Initialize Finder with db
	finder := NewFinder(db, nil)

	// 4. Perform SearchBooks using custom provider
	res, err := finder.SearchBooks(context.Background(), "custom-test-prov-id", "test query")
	if err != nil {
		t.Fatalf("SearchBooks failed: %v", err)
	}

	if len(res) != 1 || res[0].Title != "Custom Book" || len(res[0].Authors) != 1 || res[0].Authors[0] != "Custom Author" {
		t.Errorf("Unexpected results: %+v", res)
	}

	// 5. Test searchAllBooks which should query custom providers as well
	allRes, err := finder.SearchBooks(context.Background(), "all", "test query")
	if err != nil {
		t.Fatalf("SearchBooks with 'all' failed: %v", err)
	}

	foundCustom := false
	for _, r := range allRes {
		if r.Title == "Custom Book" {
			foundCustom = true
			break
		}
	}
	if !foundCustom {
		t.Error("Expected SearchBooks with 'all' to include results from custom provider")
	}
}
