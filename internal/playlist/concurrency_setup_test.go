package playlist

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// helper to setup a real file-based SQLite database using WAL mode and busy_timeout, mimicking production.
func setupRealStressDB(t *testing.T, maxOpenConns int) (*sql.DB, string) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "stress_test.db")

	// Same DSN as internal/db/db.go InitDB
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode=WAL&_pragma=busy_timeout=5000", dbPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("failed to open sqlite DB: %v", err)
	}

	if err := db.Ping(); err != nil {
		db.Close()
		t.Fatalf("failed to ping sqlite DB: %v", err)
	}

	db.SetMaxOpenConns(maxOpenConns)
	db.SetMaxIdleConns(maxOpenConns)

	// Create tables
	queries := []string{
		`CREATE TABLE playlists (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			description TEXT,
			createdAt TEXT,
			updatedAt TEXT,
			libraryId TEXT,
			userId TEXT
		);`,
		`CREATE TABLE playlistMediaItems (
			id TEXT PRIMARY KEY,
			mediaItemId TEXT,
			mediaItemType TEXT,
			"order" INTEGER,
			createdAt TEXT,
			playlistId TEXT
		);`,
		`CREATE TABLE collectionBooks (
			id TEXT PRIMARY KEY,
			"order" INTEGER,
			createdAt TEXT,
			bookId TEXT,
			collectionId TEXT
		);`,
		`CREATE TABLE books (
			id TEXT PRIMARY KEY,
			title TEXT,
			subtitle TEXT,
			description TEXT,
			genres TEXT,
			tags TEXT,
			narrators TEXT,
			publishedYear TEXT
		);`,
		`CREATE TABLE podcastEpisodes (
			id TEXT PRIMARY KEY
		);`,
		`CREATE TABLE libraryItems (
			id TEXT PRIMARY KEY,
			libraryId TEXT,
			mediaId TEXT,
			mediaType TEXT,
			isMissing INTEGER DEFAULT 0,
			isInvalid INTEGER DEFAULT 0
		);`,
		`CREATE TABLE bookAuthors (
			bookId TEXT,
			authorId TEXT
		);`,
		`CREATE TABLE authors (
			id TEXT PRIMARY KEY,
			name TEXT
		);`,
		`CREATE TABLE bookSeries (
			bookId TEXT,
			seriesId TEXT
		);`,
		`CREATE TABLE series (
			id TEXT PRIMARY KEY,
			name TEXT
		);`,
		`CREATE TABLE collections (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			description TEXT,
			createdAt TEXT,
			updatedAt TEXT,
			libraryId TEXT,
			displayOrder INTEGER,
			isSmart INTEGER DEFAULT 0,
			rules TEXT
		);`,
	}

	for _, q := range queries {
		if _, err := db.Exec(q); err != nil {
			db.Close()
			t.Fatalf("failed to execute query %q: %v", q, err)
		}
	}

	return db, dbPath
}
