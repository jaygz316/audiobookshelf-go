package db_test

import (
	"database/sql"
	"fmt"
	"testing"

	"audiobookshelf/internal/db"
	_ "modernc.org/sqlite"
)

func BenchmarkDeleteLibrary(b *testing.B) {
	database, err := sql.Open("sqlite", fmt.Sprintf("file:%s?mode=memory&cache=shared", b.Name()))
	if err != nil {
		b.Fatalf("Failed to open db: %v", err)
	}
	defer database.Close()

	// Initial setup schema
	_, err = database.Exec(`
		CREATE TABLE libraries (id TEXT PRIMARY KEY, name TEXT, mediaType TEXT, settings TEXT, provider TEXT, displayOrder INTEGER, icon TEXT, createdAt TEXT, updatedAt TEXT);
		CREATE TABLE libraryFolders (id TEXT PRIMARY KEY, path TEXT, libraryId TEXT, createdAt TEXT, updatedAt TEXT);
		CREATE TABLE libraryItems (id TEXT PRIMARY KEY, libraryId TEXT, libraryFolderId TEXT, mediaId TEXT, mediaType TEXT);
		CREATE TABLE mediaProgresses (id TEXT PRIMARY KEY, mediaItemId TEXT);
		CREATE TABLE playlistItems (id TEXT PRIMARY KEY, libraryItemId TEXT);
		CREATE TABLE books (id TEXT PRIMARY KEY);
		CREATE TABLE podcasts (id TEXT PRIMARY KEY);
		CREATE TABLE bookAuthors (bookId TEXT, authorId TEXT);
		CREATE TABLE bookSeries (bookId TEXT, seriesId TEXT);
		CREATE TABLE authors (id TEXT PRIMARY KEY, asin TEXT, description TEXT, imagePath TEXT);
		CREATE TABLE series (id TEXT PRIMARY KEY);
		CREATE TABLE collections (id TEXT PRIMARY KEY, libraryId TEXT);
		CREATE TABLE playbackSessions (id TEXT PRIMARY KEY, libraryId TEXT);
	`)
	if err != nil {
		b.Fatalf("Failed to create tables: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		libraryID := fmt.Sprintf("lib-%d", i)
		_, err = database.Exec("INSERT INTO libraries (id, name, mediaType, settings) VALUES (?, ?, ?, ?)", libraryID, "Test Lib", "book", "{}")

		tx, _ := database.Begin()
		for k := 0; k < 100; k++ {
			folderID := fmt.Sprintf("folder-%d-%d", i, k)
			tx.Exec("INSERT INTO libraryFolders (id, libraryId, path) VALUES (?, ?, ?)", folderID, libraryID, fmt.Sprintf("/path/%d", k))
			for j := 0; j < 10; j++ {
				itemID := fmt.Sprintf("item-%d-%d-%d", i, k, j)
				mediaID := fmt.Sprintf("media-%d-%d-%d", i, k, j)
				tx.Exec("INSERT INTO libraryItems (id, libraryId, libraryFolderId, mediaId, mediaType) VALUES (?, ?, ?, ?, ?)", itemID, libraryID, folderID, mediaID, "book")
			}
		}
		tx.Commit()

		b.StartTimer()
		db.DeleteLibrary(database, libraryID)
	}
}
