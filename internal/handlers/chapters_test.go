package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"audiobookshelf/internal/core"
)

func TestUpdateChapters(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Seed database with a book and library item
	_, err := db.Exec(`INSERT INTO books (id, title, asin, chapters) VALUES ('book-1', 'Test Book', 'B000000001', '[]')`)
	if err != nil {
		t.Fatalf("Failed to seed book: %v", err)
	}

	_, err = db.Exec(`INSERT INTO libraryItems (id, mediaId, mediaType, updatedAt) VALUES ('item-1', 'book-1', 'book', '2026-06-08 12:00:00.000')`)
	if err != nil {
		t.Fatalf("Failed to seed library item: %v", err)
	}

	userSess := &core.UserSession{
		ID:       "user-1",
		Username: "adminuser",
		Type:     "admin",
		IsActive: true,
	}

	payload := map[string]interface{}{
		"chapters": []map[string]interface{}{
			{
				"start": 0.0,
				"end":   100.5,
				"title": "Chapter 1",
			},
			{
				"start": 100.5,
				"end":   200.0,
				"title": "Chapter 2",
			},
		},
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest("POST", "/api/items/item-1/chapters", bytes.NewReader(body))
	req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, userSess))
	rr := httptest.NewRecorder()

	handler := handleUpdateChapters(db, "item-1")
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected status 200 OK, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	chaps, ok := resp["chapters"].([]interface{})
	if !ok || len(chaps) != 2 {
		t.Fatalf("Expected 2 chapters in response, got %v", resp["chapters"])
	}

	c1 := chaps[0].(map[string]interface{})
	if c1["id"].(float64) != 1 || c1["title"].(string) != "Chapter 1" {
		t.Errorf("Unexpected chapter 1: %v", c1)
	}

	// Verify database was updated
	var dbChapters string
	err = db.QueryRow("SELECT chapters FROM books WHERE id = 'book-1'").Scan(&dbChapters)
	if err != nil {
		t.Fatalf("Failed to query DB for chapters: %v", err)
	}

	var savedChaps []ChapterPayload
	if err := json.Unmarshal([]byte(dbChapters), &savedChaps); err != nil {
		t.Fatalf("Failed to unmarshal DB chapters: %v", err)
	}

	if len(savedChaps) != 2 || savedChaps[0].Title != "Chapter 1" || savedChaps[0].ID != 1 {
		t.Errorf("Unexpected saved chapters: %v", savedChaps)
	}
}

func TestLookupChapters(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Seed database with a book and library item
	_, err := db.Exec(`INSERT INTO books (id, title, asin, chapters) VALUES ('book-1', 'Test Book', 'B001234567', '[]')`)
	if err != nil {
		t.Fatalf("Failed to seed book: %v", err)
	}

	_, err = db.Exec(`INSERT INTO libraryItems (id, mediaId, mediaType, updatedAt) VALUES ('item-1', 'book-1', 'book', '2026-06-08 12:00:00.000')`)
	if err != nil {
		t.Fatalf("Failed to seed library item: %v", err)
	}

	userSess := &core.UserSession{
		ID:       "user-1",
		Username: "adminuser",
		Type:     "admin",
		IsActive: true,
	}

	// Spin up mock Audnexus server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/books/B001234567/chapters" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"chapters": [
				{
					"title": "Audnexus Chapter 1",
					"start": 0.0,
					"duration": 500.0
				},
				{
					"title": "Audnexus Chapter 2",
					"start": 500.0,
					"end": 1000.0
				}
			]
		}`))
	}))
	defer mockServer.Close()

	// Temporarily override AudnexusBaseURL
	oldURL := AudnexusBaseURL
	AudnexusBaseURL = mockServer.URL
	defer func() { AudnexusBaseURL = oldURL }()

	req := httptest.NewRequest("POST", "/api/items/item-1/chapters/lookup", nil)
	req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, userSess))
	rr := httptest.NewRecorder()

	handler := handleLookupChapters(db, "item-1")
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected status 200 OK, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	chaps, ok := resp["chapters"].([]interface{})
	if !ok || len(chaps) != 2 {
		t.Fatalf("Expected 2 chapters, got %v", resp["chapters"])
	}

	c1 := chaps[0].(map[string]interface{})
	if c1["title"].(string) != "Audnexus Chapter 1" || c1["end"].(float64) != 500.0 {
		t.Errorf("Unexpected chapter 1: %v", c1)
	}

	c2 := chaps[1].(map[string]interface{})
	if c2["title"].(string) != "Audnexus Chapter 2" || c2["end"].(float64) != 1000.0 {
		t.Errorf("Unexpected chapter 2: %v", c2)
	}
}

func TestHandleLookupChapters_InvalidASIN(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Seed database with a book having an invalid ASIN (with trailing injection characters or directory traversal)
	_, err := db.Exec(`INSERT INTO books (id, title, asin, chapters) VALUES ('book-invalid', 'Test Book', '../traversal', '[]')`)
	if err != nil {
		t.Fatalf("Failed to seed book: %v", err)
	}

	_, err = db.Exec(`INSERT INTO libraryItems (id, mediaId, mediaType, updatedAt) VALUES ('item-invalid', 'book-invalid', 'book', '2026-06-08 12:00:00.000')`)
	if err != nil {
		t.Fatalf("Failed to seed library item: %v", err)
	}

	userSess := &core.UserSession{
		ID:       "user-1",
		Username: "adminuser",
		Type:     "admin",
		IsActive: true,
	}

	req := httptest.NewRequest("POST", "/api/items/item-invalid/chapters/lookup", nil)
	req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, userSess))
	rr := httptest.NewRecorder()

	handler := handleLookupChapters(db, "item-invalid")
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("Expected status 400 Bad Request, got %d: %s", rr.Code, rr.Body.String())
	}
}
