package watcher

import (
	"database/sql"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// ScanFunc is the type for the library scan callback.
type ScanFunc func(db *sql.DB, libraryID string) error

// FSWatcher watches filesystem paths for changes and triggers library scans.
type FSWatcher struct {
	mu       sync.Mutex
	watcher  *fsnotify.Watcher
	paths    map[string]string      // maps watched path -> libraryId
	timers   map[string]*time.Timer // maps libraryId -> debounce timer
	db       *sql.DB
	done     chan struct{}
	wg       sync.WaitGroup
	scanFunc ScanFunc
}

// GlobalWatcher is the global FSWatcher instance.
var GlobalWatcher *FSWatcher

// InitFSWatcher initializes the global file system watcher.
func InitFSWatcher(database *sql.DB, scan ScanFunc) {
	if database == nil {
		log.Printf("[Watcher] Database is nil, skipping fs watcher initialization")
		return
	}
	w, err := fsnotify.NewWatcher()
	if err != nil {
		log.Printf("[Watcher] Failed to create fsnotify watcher: %v", err)
		return
	}
	GlobalWatcher = &FSWatcher{
		watcher:  w,
		paths:    make(map[string]string),
		timers:   make(map[string]*time.Timer),
		db:       database,
		done:     make(chan struct{}),
		scanFunc: scan,
	}
	GlobalWatcher.wg.Add(1)
	go GlobalWatcher.start()
	GlobalWatcher.Reload()
}

func (fw *FSWatcher) start() {
	defer fw.wg.Done()
	for {
		select {
		case <-fw.done:
			return
		case event, ok := <-fw.watcher.Events:
			if !ok {
				return
			}
			if ShouldIgnorePath(event.Name) {
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
					select {
					case <-fw.done:
						fw.mu.Unlock()
						return
					default:
					}
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

// Close gracefully shuts down the watcher.
func (fw *FSWatcher) Close() error {
	if fw == nil {
		return nil
	}

	fw.mu.Lock()
	if fw.done != nil {
		select {
		case <-fw.done:
			// Already closed
			fw.mu.Unlock()
			return nil
		default:
		}
		close(fw.done)
	}

	// Stop all active timers
	for _, timer := range fw.timers {
		timer.Stop()
	}
	fw.timers = make(map[string]*time.Timer)
	fw.mu.Unlock()

	var err error
	if fw.watcher != nil {
		err = fw.watcher.Close()
	}

	// Wait for the start goroutine to exit
	fw.wg.Wait()

	return err
}

// ShouldIgnorePath returns true if the path should be ignored during scanning.
func ShouldIgnorePath(path string) bool {
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
			time.Sleep(2 * time.Second)
			if fw.scanFunc != nil {
				if err := fw.scanFunc(fw.db, libraryID); err != nil {
					log.Printf("[Watcher] Scan library failed: %v", err)
				}
			}
		}()
	})
}

// Reload reloads the watched paths from the database.
func (fw *FSWatcher) Reload() {
	fw.mu.Lock()
	defer fw.mu.Unlock()

	var valStr string
	err := fw.db.QueryRow("SELECT value FROM settings WHERE key = 'server-settings'").Scan(&valStr)
	if err == nil && valStr != "" {
		var s struct {
			WatchLibraryChanges bool `json:"watchLibraryChanges"`
		}
		s.WatchLibraryChanges = true // Default is true
		if err := json.Unmarshal([]byte(valStr), &s); err == nil {
			if !s.WatchLibraryChanges {
				for path := range fw.paths {
					fw.watcher.Remove(path)
				}
				fw.paths = make(map[string]string)
				return
			}
		}
	}

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
		if err := rows.Scan(&path, &libID); err != nil {
			log.Printf("[Watcher] Failed to scan library folder: %v", err)
			continue
		}
		fw.watchRecursive(path, libID)
	}
	if err := rows.Err(); err != nil {
		log.Printf("[Watcher] Library folders query iteration error: %v", err)
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
