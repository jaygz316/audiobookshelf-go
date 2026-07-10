package scanner

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func setupScannerTestDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory database: %v", err)
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

func TestIsMediaFile(t *testing.T) {
	tests := []struct {
		mediaType      string
		ext            string
		audiobooksOnly bool
		want           bool
	}{
		{"book", ".mp3", true, true},
		{"book", ".mp3", false, true},
		{"book", ".epub", false, true},
		{"book", ".epub", true, false},
		{"book", ".txt", false, false},
		{"podcast", ".mp3", false, true},
		{"podcast", ".epub", false, false},
	}

	for _, tt := range tests {
		got := IsMediaFile(tt.mediaType, tt.ext, tt.audiobooksOnly)
		if got != tt.want {
			t.Errorf("IsMediaFile(%q, %q, %t) = %t; want %t", tt.mediaType, tt.ext, tt.audiobooksOnly, got, tt.want)
		}
	}
}

func TestFilenameParsing(t *testing.T) {
	tests := []struct {
		relPath   string
		wantTitle string
		wantASIN  string
		wantYear  string
	}{
		{"Stephen King/The Shining [B001111111]", "The Shining", "B001111111", ""},
		{"Brandon Sanderson/Mistborn/(2006) - Mistborn - The Final Empire - Subtitle", "Mistborn", "", "2006"},
	}

	for _, tt := range tests {
		meta := GetBookDataFromDir(tt.relPath)
		if meta.Title != tt.wantTitle {
			t.Errorf("Title parsed = %q; want %q", meta.Title, tt.wantTitle)
		}
		if meta.ASIN != tt.wantASIN {
			t.Errorf("ASIN parsed = %q; want %q", meta.ASIN, tt.wantASIN)
		}
		if meta.PublishedYear != tt.wantYear {
			t.Errorf("Year parsed = %q; want %q", meta.PublishedYear, tt.wantYear)
		}
	}
}

func TestScanLibraryIntegration(t *testing.T) {
	db := setupScannerTestDB(t)
	defer db.Close()

	// Create a temp folder to act as library folder
	tempDir, err := os.MkdirTemp("", "abs-test-library")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create Author/Book structure
	bookDir := filepath.Join(tempDir, "J.R.R. Tolkien", "The Hobbit")
	if err := os.MkdirAll(bookDir, 0755); err != nil {
		t.Fatalf("failed to create book dir: %v", err)
	}

	// Write mock audio file
	mockAudioPath := filepath.Join(bookDir, "01 - An Unexpected Party.mp3")
	if err := os.WriteFile(mockAudioPath, []byte("ID3mock-audio-data"), 0644); err != nil {
		t.Fatalf("failed to write mock audio file: %v", err)
	}

	// Insert Library and Library Folder into db
	libraryID := "lib-1"
	folderID := "folder-1"
	_, err = db.Exec("INSERT INTO libraries (id, name, mediaType, settings) VALUES (?, ?, ?, ?)",
		libraryID, "Audiobooks", "book", `{"audiobooksOnly":true}`)
	if err != nil {
		t.Fatalf("failed to insert library: %v", err)
	}

	_, err = db.Exec("INSERT INTO libraryFolders (id, path, libraryId) VALUES (?, ?, ?)",
		folderID, tempDir, libraryID)
	if err != nil {
		t.Fatalf("failed to insert library folder: %v", err)
	}

	// Run ScanLibrary
	err = ScanLibrary(db, libraryID, nil)
	if err != nil {
		t.Fatalf("ScanLibrary failed: %v", err)
	}

	// Verify that libraryItems contains J.R.R. Tolkien/The Hobbit
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM libraryItems WHERE libraryId = ?", libraryID).Scan(&count)
	if err != nil {
		t.Fatalf("failed to query library items count: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 library item, found %d", count)
	}

	var itemPath, itemTitle, itemMediaType string
	err = db.QueryRow("SELECT path, title, mediaType FROM libraryItems WHERE libraryId = ?", libraryID).Scan(&itemPath, &itemTitle, &itemMediaType)
	if err != nil {
		t.Fatalf("failed to query library item: %v", err)
	}

	expectedPath := filepath.ToSlash(bookDir)
	if itemPath != expectedPath {
		t.Errorf("expected item path %q, got %q", expectedPath, itemPath)
	}
	if itemTitle != "The Hobbit" {
		t.Errorf("expected item title 'The Hobbit', got %q", itemTitle)
	}
	if itemMediaType != "book" {
		t.Errorf("expected mediaType 'book', got %q", itemMediaType)
	}

	// Verify that the book metadata is populated
	var bookTitle, bookAuthorNames string
	err = db.QueryRow("SELECT b.title, li.authorNamesFirstLast FROM books b JOIN libraryItems li ON b.id = li.mediaId WHERE li.libraryId = ?", libraryID).Scan(&bookTitle, &bookAuthorNames)
	if err != nil {
		t.Fatalf("failed to query book: %v", err)
	}
	if bookTitle != "The Hobbit" {
		t.Errorf("expected book title 'The Hobbit', got %q", bookTitle)
	}
	if bookAuthorNames != "J.R.R. Tolkien" {
		t.Errorf("expected author 'J.R.R. Tolkien', got %q", bookAuthorNames)
	}
}

func TestMetadataFieldLocking(t *testing.T) {
	db := setupScannerTestDB(t)
	defer db.Close()

	// Create a temp folder
	tempDir, err := os.MkdirTemp("", "abs-test-locking")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create Book folder
	bookDir := filepath.Join(tempDir, "NewAuthor", "NewTitle")
	if err := os.MkdirAll(bookDir, 0755); err != nil {
		t.Fatalf("failed to create book dir: %v", err)
	}

	// Write mock audio file
	mockAudioPath := filepath.Join(bookDir, "audio.mp3")
	if err := os.WriteFile(mockAudioPath, []byte("ID3mock-audio-data"), 0644); err != nil {
		t.Fatalf("failed to write mock audio file: %v", err)
	}

	// Write desc.txt
	descPath := filepath.Join(bookDir, "desc.txt")
	if err := os.WriteFile(descPath, []byte("New Description"), 0644); err != nil {
		t.Fatalf("failed to write desc.txt: %v", err)
	}

	// Pre-insert library item and book
	libraryID := "lib-1"
	folderID := "folder-1"
	itemID := "item-1"
	mediaID := "media-1"

	_, _ = db.Exec("INSERT INTO libraries (id, name, mediaType, settings) VALUES (?, ?, ?, ?)",
		libraryID, "Audiobooks", "book", `{"audiobooksOnly":true}`)
	_, _ = db.Exec("INSERT INTO libraryFolders (id, path, libraryId) VALUES (?, ?, ?)",
		folderID, tempDir, libraryID)

	_, _ = db.Exec(`
		INSERT INTO libraryItems (id, ino, libraryId, path, relPath, isFile, mediaType, mediaId)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, itemID, "12345", libraryID, bookDir, "NewAuthor/NewTitle", 0, "book", mediaID)

	// Insert book with title locked, description locked, but publisher not locked (publisher = "Original Publisher")
	_, _ = db.Exec(`
		INSERT INTO books (id, title, titleIgnorePrefix, description, publisher, lockedFields)
		VALUES (?, ?, ?, ?, ?, ?)
	`, mediaID, "Original Title", "Original Title", "Original Description", "Original Publisher", `["title", "description"]`)

	// Construct groupFiles
	groupFiles := []FileItem{
		{
			Path:      mockAudioPath,
			RelPath:   "NewAuthor/NewTitle/audio.mp3",
			Name:      "audio.mp3",
			Extension: ".mp3",
			Size:      18,
		},
		{
			Path:      descPath,
			RelPath:   "NewAuthor/NewTitle/desc.txt",
			Name:      "desc.txt",
			Extension: ".txt",
			Size:      15,
		},
	}

	// Now call scanExistingLibraryItem directly!
	err = scanExistingLibraryItem(db, itemID, libraryID, folderID, bookDir, groupFiles, "book", false, 0, 0, 33, "12345", true, []string{"the", "a", "an"}, nil)
	if err != nil {
		t.Fatalf("scanExistingLibraryItem failed: %v", err)
	}

	// Query book to check if locked fields are preserved, unlocked fields updated
	var title, description, publisher string
	err = db.QueryRow("SELECT title, description, publisher FROM books WHERE id = ?", mediaID).Scan(&title, &description, &publisher)
	if err != nil {
		t.Fatalf("failed to query book: %v", err)
	}

	if title != "Original Title" {
		t.Errorf("expected title to remain 'Original Title' (locked), got %q", title)
	}
	if description != "Original Description" {
		t.Errorf("expected description to remain 'Original Description' (locked), got %q", description)
	}
}

