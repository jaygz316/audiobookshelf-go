package db

import (
	"database/sql"
	"fmt"
	"testing"

	"audiobookshelf/internal/core"
	_ "modernc.org/sqlite"
)

func TestGetFilteredLibraryItems_SortByProgress(t *testing.T) {
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("Failed to open test DB: %v", err)
	}
	defer db.Close()

	if err := bootstrapSchema(db); err != nil {
		t.Fatalf("Failed to bootstrap schema: %v", err)
	}

	// Insert active user
	_, err = db.Exec(`INSERT INTO users (id, username, permissions) VALUES ('user_1', 'testuser', '[]')`)
	if err != nil {
		t.Fatalf("Insert user: %v", err)
	}

	// Insert Library
	_, err = db.Exec(`INSERT INTO libraries (id, name, mediaType) VALUES ('lib_1', 'Test Library', 'book')`)
	if err != nil {
		t.Fatalf("Insert library: %v", err)
	}

	// Insert Library Items
	_, err = db.Exec(`INSERT INTO libraryItems (id, ino, libraryId, libraryFolderId, path, relPath, isFile, mtime, ctime, birthtime, createdAt, updatedAt, isMissing, isInvalid, mediaType, mediaId, size) 
		VALUES ('item_1', '', 'lib_1', '', '', '', 0, '', '', '', '', '', 0, 0, 'book', 'book_1', 1000)`)
	if err != nil {
		t.Fatalf("Insert library item 1: %v", err)
	}

	_, err = db.Exec(`INSERT INTO libraryItems (id, ino, libraryId, libraryFolderId, path, relPath, isFile, mtime, ctime, birthtime, createdAt, updatedAt, isMissing, isInvalid, mediaType, mediaId, size) 
		VALUES ('item_2', '', 'lib_1', '', '', '', 0, '', '', '', '', '', 0, 0, 'book', 'book_2', 1000)`)
	if err != nil {
		t.Fatalf("Insert library item 2: %v", err)
	}

	// Insert Books
	_, err = db.Exec(`INSERT INTO books (id, title, titleIgnorePrefix, subtitle, publishedYear, publishedDate, publisher, description, isbn, asin, language, explicit, abridged, coverPath, duration, narrators, audioFiles, ebookFile, chapters, tags, genres, lockedFields) 
		VALUES ('book_1', 'Book One', '', '', '', '', '', '', '', '', '', 0, 0, '', 120.0, '[]', '[]', '[]', '[]', '[]', '[]', '[]')`)
	if err != nil {
		t.Fatalf("Insert book 1: %v", err)
	}

	_, err = db.Exec(`INSERT INTO books (id, title, titleIgnorePrefix, subtitle, publishedYear, publishedDate, publisher, description, isbn, asin, language, explicit, abridged, coverPath, duration, narrators, audioFiles, ebookFile, chapters, tags, genres, lockedFields) 
		VALUES ('book_2', 'Book Two', '', '', '', '', '', '', '', '', '', 0, 0, '', 120.0, '[]', '[]', '[]', '[]', '[]', '[]', '[]')`)
	if err != nil {
		t.Fatalf("Insert book 2: %v", err)
	}

	// Insert Progress
	// book_1: older progress (updatedAt: earlier)
	// book_2: newer progress (updatedAt: later)
	_, err = db.Exec(`INSERT INTO mediaProgresses (id, userId, mediaItemId, duration, currentTime, isFinished, updatedAt) 
		VALUES ('prog_1', 'user_1', 'book_1', 120.0, 10.0, 0, '2026-07-01 12:00:00')`)
	if err != nil {
		t.Fatalf("Insert progress 1: %v", err)
	}

	_, err = db.Exec(`INSERT INTO mediaProgresses (id, userId, mediaItemId, duration, currentTime, isFinished, updatedAt) 
		VALUES ('prog_2', 'user_1', 'book_2', 120.0, 20.0, 0, '2026-07-02 12:00:00')`)
	if err != nil {
		t.Fatalf("Insert progress 2: %v", err)
	}

	// Test Sort DESC (Newest progress first: book_2 then book_1)
	optsDesc := GetFilteredLibraryItemsOptions{
		LibraryID: "lib_1",
		User: &core.UserSession{
			ID: "user_1",
		},
		MediaType: "book",
		SortBy:    "progress",
		SortDesc:  true,
	}

	resultsDesc, _, err := GetFilteredLibraryItems(db, optsDesc)
	if err != nil {
		t.Fatalf("GetFilteredLibraryItems DESC failed: %v", err)
	}

	if len(resultsDesc) != 2 {
		t.Fatalf("Expected 2 results, got %d", len(resultsDesc))
	}

	if resultsDesc[0].ID != "item_2" || resultsDesc[1].ID != "item_1" {
		t.Errorf("Expected item_2 first (newer progress), then item_1. Got order: %s, %s", resultsDesc[0].ID, resultsDesc[1].ID)
	}

	// Test Sort ASC (Oldest progress first: book_1 then book_2)
	optsAsc := GetFilteredLibraryItemsOptions{
		LibraryID: "lib_1",
		User: &core.UserSession{
			ID: "user_1",
		},
		MediaType: "book",
		SortBy:    "progress",
		SortDesc:  false,
	}

	resultsAsc, _, err := GetFilteredLibraryItems(db, optsAsc)
	if err != nil {
		t.Fatalf("GetFilteredLibraryItems ASC failed: %v", err)
	}

	if len(resultsAsc) != 2 {
		t.Fatalf("Expected 2 results, got %d", len(resultsAsc))
	}

	if resultsAsc[0].ID != "item_1" || resultsAsc[1].ID != "item_2" {
		t.Errorf("Expected item_1 first (older progress), then item_2. Got order: %s, %s", resultsAsc[0].ID, resultsAsc[1].ID)
	}
}
