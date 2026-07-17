package db

import (
	"database/sql"
	"encoding/base64"
	"testing"

	"audiobookshelf/internal/core"
	_ "modernc.org/sqlite"
)

func TestAdversarial_PodcastProgressFiltering(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open DB: %v", err)
	}
	defer db.Close()

	if err := bootstrapSchema(db); err != nil {
		t.Fatalf("Failed to bootstrap schema: %v", err)
	}

	// Insert active user
	_, _ = db.Exec(`INSERT INTO users (id, username, permissions) VALUES ('user_1', 'testuser', '[]')`)

	// Insert Library
	_, _ = db.Exec(`INSERT INTO libraries (id, name, mediaType) VALUES ('lib_podcast', 'Podcast Library', 'podcast')`)

	// Insert Podcast
	_, _ = db.Exec(`INSERT INTO podcasts (id, title, explicit) VALUES ('podcast_1', 'Test Podcast 1', 0)`)
	_, _ = db.Exec(`INSERT INTO libraryItems (id, libraryId, mediaType, mediaId, title, isMissing, isInvalid) 
		VALUES ('li_pod_1', 'lib_podcast', 'podcast', 'podcast_1', 'Test Podcast 1', 0, 0)`)

	// Insert Episode
	_, _ = db.Exec(`INSERT INTO podcastEpisodes (id, podcastId, title) VALUES ('ep_1', 'podcast_1', 'Episode 1')`)

	// Insert progress for Episode 1 (current time > 0, not finished)
	// Notice mediaItemId is 'ep_1' and podcastId is 'podcast_1'
	_, _ = db.Exec(`INSERT INTO mediaProgresses (id, userId, mediaItemId, mediaItemType, duration, currentTime, isFinished, podcastId, updatedAt) 
		VALUES ('prog_ep_1', 'user_1', 'ep_1', 'podcastEpisode', 1000.0, 500.0, 0, 'podcast_1', '2026-07-16 12:00:00')`)

	// Test case: Filter by progress.in-progress
	optsProgress := GetFilteredLibraryItemsOptions{
		LibraryID: "lib_podcast",
		User:      &core.UserSession{ID: "user_1", CanAccessExplicitContent: true, AccessAllTags: true},
		MediaType: "podcast",
		FilterBy:  "progress.in-progress",
	}

	results, _, err := GetFilteredLibraryItems(db, optsProgress)
	if err != nil {
		t.Fatalf("Failed to get filtered library items: %v", err)
	}

	if len(results) == 0 {
		t.Error("Podcast filtering by progress returned 0 results! Expected at least 1.")
	} else {
		t.Logf("Found %d podcast items in progress.", len(results))
	}
}

func TestAdversarial_BookProgressInconsistency(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open DB: %v", err)
	}
	defer db.Close()

	if err := bootstrapSchema(db); err != nil {
		t.Fatalf("Failed to bootstrap schema: %v", err)
	}

	// Insert active user
	_, _ = db.Exec(`INSERT INTO users (id, username, permissions) VALUES ('user_1', 'testuser', '[]')`)

	// Insert Library
	_, _ = db.Exec(`INSERT INTO libraries (id, name, mediaType) VALUES ('lib_book', 'Book Library', 'book')`)

	// Insert Library Items & Books
	// Case 1: Progress mediaItemId stored as libraryItem ID (e.g. 'li_book_1')
	_, _ = db.Exec(`INSERT INTO books (id, title, explicit) VALUES ('book_1', 'Book One', 0)`)
	_, _ = db.Exec(`INSERT INTO libraryItems (id, libraryId, mediaType, mediaId, title, isMissing, isInvalid) 
		VALUES ('li_book_1', 'lib_book', 'book', 'book_1', 'Book One', 0, 0)`)
	_, _ = db.Exec(`INSERT INTO mediaProgresses (id, userId, mediaItemId, duration, currentTime, isFinished, updatedAt) 
		VALUES ('prog_1', 'user_1', 'li_book_1', 1000.0, 500.0, 0, '2026-07-16 12:00:00')`)

	// Case 2: Progress mediaItemId stored as book ID (e.g. 'book_2')
	_, _ = db.Exec(`INSERT INTO books (id, title, explicit) VALUES ('book_2', 'Book Two', 0)`)
	_, _ = db.Exec(`INSERT INTO libraryItems (id, libraryId, mediaType, mediaId, title, isMissing, isInvalid) 
		VALUES ('li_book_2', 'lib_book', 'book', 'book_2', 'Book Two', 0, 0)`)
	_, _ = db.Exec(`INSERT INTO mediaProgresses (id, userId, mediaItemId, duration, currentTime, isFinished, updatedAt) 
		VALUES ('prog_2', 'user_1', 'book_2', 1000.0, 500.0, 0, '2026-07-16 12:01:00')`)

	// Let's see which books get matched under the new db package GetFilteredLibraryItems
	optsProgress := GetFilteredLibraryItemsOptions{
		LibraryID: "lib_book",
		User:      &core.UserSession{ID: "user_1", CanAccessExplicitContent: true, AccessAllTags: true},
		MediaType: "book",
		FilterBy:  "progress.in-progress",
	}

	results, _, err := GetFilteredLibraryItems(db, optsProgress)
	if err != nil {
		t.Fatalf("Failed to get filtered library items: %v", err)
	}

	t.Logf("GetFilteredLibraryItems with FilterBy: progress.in-progress returned %d results:", len(results))
	matchedLiBook1 := false
	matchedBook2 := false
	for _, item := range results {
		t.Logf("- Item ID: %s, Media ID: %s", item.ID, item.Media.(*BookMinifiedJSON).ID)
		if item.ID == "li_book_1" {
			matchedLiBook1 = true
		}
		if item.ID == "li_book_2" {
			matchedBook2 = true
		}
	}

	if matchedLiBook1 {
		t.Log("Matched li_book_1 (where mediaItemId = libraryItem.id)")
	}
	if matchedBook2 {
		t.Log("Matched li_book_2 (where mediaItemId = book.id)")
	}
}

func TestAdversarial_SQLInjectionFilterBy(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open DB: %v", err)
	}
	defer db.Close()

	if err := bootstrapSchema(db); err != nil {
		t.Fatalf("Failed to bootstrap schema: %v", err)
	}

	// Insert active user
	_, _ = db.Exec(`INSERT INTO users (id, username, permissions) VALUES ('user_1', 'testuser', '[]')`)

	// Insert Library
	_, _ = db.Exec(`INSERT INTO libraries (id, name, mediaType) VALUES ('lib_book', 'Book Library', 'book')`)

	// Attempt SQL injection via filter value
	// If decoded base64 contains SQL Injection payload
	payload := base64.StdEncoding.EncodeToString([]byte("123; DROP TABLE books;--"))
	opts := GetFilteredLibraryItemsOptions{
		LibraryID: "lib_book",
		User:      &core.UserSession{ID: "user_1", CanAccessExplicitContent: true, AccessAllTags: true},
		MediaType: "book",
		FilterBy:  "years." + payload,
	}

	_, _, err = GetFilteredLibraryItems(db, opts)
	if err != nil {
		t.Logf("SQL injection query error (expected/graceful safety or error): %v", err)
	}

	// Check if table still exists
	var count int
	err = db.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='table' AND name='books'").Scan(&count)
	if err != nil {
		t.Fatalf("Failed to check books table existence: %v", err)
	}
	if count == 0 {
		t.Error("CRITICAL: Books table was dropped! SQL injection succeeded.")
	} else {
		t.Log("Books table exists. SQL Injection payload was parameterized safely.")
	}
}

func TestAdversarial_PodcastAutoDeletePlayed(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open DB: %v", err)
	}
	defer db.Close()

	if err := bootstrapSchema(db); err != nil {
		t.Fatalf("Failed to bootstrap schema: %v", err)
	}

	// Insert active user
	_, _ = db.Exec(`INSERT INTO users (id, username, permissions) VALUES ('user_1', 'testuser', '[]')`)

	// Insert Library
	_, _ = db.Exec(`INSERT INTO libraries (id, name, mediaType) VALUES ('lib_pod', 'Podcast Library', 'podcast')`)

	// Insert Podcast with autoDeletePlayed = 1
	_, _ = db.Exec(`INSERT INTO podcasts (id, title, autoDeletePlayed, explicit) VALUES ('podcast_1', 'Test Podcast 1', 1, 0)`)
	_, _ = db.Exec(`INSERT INTO libraryItems (id, libraryId, mediaType, mediaId, title, isMissing, isInvalid) 
		VALUES ('li_pod_1', 'lib_pod', 'podcast', 'podcast_1', 'Test Podcast 1', 0, 0)`)

	opts := GetFilteredLibraryItemsOptions{
		LibraryID: "lib_pod",
		User:      &core.UserSession{ID: "user_1", CanAccessExplicitContent: true, AccessAllTags: true},
		MediaType: "podcast",
	}

	results, _, err := GetFilteredLibraryItems(db, opts)
	if err != nil {
		t.Fatalf("Failed to get filtered library items: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("Expected 1 podcast result, got %d", len(results))
	}

	podMin, ok := results[0].Media.(*PodcastMinifiedJSON)
	if !ok {
		t.Fatalf("Expected media to be *PodcastMinifiedJSON, got %T", results[0].Media)
	}

	if !podMin.AutoDeletePlayed {
		t.Error("AutoDeletePlayed is false in the returned results, but it was set to true (1) in the database!")
	} else {
		t.Log("PASS: AutoDeletePlayed is correctly true.")
	}
}
