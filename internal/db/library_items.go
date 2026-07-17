package db

import (
	"database/sql"
	"fmt"
)

// GetLibraryItemDownloadInfo fetches file path, relPath, and isFile status for a library item.
func GetLibraryItemDownloadInfo(db *sql.DB, itemID string) (*LibraryItemDownloadInfo, error) {
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	var info LibraryItemDownloadInfo
	var isFileVal sql.NullInt64
	var pathStr, relPathStr sql.NullString
	err := db.QueryRow("SELECT path, relPath, isFile FROM libraryItems WHERE id = ?", itemID).
		Scan(&pathStr, &relPathStr, &isFileVal)
	if err != nil {
		return nil, err
	}
	info.Path = pathStr.String
	info.RelPath = relPathStr.String
	info.IsFile = isFileVal.Valid && isFileVal.Int64 != 0
	return &info, nil
}

// GetCoverPath reads the media coverPath from books or podcasts table based on the library item ID.
func GetCoverPath(db *sql.DB, itemID string) (string, error) {
	if db == nil {
		return "", fmt.Errorf("database not initialized")
	}
	var mediaType, mediaID string
	err := db.QueryRow("SELECT mediaType, mediaId FROM libraryItems WHERE id = ?", itemID).Scan(&mediaType, &mediaID)
	if err != nil {
		return "", err
	}

	var coverPath sql.NullString
	if mediaType == "book" {
		err = db.QueryRow("SELECT coverPath FROM books WHERE id = ?", mediaID).Scan(&coverPath)
	} else if mediaType == "podcast" {
		err = db.QueryRow("SELECT coverPath FROM podcasts WHERE id = ?", mediaID).Scan(&coverPath)
	} else {
		return "", fmt.Errorf("unknown media type: %s", mediaType)
	}

	if err != nil {
		return "", err
	}

	if !coverPath.Valid {
		return "", nil
	}

	return coverPath.String, nil
}

// GetLibraryItemMinifiedByID retrieves a library item in its minified JSON form by ID.
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
		bookMin, err := fetchBookMinified(db, mediaID, size)
		if err == nil {
			li.Media = bookMin
		}
	} else if mediaType == "podcast" {
		podcastMin, err := fetchPodcastMinified(db, mediaID, size)
		if err == nil {
			li.Media = podcastMin
		}
	}

	return &li, nil
}

// GetFilteredLibraryItems retrieves filtered and sorted library items.
func GetFilteredLibraryItems(db *sql.DB, options GetFilteredLibraryItemsOptions) ([]*LibraryItemMinifiedJSON, int, error) {
	sortingIgnorePrefix := GetSortingIgnorePrefix(db)

	whereClause, args := buildFilteredItemsWhere(options)

	total, err := executeFilteredItemsCount(db, options.MediaType, whereClause, args)
	if err != nil {
		return nil, 0, err
	}

	selectQuery := buildFilteredItemsSelectQuery(options, whereClause, sortingIgnorePrefix, &args)

	rows, err := db.Query(selectQuery, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var results []*LibraryItemMinifiedJSON = []*LibraryItemMinifiedJSON{}
	var bookIDs []string
	bookMap := make(map[string]*BookMinifiedJSON)

	for rows.Next() {
		if options.MediaType == "book" {
			liMin, bookMin, bID, err := scanFilteredBookItem(rows, options.LibraryID)
			if err != nil {
				return nil, 0, err
			}
			bookIDs = append(bookIDs, bID)
			bookMap[bID] = bookMin
			results = append(results, liMin)
		} else {
			liMin, err := scanFilteredPodcastItem(rows, options.LibraryID)
			if err != nil {
				return nil, 0, err
			}
			results = append(results, liMin)
		}
	}

	_ = fetchSeriesForBooks(db, bookIDs, bookMap)

	return results, total, nil
}
