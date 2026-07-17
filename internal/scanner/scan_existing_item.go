package scanner

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	log "audiobookshelf/internal/logger"
	isocket "audiobookshelf/internal/socket"
)

func scanExistingLibraryItem(db *sql.DB, itemID, libraryID, folderID, itemPath string, groupFiles []FileItem, mediaType string, isFile bool, mtime, ctime, totalSize int64, ino string, audiobooksOnly bool, prefixes []string, socketAuth *isocket.Authority, meta *GroupMetadata) error {
	var mediaID string
	err := db.QueryRow("SELECT mediaId FROM libraryItems WHERE id = ?", itemID).Scan(&mediaID)
	if err != nil {
		return err
	}

	log.Printf("[Scanner] [%s] scanExistingLibraryItem: Beginning transaction", itemPath)
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	nowStr := time.Now().Format("2006-01-02 15:04:05.000")

	var itemRelPath string
	if isFile {
		itemRelPath = groupFiles[0].RelPath
	} else {
		itemRelPath = filepath.Dir(groupFiles[0].RelPath)
		if itemRelPath == "." {
			itemRelPath = ""
		}
	}

	var title, authorNamesFirstLast, authorNamesLastFirst string
	title = meta.Title
	if title == "" {
		title = filepath.Base(itemPath)
	}
	titleIgnorePrefix := getTitleIgnorePrefixGo(title, prefixes)

	if mediaType == "book" {
		authorNamesFirstLast, authorNamesLastFirst, err = updateExistingBook(tx, mediaID, title, titleIgnorePrefix, libraryID, itemPath, meta, nowStr)
		if err != nil {
			return err
		}
	} else if mediaType == "podcast" {
		err = updateExistingPodcast(tx, mediaID, title, titleIgnorePrefix, itemPath, meta, nowStr)
		if err != nil {
			return err
		}
	}

	mtimeStr := formatEpochMillis(mtime)
	ctimeStr := formatEpochMillis(ctime)

	colsLI := getTableColumnsTx(tx, "libraryItems")
	var setStmtsLI []string
	var argsLI []interface{}

	addColLI := func(name string, val interface{}) {
		if colsLI[name] {
			setStmtsLI = append(setStmtsLI, fmt.Sprintf("%s = ?", name))
			argsLI = append(argsLI, val)
		}
	}

	addColLI("ino", ino)
	addColLI("mtime", mtimeStr)
	addColLI("ctime", ctimeStr)
	addColLI("updatedAt", nowStr)
	addColLI("size", totalSize)
	addColLI("authorNamesFirstLast", authorNamesFirstLast)
	addColLI("authorNamesLastFirst", authorNamesLastFirst)
	addColLI("title", title)
	addColLI("titleIgnorePrefix", titleIgnorePrefix)

	argsLI = append(argsLI, itemID)
	queryLI := fmt.Sprintf("UPDATE libraryItems SET %s WHERE id = ?", strings.Join(setStmtsLI, ", "))
	log.Printf("[Scanner] [%s] scanExistingLibraryItem: Updating libraryItems table", itemPath)
	_, err = tx.Exec(queryLI, argsLI...)
	if err != nil {
		return err
	}

	log.Printf("[Scanner] [%s] scanExistingLibraryItem: Committing transaction", itemPath)
	err = tx.Commit()
	if err != nil {
		return err
	}
	log.Printf("[Scanner] [%s] scanExistingLibraryItem: Transaction committed successfully", itemPath)

	if socketAuth != nil {
		if minItem, err := GetLibraryItemMinifiedByID(db, itemID); err == nil {
			EmitLibraryItemsEvent(socketAuth, "items_updated", minItem)
		}
	}

	return nil
}
