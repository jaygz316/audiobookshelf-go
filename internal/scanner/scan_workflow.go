package scanner

import (
	"database/sql"
	"path/filepath"
	"runtime"
	"sync"

	log "audiobookshelf/internal/logger"
	isocket "audiobookshelf/internal/socket"
)

type itemInfo struct {
	folderID          string
	itemPath          string
	groupFiles        []FileItem
	isFile            bool
	maxMtime          int64
	maxCtime          int64
	totalSize         int64
	ino               string
	itemRelPath       string
	needsScan         bool
	isNew             bool
	existingID        string
	itemID            string
	existingIsMissing int
	meta              *GroupMetadata
}

func buildItemScanInfos(db *sql.DB, libraryID, folderID, folderPath, mediaType string, grouped map[string][]FileItem) []*itemInfo {
	var items []*itemInfo
	for groupDir, groupFiles := range grouped {
		var itemPath string
		var isFile bool
		if len(groupFiles) == 1 && groupFiles[0].RelDirPath == "" {
			itemPath = groupFiles[0].Path
			isFile = true
		} else {
			itemPath = filepath.ToSlash(filepath.Join(folderPath, groupDir))
			isFile = false
		}

		var maxMtime, maxCtime int64
		var totalSize int64
		for _, f := range groupFiles {
			if f.MtimeMs > maxMtime {
				maxMtime = f.MtimeMs
			}
			if f.CtimeMs > maxCtime {
				maxCtime = f.CtimeMs
			}
			totalSize += f.Size
		}

		var ino string
		if len(groupFiles) > 0 {
			ino = groupFiles[0].Ino
		}

		var itemRelPath string
		if isFile {
			itemRelPath = groupFiles[0].RelPath
		} else {
			itemRelPath = filepath.Dir(groupFiles[0].RelPath)
			if itemRelPath == "." {
				itemRelPath = ""
			}
		}

		var existingID string
		var existingMtimeStr string
		var existingIsMissing int
		err := db.QueryRow("SELECT id, mtime, isMissing FROM libraryItems WHERE path = ? AND libraryId = ?", itemPath, libraryID).Scan(&existingID, &existingMtimeStr, &existingIsMissing)

		item := &itemInfo{
			folderID:          folderID,
			itemPath:          itemPath,
			groupFiles:        groupFiles,
			isFile:            isFile,
			maxMtime:          maxMtime,
			maxCtime:          maxCtime,
			totalSize:         totalSize,
			ino:               ino,
			itemRelPath:       itemRelPath,
			existingID:        existingID,
			existingIsMissing: existingIsMissing,
		}

		if err == sql.ErrNoRows {
			item.needsScan = true
			item.isNew = true
			item.itemID = uuidStr()
		} else if err == nil {
			existingMtime := parseEpochMillis(existingMtimeStr)
			if maxMtime != existingMtime {
				item.needsScan = true
				item.isNew = false
				item.itemID = existingID
			} else {
				item.needsScan = false
				item.itemID = existingID
			}
		}

		items = append(items, item)
	}
	return items
}

func parseMetadataForItemsConcurrently(db *sql.DB, items []*itemInfo, mediaType string, audiobooksOnly bool) {
	var tasks []*itemInfo
	for _, item := range items {
		if item.needsScan {
			tasks = append(tasks, item)
		}
	}

	if len(tasks) > 0 {
		log.Printf("[Scanner] Parsing metadata concurrently for %d items", len(tasks))
		concurrency := runtime.NumCPU()
		if concurrency < 4 {
			concurrency = 4
		}
		if concurrency > 8 {
			concurrency = 8
		}
		if concurrency > len(tasks) {
			concurrency = len(tasks)
		}

		taskChan := make(chan *itemInfo, len(tasks))
		for _, t := range tasks {
			taskChan <- t
		}
		close(taskChan)

		var wg sync.WaitGroup
		for i := 0; i < concurrency; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for item := range taskChan {
					item.meta = parseMetadataForGroup(db, item.itemID, item.groupFiles, mediaType, item.itemPath, item.itemRelPath, audiobooksOnly)
				}
			}()
		}
		wg.Wait()
		log.Printf("[Scanner] Concurrent metadata parsing complete")
	}
}

func writeScanResults(db *sql.DB, libraryID, folderID, mediaType string, audiobooksOnly bool, prefixes []string, socketAuth *isocket.Authority, items []*itemInfo) ([]string, error) {
	var foundPaths []string
	for _, item := range items {
		foundPaths = append(foundPaths, item.itemPath)

		if item.needsScan {
			if item.isNew {
				log.Printf("[Scanner] Scanning new item at: %s", item.itemPath)
				err := scanNewLibraryItem(db, libraryID, item.folderID, item.itemPath, item.groupFiles, mediaType, item.isFile, item.maxMtime, item.maxCtime, item.totalSize, item.ino, audiobooksOnly, prefixes, socketAuth, item.meta)
				if err != nil {
					log.Printf("[Scanner] Error scanning new item at %s: %v", item.itemPath, err)
				}
			} else {
				if item.existingIsMissing != 0 {
					log.Printf("[Scanner] Item %s marked as missing but exists now. Restoring.", item.itemPath)
					_, _ = db.Exec("UPDATE libraryItems SET isMissing = 0 WHERE id = ?", item.existingID)
				}
				log.Printf("[Scanner] Mtime changed for existing item %s (mtime: %d != existing), rescanning", item.itemPath, item.maxMtime)
				err := scanExistingLibraryItem(db, item.existingID, libraryID, item.folderID, item.itemPath, item.groupFiles, mediaType, item.isFile, item.maxMtime, item.maxCtime, item.totalSize, item.ino, audiobooksOnly, prefixes, socketAuth, item.meta)
				if err != nil {
					log.Printf("[Scanner] Error updating existing item at %s: %v", item.itemPath, err)
				}
			}
		} else {
			if item.existingID != "" && item.existingIsMissing != 0 {
				log.Printf("[Scanner] Item %s marked as missing but exists now. Restoring.", item.itemPath)
				_, _ = db.Exec("UPDATE libraryItems SET isMissing = 0 WHERE id = ?", item.existingID)
			}
			log.Printf("[Scanner] Item %s mtime unchanged, skipping rescan", item.itemPath)
		}
	}
	return foundPaths, nil
}
