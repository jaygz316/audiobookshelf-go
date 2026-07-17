package handlers

import (
	"database/sql"
	"encoding/json"
	"strings"

	idb "audiobookshelf/internal/db"
	iscanner "audiobookshelf/internal/scanner"
	"audiobookshelf/internal/utils"
)

func updateBookDetails(tx *sql.Tx, itemID, mediaID, libraryID, nowStr string, prefixes []string, payload *updateItemPayload) error {
	authorNamesFirstLast := strings.Join(payload.Authors, ", ")
	var lfs []string
	for _, a := range payload.Authors {
		lfs = append(lfs, utils.NameToLastFirst(a))
	}
	authorNamesLastFirst := strings.Join(lfs, ", ")

	narratorsJSON, _ := json.Marshal(payload.Narrators)
	tagsJSON, _ := json.Marshal(payload.Tags)
	genresJSON, _ := json.Marshal(payload.Genres)
	lockedFieldsJSON, _ := json.Marshal(payload.LockedFields)

	titleIgnorePrefix := getTitleIgnorePrefixGo(payload.Title, prefixes)

	_, err := tx.Exec(`
		UPDATE books
		SET title = ?, titleIgnorePrefix = ?, subtitle = ?, publishedYear = ?, publishedDate = ?, publisher = ?, description = ?, isbn = ?, asin = ?, language = ?, explicit = ?, abridged = ?, narrators = ?, tags = ?, genres = ?, lockedFields = ?
		WHERE id = ?
	`, payload.Title, titleIgnorePrefix, payload.Subtitle, payload.PublishedYear, payload.PublishedDate, payload.Publisher, payload.Description, payload.Isbn, payload.Asin, payload.Language, boolToInt(payload.Explicit), boolToInt(payload.Abridged), narratorsJSON, tagsJSON, genresJSON, lockedFieldsJSON, mediaID)
	if err != nil {
		return err
	}

	if idb.TableExistsTx(tx, "bookAuthors") {
		_, _ = tx.Exec("DELETE FROM bookAuthors WHERE bookId = ?", mediaID)
	}
	for _, author := range payload.Authors {
		trimmed := strings.TrimSpace(author)
		if trimmed == "" {
			continue
		}
		authorID := utils.UUIDStr()
		lastFirst := utils.NameToLastFirst(trimmed)
		_ = iscanner.InsertAuthor(tx, authorID, trimmed, lastFirst, libraryID)

		var existingAuthorID string
		_ = tx.QueryRow("SELECT id FROM authors WHERE name = ? AND libraryId = ?", trimmed, libraryID).Scan(&existingAuthorID)
		if existingAuthorID != "" {
			authorID = existingAuthorID
		}
		_ = iscanner.InsertBookAuthor(tx, mediaID, authorID)
	}

	if idb.TableExistsTx(tx, "bookSeries") {
		_, _ = tx.Exec("DELETE FROM bookSeries WHERE bookId = ?", mediaID)
	}
	if payload.SeriesName != "" {
		seriesID := utils.UUIDStr()
		_ = iscanner.InsertSeries(tx, seriesID, payload.SeriesName, libraryID)

		var existingSeriesID string
		_ = tx.QueryRow("SELECT id FROM series WHERE name = ? AND libraryId = ?", payload.SeriesName, libraryID).Scan(&existingSeriesID)
		if existingSeriesID != "" {
			seriesID = existingSeriesID
		}
		_ = iscanner.InsertBookSeries(tx, mediaID, seriesID, payload.SeriesSequence)
	}

	_, err = tx.Exec(`
		UPDATE libraryItems
		SET title = ?, titleIgnorePrefix = ?, authorNamesFirstLast = ?, authorNamesLastFirst = ?, updatedAt = ?
		WHERE id = ?
	`, payload.Title, titleIgnorePrefix, authorNamesFirstLast, authorNamesLastFirst, nowStr, itemID)
	return err
}
