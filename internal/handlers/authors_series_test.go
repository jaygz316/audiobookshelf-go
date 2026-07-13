package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/doyensec/safeurl"
	_ "modernc.org/sqlite"

	"audiobookshelf/internal/core"
)

func setupTestDBShared(t *testing.T) *sql.DB {
	globalFinder = nil
	globalPlaylistManager = nil
	globalShareManager = nil
	globalFeedManager = nil
	globalPodcastManager = nil

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
		`CREATE TABLE IF NOT EXISTS books (id TEXT PRIMARY KEY, title TEXT, titleIgnorePrefix TEXT, subtitle TEXT, publishedYear TEXT, publishedDate TEXT, publisher TEXT, description TEXT, isbn TEXT, asin TEXT, language TEXT, explicit INTEGER, abridged INTEGER, coverPath TEXT, duration REAL, narrators BLOB, audioFiles BLOB, ebookFile BLOB, chapters BLOB, tags BLOB, genres BLOB, lockedFields BLOB)`,
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

	user := &core.UserSession{
		ID:                 "user1",
		Username:           "admin",
		Type:               "admin",
		IsActive:           true,
		AccessAllLibraries: true,
	}
	req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, user))

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

	user := &core.UserSession{
		ID:                 "user1",
		Username:           "admin",
		Type:               "admin",
		IsActive:           true,
		AccessAllLibraries: true,
	}
	req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, user))

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

	user := &core.UserSession{
		ID:                 "user1",
		Username:           "admin",
		Type:               "admin",
		IsActive:           true,
		AccessAllLibraries: true,
	}
	req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, user))

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

	user := &core.UserSession{
		ID:                 "user1",
		Username:           "admin",
		Type:               "admin",
		IsActive:           true,
		AccessAllLibraries: true,
	}
	req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, user))

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

	user := &core.UserSession{
		ID:                 "user1",
		Username:           "admin",
		Type:               "admin",
		IsActive:           true,
		AccessAllLibraries: true,
	}
	req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, user))

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

	user := &core.UserSession{
		ID:                 "user1",
		Username:           "admin",
		Type:               "admin",
		IsActive:           true,
		AccessAllLibraries: true,
	}
	req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, user))

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

func TestUpdateLibraryItem(t *testing.T) {
	oldMetaPath := MetadataPath
	MetadataPath = t.TempDir()
	defer func() { MetadataPath = oldMetaPath }()

	db := setupTestDBShared(t)
	defer db.Close()

	// 1. Insert test library, book, and libraryItem
	_, err := db.Exec(`INSERT INTO libraries (id, name, displayOrder, icon, mediaType, provider, settings, createdAt, updatedAt) VALUES ('lib1', 'Audiobooks', 1, 'book', 'book', 'local', '{}', '2026-06-08 12:00:00.000', '2026-06-08 12:00:00.000')`)
	if err != nil {
		t.Fatalf("Failed to insert library: %v", err)
	}

	_, err = db.Exec(`INSERT INTO books (id, title, duration, coverPath, narrators, audioFiles, ebookFile, chapters, tags, genres) VALUES 
		('book1', 'Old Title', 3600, '', '[]', '[]', 'null', '[]', '[]', '[]')`)
	if err != nil {
		t.Fatalf("Failed to insert book: %v", err)
	}

	_, err = db.Exec(`INSERT INTO libraryItems (
		id, ino, libraryId, libraryFolderId, path, relPath, isFile, 
		mtime, ctime, birthtime, createdAt, updatedAt, 
		isMissing, isInvalid, mediaType, mediaId, size,
		authorNamesFirstLast, authorNamesLastFirst, title, titleIgnorePrefix
	) VALUES (
		'item1', 'inode-item', 'lib1', 'folder1', '/path1', 'rel1', 1, 
		'2026-06-08 12:00:00.000', '2026-06-08 12:00:00.000', '2026-06-08 12:00:00.000', '2026-06-08 12:00:00.000', '2026-06-08 12:00:00.000', 
		0, 0, 'book', 'book1', 5000,
		'Old Author', 'Author, Old', 'Old Title', 'Old Title'
	)`)
	if err != nil {
		t.Fatalf("Failed to insert libraryItem: %v", err)
	}

	// 2. Prepare payload
	payload := map[string]interface{}{
		"title":          "New Title",
		"subtitle":       "New Subtitle",
		"authors":        []string{"New Author"},
		"narrators":      []string{"Narrator A"},
		"seriesName":     "New Series",
		"seriesSequence": "2",
		"publisher":      "New Publisher",
		"publishedYear":  "2024",
		"publishedDate":  "2024-01-01",
		"description":    "New Description",
		"isbn":           "1234567890",
		"asin":           "ASIN123",
		"language":       "en",
		"explicit":       true,
		"abridged":       false,
		"tags":           []string{"Tag1"},
		"genres":         []string{"Genre1"},
	}

	bodyBytes, _ := json.Marshal(payload)

	// 3. Make request
	handler := handleUpdateLibraryItemByID(db, "item1")
	req := httptest.NewRequest("PATCH", "/api/items/item1", strings.NewReader(string(bodyBytes)))

	user := &core.UserSession{
		ID:                 "user1",
		Username:           "admin",
		Type:               "admin",
		IsActive:           true,
		AccessAllLibraries: true,
	}
	req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, user))

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d. Body: %s", rr.Code, rr.Body.String())
	}

	// 4. Verify DB changes
	var bTitle, bSubtitle, bPublisher, bDescription, bIsbn, bAsin, bLanguage string
	var bExplicit, bAbridged int
	var bNarratorsBytes, bTagsBytes, bGenresBytes []byte
	err = db.QueryRow(`
		SELECT title, COALESCE(subtitle, ''), COALESCE(publisher, ''), COALESCE(description, ''), COALESCE(isbn, ''), COALESCE(asin, ''), COALESCE(language, ''), explicit, abridged, narrators, tags, genres
		FROM books WHERE id = 'book1'
	`).Scan(&bTitle, &bSubtitle, &bPublisher, &bDescription, &bIsbn, &bAsin, &bLanguage, &bExplicit, &bAbridged, &bNarratorsBytes, &bTagsBytes, &bGenresBytes)
	if err != nil {
		t.Fatalf("Failed to query updated book: %v", err)
	}

	if bTitle != "New Title" || bSubtitle != "New Subtitle" || bPublisher != "New Publisher" || bDescription != "New Description" || bIsbn != "1234567890" || bAsin != "ASIN123" || bLanguage != "en" {
		t.Errorf("Unexpected book details after update: title=%q subtitle=%q publisher=%q desc=%q isbn=%q asin=%q lang=%q", bTitle, bSubtitle, bPublisher, bDescription, bIsbn, bAsin, bLanguage)
	}
	if bExplicit != 1 || bAbridged != 0 {
		t.Errorf("Unexpected explicit/abridged after update: explicit=%d abridged=%d", bExplicit, bAbridged)
	}

	var narrators []string
	json.Unmarshal(bNarratorsBytes, &narrators)
	if len(narrators) != 1 || narrators[0] != "Narrator A" {
		t.Errorf("Unexpected narrators: %v", narrators)
	}

	var tags []string
	json.Unmarshal(bTagsBytes, &tags)
	if len(tags) != 1 || tags[0] != "Tag1" {
		t.Errorf("Unexpected tags: %v", tags)
	}

	// Verify libraryItem table is updated
	var liTitle, liAuthorNamesFirstLast, liAuthorNamesLastFirst string
	err = db.QueryRow(`
		SELECT title, authorNamesFirstLast, authorNamesLastFirst
		FROM libraryItems WHERE id = 'item1'
	`).Scan(&liTitle, &liAuthorNamesFirstLast, &liAuthorNamesLastFirst)
	if err != nil {
		t.Fatalf("Failed to query library item: %v", err)
	}

	if liTitle != "New Title" || liAuthorNamesFirstLast != "New Author" || liAuthorNamesLastFirst != "Author, New" {
		t.Errorf("Library item table fields were not mirrored: title=%q authorNamesFirstLast=%q authorNamesLastFirst=%q", liTitle, liAuthorNamesFirstLast, liAuthorNamesLastFirst)
	}
}

func TestUpdateCoverFromURL(t *testing.T) {
	// Start local mock server to serve fake cover image
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("fake cover image data"))
	}))
	defer mockServer.Close()

	u, err := url.Parse(mockServer.URL)
	if err != nil {
		t.Fatalf("failed to parse mock server url: %v", err)
	}
	port, _ := strconv.Atoi(u.Port())

	// Override coverHTTPClient to allow local connection for test
	origClient := coverHTTPClient
	config := safeurl.GetConfigBuilder().
		SetAllowedIPs("127.0.0.1", "::1").
		SetAllowedPorts(port).
		Build()
	coverHTTPClient = safeurl.Client(config)
	defer func() {
		coverHTTPClient = origClient
	}()

	db := setupTestDBShared(t)
	defer db.Close()

	tempDir := t.TempDir()
	cfg := &core.Config{
		MetadataPath: tempDir,
	}

	// 1. Insert test library, book, and libraryItem
	_, err = db.Exec(`INSERT INTO libraries (id, name, displayOrder, icon, mediaType, provider, settings, createdAt, updatedAt) VALUES ('lib1', 'Audiobooks', 1, 'book', 'book', 'local', '{}', '2026-06-08 12:00:00.000', '2026-06-08 12:00:00.000')`)
	if err != nil {
		t.Fatalf("Failed to insert library: %v", err)
	}

	_, err = db.Exec(`INSERT INTO books (id, title, duration, coverPath, narrators, audioFiles, ebookFile, chapters, tags, genres) VALUES 
		('book1', 'Old Title', 3600, '', '[]', '[]', 'null', '[]', '[]', '[]')`)
	if err != nil {
		t.Fatalf("Failed to insert book: %v", err)
	}

	_, err = db.Exec(`INSERT INTO libraryItems (
		id, ino, libraryId, libraryFolderId, path, relPath, isFile, 
		mtime, ctime, birthtime, createdAt, updatedAt, 
		isMissing, isInvalid, mediaType, mediaId, size,
		authorNamesFirstLast, authorNamesLastFirst, title, titleIgnorePrefix
	) VALUES (
		'item1', 'inode-item', 'lib1', 'folder1', '/path1', 'rel1', 1, 
		'2026-06-08 12:00:00.000', '2026-06-08 12:00:00.000', '2026-06-08 12:00:00.000', '2026-06-08 12:00:00.000', '2026-06-08 12:00:00.000', 
		0, 0, 'book', 'book1', 5000,
		'Old Author', 'Author, Old', 'Old Title', 'Old Title'
	)`)
	if err != nil {
		t.Fatalf("Failed to insert libraryItem: %v", err)
	}

	// 2. Prepare payload
	payload := map[string]interface{}{
		"coverUrl": mockServer.URL,
	}
	bodyBytes, _ := json.Marshal(payload)

	// 3. Make request
	handler := handleUpdateCoverFromURL(db, cfg, "item1")
	req := httptest.NewRequest("POST", "/api/items/item1/cover-from-url", strings.NewReader(string(bodyBytes)))

	user := &core.UserSession{
		ID:                 "user1",
		Username:           "admin",
		Type:               "admin",
		IsActive:           true,
		AccessAllLibraries: true,
	}
	req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, user))

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d. Body: %s", rr.Code, rr.Body.String())
	}

	// 4. Verify DB changes
	var coverPath string
	err = db.QueryRow("SELECT coverPath FROM books WHERE id = 'book1'").Scan(&coverPath)
	if err != nil {
		t.Fatalf("Failed to query cover path: %v", err)
	}

	if coverPath == "" {
		t.Fatalf("Expected coverPath to be non-empty")
	}

	// Verify that the file was created and contains the correct data
	data, err := os.ReadFile(coverPath)
	if err != nil {
		t.Fatalf("Failed to read downloaded cover file at %s: %v", coverPath, err)
	}

	if string(data) != "fake cover image data" {
		t.Errorf("Expected file content 'fake cover image data', got %q", string(data))
	}
}

func TestUpdateAuthor(t *testing.T) {
	oldMetaPath := MetadataPath
	MetadataPath = t.TempDir()
	defer func() { MetadataPath = oldMetaPath }()

	db := setupTestDBShared(t)
	defer db.Close()

	// 1. Insert test library, book, author, and libraryItem
	_, _ = db.Exec(`INSERT INTO libraries (id, name, displayOrder, icon, mediaType, provider, settings, createdAt, updatedAt) VALUES ('lib1', 'Audiobooks', 1, 'book', 'book', 'local', '{}', '2026-06-08 12:00:00.000', '2026-06-08 12:00:00.000')`)
	_, _ = db.Exec(`INSERT INTO books (id, title, duration, coverPath, narrators, audioFiles, ebookFile, chapters, tags, genres) VALUES ('book1', 'Old Title', 3600, '', '[]', '[]', 'null', '[]', '[]', '[]')`)
	_, _ = db.Exec(`INSERT INTO authors (id, name, lastFirst, asin, description, libraryId) VALUES ('author1', 'Old Author Name', 'Name, Old Author', 'OldASIN', 'Old bio', 'lib1')`)
	_, _ = db.Exec(`INSERT INTO bookAuthors (bookId, authorId) VALUES ('book1', 'author1')`)
	_, _ = db.Exec(`INSERT INTO libraryItems (
		id, ino, libraryId, libraryFolderId, path, relPath, isFile, 
		mtime, ctime, birthtime, createdAt, updatedAt, 
		isMissing, isInvalid, mediaType, mediaId, size,
		authorNamesFirstLast, authorNamesLastFirst, title, titleIgnorePrefix
	) VALUES (
		'item1', 'inode-item', 'lib1', 'folder1', '/path1', 'rel1', 1, 
		'2026-06-08 12:00:00.000', '2026-06-08 12:00:00.000', '2026-06-08 12:00:00.000', '2026-06-08 12:00:00.000', '2026-06-08 12:00:00.000', 
		0, 0, 'book', 'book1', 5000,
		'Old Author Name', 'Name, Old Author', 'Old Title', 'Old Title'
	)`)

	// 2. Prepare PATCH payload
	payload := map[string]interface{}{
		"name":        "New Author Name",
		"lastFirst":   "Name, New Author",
		"asin":        "NewASIN",
		"description": "New bio description",
	}
	bodyBytes, _ := json.Marshal(payload)

	// 3. Make request
	handler := handleUpdateAuthor(db, "author1")
	req := httptest.NewRequest("PATCH", "/api/authors/author1", strings.NewReader(string(bodyBytes)))

	user := &core.UserSession{
		ID:                 "user1",
		Username:           "admin",
		Type:               "admin",
		IsActive:           true,
		AccessAllLibraries: true,
	}
	req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, user))

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d. Body: %s", rr.Code, rr.Body.String())
	}

	// 4. Verify DB changes on author
	var name, lastFirst, asin, description string
	err := db.QueryRow("SELECT name, lastFirst, asin, description FROM authors WHERE id = 'author1'").Scan(&name, &lastFirst, &asin, &description)
	if err != nil {
		t.Fatalf("Failed to query updated author: %v", err)
	}
	if name != "New Author Name" || lastFirst != "Name, New Author" || asin != "NewASIN" || description != "New bio description" {
		t.Errorf("Author details not updated correctly: name=%q, lastFirst=%q, asin=%q, description=%q", name, lastFirst, asin, description)
	}

	// 5. Verify libraryItem denormalized fields are updated
	var liAuthorNamesFirstLast, liAuthorNamesLastFirst string
	err = db.QueryRow("SELECT authorNamesFirstLast, authorNamesLastFirst FROM libraryItems WHERE id = 'item1'").Scan(&liAuthorNamesFirstLast, &liAuthorNamesLastFirst)
	if err != nil {
		t.Fatalf("Failed to query updated libraryItem: %v", err)
	}
	if liAuthorNamesFirstLast != "New Author Name" || liAuthorNamesLastFirst != "Name, New Author" {
		t.Errorf("LibraryItem denormalized author fields not updated correctly: firstLast=%q, lastFirst=%q", liAuthorNamesFirstLast, liAuthorNamesLastFirst)
	}
}

func TestUpdateSeries(t *testing.T) {
	db := setupTestDBShared(t)
	defer db.Close()

	// 1. Insert series and bookSeries
	_, _ = db.Exec(`INSERT INTO series (id, name, nameIgnorePrefix, description, libraryId) VALUES ('series1', 'Old Series', 'Old Series', 'Old description', 'lib1')`)

	// 2. Prepare PATCH payload
	payload := map[string]interface{}{
		"name":             "New Series Name",
		"nameIgnorePrefix": "New Series Name",
		"description":      "New description detail",
	}
	bodyBytes, _ := json.Marshal(payload)

	// 3. Make request
	handler := handleUpdateSeries(db, "series1")
	req := httptest.NewRequest("PATCH", "/api/series/series1", strings.NewReader(string(bodyBytes)))

	user := &core.UserSession{
		ID:                 "user1",
		Username:           "admin",
		Type:               "admin",
		IsActive:           true,
		AccessAllLibraries: true,
	}
	req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, user))

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d. Body: %s", rr.Code, rr.Body.String())
	}

	// 4. Verify DB changes on series
	var name, nameIgnorePrefix, description string
	err := db.QueryRow("SELECT name, nameIgnorePrefix, description FROM series WHERE id = 'series1'").Scan(&name, &nameIgnorePrefix, &description)
	if err != nil {
		t.Fatalf("Failed to query updated series: %v", err)
	}
	if name != "New Series Name" || nameIgnorePrefix != "New Series Name" || description != "New description detail" {
		t.Errorf("Series details not updated correctly: name=%q, nameIgnorePrefix=%q, description=%q", name, nameIgnorePrefix, description)
	}
}

func TestHandleAutoNumberSeries(t *testing.T) {
	db := setupTestDBShared(t)
	defer db.Close()

	// 1. Insert series
	_, _ = db.Exec(`INSERT INTO series (id, name, nameIgnorePrefix, description, libraryId) VALUES ('series1', 'Test Series', 'Test Series', 'Test description', 'lib1')`)

	// 2. Insert books in the series
	// book1: pubYear 2020, title "B book"
	// book2: pubYear 2010, title "A book"
	// book3: pubYear 2020, title "C book"
	// book4: pubYear 2020, title "C book (Narrated by Narrator)"
	_, _ = db.Exec(`INSERT INTO books (id, title, publishedYear) VALUES ('book1', 'B book', '2020')`)
	_, _ = db.Exec(`INSERT INTO books (id, title, publishedYear) VALUES ('book2', 'A book', '2010')`)
	_, _ = db.Exec(`INSERT INTO books (id, title, publishedYear) VALUES ('book3', 'C book', '2020')`)
	_, _ = db.Exec(`INSERT INTO books (id, title, publishedYear) VALUES ('book4', 'C book (Narrated by Narrator)', '2020')`)

	// Link books to series in bookSeries
	_, _ = db.Exec(`INSERT INTO bookSeries (bookId, seriesId, sequence) VALUES ('book1', 'series1', '99')`)
	_, _ = db.Exec(`INSERT INTO bookSeries (bookId, seriesId, sequence) VALUES ('book2', 'series1', '98')`)
	_, _ = db.Exec(`INSERT INTO bookSeries (bookId, seriesId, sequence) VALUES ('book3', 'series1', '97')`)
	_, _ = db.Exec(`INSERT INTO bookSeries (bookId, seriesId, sequence) VALUES ('book4', 'series1', '96')`)

	// 3. Make auto-number request
	handler := handleAutoNumberSeries(db, "series1")
	req := httptest.NewRequest("POST", "/api/series/series1/auto-number", nil)

	user := &core.UserSession{
		ID:                 "user1",
		Username:           "admin",
		Type:               "admin",
		IsActive:           true,
		AccessAllLibraries: true,
	}
	req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, user))

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d. Body: %s", rr.Code, rr.Body.String())
	}

	// 4. Verify new sequences:
	// Chronological order:
	// - book2 (2010) => sequence should be "1"
	// - book1 (2020, title "B book") => sequence should be "2"
	// - book3 (2020, title "C book") => sequence should be "3"
	// - book4 (2020, title "C book (Narrated by Narrator)") => normalized to "c book", sequence should be "3"
	var seq1, seq2, seq3, seq4 string
	_ = db.QueryRow("SELECT sequence FROM bookSeries WHERE bookId = 'book2' AND seriesId = 'series1'").Scan(&seq1)
	_ = db.QueryRow("SELECT sequence FROM bookSeries WHERE bookId = 'book1' AND seriesId = 'series1'").Scan(&seq2)
	_ = db.QueryRow("SELECT sequence FROM bookSeries WHERE bookId = 'book3' AND seriesId = 'series1'").Scan(&seq3)
	_ = db.QueryRow("SELECT sequence FROM bookSeries WHERE bookId = 'book4' AND seriesId = 'series1'").Scan(&seq4)

	if seq1 != "1" {
		t.Errorf("Expected book2 sequence to be '1', got %q", seq1)
	}
	if seq2 != "2" {
		t.Errorf("Expected book1 sequence to be '2', got %q", seq2)
	}
	if seq3 != "3" {
		t.Errorf("Expected book3 sequence to be '3', got %q", seq3)
	}
	if seq4 != "3" {
		t.Errorf("Expected book4 sequence to be '3', got %q", seq4)
	}
}
