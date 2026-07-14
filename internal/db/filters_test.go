package db

import (
	"database/sql"
	"encoding/base64"
	"testing"

	"audiobookshelf/internal/core"
	_ "modernc.org/sqlite"
)

func TestGetFilteredLibraryItems_NewFilters(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open DB: %v", err)
	}
	defer db.Close()

	// Create tables
	tables := []string{
		`CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT, createdAt TEXT, updatedAt TEXT)`,
		`CREATE TABLE users (id TEXT PRIMARY KEY, username TEXT, email TEXT, pash TEXT, type TEXT, token TEXT, isActive INTEGER, isLocked INTEGER, lastSeen INTEGER, permissions TEXT, bookmarks TEXT, extraData TEXT, createdAt TEXT, updatedAt TEXT)`,
		`CREATE TABLE libraries (id TEXT PRIMARY KEY, name TEXT, displayOrder INTEGER, icon TEXT, mediaType TEXT, provider TEXT, lastScan TEXT, lastScanVersion TEXT, settings TEXT, createdAt TEXT, updatedAt TEXT)`,
		`CREATE TABLE libraryItems (id TEXT PRIMARY KEY, ino TEXT, libraryId TEXT, path TEXT, relPath TEXT, isFile INTEGER, mtime TEXT, ctime TEXT, birthtime TEXT, createdAt TEXT, updatedAt TEXT, isMissing INTEGER, isInvalid INTEGER, mediaType TEXT, mediaId TEXT, size INTEGER, libraryFolderId TEXT, authorNamesFirstLast TEXT, authorNamesLastFirst TEXT, title TEXT, titleIgnorePrefix TEXT)`,
		`CREATE TABLE books (id TEXT PRIMARY KEY, title TEXT, titleIgnorePrefix TEXT, subtitle TEXT, publishedYear TEXT, publishedDate TEXT, publisher TEXT, description TEXT, isbn TEXT, asin TEXT, language TEXT, explicit INTEGER, abridged INTEGER, coverPath TEXT, duration REAL, narrators BLOB, audioFiles BLOB, ebookFile BLOB, chapters BLOB, tags BLOB, genres BLOB, lockedFields BLOB)`,
		`CREATE TABLE bookAuthors (bookId TEXT, authorId TEXT)`,
		`CREATE TABLE bookSeries (bookId TEXT, seriesId TEXT, sequence TEXT)`,
		`CREATE TABLE series (id TEXT PRIMARY KEY, libraryId TEXT, name TEXT, nameIgnorePrefix TEXT, description TEXT, createdAt TEXT, updatedAt TEXT)`,
	}

	for _, tbl := range tables {
		if _, err := db.Exec(tbl); err != nil {
			t.Fatalf("Failed to create table: %v", err)
		}
	}

	// Insert test data
	_, _ = db.Exec(`INSERT INTO users (id, username, permissions) VALUES ('user_1', 'testuser', '[]')`)
	_, _ = db.Exec(`INSERT INTO libraries (id, name, mediaType) VALUES ('lib_1', 'Test Library', 'book')`)

	// Item 1: Year 2025 (Decade 2020), Duration 7200s (2h), Folder f1
	_, _ = db.Exec(`INSERT INTO books (id, title, publishedYear, duration) VALUES ('book_1', 'Book One', '2025', 7200.0)`)
	_, _ = db.Exec(`INSERT INTO libraryItems (id, libraryId, mediaType, mediaId, libraryFolderId, title) VALUES ('li_1', 'lib_1', 'book', 'book_1', 'f1', 'Book One')`)

	// Item 2: Year 2015 (Decade 2010), Duration 500s (<1h), Folder f2
	_, _ = db.Exec(`INSERT INTO books (id, title, publishedYear, duration) VALUES ('book_2', 'Book Two', '2015', 500.0)`)
	_, _ = db.Exec(`INSERT INTO libraryItems (id, libraryId, mediaType, mediaId, libraryFolderId, title) VALUES ('li_2', 'lib_1', 'book', 'book_2', 'f2', 'Book Two')`)

	// Test case: Decade filter
	optsDecade := GetFilteredLibraryItemsOptions{
		LibraryID: "lib_1",
		User:      &core.UserSession{ID: "user_1", CanAccessExplicitContent: true, AccessAllTags: true},
		MediaType: "book",
		FilterBy:  "decades." + base64.StdEncoding.EncodeToString([]byte("2020")),
	}
	resDecade, _, err := GetFilteredLibraryItems(db, optsDecade)
	if err != nil {
		t.Fatalf("Decade filter failed: %v", err)
	}
	t.Logf("Decade 2020 results:")
	for _, item := range resDecade {
		bMin := item.Media.(*BookMinifiedJSON)
		pubYear := ""
		if bMin.Metadata.PublishedYear != nil {
			pubYear = *bMin.Metadata.PublishedYear
		}
		t.Logf("- Item: %s, MediaID: %s, Title: %s, PublishedYear: %s", item.ID, bMin.ID, bMin.Metadata.Title, pubYear)
	}
	if len(resDecade) != 1 || resDecade[0].ID != "li_1" {
		t.Errorf("Expected only li_1 for decades.2020, got %d results", len(resDecade))
	}

	// Test case: Year filter
	optsYear := GetFilteredLibraryItemsOptions{
		LibraryID: "lib_1",
		User:      &core.UserSession{ID: "user_1", CanAccessExplicitContent: true, AccessAllTags: true},
		MediaType: "book",
		FilterBy:  "years." + base64.StdEncoding.EncodeToString([]byte("2015")),
	}
	resYear, _, err := GetFilteredLibraryItems(db, optsYear)
	if err != nil {
		t.Fatalf("Year filter failed: %v", err)
	}
	t.Logf("Year 2015 results:")
	for _, item := range resYear {
		bMin := item.Media.(*BookMinifiedJSON)
		t.Logf("- Item: %s, MediaID: %s, Title: %s", item.ID, bMin.ID, bMin.Metadata.Title)
	}
	if len(resYear) != 1 || resYear[0].ID != "li_2" {
		t.Errorf("Expected only li_2 for years.2015, got %d results", len(resYear))
	}

	// Test case: Duration filter (1h-5h)
	optsDuration := GetFilteredLibraryItemsOptions{
		LibraryID: "lib_1",
		User:      &core.UserSession{ID: "user_1", CanAccessExplicitContent: true, AccessAllTags: true},
		MediaType: "book",
		FilterBy:  "duration.1h-5h",
	}
	resDuration, _, err := GetFilteredLibraryItems(db, optsDuration)
	if err != nil {
		t.Fatalf("Duration filter failed: %v", err)
	}
	if len(resDuration) != 1 || resDuration[0].ID != "li_1" {
		t.Errorf("Expected only li_1 for duration.1h-5h, got %d results", len(resDuration))
	}

	// Test case: Folder filter
	optsFolder := GetFilteredLibraryItemsOptions{
		LibraryID: "lib_1",
		User:      &core.UserSession{ID: "user_1", CanAccessExplicitContent: true, AccessAllTags: true},
		MediaType: "book",
		FilterBy:  "folder.f2",
	}
	resFolder, _, err := GetFilteredLibraryItems(db, optsFolder)
	if err != nil {
		t.Fatalf("Folder filter failed: %v", err)
	}
	if len(resFolder) != 1 || resFolder[0].ID != "li_2" {
		t.Errorf("Expected only li_2 for folder.f2, got %d results", len(resFolder))
	}
}
