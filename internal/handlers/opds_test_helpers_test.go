package handlers

import (
	"database/sql"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func prepareOPDSTestDB(t *testing.T) *sql.DB {
	db := setupTestDB(t)

	hashed, err := bcrypt.GenerateFromPassword([]byte("mypassword"), 8)
	if err != nil {
		t.Fatalf("Failed to hash password: %v", err)
	}

	queries := []string{
		"DROP TABLE IF EXISTS users",
		"CREATE TABLE users (id TEXT PRIMARY KEY, username TEXT, email TEXT, pash TEXT, type TEXT, token TEXT, isActive INTEGER, isLocked INTEGER, lastSeen TEXT, permissions TEXT, bookmarks TEXT, extraData TEXT, createdAt TEXT, updatedAt TEXT)",
		"DROP TABLE IF EXISTS authors",
		"CREATE TABLE authors (id TEXT PRIMARY KEY, name TEXT, description TEXT, libraryId TEXT)",
		"DROP TABLE IF EXISTS bookAuthors",
		"CREATE TABLE bookAuthors (bookId TEXT, authorId TEXT)",
		"DROP TABLE IF EXISTS series",
		"CREATE TABLE series (id TEXT PRIMARY KEY, name TEXT, description TEXT, libraryId TEXT)",
		"DROP TABLE IF EXISTS bookSeries",
		"CREATE TABLE bookSeries (bookId TEXT, seriesId TEXT, sequence TEXT)",
		"DROP TABLE IF EXISTS collections",
		"CREATE TABLE collections (id TEXT PRIMARY KEY, name TEXT NOT NULL, description TEXT, libraryId TEXT, isSmart INTEGER DEFAULT 0, rules TEXT)",
		"DROP TABLE IF EXISTS collectionBooks",
		"CREATE TABLE collectionBooks (id TEXT PRIMARY KEY, \"order\" INTEGER, bookId TEXT, collectionId TEXT)",
		"INSERT INTO libraries (id, name, displayOrder, icon, mediaType, provider, lastScan, lastScanVersion, settings, createdAt, updatedAt) VALUES ('lib-1', 'Audiobooks', 1, '', 'book', '', '2026-07-10T00:00:00Z', '', '{}', '2026-07-10T00:00:00Z', '2026-07-10T00:00:00Z')",
		"INSERT INTO libraryItems (id, ino, libraryId, path, relPath, isFile, mtime, ctime, birthtime, createdAt, updatedAt, isMissing, isInvalid, mediaType, mediaId, size, libraryFolderId, authorNamesFirstLast, authorNamesLastFirst, title, titleIgnorePrefix) VALUES ('item-1', '123', 'lib-1', '/books/book1', 'book1', 0, '123', '123', '123', '123', '1710000000000', 0, 0, 'book', 'book-1', 500, '', 'Author One', 'Author One', 'Test Book', 'Test Book')",
		"INSERT INTO books (id, title, titleIgnorePrefix, subtitle, publishedYear, publishedDate, publisher, description, isbn, asin, language, explicit, abridged, coverPath, duration, narrators, audioFiles, ebookFile, chapters, tags, genres, lockedFields) VALUES ('book-1', 'Test Book', 'Test Book', 'A Cool Test', '2025', '2025', 'Publisher X', 'A descriptive summary', '', '', 'en', 0, 0, '/covers/book1.jpg', 0.0, '[]', '[]', '{\"ebookFormat\": \"epub\", \"metadata\": {\"size\": 200}}', '[]', '[]', '[]', '[]')",
		"INSERT INTO authors (id, name, description, libraryId) VALUES ('author-1', 'Author One', 'Awesome Author', 'lib-1')",
		"INSERT INTO bookAuthors (bookId, authorId) VALUES ('book-1', 'author-1')",
		"INSERT INTO series (id, name, description, libraryId) VALUES ('series-1', 'Series One', 'Epic Series', 'lib-1')",
		"INSERT INTO bookSeries (bookId, seriesId, sequence) VALUES ('book-1', 'series-1', '1.0')",
		"INSERT INTO collections (id, name, description, libraryId) VALUES ('coll-1', 'Collection One', 'Super Collection', 'lib-1')",
		"INSERT INTO collectionBooks (id, \"order\", bookId, collectionId) VALUES ('cb-1', 1, 'book-1', 'coll-1')",
		"INSERT INTO playlists (id, name, description, libraryId, userId) VALUES ('play-1', 'Playlist One', 'Chill Playlist', 'lib-1', 'user-root')",
		"INSERT INTO playlistMediaItems (id, mediaItemId, mediaItemType, \"order\", playlistId) VALUES ('pmi-1', 'item-1', 'book', 1, 'play-1')",
	}

	for _, q := range queries {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("Failed to execute setup query %q: %v", q, err)
		}
	}

	_, err = db.Exec(`
		INSERT INTO users (id, username, type, pash, isActive, permissions) 
		VALUES ('user-root', 'admin-user', 'root', ?, 1, '{"accessAllLibraries": true}')
	`, string(hashed))
	if err != nil {
		t.Fatalf("Failed to insert test user: %v", err)
	}

	return db
}
