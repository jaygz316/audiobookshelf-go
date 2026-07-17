package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"encoding/json"

	idb "audiobookshelf/internal/db"
	"audiobookshelf/internal/utils"
)

func fetchAuthorLibraryItems(db *sql.DB, authorID string) []interface{} {
	rows, err := db.Query(`
		SELECT li.id, li.ino, li.path, li.relPath, li.isFile, li.mtime, li.ctime, li.birthtime, li.createdAt, li.updatedAt, li.isMissing, li.isInvalid, li.mediaType, li.mediaId, li.size, li.libraryFolderId,
			b.title, b.titleIgnorePrefix, b.coverPath, b.tags, b.genres
		FROM libraryItems li
		JOIN bookAuthors ba ON ba.bookId = li.mediaId
		JOIN books b ON b.id = li.mediaId
		WHERE ba.authorId = ? AND li.mediaType = 'book'
	`, authorID)

	items := []interface{}{}
	if err != nil {
		log.Errorf("[Go] Failed to query author items: %v", err)
		return items
	}
	defer rows.Close()
	for rows.Next() {
		var itemID, path, relPath, mediaType, mediaID, title string
		var ino, folderID, titleIgnorePrefix, coverPath sql.NullString
		var isFileVal, isMissingVal, isInvalidVal int
		var mtimeStr, ctimeStr, birthtimeStr, createdAtStr, updatedAtStr string
		var size int64
		var tagsBytes, genresBytes []byte

		err := rows.Scan(
			&itemID, &ino, &path, &relPath, &isFileVal, &mtimeStr, &ctimeStr, &birthtimeStr, &createdAtStr, &updatedAtStr, &isMissingVal, &isInvalidVal, &mediaType, &mediaID, &size, &folderID,
			&title, &titleIgnorePrefix, &coverPath, &tagsBytes, &genresBytes,
		)
		if err == nil {
			var tags []string
			_ = json.Unmarshal(tagsBytes, &tags)
			var genres []string
			_ = json.Unmarshal(genresBytes, &genres)

			items = append(items, map[string]interface{}{
				"id":              itemID,
				"ino":             ino.String,
				"path":            path,
				"relPath":         relPath,
				"isFile":          isFileVal != 0,
				"mtimeMs":         idb.ParseEpochMillis(mtimeStr),
				"ctimeMs":         idb.ParseEpochMillis(ctimeStr),
				"birthtimeMs":     idb.ParseEpochMillis(birthtimeStr),
				"addedAt":         idb.ParseEpochMillis(createdAtStr),
				"updatedAt":       idb.ParseEpochMillis(updatedAtStr),
				"isMissing":       isMissingVal != 0,
				"isInvalid":       isInvalidVal != 0,
				"mediaType":       mediaType,
				"size":            size,
				"libraryFolderId": folderID.String,
				"media": map[string]interface{}{
					"id":        mediaID,
					"coverPath": utils.NullIfEmpty(coverPath.String),
					"tags":      tags,
					"metadata": map[string]interface{}{
						"title":             title,
						"titleIgnorePrefix": titleIgnorePrefix.String,
						"genres":            genres,
					},
				},
			})
		} else {
			log.Warnf("[Go Warning] Failed to scan author item: %v", err)
		}
	}
	return items
}

func fetchAuthorSeries(db *sql.DB, authorID string) []interface{} {
	rows, err := db.Query(`
		SELECT DISTINCT s.id, s.name
		FROM series s
		JOIN bookSeries bs ON bs.seriesId = s.id
		JOIN bookAuthors ba ON ba.bookId = bs.bookId
		WHERE ba.authorId = ?
	`, authorID)

	var series []interface{}
	if err != nil {
		log.Errorf("[Go] Failed to query author series: %v", err)
		return series
	}
	defer rows.Close()
	for rows.Next() {
		var sID, sName string
		if err := rows.Scan(&sID, &sName); err == nil {
			// Query books inside this series by this author
			bookRows, err := db.Query(`
				SELECT li.id, li.ino, li.path, li.relPath, li.isFile, li.mtime, li.ctime, li.birthtime, li.createdAt, li.updatedAt, li.isMissing, li.isInvalid, li.mediaType, li.mediaId, li.size, li.libraryFolderId,
					b.title, b.titleIgnorePrefix, b.coverPath, b.tags, b.genres, bs.sequence
				FROM libraryItems li
				JOIN bookAuthors ba ON ba.bookId = li.mediaId
				JOIN bookSeries bs ON bs.bookId = li.mediaId
				JOIN books b ON b.id = li.mediaId
				WHERE ba.authorId = ? AND bs.seriesId = ? AND li.mediaType = 'book'
			`, authorID, sID)

			books := []interface{}{}
			if err == nil {
				for bookRows.Next() {
					var itemID, path, relPath, mediaType, mediaID, title string
					var ino, folderID, titleIgnorePrefix, coverPath, sequence sql.NullString
					var isFileVal, isMissingVal, isInvalidVal int
					var mtimeStr, ctimeStr, birthtimeStr, createdAtStr, updatedAtStr string
					var size int64
					var tagsBytes, genresBytes []byte

					err := bookRows.Scan(
						&itemID, &ino, &path, &relPath, &isFileVal, &mtimeStr, &ctimeStr, &birthtimeStr, &createdAtStr, &updatedAtStr, &isMissingVal, &isInvalidVal, &mediaType, &mediaID, &size, &folderID,
						&title, &titleIgnorePrefix, &coverPath, &tagsBytes, &genresBytes, &sequence,
					)
					if err == nil {
						var tags []string
						_ = json.Unmarshal(tagsBytes, &tags)
						var genres []string
						_ = json.Unmarshal(genresBytes, &genres)

						books = append(books, map[string]interface{}{
							"id":              itemID,
							"ino":             ino.String,
							"path":            path,
							"relPath":         relPath,
							"isFile":          isFileVal != 0,
							"mtimeMs":         idb.ParseEpochMillis(mtimeStr),
							"ctimeMs":         idb.ParseEpochMillis(ctimeStr),
							"birthtimeMs":     idb.ParseEpochMillis(birthtimeStr),
							"addedAt":         idb.ParseEpochMillis(createdAtStr),
							"updatedAt":       idb.ParseEpochMillis(updatedAtStr),
							"isMissing":       isMissingVal != 0,
							"isInvalid":       isInvalidVal != 0,
							"mediaType":       mediaType,
							"size":            size,
							"libraryFolderId": folderID.String,
							"sequence":        sequence.String,
							"media": map[string]interface{}{
								"id":        mediaID,
								"coverPath": utils.NullIfEmpty(coverPath.String),
								"tags":      tags,
								"metadata": map[string]interface{}{
									"title":             title,
									"titleIgnorePrefix": titleIgnorePrefix.String,
									"genres":            genres,
								},
							},
						})
					} else {
						log.Warnf("[Go Warning] Failed to scan author series book: %v", err)
					}
				}
				bookRows.Close()
			}

			series = append(series, map[string]interface{}{
				"id":    sID,
				"name":  sName,
				"items": books,
			})
		}
	}
	return series
}
