package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func setupTestDBShared(t *testing.T) *sql.DB {
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("Failed to open shared memory db: %v", err)
	}

	db.SetMaxOpenConns(10)

	queries := []string{
		`CREATE TABLE IF NOT EXISTS settings (key TEXT PRIMARY KEY, value TEXT)`,
		`CREATE TABLE IF NOT EXISTS users (id TEXT PRIMARY KEY, username TEXT, type TEXT, isActive INTEGER, permissions TEXT, extraData TEXT)`,
		`CREATE TABLE IF NOT EXISTS apiKeys (id TEXT PRIMARY KEY, isActive INTEGER, expiresAt TEXT, userId TEXT)`,
		`CREATE TABLE IF NOT EXISTS libraries (id TEXT PRIMARY KEY, name TEXT, displayOrder INTEGER, icon TEXT, mediaType TEXT, provider TEXT, lastScan TEXT, lastScanVersion TEXT, settings TEXT, createdAt TEXT, updatedAt TEXT)`,
		`CREATE TABLE IF NOT EXISTS libraryFolders (id TEXT PRIMARY KEY, path TEXT, libraryId TEXT, createdAt TEXT, updatedAt TEXT)`,
		`CREATE TABLE IF NOT EXISTS libraryItems (id TEXT PRIMARY KEY, ino TEXT, libraryId TEXT, path TEXT, relPath TEXT, isFile INTEGER, mtime TEXT, ctime TEXT, birthtime TEXT, createdAt TEXT, updatedAt TEXT, isMissing INTEGER, isInvalid INTEGER, mediaType TEXT, mediaId TEXT, size INTEGER, libraryFolderId TEXT, authorNamesFirstLast TEXT, authorNamesLastFirst TEXT, title TEXT, titleIgnorePrefix TEXT)`,
		`CREATE TABLE IF NOT EXISTS books (id TEXT PRIMARY KEY, title TEXT, titleIgnorePrefix TEXT, subtitle TEXT, publishedYear TEXT, publishedDate TEXT, publisher TEXT, description TEXT, isbn TEXT, asin TEXT, language TEXT, explicit INTEGER, abridged INTEGER, coverPath TEXT, duration REAL, narrators BLOB, audioFiles BLOB, ebookFile BLOB, chapters BLOB, tags BLOB, genres BLOB)`,
		`CREATE TABLE IF NOT EXISTS bookSeries (bookId TEXT, seriesId TEXT, sequence TEXT)`,
		`CREATE TABLE IF NOT EXISTS series (id TEXT PRIMARY KEY, libraryId TEXT, name TEXT, nameIgnorePrefix TEXT, description TEXT, createdAt TEXT, updatedAt TEXT)`,
		`CREATE TABLE IF NOT EXISTS mediaProgresses (id TEXT PRIMARY KEY, userId TEXT, mediaItemId TEXT, isFinished INTEGER, currentTime REAL, updatedAt TEXT)`,
		`CREATE TABLE IF NOT EXISTS authors (id TEXT PRIMARY KEY, libraryId TEXT, name TEXT, lastFirst TEXT, asin TEXT, description TEXT, imagePath TEXT, createdAt TEXT, updatedAt TEXT)`,
		`CREATE TABLE IF NOT EXISTS bookAuthors (bookId TEXT, authorId TEXT)`,
	}

	for _, q := range queries {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("Failed to execute query %q: %v", q, err)
		}
	}

	_, err = db.Exec(`INSERT INTO settings (key, value) VALUES ('server-settings', '{"sortingIgnorePrefix": true}')`)
	if err != nil {
		t.Fatalf("Failed to insert settings: %v", err)
	}

	return db
}

func TestGetLibraryAuthors(t *testing.T) {
	db := setupTestDBShared(t)
	defer db.Close()

	// Insert test data
	_, err := db.Exec(`INSERT INTO libraries (id, name, displayOrder, icon, mediaType, provider, settings, createdAt, updatedAt) VALUES ('lib1', 'Audiobooks', 1, 'book', 'book', 'local', '{}', '2026-06-08 12:00:00.000', '2026-06-08 12:00:00.000')`)
	if err != nil {
		t.Fatalf("Failed to insert library: %v", err)
	}

	_, err = db.Exec(`INSERT INTO authors (id, libraryId, name, lastFirst, asin, description, imagePath, createdAt, updatedAt) VALUES 
		('author1', 'lib1', 'Stephen King', 'King, Stephen', 'asin1', 'Horror writer', 'covers/stephen_king.jpg', '2026-06-08 12:00:00.000', '2026-06-08 12:00:00.000'),
		('author2', 'lib1', 'J.K. Rowling', 'Rowling, J.K.', 'asin2', 'Fantasy writer', '', '2026-06-08 12:00:00.000', '2026-06-08 12:00:00.000')`)
	if err != nil {
		t.Fatalf("Failed to insert authors: %v", err)
	}

	// Associate author1 with a book
	_, err = db.Exec(`INSERT INTO books (id, title, duration, narrators, audioFiles, ebookFile, chapters, tags, genres) VALUES ('book1', 'The Shining', 3600, '[]', '[]', 'null', '[]', '[]', '[]')`)
	if err != nil {
		t.Fatalf("Failed to insert book: %v", err)
	}
	_, err = db.Exec(`INSERT INTO libraryItems (id, ino, libraryId, path, relPath, isFile, createdAt, updatedAt, mediaType, mediaId) VALUES ('item1', '1', 'lib1', '/path1', 'rel1', 1, '2026-06-08 12:00:00.000', '2026-06-08 12:00:00.000', 'book', 'book1')`)
	if err != nil {
		t.Fatalf("Failed to insert libraryItem: %v", err)
	}
	_, err = db.Exec(`INSERT INTO bookAuthors (bookId, authorId) VALUES ('book1', 'author1')`)
	if err != nil {
		t.Fatalf("Failed to insert bookAuthor: %v", err)
	}

	handler := handleGetLibraryAuthors(db, "lib1")
	req := httptest.NewRequest("GET", "/api/libraries/lib1/authors?sort=name", nil)

	user := &UserSession{
		ID:                 "user1",
		Username:           "admin",
		Type:               "admin",
		IsActive:           true,
		AccessAllLibraries: true,
	}
	req = req.WithContext(context.WithValue(req.Context(), UserContextKey, user))

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d. Body: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse JSON response: %v", err)
	}

	results, ok := resp["results"].([]interface{})
	if !ok || len(results) != 2 {
		t.Fatalf("Expected 2 authors in results, got: %v", resp)
	}

	// Verify rowling is first alphabetically, king is second
	first := results[0].(map[string]interface{})
	if first["name"] != "J.K. Rowling" {
		t.Errorf("Expected J.K. Rowling first alphabetically, got %v", first["name"])
	}

	second := results[1].(map[string]interface{})
	if second["name"] != "Stephen King" {
		t.Errorf("Expected Stephen King second alphabetically, got %v", second["name"])
	}
	if numBooks, ok := second["numBooks"].(float64); !ok || numBooks != 1 {
		t.Errorf("Expected Stephen King to have 1 book, got %v", second["numBooks"])
	}
}

func TestGetLibrarySeries(t *testing.T) {
	db := setupTestDBShared(t)
	defer db.Close()

	// Insert test data
	_, err := db.Exec(`INSERT INTO libraries (id, name, displayOrder, icon, mediaType, provider, settings, createdAt, updatedAt) VALUES ('lib1', 'Audiobooks', 1, 'book', 'book', 'local', '{}', '2026-06-08 12:00:00.000', '2026-06-08 12:00:00.000')`)
	if err != nil {
		t.Fatalf("Failed to insert library: %v", err)
	}

	_, err = db.Exec(`INSERT INTO series (id, libraryId, name, nameIgnorePrefix, description, createdAt, updatedAt) VALUES 
		('series1', 'lib1', 'Harry Potter', 'Harry Potter', 'Boy wizard', '2026-06-08 12:00:00.000', '2026-06-08 12:00:00.000')`)
	if err != nil {
		t.Fatalf("Failed to insert series: %v", err)
	}

	_, err = db.Exec(`INSERT INTO books (id, title, duration, coverPath, narrators, audioFiles, ebookFile, chapters, tags, genres) VALUES 
		('book1', 'Harry Potter 1', 7200, 'covers/hp1.jpg', '[]', '[]', 'null', '[]', '[]', '[]')`)
	if err != nil {
		t.Fatalf("Failed to insert book: %v", err)
	}

	_, err = db.Exec(`INSERT INTO libraryItems (id, ino, libraryId, path, relPath, isFile, createdAt, updatedAt, mediaType, mediaId) VALUES 
		('item1', '1', 'lib1', '/path1', 'rel1', 1, '2026-06-08 12:00:00.000', '2026-06-08 12:00:00.000', 'book', 'book1')`)
	if err != nil {
		t.Fatalf("Failed to insert libraryItem: %v", err)
	}

	_, err = db.Exec(`INSERT INTO bookSeries (bookId, seriesId, sequence) VALUES ('book1', 'series1', '1')`)
	if err != nil {
		t.Fatalf("Failed to insert bookSeries: %v", err)
	}

	handler := handleGetLibrarySeries(db, "lib1")
	req := httptest.NewRequest("GET", "/api/libraries/lib1/series", nil)

	user := &UserSession{
		ID:                 "user1",
		Username:           "admin",
		Type:               "admin",
		IsActive:           true,
		AccessAllLibraries: true,
	}
	req = req.WithContext(context.WithValue(req.Context(), UserContextKey, user))

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d. Body: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse JSON response: %v", err)
	}

	results, ok := resp["results"].([]interface{})
	if !ok || len(results) != 1 {
		t.Fatalf("Expected 1 series in results, got: %v", resp)
	}

	series := results[0].(map[string]interface{})
	if series["name"] != "Harry Potter" {
		t.Errorf("Expected series Harry Potter, got %v", series["name"])
	}

	books := series["books"].([]interface{})
	if len(books) != 1 {
		t.Fatalf("Expected 1 book inside series, got: %v", books)
	}

	book := books[0].(map[string]interface{})
	if book["id"] != "item1" || book["sequence"] != "1" {
		t.Errorf("Unexpected book in series: %v", book)
	}
}

func TestGetLibrarySeriesByID(t *testing.T) {
	db := setupTestDBShared(t)
	defer db.Close()

	// Insert test data
	_, err := db.Exec(`INSERT INTO libraries (id, name, displayOrder, icon, mediaType, provider, settings, createdAt, updatedAt) VALUES ('lib1', 'Audiobooks', 1, 'book', 'book', 'local', '{}', '2026-06-08 12:00:00.000', '2026-06-08 12:00:00.000')`)
	if err != nil {
		t.Fatalf("Failed to insert library: %v", err)
	}

	_, err = db.Exec(`INSERT INTO series (id, libraryId, name, nameIgnorePrefix, description, createdAt, updatedAt) VALUES 
		('series1', 'lib1', 'Harry Potter', 'Harry Potter', 'Boy wizard', '2026-06-08 12:00:00.000', '2026-06-08 12:00:00.000')`)
	if err != nil {
		t.Fatalf("Failed to insert series: %v", err)
	}

	_, err = db.Exec(`INSERT INTO books (id, title, duration, narrators, audioFiles, ebookFile, chapters, tags, genres) VALUES 
		('book1', 'Harry Potter 1', 7200, '[]', '[]', 'null', '[]', '[]', '[]')`)
	if err != nil {
		t.Fatalf("Failed to insert book: %v", err)
	}

	_, err = db.Exec(`INSERT INTO libraryItems (id, ino, libraryId, path, relPath, isFile, createdAt, updatedAt, mediaType, mediaId) VALUES 
		('item1', '1', 'lib1', '/path1', 'rel1', 1, '2026-06-08 12:00:00.000', '2026-06-08 12:00:00.000', 'book', 'book1')`)
	if err != nil {
		t.Fatalf("Failed to insert libraryItem: %v", err)
	}

	_, err = db.Exec(`INSERT INTO bookSeries (bookId, seriesId, sequence) VALUES ('book1', 'series1', '1')`)
	if err != nil {
		t.Fatalf("Failed to insert bookSeries: %v", err)
	}

	// Insert finished progress
	_, err = db.Exec(`INSERT INTO mediaProgresses (id, userId, mediaItemId, isFinished, currentTime, updatedAt) VALUES 
		('progress1', 'user1', 'book1', 1, 7200.0, '2026-06-08 12:00:00.000')`)
	if err != nil {
		t.Fatalf("Failed to insert mediaProgresses: %v", err)
	}

	handler := handleGetLibrarySeriesByID(db, "lib1", "series1")
	req := httptest.NewRequest("GET", "/api/libraries/lib1/series/series1", nil)

	user := &UserSession{
		ID:                 "user1",
		Username:           "admin",
		Type:               "admin",
		IsActive:           true,
		AccessAllLibraries: true,
	}
	req = req.WithContext(context.WithValue(req.Context(), UserContextKey, user))

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d. Body: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse JSON response: %v", err)
	}

	if resp["name"] != "Harry Potter" {
		t.Errorf("Expected series name Harry Potter, got %v", resp["name"])
	}

	progress, ok := resp["progress"].(map[string]interface{})
	if !ok {
		t.Fatalf("Expected progress field: %v", resp)
	}

	libItemIds := progress["libraryItemIds"].([]interface{})
	if len(libItemIds) != 1 || libItemIds[0] != "item1" {
		t.Errorf("Expected libraryItemIds contain item1, got %v", libItemIds)
	}

	libItemIdsFinished := progress["libraryItemIdsFinished"].([]interface{})
	if len(libItemIdsFinished) != 1 || libItemIdsFinished[0] != "item1" {
		t.Errorf("Expected libraryItemIdsFinished contain item1, got %v", libItemIdsFinished)
	}

	if isFinished, ok := progress["isFinished"].(bool); !ok || !isFinished {
		t.Errorf("Expected isFinished to be true, got %v", progress["isFinished"])
	}
}

func TestGetAuthorByID(t *testing.T) {
	db := setupTestDBShared(t)
	defer db.Close()

	// Insert author
	_, err := db.Exec(`INSERT INTO authors (id, libraryId, name, lastFirst, asin, description, imagePath, createdAt, updatedAt) VALUES 
		('author1', 'lib1', 'Stephen King', 'King, Stephen', 'asin1', 'Horror writer', 'covers/stephen_king.jpg', '2026-06-08 12:00:00.000', '2026-06-08 12:00:00.000')`)
	if err != nil {
		t.Fatalf("Failed to insert author: %v", err)
	}

	// Insert book
	_, err = db.Exec(`INSERT INTO books (id, title, duration, narrators, audioFiles, ebookFile, chapters, tags, genres) VALUES ('book1', 'The Shining', 3600, '[]', '[]', 'null', '[]', '[]', '[]')`)
	if err != nil {
		t.Fatalf("Failed to insert book: %v", err)
	}
	_, err = db.Exec(`INSERT INTO libraryItems (
		id, ino, libraryId, libraryFolderId, path, relPath, isFile, 
		mtime, ctime, birthtime, createdAt, updatedAt, 
		isMissing, isInvalid, mediaType, mediaId, size
	) VALUES (
		'item1', '1', 'lib1', 'folder1', '/path1', 'rel1', 1, 
		'2026-06-08 12:00:00.000', '2026-06-08 12:00:00.000', '2026-06-08 12:00:00.000', '2026-06-08 12:00:00.000', '2026-06-08 12:00:00.000', 
		0, 0, 'book', 'book1', 5000
	)`)
	if err != nil {
		t.Fatalf("Failed to insert libraryItem: %v", err)
	}
	_, err = db.Exec(`INSERT INTO bookAuthors (bookId, authorId) VALUES ('book1', 'author1')`)
	if err != nil {
		t.Fatalf("Failed to insert bookAuthor: %v", err)
	}

	// Insert series
	_, err = db.Exec(`INSERT INTO series (id, libraryId, name, nameIgnorePrefix, description, createdAt, updatedAt) VALUES 
		('series1', 'lib1', 'The Dark Tower', 'Dark Tower', 'Dark fantasy', '2026-06-08 12:00:00.000', '2026-06-08 12:00:00.000')`)
	if err != nil {
		t.Fatalf("Failed to insert series: %v", err)
	}
	_, err = db.Exec(`INSERT INTO bookSeries (bookId, seriesId, sequence) VALUES ('book1', 'series1', '1')`)
	if err != nil {
		t.Fatalf("Failed to insert bookSeries: %v", err)
	}

	handler := handleGetAuthorByID(db, "author1")
	req := httptest.NewRequest("GET", "/api/authors/author1?include=items,series", nil)

	user := &UserSession{
		ID:                 "user1",
		Username:           "admin",
		Type:               "admin",
		IsActive:           true,
		AccessAllLibraries: true,
	}
	req = req.WithContext(context.WithValue(req.Context(), UserContextKey, user))

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d. Body: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse JSON response: %v", err)
	}

	if resp["name"] != "Stephen King" {
		t.Errorf("Expected name Stephen King, got %v", resp["name"])
	}

	libraryItems, ok := resp["libraryItems"].([]interface{})
	if !ok || len(libraryItems) != 1 {
		t.Fatalf("Expected 1 library item, got: %v", resp["libraryItems"])
	}

	itemMap := libraryItems[0].(map[string]interface{})
	if itemMap["id"] != "item1" {
		t.Errorf("Expected library item ID item1, got %v", itemMap["id"])
	}

	seriesList, ok := resp["series"].([]interface{})
	if !ok || len(seriesList) != 1 {
		t.Fatalf("Expected 1 series, got: %v", resp["series"])
	}

	seriesMap := seriesList[0].(map[string]interface{})
	if seriesMap["name"] != "The Dark Tower" {
		t.Errorf("Expected series name The Dark Tower, got %v", seriesMap["name"])
	}
}

func TestGetLibraryItemByID(t *testing.T) {
	db := setupTestDBShared(t)
	defer db.Close()

	// Insert book with ebookFile and audioFiles JSON
	ebookJSON := `{"ebookFormat":"epub", "metadata":{"filename":"book.epub", "ext":".epub", "path":"/fake/book.epub", "relPath":"book.epub", "size":1048576, "ctime":1718000000000, "mtime":1718000000000}}`
	audioFilesJSON := `[{"ino":"audio1", "filename":"ch1.mp3", "ext":".mp3", "size":5000000, "metadata":{"path":"/fake/ch1.mp3", "relPath":"ch1.mp3", "mtime":1718000000000, "ctime":1718000000000}}]`

	_, err := db.Exec(`INSERT INTO books (id, title, duration, coverPath, narrators, audioFiles, ebookFile, chapters, tags, genres) VALUES 
		('book1', 'Ebook Test', 3600, 'covers/hp1.jpg', '[]', ?, ?, '[]', '[]', '[]')`, audioFilesJSON, ebookJSON)
	if err != nil {
		t.Fatalf("Failed to insert book: %v", err)
	}

	_, err = db.Exec(`INSERT INTO libraryItems (
		id, ino, libraryId, libraryFolderId, path, relPath, isFile, 
		mtime, ctime, birthtime, createdAt, updatedAt, 
		isMissing, isInvalid, mediaType, mediaId, size
	) VALUES (
		'item1', 'inode-item', 'lib1', 'folder1', '/path1', 'rel1', 1, 
		'2026-06-08 12:00:00.000', '2026-06-08 12:00:00.000', '2026-06-08 12:00:00.000', '2026-06-08 12:00:00.000', '2026-06-08 12:00:00.000', 
		0, 0, 'book', 'book1', 6048576
	)`)
	if err != nil {
		t.Fatalf("Failed to insert libraryItem: %v", err)
	}

	handler := handleGetLibraryItemByID(db, "item1")
	req := httptest.NewRequest("GET", "/api/items/item1", nil)

	user := &UserSession{
		ID:                 "user1",
		Username:           "admin",
		Type:               "admin",
		IsActive:           true,
		AccessAllLibraries: true,
	}
	req = req.WithContext(context.WithValue(req.Context(), UserContextKey, user))

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d. Body: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse JSON response: %v", err)
	}

	if resp["id"] != "item1" || resp["ino"] != "inode-item" {
		t.Errorf("Unexpected item response details: %v", resp)
	}

	libraryFiles, ok := resp["libraryFiles"].([]interface{})
	if !ok || len(libraryFiles) != 2 {
		t.Fatalf("Expected 2 libraryFiles, got: %v", resp["libraryFiles"])
	}

	// First is ebook
	ebFile := libraryFiles[0].(map[string]interface{})
	if ebFile["fileType"] != "ebook" || ebFile["filename"] != "book.epub" || ebFile["ino"] != "inode-item" {
		t.Errorf("Unexpected ebook file info: %v", ebFile)
	}

	// Second is audio
	auFile := libraryFiles[1].(map[string]interface{})
	if auFile["fileType"] != "audio" || auFile["filename"] != "ch1.mp3" || auFile["ino"] != "audio1" {
		t.Errorf("Unexpected audio file info: %v", auFile)
	}
}

func TestServeEbook(t *testing.T) {
	db := setupTestDBShared(t)
	defer db.Close()

	// Write a fake epub file to test path
	tempDir := t.TempDir()
	fakeEpubPath := filepath.Join(tempDir, "test.epub")
	err := os.WriteFile(fakeEpubPath, []byte("epub content"), 0644)
	if err != nil {
		t.Fatalf("Failed to write fake epub file: %v", err)
	}

	ebookJSON := `{"ebookFormat":"epub", "metadata":{"filename":"test.epub", "ext":".epub", "path":"` + filepath.ToSlash(fakeEpubPath) + `", "size":12}}`

	_, err = db.Exec(`INSERT INTO books (id, title, duration, coverPath, narrators, audioFiles, ebookFile, chapters, tags, genres) VALUES 
		('book1', 'Ebook Serve Test', 0, '', '[]', '[]', ?, '[]', '[]', '[]')`, ebookJSON)
	if err != nil {
		t.Fatalf("Failed to insert book: %v", err)
	}

	_, err = db.Exec(`INSERT INTO libraryItems (
		id, ino, libraryId, libraryFolderId, path, relPath, isFile, 
		mtime, ctime, birthtime, createdAt, updatedAt, 
		isMissing, isInvalid, mediaType, mediaId, size
	) VALUES (
		'item1', 'inode-item', 'lib1', 'folder1', '/path1', 'rel1', 1, 
		'2026-06-08 12:00:00.000', '2026-06-08 12:00:00.000', '2026-06-08 12:00:00.000', '2026-06-08 12:00:00.000', '2026-06-08 12:00:00.000', 
		0, 0, 'book', 'book1', 12
	)`)
	if err != nil {
		t.Fatalf("Failed to insert libraryItem: %v", err)
	}

	handler := handleServeEbook(db, "item1", "some-file-id")
	req := httptest.NewRequest("GET", "/api/items/item1/ebook", nil)

	user := &UserSession{
		ID:                 "user1",
		Username:           "admin",
		Type:               "admin",
		IsActive:           true,
		AccessAllLibraries: true,
	}
	req = req.WithContext(context.WithValue(req.Context(), UserContextKey, user))

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d. Body: %s", rr.Code, rr.Body.String())
	}

	if rr.Header().Get("Content-Type") != "application/epub+zip" {
		t.Errorf("Expected Content-Type application/epub+zip, got %q", rr.Header().Get("Content-Type"))
	}

	if rr.Body.String() != "epub content" {
		t.Errorf("Expected file body 'epub content', got %q", rr.Body.String())
	}
}
