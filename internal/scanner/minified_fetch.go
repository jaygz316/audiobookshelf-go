package scanner

import (
	"database/sql"
)

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
		bookMin, err := getBookMinified(db, mediaID, size)
		if err == nil {
			li.Media = bookMin
		}
	} else if mediaType == "podcast" {
		podcastMin, err := getPodcastMinified(db, mediaID, size)
		if err == nil {
			li.Media = podcastMin
		}
	}

	return &li, nil
}
