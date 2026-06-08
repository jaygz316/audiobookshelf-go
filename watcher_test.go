package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
)

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
		got := shouldIgnorePath(tt.path)
		if got != tt.want {
			t.Errorf("shouldIgnorePath(%q) = %t; want %t", tt.path, got, tt.want)
		}
	}
}

func TestFSWatcherSetupAndReload(t *testing.T) {
	db := setupScannerTestDB(t)
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
