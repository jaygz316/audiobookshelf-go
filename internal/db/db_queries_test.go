package db

import (
	"database/sql"
	"fmt"
	"testing"

	_ "modernc.org/sqlite"
)

func setupTestDB(t *testing.T) *sql.DB {
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("Failed to open shared memory db: %v", err)
	}

	queries := []string{
		`CREATE TABLE IF NOT EXISTS libraryItems (
			id TEXT PRIMARY KEY,
			libraryId TEXT,
			mediaType TEXT,
			mediaId TEXT,
			size INTEGER
		)`,
		`CREATE TABLE IF NOT EXISTS podcasts (
			id TEXT PRIMARY KEY
		)`,
		`CREATE TABLE IF NOT EXISTS podcastEpisodes (
			id TEXT PRIMARY KEY,
			podcastId TEXT,
			audioFile TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS books (
			id TEXT PRIMARY KEY,
			duration REAL,
			audioFiles BLOB
		)`,
	}

	for _, query := range queries {
		if _, err := db.Exec(query); err != nil {
			t.Fatalf("Failed to execute setup query %s: %v", query, err)
		}
	}

	return db
}

func TestGetPodcastLibraryStats(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Empty library test
	stats, err := GetPodcastLibraryStats(db, "lib_empty")
	if err != nil {
		t.Fatalf("Expected no error for empty library, got %v", err)
	}
	if stats.TotalSize != 0 || stats.TotalDuration != 0 || stats.TotalItems != 0 || stats.NumAudioFiles != 0 {
		t.Errorf("Expected zeros for empty library stats, got %+v", stats)
	}

	// Insert test data
	queries := []string{
		`INSERT INTO libraryItems (id, libraryId, mediaType, mediaId, size) VALUES ('li_1', 'lib_podcast', 'podcast', 'pod_1', 1000)`,
		`INSERT INTO libraryItems (id, libraryId, mediaType, mediaId, size) VALUES ('li_2', 'lib_podcast', 'podcast', 'pod_2', 2500)`,
		`INSERT INTO podcasts (id) VALUES ('pod_1')`,
		`INSERT INTO podcasts (id) VALUES ('pod_2')`,
		`INSERT INTO podcastEpisodes (id, podcastId, audioFile) VALUES ('ep_1', 'pod_1', '{"duration": 120.5}')`,
		`INSERT INTO podcastEpisodes (id, podcastId, audioFile) VALUES ('ep_2', 'pod_1', '{"duration": 150.0}')`,
		`INSERT INTO podcastEpisodes (id, podcastId, audioFile) VALUES ('ep_3', 'pod_2', '{"duration": 300.25}')`,
	}
	for _, q := range queries {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("Failed to execute query %s: %v", q, err)
		}
	}

	// Populated library test
	stats, err = GetPodcastLibraryStats(db, "lib_podcast")
	if err != nil {
		t.Fatalf("Expected no error for populated library, got %v", err)
	}
	if stats.TotalSize != 3500 {
		t.Errorf("Expected TotalSize=3500, got %d", stats.TotalSize)
	}
	if stats.TotalDuration != 570.75 {
		t.Errorf("Expected TotalDuration=570.75, got %f", stats.TotalDuration)
	}
	if stats.TotalItems != 2 {
		t.Errorf("Expected TotalItems=2, got %d", stats.TotalItems)
	}
	if stats.NumAudioFiles != 3 {
		t.Errorf("Expected NumAudioFiles=3, got %d", stats.NumAudioFiles)
	}
}

func TestGetBookLibraryStats(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Empty library test
	stats, err := GetBookLibraryStats(db, "lib_empty")
	if err != nil {
		t.Fatalf("Expected no error for empty library, got %v", err)
	}
	if stats.TotalSize != 0 || stats.TotalDuration != 0 || stats.TotalItems != 0 || stats.NumAudioFiles != 0 {
		t.Errorf("Expected zeros for empty library stats, got %+v", stats)
	}

	// Insert test data
	queries := []string{
		`INSERT INTO libraryItems (id, libraryId, mediaType, mediaId, size) VALUES ('li_3', 'lib_book', 'book', 'book_1', 4000)`,
		`INSERT INTO libraryItems (id, libraryId, mediaType, mediaId, size) VALUES ('li_4', 'lib_book', 'book', 'book_2', 1500)`,
		`INSERT INTO books (id, duration, audioFiles) VALUES ('book_1', 3600.5, '[{"id": 1}, {"id": 2}]')`,
		`INSERT INTO books (id, duration, audioFiles) VALUES ('book_2', 1800.0, '[{"id": 3}]')`,
	}
	for _, q := range queries {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("Failed to execute query %s: %v", q, err)
		}
	}

	// Populated library test
	stats, err = GetBookLibraryStats(db, "lib_book")
	if err != nil {
		t.Fatalf("Expected no error for populated library, got %v", err)
	}
	if stats.TotalSize != 5500 {
		t.Errorf("Expected TotalSize=5500, got %d", stats.TotalSize)
	}
	if stats.TotalDuration != 5400.5 {
		t.Errorf("Expected TotalDuration=5400.5, got %f", stats.TotalDuration)
	}
	if stats.TotalItems != 2 {
		t.Errorf("Expected TotalItems=2, got %d", stats.TotalItems)
	}
	if stats.NumAudioFiles != 3 {
		t.Errorf("Expected NumAudioFiles=3, got %d", stats.NumAudioFiles)
	}
}
