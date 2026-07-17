package watcher

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	log "audiobookshelf/internal/logger"
)

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
