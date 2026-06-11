package main

import (
	"database/sql"

	"audiobookshelf/internal/watcher"
)

// FSWatcher is an alias for the internal watcher type.
type FSWatcher = watcher.FSWatcher

// GlobalWatcher is the global FSWatcher instance.
var GlobalWatcher *FSWatcher

// InitFSWatcher initializes the global filesystem watcher with a scan callback.
func InitFSWatcher(database *sql.DB) {
	watcher.InitFSWatcher(database, ScanLibrary)
	GlobalWatcher = watcher.GlobalWatcher
}

// shouldIgnorePath returns true if the path should be ignored.
func shouldIgnorePath(path string) bool {
	return watcher.ShouldIgnorePath(path)
}
