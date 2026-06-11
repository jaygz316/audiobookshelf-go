package watcher

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"

	_ "modernc.org/sqlite"
)

// setupWatcherTestDB creates a minimal in-memory SQLite database for tests.
func setupWatcherTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory database: %v", err)
	}

	schema := []string{
		`CREATE TABLE libraries (id TEXT PRIMARY KEY, name TEXT, mediaType TEXT, settings TEXT, provider TEXT, displayOrder INTEGER, createdAt TEXT, updatedAt TEXT);`,
		`CREATE TABLE libraryFolders (id TEXT PRIMARY KEY, path TEXT, libraryId TEXT, createdAt TEXT, updatedAt TEXT);`,
	}
	for _, q := range schema {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("failed to execute schema query: %v", err)
		}
	}
	return db
}

func TestShouldIgnorePath(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"/path/to/.DS_Store", true},
		{"/path/to/normal.mp3", false},
		{"/path/to/downloading.tmp", true},
		{"/path/to/downloading.crdownload", true},
		{"/path/to/.somefile.m4b", true},
		{"/path/to/normal.epub", false},
	}

	for _, tt := range tests {
		got := ShouldIgnorePath(tt.path)
		if got != tt.want {
			t.Errorf("ShouldIgnorePath(%q) = %t; want %t", tt.path, got, tt.want)
		}
	}
}

func TestFSWatcherSetupAndReload(t *testing.T) {
	db := setupWatcherTestDB(t)
	defer db.Close()

	tempDir, err := os.MkdirTemp("", "abs-watcher-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	subDir := filepath.Join(tempDir, "SubDir")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("failed to create sub dir: %v", err)
	}

	// Insert mock library folder
	_, err = db.Exec("INSERT INTO libraries (id, name, mediaType, settings) VALUES ('lib-1', 'Audiobooks', 'book', '{}')")
	if err != nil {
		t.Fatalf("failed to insert library: %v", err)
	}
	_, err = db.Exec("INSERT INTO libraryFolders (id, path, libraryId) VALUES ('folder-1', ?, 'lib-1')", tempDir)
	if err != nil {
		t.Fatalf("failed to insert library folder: %v", err)
	}

	w, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatalf("failed to create watcher: %v", err)
	}
	defer w.Close()

	fw := &FSWatcher{
		watcher: w,
		paths:   make(map[string]string),
		timers:  make(map[string]*time.Timer),
		db:      db,
	}

	fw.Reload()

	// Verify paths are being watched
	fw.mu.Lock()
	_, watchingRoot := fw.paths[filepath.ToSlash(tempDir)]
	_, watchingSub := fw.paths[filepath.ToSlash(subDir)]
	fw.mu.Unlock()

	if !watchingRoot {
		t.Errorf("expected watcher to watch root directory %q", tempDir)
	}
	if !watchingSub {
		t.Errorf("expected watcher to watch subdirectory %q", subDir)
	}

	// Test findLibraryForPath
	libID := fw.findLibraryForPath(filepath.Join(subDir, "book.mp3"))
	if libID != "lib-1" {
		t.Errorf("findLibraryForPath returned %q; want 'lib-1'", libID)
	}
}

func TestFSWatcherClose(t *testing.T) {
	db := setupWatcherTestDB(t)
	defer db.Close()

	w, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatalf("failed to create watcher: %v", err)
	}

	fw := &FSWatcher{
		watcher: w,
		paths:   make(map[string]string),
		timers:  make(map[string]*time.Timer),
		db:      db,
		done:    make(chan struct{}),
	}

	fw.wg.Add(1)
	go fw.start()

	// Add a dummy timer to ensure it is stopped on Close()
	fw.mu.Lock()
	timerFired := false
	fw.timers["test-lib"] = time.AfterFunc(10*time.Second, func() {
		timerFired = true
	})
	fw.mu.Unlock()

	// Close the watcher
	if err := fw.Close(); err != nil {
		t.Errorf("Close returned error: %v", err)
	}

	// Verify timer is stopped (should be stopped immediately and timers map cleared)
	fw.mu.Lock()
	if len(fw.timers) != 0 {
		t.Errorf("expected timers map to be cleared, got size %d", len(fw.timers))
	}
	fw.mu.Unlock()

	if timerFired {
		t.Errorf("expected timer to be cancelled and not fire")
	}
}
