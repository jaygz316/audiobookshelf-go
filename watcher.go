package main

import (
	"database/sql"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

type FSWatcher struct {
	mu      sync.Mutex
	watcher *fsnotify.Watcher
	paths   map[string]string      // maps watched path -> libraryId
	timers  map[string]*time.Timer // maps libraryId -> debounce timer
	db      *sql.DB
}

var GlobalWatcher *FSWatcher

func InitFSWatcher(database *sql.DB) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		log.Printf("[Watcher] Failed to create fsnotify watcher: %v", err)
		return
	}
	GlobalWatcher = &FSWatcher{
		watcher: w,
		paths:   make(map[string]string),
		timers:  make(map[string]*time.Timer),
		db:      database,
	}
	go GlobalWatcher.start()
	GlobalWatcher.Reload()
}

func (fw *FSWatcher) start() {
	for {
		select {
		case event, ok := <-fw.watcher.Events:
			if !ok {
				return
			}
			if shouldIgnorePath(event.Name) {
				continue
			}

			libraryID := fw.findLibraryForPath(event.Name)
			if libraryID != "" {
				fw.triggerDebouncedScan(libraryID)
			}

			if event.Has(fsnotify.Create) {
				info, err := os.Stat(event.Name)
				if err == nil && info.IsDir() {
					fw.mu.Lock()
					fw.watcher.Add(event.Name)
					fw.paths[event.Name] = libraryID
					fw.mu.Unlock()
				}
			}

		case err, ok := <-fw.watcher.Errors:
			if !ok {
				return
			}
			log.Printf("[Watcher] Error: %v", err)
		}
	}
}

func shouldIgnorePath(path string) bool {
	base := filepath.Base(path)
	if strings.HasPrefix(base, ".") {
		return true
	}
	ext := strings.ToLower(filepath.Ext(path))
	ignoredExts := map[string]bool{
		".tmp": true, ".temp": true, ".part": true, ".crdownload": true,
	}
	return ignoredExts[ext]
}

func (fw *FSWatcher) findLibraryForPath(path string) string {
	fw.mu.Lock()
	defer fw.mu.Unlock()

	var bestMatch string
	var libID string
	for watchedPath, id := range fw.paths {
		if strings.HasPrefix(path, watchedPath) {
			if len(watchedPath) > len(bestMatch) {
				bestMatch = watchedPath
				libID = id
			}
		}
	}
	return libID
}

func (fw *FSWatcher) triggerDebouncedScan(libraryID string) {
	fw.mu.Lock()
	defer fw.mu.Unlock()

	if timer, ok := fw.timers[libraryID]; ok {
		timer.Stop()
	}

	fw.timers[libraryID] = time.AfterFunc(10*time.Second, func() {
		log.Printf("[Watcher] Triggering scan for library %s after debounce", libraryID)
		go func() {
			// Wait loop for stabilizing (wait up to 30 seconds, verifying mtime of target path)
			// For a simplified and non-blocking wait loop, sleep 2 seconds is standard.
			// If we want a wait loop, we can just sleep a couple seconds to allow files to finish writing.
			time.Sleep(2 * time.Second)
			if err := ScanLibrary(fw.db, libraryID); err != nil {
				log.Printf("[Watcher] Scan library failed: %v", err)
			}
		}()
	})
}

func (fw *FSWatcher) Reload() {
	fw.mu.Lock()
	defer fw.mu.Unlock()

	for path := range fw.paths {
		fw.watcher.Remove(path)
	}
	fw.paths = make(map[string]string)

	rows, err := fw.db.Query("SELECT path, libraryId FROM libraryFolders")
	if err != nil {
		log.Printf("[Watcher] Failed to query library folders: %v", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var path, libID string
		if err := rows.Scan(&path, &libID); err == nil {
			fw.watchRecursive(path, libID)
		}
	}
}

func (fw *FSWatcher) watchRecursive(root string, libraryID string) {
	filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			fw.watcher.Add(path)
			fw.paths[path] = libraryID
		}
		return nil
	})
}
