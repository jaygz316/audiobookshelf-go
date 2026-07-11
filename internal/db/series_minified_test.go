package db

import (
	"database/sql"
	"fmt"
	"testing"

	"audiobookshelf/internal/core"
	_ "modernc.org/sqlite"
)

func TestGetSeriesMinified(t *testing.T) {
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("Failed to open test DB: %v", err)
	}
	defer db.Close()

	if err := bootstrapSchema(db); err != nil {
		t.Fatalf("Failed to bootstrap schema: %v", err)
	}

	// Insert Test Data
	// Library
	_, err = db.Exec(`INSERT INTO libraries (id, name, mediaType) VALUES ('lib_1', 'Test Library', 'book')`)
	if err != nil {
		t.Fatalf("Insert library: %v", err)
	}

	// Library Item
	_, err = db.Exec(`INSERT INTO libraryItems (id, ino, libraryId, libraryFolderId, path, relPath, isFile, mtime, ctime, birthtime, createdAt, updatedAt, isMissing, isInvalid, mediaType, mediaId, size) 
		VALUES ('item_1', '', 'lib_1', '', '', '', 0, '', '', '', '', '', 0, 0, 'book', 'book_1', 1000)`)
	if err != nil {
		t.Fatalf("Insert library item: %v", err)
	}

	// Book
	_, err = db.Exec(`INSERT INTO books (id, title, titleIgnorePrefix, subtitle, publishedYear, publishedDate, publisher, description, isbn, asin, language, explicit, abridged, coverPath, duration, narrators, audioFiles, ebookFile, chapters, tags, genres, lockedFields) 
		VALUES ('book_1', 'Test Book', '', '', '', '', '', '', '', '', '', 0, 0, '', 120.0, '["Narrator X"]', '[]', '[]', '[]', '[]', '[]', '[]')`)
	if err != nil {
		t.Fatalf("Insert book: %v", err)
	}

	// Series
	_, err = db.Exec(`INSERT INTO series (id, libraryId, name) VALUES ('series_1', 'lib_1', 'Test Series')`)
	if err != nil {
		t.Fatalf("Insert series: %v", err)
	}

	// BookSeries Relation
	_, err = db.Exec(`INSERT INTO bookSeries (bookId, seriesId, sequence) VALUES ('book_1', 'series_1', '1.5')`)
	if err != nil {
		t.Fatalf("Insert bookSeries: %v", err)
	}

	// Test 1: GetLibraryItemMinifiedByID
	minified, err := GetLibraryItemMinifiedByID(db, "item_1")
	if err != nil {
		t.Fatalf("GetLibraryItemMinifiedByID failed: %v", err)
	}

	if minified.Media == nil {
		t.Fatalf("Expected minified.Media to not be nil")
	}

	bookMin, ok := minified.Media.(*BookMinifiedJSON)
	if !ok {
		t.Fatalf("Expected Media to be *BookMinifiedJSON, got %T", minified.Media)
	}

	metadata := bookMin.Metadata
	if metadata.SeriesName != "Test Series #1.5" {
		t.Errorf("Expected SeriesName 'Test Series #1.5', got '%s'", metadata.SeriesName)
	}

	if metadata.SeriesSequence == nil || *metadata.SeriesSequence != "1.5" {
		t.Errorf("Expected SeriesSequence '1.5', got '%v'", metadata.SeriesSequence)
	}

	if len(metadata.Series) != 1 {
		t.Fatalf("Expected 1 series, got %d", len(metadata.Series))
	}

	s := metadata.Series[0]
	if s.ID != "series_1" || s.Name != "Test Series" || s.Sequence != "1.5" {
		t.Errorf("Unexpected series info: %+v", s)
	}

	// Test 2: GetFilteredLibraryItems
	opts := GetFilteredLibraryItemsOptions{
		LibraryID: "lib_1",
		User: &core.UserSession{
			ID: "user_1",
		},
		MediaType: "book",
	}
	// Add user row to make getUserPermissionWhere work if needed (actually permissions check might pass)
	_, _ = db.Exec(`INSERT INTO users (id, username, permissions) VALUES ('user_1', 'testuser', '[]')`)

	results, _, err := GetFilteredLibraryItems(db, opts)
	if err != nil {
		t.Fatalf("GetFilteredLibraryItems failed: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("Expected 1 result from GetFilteredLibraryItems, got %d", len(results))
	}

	bookMin2, ok := results[0].Media.(*BookMinifiedJSON)
	if !ok {
		t.Fatalf("Expected result Media to be *BookMinifiedJSON")
	}

	metadata2 := bookMin2.Metadata
	if metadata2.SeriesName != "Test Series #1.5" {
		t.Errorf("GetFilteredLibraryItems: Expected SeriesName 'Test Series #1.5', got '%s'", metadata2.SeriesName)
	}

	if metadata2.SeriesSequence == nil || *metadata2.SeriesSequence != "1.5" {
		t.Errorf("GetFilteredLibraryItems: Expected SeriesSequence '1.5', got '%v'", metadata2.SeriesSequence)
	}

	if len(metadata2.Series) != 1 {
		t.Fatalf("GetFilteredLibraryItems: Expected 1 series, got %d", len(metadata2.Series))
	}
}
