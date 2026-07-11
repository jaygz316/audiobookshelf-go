package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	_ "modernc.org/sqlite"
)

func setupNarratorsTestDB(t *testing.T) *sql.DB {
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("Failed to open shared memory db: %v", err)
	}

	db.SetMaxOpenConns(1)

	queries := []string{
		`CREATE TABLE IF NOT EXISTS libraries (id TEXT PRIMARY KEY, name TEXT, displayOrder INTEGER, icon TEXT, mediaType TEXT, provider TEXT, settings TEXT, createdAt TEXT, updatedAt TEXT)`,
		`CREATE TABLE IF NOT EXISTS libraryItems (id TEXT PRIMARY KEY, ino TEXT, libraryId TEXT, path TEXT, relPath TEXT, isFile INTEGER, mtime TEXT, ctime TEXT, birthtime TEXT, createdAt TEXT, updatedAt TEXT, isMissing INTEGER, isInvalid INTEGER, mediaType TEXT, mediaId TEXT, size INTEGER, libraryFolderId TEXT, authorNamesFirstLast TEXT, authorNamesLastFirst TEXT, title TEXT, titleIgnorePrefix TEXT)`,
		`CREATE TABLE IF NOT EXISTS books (id TEXT PRIMARY KEY, title TEXT, titleIgnorePrefix TEXT, subtitle TEXT, publishedYear TEXT, publishedDate TEXT, publisher TEXT, description TEXT, isbn TEXT, asin TEXT, language TEXT, explicit INTEGER, abridged INTEGER, coverPath TEXT, duration REAL, narrators BLOB, audioFiles BLOB, ebookFile BLOB, chapters BLOB, tags BLOB, genres BLOB, lockedFields BLOB)`,
	}

	for _, q := range queries {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("Failed to execute query %q: %v", q, err)
		}
	}

	return db
}

func TestGetLibraryNarrators(t *testing.T) {
	db := setupNarratorsTestDB(t)
	defer db.Close()

	// Insert library
	_, err := db.Exec(`INSERT INTO libraries (id, name, mediaType) VALUES ('lib1', 'Audiobooks', 'book')`)
	if err != nil {
		t.Fatalf("Failed to insert library: %v", err)
	}

	// Insert books with narrators
	// Book 1: Frank Muller, Stephen King
	// Book 2: Frank Muller
	// Book 3: George Guidall
	booksData := []struct {
		id        string
		title     string
		narrators string
	}{
		{"book1", "The Dark Tower", `["Frank Muller", "Stephen King"]`},
		{"book2", "The Green Mile", `["Frank Muller"]`},
		{"book3", "American Gods", `["George Guidall"]`},
	}

	for _, b := range booksData {
		_, err = db.Exec(`INSERT INTO books (id, title, narrators) VALUES (?, ?, ?)`, b.id, b.title, b.narrators)
		if err != nil {
			t.Fatalf("Failed to insert book: %v", err)
		}
		// Associate with libraryItems
		_, err = db.Exec(`INSERT INTO libraryItems (id, libraryId, mediaType, mediaId, isMissing, isInvalid) VALUES (?, 'lib1', 'book', ?, 0, 0)`, "item_"+b.id, b.id)
		if err != nil {
			t.Fatalf("Failed to insert library item: %v", err)
		}
	}

	handler := handleGetLibraryNarrators(db, "lib1")

	// Test case 1: Retrieve all, default sort (name ascending)
	t.Run("Default Sort", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/libraries/lib1/narrators", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("Expected 200 OK, got %d", w.Code)
		}

		var resp map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to parse JSON response: %v", err)
		}

		results := resp["results"].([]interface{})
		if len(results) != 3 {
			t.Fatalf("Expected 3 results, got %d", len(results))
		}

		// Alphabetical: Frank Muller -> George Guidall -> Stephen King
		if results[0].(map[string]interface{})["name"] != "Frank Muller" || results[0].(map[string]interface{})["numBooks"].(float64) != 2 {
			t.Errorf("Unexpected first element: %v", results[0])
		}
		if results[1].(map[string]interface{})["name"] != "George Guidall" || results[1].(map[string]interface{})["numBooks"].(float64) != 1 {
			t.Errorf("Unexpected second element: %v", results[1])
		}
		if results[2].(map[string]interface{})["name"] != "Stephen King" || results[2].(map[string]interface{})["numBooks"].(float64) != 1 {
			t.Errorf("Unexpected third element: %v", results[2])
		}
	})

	// Test case 2: Sort by numBooks descending
	t.Run("Sort by Book Count Descending", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/libraries/lib1/narrators?sort=numBooks&desc=true", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("Expected 200 OK, got %d", w.Code)
		}

		var resp map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to parse JSON response: %v", err)
		}

		results := resp["results"].([]interface{})
		if len(results) != 3 {
			t.Fatalf("Expected 3 results, got %d", len(results))
		}

		// Counts descending: Frank Muller (2) -> Stephen King (1) -> George Guidall (1) (or vice versa depending on secondary sort, since desc also flips secondary name sort)
		// Wait: our sorting function says:
		// less = count_i < count_j. If counts equal: less = name_i < name_j.
		// If desc, return !less.
		// So if count equal (1 vs 1), less(George, Stephen) is true. !less is false.
		// So Stephen King (1) comes before George Guidall (1).
		if results[0].(map[string]interface{})["name"] != "Frank Muller" {
			t.Errorf("Expected Frank Muller first, got %v", results[0])
		}
		if results[1].(map[string]interface{})["name"] != "Stephen King" {
			t.Errorf("Expected Stephen King second, got %v", results[1])
		}
	})

	// Test case 3: Search filter
	t.Run("Search Filter", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/libraries/lib1/narrators?search=george", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("Expected 200 OK, got %d", w.Code)
		}

		var resp map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to parse JSON response: %v", err)
		}

		results := resp["results"].([]interface{})
		if len(results) != 1 {
			t.Fatalf("Expected 1 result, got %d", len(results))
		}
		if results[0].(map[string]interface{})["name"] != "George Guidall" {
			t.Errorf("Expected George Guidall, got %v", results[0])
		}
	})

	// Test case 4: Pagination
	t.Run("Pagination", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/libraries/lib1/narrators?limit=2&page=0", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("Expected 200 OK, got %d", w.Code)
		}

		var resp map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to parse JSON response: %v", err)
		}

		results := resp["results"].([]interface{})
		if len(results) != 2 {
			t.Fatalf("Expected 2 results on page 0, got %d", len(results))
		}
		if resp["total"].(float64) != 3 {
			t.Errorf("Expected total 3, got %v", resp["total"])
		}
	})
}
