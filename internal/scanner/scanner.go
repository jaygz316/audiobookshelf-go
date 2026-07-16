// Package scanner provides library scanning functionality for audiobookshelf.
package scanner

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"audiobookshelf/internal/core"
	log "audiobookshelf/internal/logger"
	inotification "audiobookshelf/internal/notification"
	isocket "audiobookshelf/internal/socket"
)

var MetadataPath string
var probeSemaphore chan struct{}

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

		var items []*itemInfo

		// Phase 1: Sequential Database Verification (Read-Only)
		for groupDir, groupFiles := range grouped {
			var itemPath string
			var isFile bool
			if len(groupFiles) == 1 && groupFiles[0].RelDirPath == "" {
				itemPath = groupFiles[0].Path
				isFile = true
			} else {
				itemPath = filepath.ToSlash(filepath.Join(folder.path, groupDir))
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
			err = db.QueryRow("SELECT id, mtime, isMissing FROM libraryItems WHERE path = ? AND libraryId = ?", itemPath, libraryID).Scan(&existingID, &existingMtimeStr, &existingIsMissing)

			item := &itemInfo{
				folderID:          folder.id,
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

		// Phase 2: Concurrent Metadata Parsing
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
						item.meta = parseMetadataForGroup(db, item.itemID, item.groupFiles, mediaType, item.itemPath, item.itemRelPath, libSettings.AudiobooksOnly)
					}
				}()
			}
			wg.Wait()
			log.Printf("[Scanner] Concurrent metadata parsing complete")
		}

		// Phase 3: Sequential Database Writes
		for _, item := range items {
			foundPaths = append(foundPaths, item.itemPath)

			if item.needsScan {
				if item.isNew {
					log.Printf("[Scanner] Scanning new item at: %s", item.itemPath)
					err := scanNewLibraryItem(db, libraryID, item.folderID, item.itemPath, item.groupFiles, mediaType, item.isFile, item.maxMtime, item.maxCtime, item.totalSize, item.ino, libSettings.AudiobooksOnly, prefixes, socketAuth, item.meta)
					if err != nil {
						log.Printf("[Scanner] Error scanning new item at %s: %v", item.itemPath, err)
					}
				} else {
					if item.existingIsMissing != 0 {
						log.Printf("[Scanner] Item %s marked as missing but exists now. Restoring.", item.itemPath)
						_, _ = db.Exec("UPDATE libraryItems SET isMissing = 0 WHERE id = ?", item.existingID)
					}
					log.Printf("[Scanner] Mtime changed for existing item %s (mtime: %d != existing), rescanning", item.itemPath, item.maxMtime)
					err := scanExistingLibraryItem(db, item.existingID, libraryID, item.folderID, item.itemPath, item.groupFiles, mediaType, item.isFile, item.maxMtime, item.maxCtime, item.totalSize, item.ino, libSettings.AudiobooksOnly, prefixes, socketAuth, item.meta)
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
	}

	log.Printf("[Scanner] Checking for missing library items...")
	dbItems, err := db.Query("SELECT id, path FROM libraryItems WHERE libraryId = ? AND isMissing = 0", libraryID)
	if err != nil {
		return err
	}
	defer dbItems.Close()
	foundPathsMap := make(map[string]bool)
	for _, p := range foundPaths {
		foundPathsMap[p] = true
	}

	for dbItems.Next() {
		var id, path string
		if err := dbItems.Scan(&id, &path); err != nil {
			return err
		}
		if !foundPathsMap[path] {
			log.Printf("[Scanner] Item %s not found on disk, marking as missing", path)
			_, err = db.Exec("UPDATE libraryItems SET isMissing = 1 WHERE id = ?", id)
			if err != nil {
				return err
			}

			if socketAuth != nil {
				if minItem, err := GetLibraryItemMinifiedByID(db, id); err == nil {
					EmitLibraryItemEvent(socketAuth, "item_updated", minItem)
				}
			}
		}
	}
	if err := dbItems.Err(); err != nil {
		return err
	}

	log.Printf("[Scanner] Scan complete for library ID: %s", libraryID)
	return nil
}

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

		narratorsJSON, _ := json.Marshal(meta.Narrators)
		audioFilesJSON, _ := json.Marshal(meta.AudioFiles)
		ebookFileJSON, _ := json.Marshal(meta.EbookFile)
		chaptersJSON, _ := json.Marshal(meta.Chapters)
		tagsJSON, _ := json.Marshal(meta.Tags)
		genresJSON, _ := json.Marshal(meta.Genres)

		var coverPath interface{}
		if meta.CoverPath != "" {
			coverPath = meta.CoverPath
		}

		cols := getTableColumnsTx(tx, "books")
		var colNames []string
		var placeholders []string
		var args []interface{}

		addCol := func(name string, val interface{}) {
			if cols[name] {
				colNames = append(colNames, name)
				placeholders = append(placeholders, "?")
				args = append(args, val)
			}
		}

		addCol("id", mediaID)
		addCol("title", title)
		addCol("titleIgnorePrefix", titleIgnorePrefix)
		addCol("subtitle", meta.Subtitle)
		addCol("publishedYear", meta.PublishedYear)
		addCol("publishedDate", meta.PublishedDate)
		addCol("publisher", meta.Publisher)
		addCol("description", meta.Description)
		addCol("isbn", meta.ISBN)
		addCol("asin", meta.ASIN)
		addCol("language", meta.Language)
		addCol("explicit", 0)
		addCol("abridged", 0)
		addCol("coverPath", coverPath)
		addCol("duration", meta.Duration)
		addCol("narrators", narratorsJSON)
		addCol("audioFiles", audioFilesJSON)
		addCol("ebookFile", ebookFileJSON)
		addCol("chapters", chaptersJSON)
		addCol("tags", tagsJSON)
		addCol("genres", genresJSON)
		addCol("createdAt", nowStr)
		addCol("updatedAt", nowStr)

		query := fmt.Sprintf("INSERT INTO books (%s) VALUES (%s)", strings.Join(colNames, ", "), strings.Join(placeholders, ", "))
		log.Printf("[Scanner] [%s] scanNewLibraryItem: Inserting into books table", itemPath)
		_, err = tx.Exec(query, args...)
		if err != nil {
			return err
		}

		log.Printf("[Scanner] [%s] scanNewLibraryItem: Inserting authors", itemPath)
		for _, author := range meta.Authors {
			authorID := uuidStr()
			lastFirst := NameToLastFirst(author)
			_ = insertAuthor(tx, authorID, author, lastFirst, libraryID)

			var existingAuthorID string
			_ = tx.QueryRow("SELECT id FROM authors WHERE name = ? AND libraryId = ?", author, libraryID).Scan(&existingAuthorID)
			if existingAuthorID != "" {
				authorID = existingAuthorID
			}
			_ = insertBookAuthor(tx, mediaID, authorID)
		}

		if meta.SeriesName != "" {
			log.Printf("[Scanner] [%s] scanNewLibraryItem: Inserting series", itemPath)
			seriesID := uuidStr()
			_ = insertSeries(tx, seriesID, meta.SeriesName, libraryID)

			var existingSeriesID string
			_ = tx.QueryRow("SELECT id FROM series WHERE name = ? AND libraryId = ?", meta.SeriesName, libraryID).Scan(&existingSeriesID)
			if existingSeriesID != "" {
				seriesID = existingSeriesID
			}
			_ = insertBookSeries(tx, mediaID, seriesID, meta.SeriesSequence)
		}

	} else if mediaType == "podcast" {
		tagsJSON, _ := json.Marshal(meta.Tags)
		genresJSON, _ := json.Marshal(meta.Genres)
		var author string
		if len(meta.Authors) > 0 {
			author = meta.Authors[0]
		}

		cols := getTableColumnsTx(tx, "podcasts")
		var colNames []string
		var placeholders []string
		var args []interface{}

		addCol := func(name string, val interface{}) {
			if cols[name] {
				colNames = append(colNames, name)
				placeholders = append(placeholders, "?")
				args = append(args, val)
			}
		}

		addCol("id", mediaID)
		addCol("title", title)
		addCol("titleIgnorePrefix", titleIgnorePrefix)
		addCol("author", author)
		addCol("releaseDate", meta.PublishedDate)
		addCol("feedURL", "")
		addCol("imageURL", "")
		addCol("description", meta.Description)
		addCol("itunesPageURL", "")
		addCol("itunesId", "")
		addCol("itunesArtistId", "")
		addCol("language", meta.Language)
		addCol("podcastType", "")
		addCol("explicit", 0)
		addCol("autoDownloadEpisodes", 0)
		addCol("autoDownloadSchedule", "")
		addCol("lastEpisodeCheck", "")
		addCol("maxEpisodesToKeep", 0)
		addCol("maxNewEpisodesToDownload", 0)
		addCol("coverPath", meta.CoverPath)
		addCol("tags", tagsJSON)
		addCol("genres", genresJSON)
		addCol("numEpisodes", len(meta.AudioFiles))
		addCol("createdAt", nowStr)
		addCol("updatedAt", nowStr)

		query := fmt.Sprintf("INSERT INTO podcasts (%s) VALUES (%s)", strings.Join(colNames, ", "), strings.Join(placeholders, ", "))
		log.Printf("[Scanner] [%s] scanNewLibraryItem: Inserting into podcasts table", itemPath)
		_, err = tx.Exec(query, args...)
		if err != nil {
			return err
		}

		log.Printf("[Scanner] [%s] scanNewLibraryItem: Inserting podcast episodes", itemPath)
		for _, ep := range meta.PodcastEpisodes {
			audioFileJSON, _ := json.Marshal(ep.AudioFile)

			colsEp := getTableColumnsTx(tx, "podcastEpisodes")
			var colNamesEp []string
			var placeholdersEp []string
			var argsEp []interface{}

			addColEp := func(name string, val interface{}) {
				if colsEp[name] {
					colNamesEp = append(colNamesEp, name)
					placeholdersEp = append(placeholdersEp, "?")
					argsEp = append(argsEp, val)
				}
			}

			addColEp("id", ep.ID)
			addColEp("podcastId", mediaID)
			addColEp("title", ep.Title)
			addColEp("audioFile", string(audioFileJSON))
			addColEp("createdAt", nowStr)
			addColEp("updatedAt", nowStr)

			qEp := fmt.Sprintf("INSERT INTO podcastEpisodes (%s) VALUES (%s)", strings.Join(colNamesEp, ", "), strings.Join(placeholdersEp, ", "))
			_, err = tx.Exec(qEp, argsEp...)
			if err != nil {
				return err
			}
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

	if mediaType == "podcast" {
		var libraryName string
		_ = db.QueryRow("SELECT name FROM libraries WHERE id = ?", libraryID).Scan(&libraryName)
		for _, ep := range meta.PodcastEpisodes {
			extraData := map[string]string{
				"podcastTitle": title,
				"episodeTitle": ep.Title,
				"libraryName":  libraryName,
			}
			inotification.TriggerEvent(context.Background(), db, "onPodcastEpisodeDownloaded", &libraryID, "New Episode", fmt.Sprintf("%s - %s", title, ep.Title), extraData)
		}
	} else if mediaType == "book" {
		var libraryName string
		_ = db.QueryRow("SELECT name FROM libraries WHERE id = ?", libraryID).Scan(&libraryName)
		extraData := map[string]string{
			"title":       title,
			"author":      authorNamesFirstLast,
			"libraryName": libraryName,
		}
		inotification.TriggerEvent(context.Background(), db, "onItemAdded", &libraryID, "New Book Added", fmt.Sprintf("%s by %s", title, authorNamesFirstLast), extraData)
	}

	if socketAuth != nil {
		if minItem, err := GetLibraryItemMinifiedByID(db, itemID); err == nil {
			EmitLibraryItemsEvent(socketAuth, "items_added", minItem)
		}
	}

	return nil
}

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
		var bLockedFields []byte
		var dbTitle, dbSubtitle, dbPublishedYear, dbPublishedDate, dbPublisher, dbDescription, dbIsbn, dbAsin, dbLanguage, dbCoverPath sql.NullString
		var dbNarrators, dbTags, dbGenres []byte

		_ = tx.QueryRow(`
			SELECT title, subtitle, publishedYear, publishedDate, publisher, description, isbn, asin, language, coverPath, narrators, tags, genres, lockedFields
			FROM books WHERE id = ?
		`, mediaID).Scan(
			&dbTitle, &dbSubtitle, &dbPublishedYear, &dbPublishedDate, &dbPublisher, &dbDescription, &dbIsbn, &dbAsin, &dbLanguage, &dbCoverPath, &dbNarrators, &dbTags, &dbGenres, &bLockedFields,
		)

		var lockedFields []string
		if len(bLockedFields) > 0 {
			_ = json.Unmarshal(bLockedFields, &lockedFields)
		}

		isLocked := func(field string) bool {
			for _, f := range lockedFields {
				if f == field {
					return true
				}
			}
			return false
		}

		if isLocked("title") && dbTitle.String != "" {
			title = dbTitle.String
			titleIgnorePrefix = getTitleIgnorePrefixGo(title, prefixes)
		}
		if isLocked("subtitle") && dbSubtitle.Valid {
			meta.Subtitle = dbSubtitle.String
		}
		if isLocked("publishedYear") && dbPublishedYear.Valid {
			meta.PublishedYear = dbPublishedYear.String
		}
		if isLocked("publishedDate") && dbPublishedDate.Valid {
			meta.PublishedDate = dbPublishedDate.String
		}
		if isLocked("publisher") && dbPublisher.Valid {
			meta.Publisher = dbPublisher.String
		}
		if isLocked("description") && dbDescription.Valid {
			meta.Description = dbDescription.String
		}
		if isLocked("isbn") && dbIsbn.Valid {
			meta.ISBN = dbIsbn.String
		}
		if isLocked("asin") && dbAsin.Valid {
			meta.ASIN = dbAsin.String
		}
		if isLocked("language") && dbLanguage.Valid {
			meta.Language = dbLanguage.String
		}
		if (isLocked("cover") || isLocked("coverPath")) && dbCoverPath.Valid {
			meta.CoverPath = dbCoverPath.String
		}
		if (isLocked("narrators") || isLocked("narrator")) && len(dbNarrators) > 0 {
			var narrators []string
			if err := json.Unmarshal(dbNarrators, &narrators); err == nil {
				meta.Narrators = narrators
			}
		}
		if isLocked("tags") && len(dbTags) > 0 {
			var tags []string
			if err := json.Unmarshal(dbTags, &tags); err == nil {
				meta.Tags = tags
			}
		}
		if isLocked("genres") && len(dbGenres) > 0 {
			var genres []string
			if err := json.Unmarshal(dbGenres, &genres); err == nil {
				meta.Genres = genres
			}
		}
		if isLocked("authors") || isLocked("author") {
			rows, err := tx.Query("SELECT name FROM authors WHERE id IN (SELECT authorId FROM bookAuthors WHERE bookId = ?)", mediaID)
			if err == nil {
				defer rows.Close()
				var dbAuthors []string
				for rows.Next() {
					var name string
					if err := rows.Scan(&name); err == nil {
						dbAuthors = append(dbAuthors, name)
					}
				}
				if len(dbAuthors) > 0 {
					meta.Authors = dbAuthors
				}
			}
		}
		if isLocked("series") {
			var dbSeriesName string
			var dbSequence string
			err := tx.QueryRow(`
				SELECT s.name, bs.sequence
				FROM series s
				JOIN bookSeries bs ON s.id = bs.seriesId
				WHERE bs.bookId = ?
			`, mediaID).Scan(&dbSeriesName, &dbSequence)
			if err == nil {
				meta.SeriesName = dbSeriesName
				meta.SeriesSequence = dbSequence
			}
		}

		authorNamesFirstLast = strings.Join(meta.Authors, ", ")
		var lfs []string
		for _, a := range meta.Authors {
			lfs = append(lfs, NameToLastFirst(a))
		}
		authorNamesLastFirst = strings.Join(lfs, ", ")

		narratorsJSON, _ := json.Marshal(meta.Narrators)
		audioFilesJSON, _ := json.Marshal(meta.AudioFiles)
		ebookFileJSON, _ := json.Marshal(meta.EbookFile)
		chaptersJSON, _ := json.Marshal(meta.Chapters)
		tagsJSON, _ := json.Marshal(meta.Tags)
		genresJSON, _ := json.Marshal(meta.Genres)

		var coverPath interface{}
		if meta.CoverPath != "" {
			coverPath = meta.CoverPath
		}

		cols := getTableColumnsTx(tx, "books")
		var setStmts []string
		var args []interface{}

		addCol := func(name string, val interface{}) {
			if cols[name] {
				setStmts = append(setStmts, fmt.Sprintf("%s = ?", name))
				args = append(args, val)
			}
		}

		addCol("title", title)
		addCol("titleIgnorePrefix", titleIgnorePrefix)
		addCol("subtitle", meta.Subtitle)
		addCol("publishedYear", meta.PublishedYear)
		addCol("publishedDate", meta.PublishedDate)
		addCol("publisher", meta.Publisher)
		addCol("description", meta.Description)
		addCol("isbn", meta.ISBN)
		addCol("asin", meta.ASIN)
		addCol("language", meta.Language)
		addCol("coverPath", coverPath)
		addCol("duration", meta.Duration)
		addCol("narrators", narratorsJSON)
		addCol("audioFiles", audioFilesJSON)
		addCol("ebookFile", ebookFileJSON)
		addCol("chapters", chaptersJSON)
		addCol("tags", tagsJSON)
		addCol("genres", genresJSON)
		addCol("updatedAt", nowStr)

		args = append(args, mediaID)
		query := fmt.Sprintf("UPDATE books SET %s WHERE id = ?", strings.Join(setStmts, ", "))
		log.Printf("[Scanner] [%s] scanExistingLibraryItem: Updating books table", itemPath)
		_, err = tx.Exec(query, args...)
		if err != nil {
			return err
		}

		log.Printf("[Scanner] [%s] scanExistingLibraryItem: Updating authors", itemPath)
		if tableExistsTx(tx, "bookAuthors") {
			_, _ = tx.Exec("DELETE FROM bookAuthors WHERE bookId = ?", mediaID)
		}
		for _, author := range meta.Authors {
			authorID := uuidStr()
			lastFirst := NameToLastFirst(author)
			_ = insertAuthor(tx, authorID, author, lastFirst, libraryID)

			var existingAuthorID string
			_ = tx.QueryRow("SELECT id FROM authors WHERE name = ? AND libraryId = ?", author, libraryID).Scan(&existingAuthorID)
			if existingAuthorID != "" {
				authorID = existingAuthorID
			}
			_ = insertBookAuthor(tx, mediaID, authorID)
		}

		log.Printf("[Scanner] [%s] scanExistingLibraryItem: Updating series", itemPath)
		if tableExistsTx(tx, "bookSeries") {
			_, _ = tx.Exec("DELETE FROM bookSeries WHERE bookId = ?", mediaID)
		}
		if meta.SeriesName != "" {
			seriesID := uuidStr()
			_ = insertSeries(tx, seriesID, meta.SeriesName, libraryID)

			var existingSeriesID string
			_ = tx.QueryRow("SELECT id FROM series WHERE name = ? AND libraryId = ?", meta.SeriesName, libraryID).Scan(&existingSeriesID)
			if existingSeriesID != "" {
				seriesID = existingSeriesID
			}
			_ = insertBookSeries(tx, mediaID, seriesID, meta.SeriesSequence)
		}

	} else if mediaType == "podcast" {
		var pLockedFields []byte
		var dbTitle, dbAuthor, dbDescription, dbLanguage, dbCoverPath sql.NullString
		var dbTags, dbGenres []byte

		_ = tx.QueryRow(`
			SELECT title, author, description, language, coverPath, tags, genres, lockedFields
			FROM podcasts WHERE id = ?
		`, mediaID).Scan(
			&dbTitle, &dbAuthor, &dbDescription, &dbLanguage, &dbCoverPath, &dbTags, &dbGenres, &pLockedFields,
		)

		var lockedFields []string
		if len(pLockedFields) > 0 {
			_ = json.Unmarshal(pLockedFields, &lockedFields)
		}

		isLocked := func(field string) bool {
			for _, f := range lockedFields {
				if f == field {
					return true
				}
			}
			return false
		}

		if isLocked("title") && dbTitle.String != "" {
			title = dbTitle.String
			titleIgnorePrefix = getTitleIgnorePrefixGo(title, prefixes)
		}
		var author string
		if len(meta.Authors) > 0 {
			author = meta.Authors[0]
		}
		if (isLocked("author") || isLocked("authors")) && dbAuthor.Valid {
			author = dbAuthor.String
		}
		if isLocked("description") && dbDescription.Valid {
			meta.Description = dbDescription.String
		}
		if isLocked("language") && dbLanguage.Valid {
			meta.Language = dbLanguage.String
		}
		if (isLocked("cover") || isLocked("coverPath")) && dbCoverPath.Valid {
			meta.CoverPath = dbCoverPath.String
		}
		if isLocked("tags") && len(dbTags) > 0 {
			var tags []string
			if err := json.Unmarshal(dbTags, &tags); err == nil {
				meta.Tags = tags
			}
		}
		if isLocked("genres") && len(dbGenres) > 0 {
			var genres []string
			if err := json.Unmarshal(dbGenres, &genres); err == nil {
				meta.Genres = genres
			}
		}

		tagsJSON, _ := json.Marshal(meta.Tags)
		genresJSON, _ := json.Marshal(meta.Genres)

		cols := getTableColumnsTx(tx, "podcasts")
		var setStmts []string
		var args []interface{}

		addCol := func(name string, val interface{}) {
			if cols[name] {
				setStmts = append(setStmts, fmt.Sprintf("%s = ?", name))
				args = append(args, val)
			}
		}

		addCol("title", title)
		addCol("titleIgnorePrefix", titleIgnorePrefix)
		addCol("author", author)
		addCol("releaseDate", meta.PublishedDate)
		addCol("description", meta.Description)
		addCol("language", meta.Language)
		addCol("coverPath", meta.CoverPath)
		addCol("tags", tagsJSON)
		addCol("genres", genresJSON)
		addCol("numEpisodes", len(meta.AudioFiles))
		addCol("updatedAt", nowStr)

		args = append(args, mediaID)
		query := fmt.Sprintf("UPDATE podcasts SET %s WHERE id = ?", strings.Join(setStmts, ", "))
		log.Printf("[Scanner] [%s] scanExistingLibraryItem: Updating podcasts table", itemPath)
		_, err = tx.Exec(query, args...)
		if err != nil {
			return err
		}

		log.Printf("[Scanner] [%s] scanExistingLibraryItem: Updating podcast episodes", itemPath)
		if tableExistsTx(tx, "podcastEpisodes") {
			_, _ = tx.Exec("DELETE FROM podcastEpisodes WHERE podcastId = ?", mediaID)
		}
		for _, ep := range meta.PodcastEpisodes {
			audioFileJSON, _ := json.Marshal(ep.AudioFile)

			colsEp := getTableColumnsTx(tx, "podcastEpisodes")
			var colNamesEp []string
			var placeholdersEp []string
			var argsEp []interface{}

			addColEp := func(name string, val interface{}) {
				if colsEp[name] {
					colNamesEp = append(colNamesEp, name)
					placeholdersEp = append(placeholdersEp, "?")
					argsEp = append(argsEp, val)
				}
			}

			addColEp("id", ep.ID)
			addColEp("podcastId", mediaID)
			addColEp("title", ep.Title)
			addColEp("audioFile", string(audioFileJSON))
			addColEp("createdAt", nowStr)
			addColEp("updatedAt", nowStr)

			qEp := fmt.Sprintf("INSERT INTO podcastEpisodes (%s) VALUES (%s)", strings.Join(colNamesEp, ", "), strings.Join(placeholdersEp, ", "))
			_, err = tx.Exec(qEp, argsEp...)
			if err != nil {
				return err
			}
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

// LibraryItemMinifiedJSON is the minified library item structure for API responses.
type LibraryItemMinifiedJSON struct {
	ID               string      `json:"id"`
	Ino              string      `json:"ino"`
	OldLibraryItemID *string     `json:"oldLibraryItemId"`
	LibraryID        string      `json:"libraryId"`
	FolderID         string      `json:"folderId"`
	Path             string      `json:"path"`
	RelPath          string      `json:"relPath"`
	IsFile           bool        `json:"isFile"`
	MtimeMs          int64       `json:"mtimeMs"`
	CtimeMs          int64       `json:"ctimeMs"`
	BirthtimeMs      int64       `json:"birthtimeMs"`
	AddedAt          int64       `json:"addedAt"`
	UpdatedAt        int64       `json:"updatedAt"`
	IsMissing        bool        `json:"isMissing"`
	IsInvalid        bool        `json:"isInvalid"`
	MediaType        string      `json:"mediaType"`
	Media            interface{} `json:"media"`
	NumFiles         int         `json:"numFiles"`
	Size             int64       `json:"size"`
}

// BookMinifiedJSON is the minified book structure.
type BookMinifiedJSON struct {
	ID            string                `json:"id"`
	Metadata      *BookMetadataMinified `json:"metadata"`
	CoverPath     *string               `json:"coverPath"`
	Tags          []string              `json:"tags"`
	NumTracks     int                   `json:"numTracks"`
	NumAudioFiles int                   `json:"numAudioFiles"`
	NumChapters   int                   `json:"numChapters"`
	Duration      float64               `json:"duration"`
	Size          int64                 `json:"size"`
	EbookFormat   *string               `json:"ebookFormat"`
}

type BookSeriesMinifiedJSON struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Sequence string `json:"sequence"`
}

// BookMetadataMinified holds minified book metadata.
type BookMetadataMinified struct {
	Title             string                    `json:"title"`
	TitleIgnorePrefix string                    `json:"titleIgnorePrefix"`
	Subtitle          *string                   `json:"subtitle"`
	AuthorName        string                    `json:"authorName"`
	AuthorNameLF      string                    `json:"authorNameLF"`
	NarratorName      string                    `json:"narratorName"`
	SeriesName        string                    `json:"seriesName"`
	SeriesSequence    *string                   `json:"seriesSequence"`
	Series            []*BookSeriesMinifiedJSON `json:"series"`
	Genres            []string                  `json:"genres"`
	PublishedYear     *string                   `json:"publishedYear"`
	PublishedDate     *string                   `json:"publishedDate"`
	Publisher         *string                   `json:"publisher"`
	Description       *string                   `json:"description"`
	Isbn              *string                   `json:"isbn"`
	Asin              *string                   `json:"asin"`
	Language          *string                   `json:"language"`
	Explicit          bool                      `json:"explicit"`
	Abridged          bool                      `json:"abridged"`
}

// PodcastMinifiedJSON is the minified podcast structure.
type PodcastMinifiedJSON struct {
	ID                       string              `json:"id"`
	Metadata                 *PodcastMetadataMin `json:"metadata"`
	CoverPath                *string             `json:"coverPath"`
	Tags                     []string            `json:"tags"`
	NumEpisodes              int                 `json:"numEpisodes"`
	AutoDownloadEpisodes     bool                `json:"autoDownloadEpisodes"`
	AutoDownloadSchedule     *string             `json:"autoDownloadSchedule"`
	LastEpisodeCheck         *int64              `json:"lastEpisodeCheck"`
	MaxEpisodesToKeep        int                 `json:"maxEpisodesToKeep"`
	MaxNewEpisodesToDownload int                 `json:"maxNewEpisodesToDownload"`
	Size                     int64               `json:"size"`
}

// PodcastMetadataMin holds minified podcast metadata.
type PodcastMetadataMin struct {
	Title             string   `json:"title"`
	TitleIgnorePrefix string   `json:"titleIgnorePrefix"`
	Author            *string  `json:"author"`
	Description       *string  `json:"description"`
	ReleaseDate       *string  `json:"releaseDate"`
	Genres            []string `json:"genres"`
	FeedURL           *string  `json:"feedUrl"`
	ImageURL          *string  `json:"imageUrl"`
	ItunesPageURL     *string  `json:"itunesPageUrl"`
	ItunesID          *string  `json:"itunesId"`
	ItunesArtistID    *string  `json:"itunesArtistId"`
	Explicit          bool     `json:"explicit"`
	Language          *string  `json:"language"`
	Type              *string  `json:"type"`
}

// GetLibraryItemMinifiedByID fetches a minified library item by ID.
func GetLibraryItemMinifiedByID(db *sql.DB, itemID string) (*LibraryItemMinifiedJSON, error) {
	var li LibraryItemMinifiedJSON
	var id, ino, libraryID, folderID, path, relPath, mediaType, mediaID, mtimeStr, ctimeStr, birthtimeStr, createdAtStr, updatedAtStr string
	var isFileVal, isMissingVal, isInvalidVal int
	var size int64

	query := `
		SELECT id, ino, libraryId, libraryFolderId, path, relPath, isFile, mtime, ctime, birthtime, createdAt, updatedAt, isMissing, isInvalid, mediaType, mediaId, size
		FROM libraryItems
		WHERE id = ?
	`
	err := db.QueryRow(query, itemID).Scan(
		&id, &ino, &libraryID, &folderID, &path, &relPath, &isFileVal, &mtimeStr, &ctimeStr, &birthtimeStr, &createdAtStr, &updatedAtStr, &isMissingVal, &isInvalidVal, &mediaType, &mediaID, &size,
	)
	if err != nil {
		return nil, err
	}

	li.ID = id
	li.Ino = ino
	li.LibraryID = libraryID
	li.FolderID = folderID
	li.Path = path
	li.RelPath = relPath
	li.IsFile = isFileVal != 0
	li.MtimeMs = parseEpochMillis(mtimeStr)
	li.CtimeMs = parseEpochMillis(ctimeStr)
	li.BirthtimeMs = parseEpochMillis(birthtimeStr)
	li.AddedAt = parseEpochMillis(createdAtStr)
	li.UpdatedAt = parseEpochMillis(updatedAtStr)
	li.IsMissing = isMissingVal != 0
	li.IsInvalid = isInvalidVal != 0
	li.MediaType = mediaType
	li.Size = size

	if mediaType == "book" {
		var bTitle, bTitleIgnorePrefix, bSubtitle, bPublishedYear, bPublishedDate, bPublisher, bDescription, bIsbn, bAsin, bLanguage, bCoverPath string
		var bDuration float64
		var bNarrators, bAudioFiles, bEbookFile, bChapters, bTags, bGenres []byte
		var bExplicit, bAbridged int

		err = db.QueryRow(`
			SELECT title, titleIgnorePrefix, subtitle, publishedYear, publishedDate, publisher, description, isbn, asin, language, explicit, abridged, coverPath, duration, narrators, audioFiles, ebookFile, chapters, tags, genres
			FROM books WHERE id = ?
		`, mediaID).Scan(
			&bTitle, &bTitleIgnorePrefix, &bSubtitle, &bPublishedYear, &bPublishedDate, &bPublisher, &bDescription, &bIsbn, &bAsin, &bLanguage, &bExplicit, &bAbridged, &bCoverPath, &bDuration, &bNarrators, &bAudioFiles, &bEbookFile, &bChapters, &bTags, &bGenres,
		)
		if err == nil {
			var tags []string
			_ = json.Unmarshal(bTags, &tags)
			var genres []string
			_ = json.Unmarshal(bGenres, &genres)
			var audioFiles []interface{}
			_ = json.Unmarshal(bAudioFiles, &audioFiles)
			var ebook interface{}
			_ = json.Unmarshal(bEbookFile, &ebook)
			var chapters []interface{}
			_ = json.Unmarshal(bChapters, &chapters)

			var authorNames []string
			var seriesNames []string
			var narratorNames []string
			_ = json.Unmarshal(bNarrators, &narratorNames)

			if tableExists(db, "bookAuthors") && tableExists(db, "authors") {
				rows, err := db.Query("SELECT name FROM authors WHERE id IN (SELECT authorId FROM bookAuthors WHERE bookId = ?)", mediaID)
				if err != nil {
					log.Printf("[Scanner] Failed to query authors: %v", err)
				} else {
					defer rows.Close()
					for rows.Next() {
						var name string
						if err := rows.Scan(&name); err != nil {
							log.Printf("[Scanner] Failed to scan author name: %v", err)
							continue
						}
						authorNames = append(authorNames, name)
					}
					if err := rows.Err(); err != nil {
						log.Printf("[Scanner] Authors iteration error: %v", err)
					}
				}
			}
			var seriesList []*BookSeriesMinifiedJSON
			if tableExists(db, "bookSeries") && tableExists(db, "series") {
				rows, err := db.Query("SELECT s.id, s.name, bs.sequence FROM series s JOIN bookSeries bs ON s.id = bs.seriesId WHERE bs.bookId = ?", mediaID)
				if err != nil {
					log.Printf("[Scanner] Failed to query series: %v", err)
				} else {
					defer rows.Close()
					for rows.Next() {
						var sid, name string
						var sequence sql.NullString
						if err := rows.Scan(&sid, &name, &sequence); err != nil {
							log.Printf("[Scanner] Failed to scan series name/sequence: %v", err)
							continue
						}
						var seqVal string
						if sequence.Valid {
							seqVal = sequence.String
						}
						seriesList = append(seriesList, &BookSeriesMinifiedJSON{
							ID:       sid,
							Name:     name,
							Sequence: seqVal,
						})
						if seqVal != "" {
							seriesNames = append(seriesNames, fmt.Sprintf("%s #%s", name, seqVal))
						} else {
							seriesNames = append(seriesNames, name)
						}
					}
					if err := rows.Err(); err != nil {
						log.Printf("[Scanner] Series iteration error: %v", err)
					}
				}
			}

			var firstSeq *string
			if len(seriesList) > 0 && seriesList[0].Sequence != "" {
				firstSeq = &seriesList[0].Sequence
			}

			authorName := strings.Join(authorNames, ", ")
			seriesName := strings.Join(seriesNames, ", ")
			narratorName := strings.Join(narratorNames, ", ")

			var ebookFormat *string
			if len(bEbookFile) > 0 {
				var eb struct {
					EbookFormat string `json:"ebookFormat"`
				}
				if json.Unmarshal(bEbookFile, &eb) == nil && eb.EbookFormat != "" {
					ebookFormat = &eb.EbookFormat
				}
			}

			bookMin := &BookMinifiedJSON{
				ID:            mediaID,
				CoverPath:     nullIfEmpty(bCoverPath),
				Tags:          tags,
				NumTracks:     len(audioFiles),
				NumAudioFiles: len(audioFiles),
				NumChapters:   len(chapters),
				Duration:      bDuration,
				Size:          size,
				EbookFormat:   ebookFormat,
				Metadata: &BookMetadataMinified{
					Title:             bTitle,
					TitleIgnorePrefix: bTitleIgnorePrefix,
					Subtitle:          nullIfEmpty(bSubtitle),
					AuthorName:        authorName,
					AuthorNameLF:      NameToLastFirst(authorName),
					NarratorName:      narratorName,
					SeriesName:        seriesName,
					SeriesSequence:    firstSeq,
					Series:            seriesList,
					Genres:            genres,
					PublishedYear:     nullIfEmpty(bPublishedYear),
					PublishedDate:     nullIfEmpty(bPublishedDate),
					Publisher:         nullIfEmpty(bPublisher),
					Description:       nullIfEmpty(bDescription),
					Isbn:              nullIfEmpty(bIsbn),
					Asin:              nullIfEmpty(bAsin),
					Language:          nullIfEmpty(bLanguage),
					Explicit:          bExplicit != 0,
					Abridged:          bAbridged != 0,
				},
			}
			li.Media = bookMin
		}
	} else if mediaType == "podcast" {
		var pTitle, pTitleIgnorePrefix, pAuthor, pReleaseDate, pFeedURL, pImageURL, pDescription, pItunesPageURL, pItunesID, pItunesArtistID, pLanguage, pPodcastType, pCoverPath string
		var pExplicit, pAutoDownloadEpisodes, pMaxEpisodesToKeep, pMaxNewEpisodesToDownload, pNumEpisodes int
		var pTags, pGenres []byte

		err = db.QueryRow(`
			SELECT title, titleIgnorePrefix, author, releaseDate, feedURL, imageURL, description, itunesPageURL, itunesId, itunesArtistId, language, podcastType, explicit, autoDownloadEpisodes, maxEpisodesToKeep, maxNewEpisodesToDownload, coverPath, tags, genres, numEpisodes
			FROM podcasts WHERE id = ?
		`, mediaID).Scan(
			&pTitle, &pTitleIgnorePrefix, &pAuthor, &pReleaseDate, &pFeedURL, &pImageURL, &pDescription, &pItunesPageURL, &pItunesID, &pItunesArtistID, &pLanguage, &pPodcastType, &pExplicit, &pAutoDownloadEpisodes, &pMaxEpisodesToKeep, &pMaxNewEpisodesToDownload, &pCoverPath, &pTags, &pGenres, &pNumEpisodes,
		)
		if err == nil {
			var tags []string
			_ = json.Unmarshal(pTags, &tags)
			var genres []string
			_ = json.Unmarshal(pGenres, &genres)

			podcastMin := &PodcastMinifiedJSON{
				ID:                       mediaID,
				CoverPath:                nullIfEmpty(pCoverPath),
				Tags:                     tags,
				NumEpisodes:              pNumEpisodes,
				AutoDownloadEpisodes:     pAutoDownloadEpisodes != 0,
				MaxEpisodesToKeep:        pMaxEpisodesToKeep,
				MaxNewEpisodesToDownload: pMaxNewEpisodesToDownload,
				Size:                     size,
				Metadata: &PodcastMetadataMin{
					Title:             pTitle,
					TitleIgnorePrefix: pTitleIgnorePrefix,
					Author:            nullIfEmpty(pAuthor),
					Description:       nullIfEmpty(pDescription),
					ReleaseDate:       nullIfEmpty(pReleaseDate),
					Genres:            genres,
					FeedURL:           nullIfEmpty(pFeedURL),
					ImageURL:          nullIfEmpty(pImageURL),
					ItunesPageURL:     nullIfEmpty(pItunesPageURL),
					ItunesID:          nullIfEmpty(pItunesID),
					ItunesArtistID:    nullIfEmpty(pItunesArtistID),
					Explicit:          pExplicit != 0,
					Language:          nullIfEmpty(pLanguage),
					Type:              nullIfEmpty(pPodcastType),
				},
			}
			li.Media = podcastMin
		}
	}

	return &li, nil
}

// HandleScanLibrary returns an HTTP handler for triggering a library scan.
func HandleScanLibrary(db *sql.DB, libraryID string, socketAuth *isocket.Authority) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userSess := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if userSess.Type != "root" && userSess.Type != "admin" && !userSess.CanUpdate {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		var count int
		err := db.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM libraries WHERE id = ?", libraryID).Scan(&count)
		if err != nil {
			log.Printf("[Scanner] Database error: %v", err)
			http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
			return
		}
		if count == 0 {
			http.Error(w, `{"error": "Library not found"}`, http.StatusNotFound)
			return
		}

		go func() {
			if err := ScanLibrary(db, libraryID, socketAuth); err != nil {
				log.Printf("[Scanner] Scan failed: %v", err)
			}
		}()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"success": true}`))
	}
}
