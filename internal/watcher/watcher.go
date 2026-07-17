package watcher

import (
	"database/sql"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	log "audiobookshelf/internal/logger"
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
