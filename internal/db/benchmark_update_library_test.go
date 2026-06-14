package db_test

import (
	"database/sql"
	"fmt"
	"testing"

	"audiobookshelf/internal/db"
	_ "modernc.org/sqlite"
)

func BenchmarkUpdateLibrary(b *testing.B) {
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
	`)
	if err != nil {
		b.Fatalf("Failed to create tables: %v", err)
	}

	libraryID := "lib-1"
	_, err = database.Exec("INSERT INTO libraries (id, name, mediaType, settings) VALUES (?, ?, ?, ?)", libraryID, "Test Lib", "book", "{}")
	if err != nil {
		b.Fatalf("Failed to insert library: %v", err)
	}

	for i := 0; i < 100; i++ {
		folderID := fmt.Sprintf("folder-%d", i)
		_, err = database.Exec("INSERT INTO libraryFolders (id, libraryId, path) VALUES (?, ?, ?)", folderID, libraryID, fmt.Sprintf("/path/%d", i))
		if err != nil {
			b.Fatalf("Failed to insert folder: %v", err)
		}

		for j := 0; j < 10; j++ {
			itemID := fmt.Sprintf("item-%d-%d", i, j)
			mediaID := fmt.Sprintf("media-%d-%d", i, j)
			_, err = database.Exec("INSERT INTO libraryItems (id, libraryId, libraryFolderId, mediaId, mediaType) VALUES (?, ?, ?, ?, ?)", itemID, libraryID, folderID, mediaID, "book")
			if err != nil {
				b.Fatalf("Failed to insert item: %v", err)
			}
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// UpdateLibrary payload that deletes all folders
		name := "Updated Lib"
		folders := []db.UpdateFolderPayload{}
		payload := &db.UpdateLibraryPayload{
			Name:    &name,
			Folders: folders,
		}

		b.StartTimer()
		db.UpdateLibrary(database, libraryID, payload)
		b.StopTimer()

		// repopulate folders and items
		tx, _ := database.Begin()
		for k := 0; k < 100; k++ {
			folderID := fmt.Sprintf("folder-%d", k)
			tx.Exec("INSERT INTO libraryFolders (id, libraryId, path) VALUES (?, ?, ?)", folderID, libraryID, fmt.Sprintf("/path/%d", k))
			for j := 0; j < 10; j++ {
				itemID := fmt.Sprintf("item-%d-%d", k, j)
				mediaID := fmt.Sprintf("media-%d-%d", k, j)
				tx.Exec("INSERT INTO libraryItems (id, libraryId, libraryFolderId, mediaId, mediaType) VALUES (?, ?, ?, ?, ?)", itemID, libraryID, folderID, mediaID, "book")
			}
		}
		tx.Commit()
	}
}
