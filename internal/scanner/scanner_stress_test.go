package scanner

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func setupStressTestDB(t *testing.T, dbPath string) *sql.DB {
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode=WAL&_pragma=busy_timeout=5000", dbPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}

	schema := []string{
		`CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT, createdAt TEXT, updatedAt TEXT);`,
		`CREATE TABLE libraries (id TEXT PRIMARY KEY, name TEXT, mediaType TEXT, settings TEXT, provider TEXT, displayOrder INTEGER, createdAt TEXT, updatedAt TEXT);`,
		`CREATE TABLE libraryFolders (id TEXT PRIMARY KEY, path TEXT, libraryId TEXT, createdAt TEXT, updatedAt TEXT);`,
		`CREATE TABLE libraryItems (id TEXT PRIMARY KEY, ino TEXT, libraryId TEXT, path TEXT, relPath TEXT, isFile INTEGER, mtime TEXT, ctime TEXT, birthtime TEXT, createdAt TEXT, updatedAt TEXT, isMissing INTEGER, isInvalid INTEGER, mediaType TEXT, mediaId TEXT, size INTEGER, libraryFolderId TEXT, authorNamesFirstLast TEXT, authorNamesLastFirst TEXT, title TEXT, titleIgnorePrefix TEXT);`,
		`CREATE TABLE books (id TEXT PRIMARY KEY, title TEXT, titleIgnorePrefix TEXT, subtitle TEXT, publishedYear TEXT, publishedDate TEXT, publisher TEXT, description TEXT, isbn TEXT, asin TEXT, language TEXT, explicit INTEGER, abridged INTEGER, coverPath TEXT, duration REAL, narrators TEXT, audioFiles TEXT, ebookFile TEXT, chapters TEXT, tags TEXT, genres TEXT, lockedFields TEXT);`,
		`CREATE TABLE authors (id TEXT PRIMARY KEY, name TEXT, lastFirst TEXT, libraryId TEXT, createdAt TEXT, updatedAt TEXT);`,
		`CREATE TABLE bookAuthors (bookId TEXT, authorId TEXT, createdAt TEXT, updatedAt TEXT, PRIMARY KEY (bookId, authorId));`,
		`CREATE TABLE series (id TEXT PRIMARY KEY, name TEXT, libraryId TEXT, createdAt TEXT, updatedAt TEXT);`,
		`CREATE TABLE bookSeries (bookId TEXT, seriesId TEXT, sequence TEXT, createdAt TEXT, updatedAt TEXT, PRIMARY KEY (bookId, seriesId));`,
		`CREATE TABLE podcasts (id TEXT PRIMARY KEY, title TEXT, titleIgnorePrefix TEXT, author TEXT, releaseDate TEXT, feedURL TEXT, imageURL TEXT, description TEXT, itunesPageURL TEXT, itunesId TEXT, itunesArtistId TEXT, language TEXT, podcastType TEXT, explicit INTEGER, autoDownloadEpisodes INTEGER, autoDownloadSchedule TEXT, lastEpisodeCheck TEXT, maxEpisodesToKeep INTEGER, maxNewEpisodesToDownload INTEGER, coverPath TEXT, tags TEXT, genres TEXT, numEpisodes INTEGER, lockedFields TEXT);`,
		`CREATE TABLE podcastEpisodes (id TEXT PRIMARY KEY, podcastId TEXT, title TEXT, audioFile TEXT);`,
	}

	for _, q := range schema {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("failed to execute schema query: %v", err)
		}
	}

	// Insert default server settings
	serverSettingsJSON := `{"sortingPrefixes":["the","a","an"]}`
	_, err = db.Exec("INSERT INTO settings (key, value) VALUES ('server-settings', ?)", serverSettingsJSON)
	if err != nil {
		t.Fatalf("failed to insert settings: %v", err)
	}

	return db
}

func TestScanLibraryConcurrentStress(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")
	db := setupStressTestDB(t, dbPath)
	defer db.Close()

	// Create physical directories to scan
	libraryDir := filepath.Join(tempDir, "library")
	book1Dir := filepath.Join(libraryDir, "Author One", "Book One")
	book2Dir := filepath.Join(libraryDir, "Author Two", "Book Two")

	if err := os.MkdirAll(book1Dir, 0755); err != nil {
		t.Fatalf("failed to create book 1 dir: %v", err)
	}
	if err := os.MkdirAll(book2Dir, 0755); err != nil {
		t.Fatalf("failed to create book 2 dir: %v", err)
	}

	// Write mock audio files
	if err := os.WriteFile(filepath.Join(book1Dir, "audio.mp3"), []byte("ID3mock-audio-data-1"), 0644); err != nil {
		t.Fatalf("failed to write mock audio 1: %v", err)
	}
	if err := os.WriteFile(filepath.Join(book2Dir, "audio.mp3"), []byte("ID3mock-audio-data-2"), 0644); err != nil {
		t.Fatalf("failed to write mock audio 2: %v", err)
	}

	libraryID := "lib-stress"
	folderID := "folder-stress"

	_, err := db.Exec("INSERT INTO libraries (id, name, mediaType, settings) VALUES (?, ?, ?, ?)",
		libraryID, "Stress Library", "book", `{"audiobooksOnly":true}`)
	if err != nil {
		t.Fatalf("failed to insert library: %v", err)
	}

	_, err = db.Exec("INSERT INTO libraryFolders (id, path, libraryId) VALUES (?, ?, ?)",
		folderID, libraryDir, libraryID)
	if err != nil {
		t.Fatalf("failed to insert library folder: %v", err)
	}

	// Set metadata path
	MetadataPath = t.TempDir()

	// Spawn multiple workers performing ScanLibrary concurrently
	const numWorkers = 8
	var wg sync.WaitGroup
	errs := make(chan error, numWorkers)

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			// Introduce slight staggered start to increase interleaved operations
			time.Sleep(time.Duration(workerID*5) * time.Millisecond)

			if err := ScanLibrary(db, libraryID, nil); err != nil {
				errs <- fmt.Errorf("worker %d failed: %w", workerID, err)
			}
		}(i)
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("Concurrent scan failed: %v", err)
	}

	// Verify database state is consistent
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM libraryItems WHERE libraryId = ?", libraryID).Scan(&count)
	if err != nil {
		t.Fatalf("failed to query libraryItems: %v", err)
	}
	if count != 2 {
		t.Errorf("Expected exactly 2 library items to be scanned, got %d", count)
	}

	var booksCount int
	err = db.QueryRow("SELECT COUNT(*) FROM books").Scan(&booksCount)
	if err != nil {
		t.Fatalf("failed to query books count: %v", err)
	}
	if booksCount != 2 {
		t.Errorf("Expected exactly 2 books to be scanned, got %d", booksCount)
	}
}
