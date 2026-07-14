package handlers

import (
	"testing"

	"audiobookshelf/internal/core"
)

func TestFetchContinueSeriesShelf(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	user := &core.UserSession{
		ID:                       "user_1",
		CanAccessExplicitContent: true,
	}
	libraryID := "lib_1"

	// 1. Setup library, items, and series
	_, err := db.Exec(`INSERT INTO libraries (id, name, mediaType) VALUES (?, 'Test Lib', 'book')`, libraryID)
	if err != nil {
		t.Fatalf("Failed to insert library: %v", err)
	}

	_, err = db.Exec(`INSERT INTO series (id, name) VALUES ('series_1', 'Test Series')`)
	if err != nil {
		t.Fatalf("Failed to insert series: %v", err)
	}

	// Insert books
	books := []struct {
		id    string
		title string
		seq   string
	}{
		{"book_1", "Book One", "1"},
		{"book_2", "Book Two", "2"},
		{"book_3", "Book Three", "3"},
	}

	for _, b := range books {
		_, err = db.Exec(`INSERT INTO books (id, title, explicit) VALUES (?, ?, 0)`, b.id, b.title)
		if err != nil {
			t.Fatalf("Failed to insert book %s: %v", b.id, err)
		}

		// Insert libraryItem for the book
		// Note: libraryItems.id matches what is stored in mediaProgresses.mediaItemId
		liID := "item_" + b.id
		_, err = db.Exec(`
			INSERT INTO libraryItems (id, libraryId, mediaType, mediaId, title, isMissing, isInvalid) 
			VALUES (?, ?, 'book', ?, ?, 0, 0)`,
			liID, libraryID, b.id, b.title)
		if err != nil {
			t.Fatalf("Failed to insert library item for book %s: %v", b.id, err)
		}

		_, err = db.Exec(`INSERT INTO bookSeries (bookId, seriesId, sequence) VALUES (?, 'series_1', ?)`, b.id, b.seq)
		if err != nil {
			t.Fatalf("Failed to insert bookSeries mapping: %v", err)
		}
	}

	// 2. Fetch with no finished books -> should be nil
	shelf, err := fetchContinueSeriesShelf(db, libraryID, user, 10)
	if err != nil {
		t.Fatalf("fetchContinueSeriesShelf error: %v", err)
	}
	if shelf != nil {
		t.Fatalf("Expected no continue series shelf, got shelf with %d items", len(shelf.Entities))
	}

	// 3. Mark book_1 finished
	_, err = db.Exec(`
		INSERT INTO mediaProgresses (id, userId, mediaItemId, isFinished, currentTime, updatedAt) 
		VALUES ('p_1', 'user_1', 'item_book_1', 1, 100.0, '2026-07-14T00:00:00Z')`)
	if err != nil {
		t.Fatalf("Failed to insert progress for book_1: %v", err)
	}

	// Fetch -> should return Book Two
	shelf, err = fetchContinueSeriesShelf(db, libraryID, user, 10)
	if err != nil {
		t.Fatalf("fetchContinueSeriesShelf error: %v", err)
	}
	if shelf == nil {
		t.Fatalf("Expected continue series shelf, got nil")
	}
	if len(shelf.Entities) != 1 {
		t.Fatalf("Expected 1 item on shelf, got %d", len(shelf.Entities))
	}
	if shelf.Entities[0].ID != "item_book_2" {
		t.Fatalf("Expected item_book_2 on shelf, got %s", shelf.Entities[0].ID)
	}

	// 4. Mark book_2 finished
	_, err = db.Exec(`
		INSERT INTO mediaProgresses (id, userId, mediaItemId, isFinished, currentTime, updatedAt) 
		VALUES ('p_2', 'user_1', 'item_book_2', 1, 100.0, '2026-07-14T00:01:00Z')`)
	if err != nil {
		t.Fatalf("Failed to insert progress for book_2: %v", err)
	}

	// Fetch -> should return Book Three
	shelf, err = fetchContinueSeriesShelf(db, libraryID, user, 10)
	if err != nil {
		t.Fatalf("fetchContinueSeriesShelf error: %v", err)
	}
	if shelf == nil {
		t.Fatalf("Expected continue series shelf, got nil")
	}
	if len(shelf.Entities) != 1 {
		t.Fatalf("Expected 1 item on shelf, got %d", len(shelf.Entities))
	}
	if shelf.Entities[0].ID != "item_book_3" {
		t.Fatalf("Expected item_book_3 on shelf, got %s", shelf.Entities[0].ID)
	}

	// 5. Mark book_3 finished
	_, err = db.Exec(`
		INSERT INTO mediaProgresses (id, userId, mediaItemId, isFinished, currentTime, updatedAt) 
		VALUES ('p_3', 'user_1', 'item_book_3', 1, 100.0, '2026-07-14T00:02:00Z')`)
	if err != nil {
		t.Fatalf("Failed to insert progress for book_3: %v", err)
	}

	// Fetch -> should be nil (all finished)
	shelf, err = fetchContinueSeriesShelf(db, libraryID, user, 10)
	if err != nil {
		t.Fatalf("fetchContinueSeriesShelf error: %v", err)
	}
	if shelf != nil {
		t.Fatalf("Expected no continue series shelf after all books finished, got shelf with %d items", len(shelf.Entities))
	}
}
