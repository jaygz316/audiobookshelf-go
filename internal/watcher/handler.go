package watcher

import (
	"os"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"

	log "audiobookshelf/internal/logger"
)

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
