package scanner

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	log "audiobookshelf/internal/logger"
	inotification "audiobookshelf/internal/notification"
	isocket "audiobookshelf/internal/socket"
)

func scanNewLibraryItem(db *sql.DB, libraryID, folderID, itemPath string, groupFiles []FileItem, mediaType string, isFile bool, mtime, ctime, totalSize int64, ino string, audiobooksOnly bool, prefixes []string, socketAuth *isocket.Authority, meta *GroupMetadata) error {
	itemID := uuidStr()
	mediaID := uuidStr()
	nowStr := time.Now().Format("2006-01-02 15:04:05.000")

	log.Printf("[Scanner] [%s] scanNewLibraryItem: Beginning transaction", itemPath)
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

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
		authorNamesFirstLast = strings.Join(meta.Authors, ", ")
		var lfs []string
		for _, a := range meta.Authors {
			lfs = append(lfs, NameToLastFirst(a))
		}
		authorNamesLastFirst = strings.Join(lfs, ", ")

		err = insertNewBook(tx, mediaID, title, titleIgnorePrefix, libraryID, itemPath, meta, nowStr)
		if err != nil {
			return err
		}
	} else if mediaType == "podcast" {
		err = insertNewPodcast(tx, mediaID, title, titleIgnorePrefix, itemPath, meta, nowStr)
		if err != nil {
			return err
		}
	}

	mtimeStr := formatEpochMillis(mtime)
	ctimeStr := formatEpochMillis(ctime)

	colsLI := getTableColumnsTx(tx, "libraryItems")
	var colNamesLI []string
	var placeholdersLI []string
	var argsLI []interface{}

	addColLI := func(name string, val interface{}) {
		if colsLI[name] {
			colNamesLI = append(colNamesLI, name)
			placeholdersLI = append(placeholdersLI, "?")
			argsLI = append(argsLI, val)
		}
	}

	addColLI("id", itemID)
	addColLI("ino", ino)
	addColLI("libraryId", libraryID)
	addColLI("path", itemPath)
	addColLI("relPath", itemRelPath)
	addColLI("isFile", isFile)
	addColLI("mtime", mtimeStr)
	addColLI("ctime", ctimeStr)
	addColLI("birthtime", ctimeStr)
	addColLI("createdAt", nowStr)
	addColLI("updatedAt", nowStr)
	addColLI("isMissing", 0)
	addColLI("isInvalid", 0)
	addColLI("mediaType", mediaType)
	addColLI("mediaId", mediaID)
	addColLI("size", totalSize)
	addColLI("libraryFolderId", folderID)
	addColLI("authorNamesFirstLast", authorNamesFirstLast)
	addColLI("authorNamesLastFirst", authorNamesLastFirst)
	addColLI("title", title)
	addColLI("titleIgnorePrefix", titleIgnorePrefix)

	queryLI := fmt.Sprintf("INSERT INTO libraryItems (%s) VALUES (%s)", strings.Join(colNamesLI, ", "), strings.Join(placeholdersLI, ", "))
	log.Printf("[Scanner] [%s] scanNewLibraryItem: Inserting into libraryItems table", itemPath)
	_, err = tx.Exec(queryLI, argsLI...)
	if err != nil {
		return err
	}
	log.Printf("[Scanner] [%s] scanNewLibraryItem: Committing transaction", itemPath)
	err = tx.Commit()
	if err != nil {
		return err
	}
	log.Printf("[Scanner] [%s] scanNewLibraryItem: Transaction committed successfully", itemPath)

	_ = triggerNewItemNotification(db, libraryID, mediaType, title, authorNamesFirstLast, meta)

	if socketAuth != nil {
		if minItem, err := GetLibraryItemMinifiedByID(db, itemID); err == nil {
			EmitLibraryItemsEvent(socketAuth, "items_added", minItem)
		}
	}

	return nil
}

func triggerNewItemNotification(db *sql.DB, libraryID, mediaType, title, authorNamesFirstLast string, meta *GroupMetadata) error {
	var libraryName string
	_ = db.QueryRow("SELECT name FROM libraries WHERE id = ?", libraryID).Scan(&libraryName)

	if mediaType == "podcast" {
		for _, ep := range meta.PodcastEpisodes {
			extraData := map[string]string{
				"podcastTitle": title,
				"episodeTitle": ep.Title,
				"libraryName":  libraryName,
			}
			inotification.TriggerEvent(context.Background(), db, "onPodcastEpisodeDownloaded", &libraryID, "New Episode", fmt.Sprintf("%s - %s", title, ep.Title), extraData)
		}
	} else if mediaType == "book" {
		extraData := map[string]string{
			"title":       title,
			"author":      authorNamesFirstLast,
			"libraryName": libraryName,
		}
		inotification.TriggerEvent(context.Background(), db, "onItemAdded", &libraryID, "New Book Added", fmt.Sprintf("%s by %s", title, authorNamesFirstLast), extraData)
	}
	return nil
}
