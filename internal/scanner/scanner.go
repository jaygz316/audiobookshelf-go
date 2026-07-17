// Package scanner provides library scanning functionality for audiobookshelf.
package scanner

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"runtime"
	"sync"

	log "audiobookshelf/internal/logger"
	isocket "audiobookshelf/internal/socket"
)

var MetadataPath string
var probeSemaphore chan struct{}

var (
	scanMu       sync.Mutex
	libraryLocks = make(map[string]*sync.Mutex)
)

func getLibraryLock(libraryID string) *sync.Mutex {
	scanMu.Lock()
	defer scanMu.Unlock()
	mu, exists := libraryLocks[libraryID]
	if !exists {
		mu = &sync.Mutex{}
		libraryLocks[libraryID] = mu
	}
	return mu
}

func init() {
	concurrency := runtime.NumCPU()
	if concurrency < 4 {
		concurrency = 4
	}
	if concurrency > 8 {
		concurrency = 8
	}
	probeSemaphore = make(chan struct{}, concurrency)
}

// ScanLibrary scans a library and updates the database.
// socketAuth may be nil (used for emitting WebSocket events).
func ScanLibrary(db *sql.DB, libraryID string, socketAuth *isocket.Authority) error {
	mu := getLibraryLock(libraryID)
	mu.Lock()
	defer mu.Unlock()

	log.Printf("[Scanner] Starting scan for library ID: %s", libraryID)

	var libName, mediaType, libSettingsStr string
	err := db.QueryRow("SELECT name, mediaType, settings FROM libraries WHERE id = ?", libraryID).Scan(&libName, &mediaType, &libSettingsStr)
	if err != nil {
		return fmt.Errorf("library not found: %w", err)
	}
	log.Printf("[Scanner] Library name: %s, Media type: %s", libName, mediaType)

	var libSettings struct {
		AudiobooksOnly bool `json:"audiobooksOnly"`
	}
	if libSettingsStr != "" {
		_ = json.Unmarshal([]byte(libSettingsStr), &libSettings)
	}

	if socketAuth != nil {
		socketAuth.Emitter("library_scan_started", libraryID, nil)
	}

	defer func() {
		log.Printf("[Scanner] defer library_scan_complete for library ID: %s", libraryID)
		if socketAuth != nil {
			socketAuth.Emitter("library_scan_complete", libraryID, nil)
		}
	}()

	prefixes := getSortingPrefixes(db)
	log.Printf("[Scanner] Loaded %d sorting prefixes", len(prefixes))

	rows, err := db.Query("SELECT id, path FROM libraryFolders WHERE libraryId = ?", libraryID)
	if err != nil {
		return err
	}
	defer rows.Close()

	var folders []struct {
		id   string
		path string
	}
	for rows.Next() {
		var id, path string
		if err := rows.Scan(&id, &path); err != nil {
			return err
		}
		folders = append(folders, struct{ id, path string }{id, path})
	}
	if err := rows.Err(); err != nil {
		return err
	}
	log.Printf("[Scanner] Found %d library folders to scan", len(folders))

	var foundPaths []string

	for _, folder := range folders {
		log.Printf("[Scanner] Walking folder: %s", folder.path)
		files, err := WalkLibraryFolder(folder.path)
		if err != nil {
			log.Printf("[Scanner] Failed to walk folder %s: %v", folder.path, err)
			continue
		}
		log.Printf("[Scanner] Walk complete. Found %d file items. Grouping them...", len(files))

		grouped := GroupFileItemsIntoLibraryItemDirs(mediaType, files, libSettings.AudiobooksOnly)
		log.Printf("[Scanner] Grouped into %d library item directories", len(grouped))

		items := buildItemScanInfos(db, libraryID, folder.id, folder.path, mediaType, grouped)

		parseMetadataForItemsConcurrently(db, items, mediaType, libSettings.AudiobooksOnly)

		paths, err := writeScanResults(db, libraryID, folder.id, mediaType, libSettings.AudiobooksOnly, prefixes, socketAuth, items)
		if err != nil {
			log.Printf("[Scanner] Error writing scan results: %v", err)
		}
		foundPaths = append(foundPaths, paths...)
	}

	return checkAndMarkMissingItems(db, libraryID, foundPaths, socketAuth)
}
