package handlers

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/doyensec/safeurl"
	_ "modernc.org/sqlite"

	"audiobookshelf/internal/core"
	"audiobookshelf/internal/providers"
)

type mockTransport struct {
	TargetURL string
}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	target, err := url.Parse(m.TargetURL)
	if err != nil {
		return nil, err
	}
	req.URL.Scheme = target.Scheme
	req.URL.Host = target.Host
	return http.DefaultTransport.RoundTrip(req)
}

func TestAuthorsSearchAndMatch(t *testing.T) {
	// 1. Setup mock Audnexus API Server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/authors") {
			name := r.URL.Query().Get("name")
			if name == "King" {
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`[{"asin": "B000AP9U0C", "name": "Stephen King"}]`))
				return
			}

			// Single author query: /authors/B000AP9U0C
			if strings.Contains(strings.ToLower(r.URL.Path), "b000ap9u0c") {
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`{
					"asin": "B000AP9U0C",
					"name": "Stephen King",
					"description": "Horror writer",
					"image": "http://mockserver/king.jpg"
				}`))
				return
			}
		}

		if r.URL.Path == "/king.jpg" {
			w.Header().Set("Content-Type", "image/jpeg")
			w.Write([]byte("fake-jpeg-bytes"))
			return
		}

		http.NotFound(w, r)
	}))
	defer mockServer.Close()

	// 2. Setup mock client for safeurl
	transport := &mockTransport{TargetURL: mockServer.URL}
	config := safeurl.GetConfigBuilder().Build()
	mockWrappedClient := safeurl.Client(config)
	mockWrappedClient.Client = &http.Client{
		Transport: transport,
	}
	providers.SetSafeHTTPClientForTest(mockWrappedClient)

	// 3. Setup test database
	db := setupTestDBShared(t)
	defer db.Close()

	// Insert initial library and author
	_, err := db.Exec(`INSERT INTO libraries (id, name, displayOrder, icon, mediaType, provider, settings, createdAt, updatedAt) VALUES ('lib1', 'Audiobooks', 1, 'book', 'book', 'local', '{}', '2026-06-08 12:00:00.000', '2026-06-08 12:00:00.000')`)
	if err != nil {
		t.Fatalf("Failed to insert library: %v", err)
	}
	_, err = db.Exec(`INSERT INTO authors (id, libraryId, name, lastFirst, asin, description, imagePath, createdAt, updatedAt) VALUES 
		('author_king', 'lib1', 'Stephen King', 'King, Stephen', '', '', '', '2026-06-08 12:00:00.000', '2026-06-08 12:00:00.000')`)
	if err != nil {
		t.Fatalf("Failed to insert author: %v", err)
	}

	// 4. Test handleSearchAuthors
	searchHandler := handleSearchAuthors(db)
	searchReq := httptest.NewRequest("GET", "/api/search/authors?name=King", nil)
	searchReq = searchReq.WithContext(context.WithValue(searchReq.Context(), core.UserContextKey, &core.UserSession{
		ID:       "user1",
		Username: "admin",
		Type:     "admin",
	}))
	searchRec := httptest.NewRecorder()
	searchHandler.ServeHTTP(searchRec, searchReq)

	if searchRec.Code != http.StatusOK {
		t.Errorf("Expected search response 200, got %d, body: %s", searchRec.Code, searchRec.Body.String())
	}

	var searchResults []*providers.AudnexusAuthorDetails
	if err := json.Unmarshal(searchRec.Body.Bytes(), &searchResults); err != nil {
		t.Fatalf("Failed to unmarshal search results: %v", err)
	}
	if len(searchResults) != 1 || searchResults[0].ASIN != "B000AP9U0C" {
		t.Errorf("Expected search results for B000AP9U0C, got: %+v", searchResults)
	}

	// 5. Test handleMatchAuthor
	tempDir, err := os.MkdirTemp("", "abs-metadata-*")
	if err != nil {
		t.Fatalf("Failed to create temp metadata dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	cfg := &core.Config{
		MetadataPath: tempDir,
	}

	matchHandler := handleMatchAuthor(db, cfg, "author_king")
	matchBody := `{"asin": "B000AP9U0C", "provider": "audnexus"}`
	matchReq := httptest.NewRequest("POST", "/api/authors/author_king/match", bytes.NewBufferString(matchBody))
	matchReq = matchReq.WithContext(context.WithValue(matchReq.Context(), core.UserContextKey, &core.UserSession{
		ID:       "user1",
		Username: "admin",
		Type:     "admin",
	}))
	matchRec := httptest.NewRecorder()
	matchHandler.ServeHTTP(matchRec, matchReq)

	if matchRec.Code != http.StatusOK {
		t.Errorf("Expected match response 200, got %d, body: %s", matchRec.Code, matchRec.Body.String())
	}

	// Verify DB values updated
	var dbName, dbAsin, dbDesc, dbImg sql.NullString
	err = db.QueryRow("SELECT name, asin, description, imagePath FROM authors WHERE id = ?", "author_king").Scan(&dbName, &dbAsin, &dbDesc, &dbImg)
	if err != nil {
		t.Fatalf("Failed to query author db: %v", err)
	}

	if dbName.String != "Stephen King" || dbAsin.String != "B000AP9U0C" || dbDesc.String != "Horror writer" || dbImg.String != "authors/author_king.jpg" {
		t.Errorf("DB values not updated correctly: name=%q, asin=%q, desc=%q, img=%q", dbName.String, dbAsin.String, dbDesc.String, dbImg.String)
	}

	// Verify image file created
	expectedImgPath := filepath.Join(tempDir, "authors", "author_king.jpg")
	if _, err := os.Stat(expectedImgPath); os.IsNotExist(err) {
		t.Errorf("Author image file not downloaded to %s", expectedImgPath)
	}
}
