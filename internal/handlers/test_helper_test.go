package handlers

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func setupTestDB(t testing.TB) *sql.DB {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open memory db: %v", err)
	}
	db.SetMaxOpenConns(1)

	// Create tables
	queries := []string{
		`CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT, createdAt TEXT, updatedAt TEXT)`,
		`CREATE TABLE users (id TEXT PRIMARY KEY, username TEXT, type TEXT, isActive INTEGER, permissions TEXT, extraData TEXT)`,
		`CREATE TABLE apiKeys (id TEXT PRIMARY KEY, isActive INTEGER, expiresAt TEXT, userId TEXT, name TEXT, createdAt TEXT)`,
		`CREATE TABLE libraries (id TEXT PRIMARY KEY, name TEXT, displayOrder INTEGER, icon TEXT, mediaType TEXT, provider TEXT, lastScan TEXT, lastScanVersion TEXT, settings TEXT, createdAt TEXT, updatedAt TEXT)`,
		`CREATE TABLE libraryFolders (id TEXT PRIMARY KEY, path TEXT, libraryId TEXT, createdAt TEXT, updatedAt TEXT)`,
		`CREATE TABLE libraryItems (id TEXT PRIMARY KEY, ino TEXT, libraryId TEXT, path TEXT, relPath TEXT, isFile INTEGER, mtime TEXT, ctime TEXT, birthtime TEXT, createdAt TEXT, updatedAt TEXT, isMissing INTEGER, isInvalid INTEGER, mediaType TEXT, mediaId TEXT, size INTEGER, libraryFolderId TEXT, authorNamesFirstLast TEXT, authorNamesLastFirst TEXT, title TEXT, titleIgnorePrefix TEXT)`,
		`CREATE TABLE books (id TEXT PRIMARY KEY, title TEXT, titleIgnorePrefix TEXT, subtitle TEXT, publishedYear TEXT, publishedDate TEXT, publisher TEXT, description TEXT, isbn TEXT, asin TEXT, language TEXT, explicit INTEGER, abridged INTEGER, coverPath TEXT, duration REAL, narrators BLOB, audioFiles BLOB, ebookFile BLOB, chapters BLOB, tags BLOB, genres BLOB)`,
		`CREATE TABLE podcasts (id TEXT PRIMARY KEY, title TEXT, titleIgnorePrefix TEXT, author TEXT, releaseDate TEXT, feedURL TEXT, imageURL TEXT, description TEXT, itunesPageURL TEXT, itunesId TEXT, itunesArtistId TEXT, language TEXT, podcastType TEXT, explicit INTEGER, autoDownloadEpisodes INTEGER, autoDownloadSchedule TEXT, lastEpisodeCheck TEXT, maxEpisodesToKeep INTEGER, maxNewEpisodesToDownload INTEGER, coverPath TEXT, tags BLOB, genres BLOB, numEpisodes INTEGER)`,
		`CREATE TABLE bookSeries (bookId TEXT, seriesId TEXT, sequence TEXT)`,
		`CREATE TABLE series (id TEXT PRIMARY KEY, name TEXT)`,
		`CREATE TABLE mediaProgresses (id TEXT PRIMARY KEY, userId TEXT, mediaItemId TEXT, isFinished INTEGER, currentTime REAL, updatedAt TEXT)`,
		`CREATE TABLE playbackSessions (id TEXT PRIMARY KEY, userId TEXT, mediaItemId TEXT, mediaItemType TEXT, startTime REAL, libraryId TEXT, extraData TEXT, createdAt TEXT, updatedAt TEXT)`,
		`CREATE TABLE podcastEpisodes (id TEXT PRIMARY KEY, podcastId TEXT, title TEXT, audioFile TEXT)`,
		`CREATE TABLE playlists (id TEXT PRIMARY KEY, name TEXT NOT NULL, description TEXT, createdAt TEXT, updatedAt TEXT, libraryId TEXT, userId TEXT)`,
		`CREATE TABLE playlistMediaItems (id TEXT PRIMARY KEY, mediaItemId TEXT, mediaItemType TEXT, "order" INTEGER, createdAt TEXT, playlistId TEXT)`,
	}

	for _, q := range queries {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("Failed to execute query %q: %v", q, err)
		}
	}

	// Insert settings
	_, err = db.Exec(`INSERT INTO settings (key, value) VALUES ('server-settings', '{"sortingIgnorePrefix": true}')`)
	if err != nil {
		t.Fatalf("Failed to insert settings: %v", err)
	}

	return db
}
