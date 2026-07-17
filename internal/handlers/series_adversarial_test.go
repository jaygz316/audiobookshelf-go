package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"audiobookshelf/internal/core"
)

func TestAdversarialSeries_GetLibrarySeries_Sorting(t *testing.T) {
	db := setupTestDBShared(t)
	defer db.Close()

	// Insert library
	_, _ = db.Exec(`INSERT INTO libraries (id, name, displayOrder, icon, mediaType, provider, settings, createdAt, updatedAt) 
		VALUES ('lib1', 'Audiobooks', 1, 'book', 'book', 'local', '{}', '2026-06-08 12:00:00.000', '2026-06-08 12:00:00.000')`)

	// Insert series
	// seriesA: added first
	_, _ = db.Exec(`INSERT INTO series (id, libraryId, name, nameIgnorePrefix, description, createdAt, updatedAt) 
		VALUES ('seriesA', 'lib1', 'Series A', 'Series A', 'Desc A', '2026-06-08 12:00:00.000', '2026-06-08 12:00:00.000')`)
	// seriesB: added second
	_, _ = db.Exec(`INSERT INTO series (id, libraryId, name, nameIgnorePrefix, description, createdAt, updatedAt) 
		VALUES ('seriesB', 'lib1', 'Series B', 'Series B', 'Desc B', '2026-06-08 13:00:00.000', '2026-06-08 13:00:00.000')`)
	// seriesC: added third
	_, _ = db.Exec(`INSERT INTO series (id, libraryId, name, nameIgnorePrefix, description, createdAt, updatedAt) 
		VALUES ('seriesC', 'lib1', 'Series C', 'Series C', 'Desc C', '2026-06-08 14:00:00.000', '2026-06-08 14:00:00.000')`)

	// Books
	// bookA1, bookA2 -> linked to seriesA. Total duration: 200
	_, _ = db.Exec(`INSERT INTO books (id, title, duration) VALUES ('bookA1', 'Book A1', 100.0)`)
	_, _ = db.Exec(`INSERT INTO books (id, title, duration) VALUES ('bookA2', 'Book A2', 100.0)`)
	_, _ = db.Exec(`INSERT INTO libraryItems (id, libraryId, createdAt, updatedAt, mediaType, mediaId) VALUES ('itemA1', 'lib1', '2026-06-08 12:05:00.000', '2026-06-08 12:05:00.000', 'book', 'bookA1')`)
	_, _ = db.Exec(`INSERT INTO libraryItems (id, libraryId, createdAt, updatedAt, mediaType, mediaId) VALUES ('itemA2', 'lib1', '2026-06-08 12:10:00.000', '2026-06-08 12:10:00.000', 'book', 'bookA2')`)
	_, _ = db.Exec(`INSERT INTO bookSeries (bookId, seriesId, sequence) VALUES ('bookA1', 'seriesA', '1')`)
	_, _ = db.Exec(`INSERT INTO bookSeries (bookId, seriesId, sequence) VALUES ('bookA2', 'seriesA', '2')`)

	// bookB1 -> linked to seriesB. Total duration: 300
	_, _ = db.Exec(`INSERT INTO books (id, title, duration) VALUES ('bookB1', 'Book B1', 300.0)`)
	_, _ = db.Exec(`INSERT INTO libraryItems (id, libraryId, createdAt, updatedAt, mediaType, mediaId) VALUES ('itemB1', 'lib1', '2026-06-08 13:05:00.000', '2026-06-08 13:05:00.000', 'book', 'bookB1')`)
	_, _ = db.Exec(`INSERT INTO bookSeries (bookId, seriesId, sequence) VALUES ('bookB1', 'seriesB', '1')`)

	// bookC1, bookC2, bookC3 -> linked to seriesC. Total duration: 150
	_, _ = db.Exec(`INSERT INTO books (id, title, duration) VALUES ('bookC1', 'Book C1', 50.0)`)
	_, _ = db.Exec(`INSERT INTO books (id, title, duration) VALUES ('bookC2', 'Book C2', 50.0)`)
	_, _ = db.Exec(`INSERT INTO books (id, title, duration) VALUES ('bookC3', 'Book C3', 50.0)`)
	_, _ = db.Exec(`INSERT INTO libraryItems (id, libraryId, createdAt, updatedAt, mediaType, mediaId) VALUES ('itemC1', 'lib1', '2026-06-08 14:05:00.000', '2026-06-08 14:05:00.000', 'book', 'bookC1')`)
	_, _ = db.Exec(`INSERT INTO libraryItems (id, libraryId, createdAt, updatedAt, mediaType, mediaId) VALUES ('itemC2', 'lib1', '2026-06-08 14:10:00.000', '2026-06-08 14:10:00.000', 'book', 'bookC2')`)
	_, _ = db.Exec(`INSERT INTO libraryItems (id, libraryId, createdAt, updatedAt, mediaType, mediaId) VALUES ('itemC3', 'lib1', '2026-06-08 14:15:00.000', '2026-06-08 14:15:00.000', 'book', 'bookC3')`)
	_, _ = db.Exec(`INSERT INTO bookSeries (bookId, seriesId, sequence) VALUES ('bookC1', 'seriesC', '1')`)
	_, _ = db.Exec(`INSERT INTO bookSeries (bookId, seriesId, sequence) VALUES ('bookC2', 'seriesC', '2')`)
	_, _ = db.Exec(`INSERT INTO bookSeries (bookId, seriesId, sequence) VALUES ('bookC3', 'seriesC', '3')`)

	user := &core.UserSession{
		ID:                 "user1",
		Username:           "admin",
		Type:               "admin",
		IsActive:           true,
		AccessAllLibraries: true,
	}

	tests := []struct {
		name          string
		sortBy        string
		desc          string
		expectedOrder []string
	}{
		{
			name:          "Sort by numBooks asc",
			sortBy:        "numBooks",
			desc:          "false",
			expectedOrder: []string{"seriesB", "seriesA", "seriesC"}, // 1, 2, 3 books
		},
		{
			name:          "Sort by numBooks desc",
			sortBy:        "numBooks",
			desc:          "true",
			expectedOrder: []string{"seriesC", "seriesA", "seriesB"}, // 3, 2, 1 books
		},
		{
			name:          "Sort by totalDuration asc",
			sortBy:        "totalDuration",
			desc:          "false",
			expectedOrder: []string{"seriesC", "seriesA", "seriesB"}, // 150, 200, 300 duration
		},
		{
			name:          "Sort by addedAt asc",
			sortBy:        "addedAt",
			desc:          "false",
			expectedOrder: []string{"seriesA", "seriesB", "seriesC"}, // chronological createdAt
		},
		{
			name:          "Sort by addedAt desc",
			sortBy:        "addedAt",
			desc:          "true",
			expectedOrder: []string{"seriesC", "seriesB", "seriesA"},
		},
		{
			name:          "Sort by lastBookAdded desc",
			sortBy:        "lastBookAdded",
			desc:          "true",
			expectedOrder: []string{"seriesC", "seriesB", "seriesA"}, // last book added: C (14:15), B (13:05), A (12:10)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := handleGetLibrarySeries(db, "lib1")
			req := httptest.NewRequest("GET", "/api/libraries/lib1/series?sort="+tt.sortBy+"&desc="+tt.desc, nil)
			req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, user))

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Fatalf("Expected 200, got %d", rr.Code)
			}

			var resp map[string]interface{}
			_ = json.Unmarshal(rr.Body.Bytes(), &resp)
			results := resp["results"].([]interface{})

			if len(results) != len(tt.expectedOrder) {
				t.Fatalf("Expected %d results, got %d", len(tt.expectedOrder), len(results))
			}

			for i, expectedID := range tt.expectedOrder {
				item := results[i].(map[string]interface{})
				if item["id"].(string) != expectedID {
					t.Errorf("At index %d: expected %s, got %s", i, expectedID, item["id"])
				}
			}
		})
	}
}

func TestAdversarialSeries_GetLibrarySeries_PaginationAndFiltering(t *testing.T) {
	db := setupTestDBShared(t)
	defer db.Close()

	// Insert library & series
	_, _ = db.Exec(`INSERT INTO libraries (id, name, mediaType) VALUES ('lib1', 'Audiobooks', 'book')`)
	_, _ = db.Exec(`INSERT INTO series (id, libraryId, name, createdAt, updatedAt) VALUES ('s1', 'lib1', 'Harry Potter', '2026-06-08', '2026-06-08')`)
	_, _ = db.Exec(`INSERT INTO series (id, libraryId, name, createdAt, updatedAt) VALUES ('s2', 'lib1', 'The Hobbit', '2026-06-08', '2026-06-08')`)
	_, _ = db.Exec(`INSERT INTO series (id, libraryId, name, createdAt, updatedAt) VALUES ('s3', 'lib1', 'The Lord of the Rings', '2026-06-08', '2026-06-08')`)

	user := &core.UserSession{
		ID:                 "user1",
		Username:           "admin",
		Type:               "admin",
		IsActive:           true,
		AccessAllLibraries: true,
	}

	handler := handleGetLibrarySeries(db, "lib1")

	// 1. Search Filter Substring Case-Insensitive
	req := httptest.NewRequest("GET", "/api/libraries/lib1/series?filter=the", nil)
	req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, user))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	var resp map[string]interface{}
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	results := resp["results"].([]interface{})
	if len(results) != 2 {
		t.Errorf("Expected 2 filtered series for 'the', got %d", len(results))
	}

	// 2. Pagination Limit and Page
	req = httptest.NewRequest("GET", "/api/libraries/lib1/series?limit=2&page=0", nil)
	req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, user))
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	results = resp["results"].([]interface{})
	if len(results) != 2 {
		t.Errorf("Expected 2 items on page 0 with limit 2, got %d", len(results))
	}
	if int(resp["total"].(float64)) != 3 {
		t.Errorf("Expected total 3, got %v", resp["total"])
	}

	req = httptest.NewRequest("GET", "/api/libraries/lib1/series?limit=2&page=1", nil)
	req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, user))
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	results = resp["results"].([]interface{})
	if len(results) != 1 {
		t.Errorf("Expected 1 item on page 1 with limit 2, got %d", len(results))
	}
}

func TestAdversarialSeries_GetLibrarySeriesByID_EdgeCases(t *testing.T) {
	db := setupTestDBShared(t)
	defer db.Close()

	_, _ = db.Exec(`INSERT INTO libraries (id, name, mediaType) VALUES ('lib1', 'Audiobooks', 'book')`)
	_, _ = db.Exec(`INSERT INTO series (id, libraryId, name, nameIgnorePrefix, description, createdAt, updatedAt) 
		VALUES ('series1', 'lib1', 'Series 1', 'Series 1', 'Desc', '2026-06-08', '2026-06-08')`)

	user := &core.UserSession{
		ID:                 "user1",
		Username:           "admin",
		Type:               "admin",
		IsActive:           true,
		AccessAllLibraries: true,
	}

	// 1. Not found case
	handlerNotFound := handleGetLibrarySeriesByID(db, "lib1", "nonexistent")
	reqNotFound := httptest.NewRequest("GET", "/api/libraries/lib1/series/nonexistent", nil)
	reqNotFound = reqNotFound.WithContext(context.WithValue(reqNotFound.Context(), core.UserContextKey, user))
	rrNotFound := httptest.NewRecorder()
	handlerNotFound.ServeHTTP(rrNotFound, reqNotFound)
	if rrNotFound.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", rrNotFound.Code)
	}

	// 2. User Progress Matching edge cases
	_, _ = db.Exec(`INSERT INTO books (id, title, duration) VALUES ('b1', 'Book 1', 100.0)`)
	_, _ = db.Exec(`INSERT INTO books (id, title, duration) VALUES ('b2', 'Book 2', 150.0)`)
	_, _ = db.Exec(`INSERT INTO libraryItems (id, libraryId, mediaType, mediaId) VALUES ('li1', 'lib1', 'book', 'b1')`)
	_, _ = db.Exec(`INSERT INTO libraryItems (id, libraryId, mediaType, mediaId) VALUES ('li2', 'lib1', 'book', 'b2')`)
	_, _ = db.Exec(`INSERT INTO bookSeries (bookId, seriesId, sequence) VALUES ('b1', 'series1', '1')`)
	_, _ = db.Exec(`INSERT INTO bookSeries (bookId, seriesId, sequence) VALUES ('b2', 'series1', '2')`)

	// Initially, no progress entries. Progress.isFinished should be false.
	handler := handleGetLibrarySeriesByID(db, "lib1", "series1")
	req := httptest.NewRequest("GET", "/api/libraries/lib1/series/series1", nil)
	req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, user))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	var resp map[string]interface{}
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	progress := resp["progress"].(map[string]interface{})
	if progress["isFinished"].(bool) != false {
		t.Errorf("Expected progress.isFinished to be false, got true")
	}

	// Insert progress for b1 finished, b2 not finished (isFinished = 0)
	_, _ = db.Exec(`INSERT INTO mediaProgresses (id, userId, mediaItemId, isFinished, currentTime, updatedAt) 
		VALUES ('p1', 'user1', 'b1', 1, 100.0, '2026-06-08')`)
	_, _ = db.Exec(`INSERT INTO mediaProgresses (id, userId, mediaItemId, isFinished, currentTime, updatedAt) 
		VALUES ('p2', 'user1', 'b2', 0, 50.0, '2026-06-08')`)

	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	progress = resp["progress"].(map[string]interface{})
	if progress["isFinished"].(bool) != false {
		t.Errorf("Expected progress.isFinished to be false when only one book finished")
	}

	// Update b2 to be finished
	_, _ = db.Exec(`UPDATE mediaProgresses SET isFinished = 1, currentTime = 150.0 WHERE id = 'p2'`)
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	progress = resp["progress"].(map[string]interface{})
	if progress["isFinished"].(bool) != true {
		t.Errorf("Expected progress.isFinished to be true when all books finished")
	}
}

func TestAdversarialSeries_UpdateSeries_ForbiddenAndError(t *testing.T) {
	db := setupTestDBShared(t)
	defer db.Close()

	_, _ = db.Exec(`INSERT INTO series (id, name, libraryId) VALUES ('series1', 'Name', 'lib1')`)

	handler := handleUpdateSeries(db, "series1")

	// 1. Missing user context (Unauthorized)
	reqUnauthorized := httptest.NewRequest("PATCH", "/api/series/series1", strings.NewReader(`{}`))
	rrUnauthorized := httptest.NewRecorder()
	handler.ServeHTTP(rrUnauthorized, reqUnauthorized)
	if rrUnauthorized.Code != http.StatusUnauthorized {
		t.Errorf("Expected 401, got %d", rrUnauthorized.Code)
	}

	// 2. Non-admin / non-root user (Forbidden)
	userForbidden := &core.UserSession{
		ID:       "user2",
		Username: "guest",
		Type:     "user",
		IsActive: true,
	}
	reqForbidden := httptest.NewRequest("PATCH", "/api/series/series1", strings.NewReader(`{}`))
	reqForbidden = reqForbidden.WithContext(context.WithValue(reqForbidden.Context(), core.UserContextKey, userForbidden))
	rrForbidden := httptest.NewRecorder()
	handler.ServeHTTP(rrForbidden, reqForbidden)
	if rrForbidden.Code != http.StatusForbidden {
		t.Errorf("Expected 403, got %d", rrForbidden.Code)
	}

	// 3. Malformed JSON request body
	userAdmin := &core.UserSession{
		ID:       "user1",
		Username: "admin",
		Type:     "admin",
		IsActive: true,
	}
	reqMalformed := httptest.NewRequest("PATCH", "/api/series/series1", strings.NewReader(`{invalidjson`))
	reqMalformed = reqMalformed.WithContext(context.WithValue(reqMalformed.Context(), core.UserContextKey, userAdmin))
	rrMalformed := httptest.NewRecorder()
	handler.ServeHTTP(rrMalformed, reqMalformed)
	if rrMalformed.Code != http.StatusBadRequest {
		t.Errorf("Expected 400, got %d", rrMalformed.Code)
	}
}

func TestAdversarialSeries_AutoNumber_EdgeCases(t *testing.T) {
	db := setupTestDBShared(t)
	defer db.Close()

	// 1. Insert series
	_, _ = db.Exec(`INSERT INTO series (id, name, libraryId) VALUES ('series1', 'Test Series', 'lib1')`)

	// 2. Insert books with varying publishedYear/publishedDate
	// Book A: publishedYear 2021
	_, _ = db.Exec(`INSERT INTO books (id, title, publishedYear) VALUES ('bookA', 'Book A', '2021')`)
	// Book B: no publishedYear, but publishedDate "2020-05-01"
	_, _ = db.Exec(`INSERT INTO books (id, title, publishedDate) VALUES ('bookB', 'Book B', '2020-05-01')`)
	// Book C: no pub info, title "Book C"
	_, _ = db.Exec(`INSERT INTO books (id, title) VALUES ('bookC', 'Book C')`)
	// Book D: no pub info, title "Book C (Narrated by Narrator)"
	_, _ = db.Exec(`INSERT INTO books (id, title) VALUES ('bookD', 'Book C (Narrated by Narrator)')`)

	// Link all to series with dummy sequence
	_, _ = db.Exec(`INSERT INTO bookSeries (bookId, seriesId, sequence) VALUES ('bookA', 'series1', '99')`)
	_, _ = db.Exec(`INSERT INTO bookSeries (bookId, seriesId, sequence) VALUES ('bookB', 'series1', '98')`)
	_, _ = db.Exec(`INSERT INTO bookSeries (bookId, seriesId, sequence) VALUES ('bookC', 'series1', '97')`)
	_, _ = db.Exec(`INSERT INTO bookSeries (bookId, seriesId, sequence) VALUES ('bookD', 'series1', '96')`)

	user := &core.UserSession{
		ID:                 "user1",
		Username:           "admin",
		Type:               "admin",
		IsActive:           true,
		AccessAllLibraries: true,
	}

	handler := handleAutoNumberSeries(db, "series1")
	req := httptest.NewRequest("POST", "/api/series/series1/auto-number", nil)
	req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, user))

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d. Body: %s", rr.Code, rr.Body.String())
	}

	// Verify chronological and alphabetical/normalization ordering:
	// - bookB (2020 date) -> 1
	// - bookA (2021 year) -> 2
	// - bookC & bookD (normalized to "book c") -> 3
	var seqA, seqB, seqC, seqD string
	_ = db.QueryRow("SELECT sequence FROM bookSeries WHERE bookId = 'bookA'").Scan(&seqA)
	_ = db.QueryRow("SELECT sequence FROM bookSeries WHERE bookId = 'bookB'").Scan(&seqB)
	_ = db.QueryRow("SELECT sequence FROM bookSeries WHERE bookId = 'bookC'").Scan(&seqC)
	_ = db.QueryRow("SELECT sequence FROM bookSeries WHERE bookId = 'bookD'").Scan(&seqD)

	if seqA != "1" {
		t.Errorf("Expected bookA sequence to be '1', got %q", seqA)
	}
	if seqB != "2" {
		t.Errorf("Expected bookB sequence to be '2', got %q", seqB)
	}
	if seqC != "3" {
		t.Errorf("Expected bookC sequence to be '3', got %q", seqC)
	}
	if seqD != "3" {
		t.Errorf("Expected bookD sequence to be '3' due to normalization, got %q", seqD)
	}
}
