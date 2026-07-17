package playlist

import (
	"database/sql"
	"fmt"
	"sync/atomic"
	"testing"

	_ "modernc.org/sqlite"
)

var dbCounter int64

func setupDB(t *testing.T, withDisplayOrder bool) *sql.DB {
	id := atomic.AddInt64(&dbCounter, 1)
	dsn := fmt.Sprintf("file:memdb%d?mode=memory&cache=shared", id)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("failed to open in-memory sqlite db: %v", err)
	}
	db.SetMaxIdleConns(2)

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
	}

	for _, q := range queries {
		if _, err := db.Exec(q); err != nil {
			db.Close()
			t.Fatalf("failed to execute query %q: %v", q, err)
		}
	}

	// Create collections table depending on withDisplayOrder
	var createCollectionsQuery string
	if withDisplayOrder {
		createCollectionsQuery = `CREATE TABLE collections (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			description TEXT,
			createdAt TEXT,
			updatedAt TEXT,
			libraryId TEXT,
			displayOrder INTEGER,
			isSmart INTEGER DEFAULT 0,
			rules TEXT
		);`
	} else {
		createCollectionsQuery = `CREATE TABLE collections (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			description TEXT,
			createdAt TEXT,
			updatedAt TEXT,
			libraryId TEXT,
			isSmart INTEGER DEFAULT 0,
			rules TEXT
		);`
	}

	if _, err := db.Exec(createCollectionsQuery); err != nil {
		db.Close()
		t.Fatalf("failed to create collections table: %v", err)
	}

	return db
}
