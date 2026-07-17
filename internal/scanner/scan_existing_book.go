package scanner

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	log "audiobookshelf/internal/logger"
)

func getSortingPrefixesTx(tx *sql.Tx) []string {
	var prefixes []string
	var valStr string
	_ = tx.QueryRow("SELECT value FROM settings WHERE key = 'server-settings'").Scan(&valStr)
	if valStr != "" {
		var s struct {
			SortingPrefixes []string `json:"sortingPrefixes"`
		}
		if json.Unmarshal([]byte(valStr), &s) == nil {
			prefixes = s.SortingPrefixes
		}
	}
	if len(prefixes) == 0 {
		prefixes = []string{"the", "a", "an"}
	}
	return prefixes
}

func setIfLocked(locked bool, target *string, source sql.NullString) {
	if locked && source.Valid {
		*target = source.String
	}
}

func unmarshalIfLocked(locked bool, target *[]string, source []byte) {
	if locked && len(source) > 0 {
		_ = json.Unmarshal(source, target)
	}
}

func applyLockedBookFields(tx *sql.Tx, mediaID string, meta *GroupMetadata, prefixes []string) (string, string, error) {
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

	var title, titleIgnorePrefix string
	if isLocked("title") && dbTitle.String != "" {
		title = dbTitle.String
		titleIgnorePrefix = getTitleIgnorePrefixGo(title, prefixes)
	}
	setIfLocked(isLocked("subtitle"), &meta.Subtitle, dbSubtitle)
	setIfLocked(isLocked("publishedYear"), &meta.PublishedYear, dbPublishedYear)
	setIfLocked(isLocked("publishedDate"), &meta.PublishedDate, dbPublishedDate)
	setIfLocked(isLocked("publisher"), &meta.Publisher, dbPublisher)
	setIfLocked(isLocked("description"), &meta.Description, dbDescription)
	setIfLocked(isLocked("isbn"), &meta.ISBN, dbIsbn)
	setIfLocked(isLocked("asin"), &meta.ASIN, dbAsin)
	setIfLocked(isLocked("language"), &meta.Language, dbLanguage)
	setIfLocked(isLocked("cover") || isLocked("coverPath"), &meta.CoverPath, dbCoverPath)
	unmarshalIfLocked(isLocked("narrators") || isLocked("narrator"), &meta.Narrators, dbNarrators)
	unmarshalIfLocked(isLocked("tags"), &meta.Tags, dbTags)
	unmarshalIfLocked(isLocked("genres"), &meta.Genres, dbGenres)

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
	return title, titleIgnorePrefix, nil
}

func updateExistingBook(tx *sql.Tx, mediaID, title, titleIgnorePrefix, libraryID, itemPath string, meta *GroupMetadata, nowStr string) (string, string, error) {
	prefixes := getSortingPrefixesTx(tx)
	lockedTitle, lockedTitleIgnorePrefix, err := applyLockedBookFields(tx, mediaID, meta, prefixes)
	if err == nil && lockedTitle != "" {
		title = lockedTitle
		titleIgnorePrefix = lockedTitleIgnorePrefix
	}

	authorNamesFirstLast := strings.Join(meta.Authors, ", ")
	var lfs []string
	for _, a := range meta.Authors {
		lfs = append(lfs, NameToLastFirst(a))
	}
	authorNamesLastFirst := strings.Join(lfs, ", ")

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
		return "", "", err
	}

	log.Printf("[Scanner] [%s] scanExistingLibraryItem: Updating authors", itemPath)
	if tableExistsTx(tx, "bookAuthors") {
		_, _ = tx.Exec("DELETE FROM bookAuthors WHERE bookId = ?", mediaID)
	}
	associateBookAuthors(tx, mediaID, meta.Authors, libraryID, itemPath)

	log.Printf("[Scanner] [%s] scanExistingLibraryItem: Updating series", itemPath)
	if tableExistsTx(tx, "bookSeries") {
		_, _ = tx.Exec("DELETE FROM bookSeries WHERE bookId = ?", mediaID)
	}
	associateBookSeries(tx, mediaID, meta.SeriesName, meta.SeriesSequence, libraryID, itemPath)

	return authorNamesFirstLast, authorNamesLastFirst, nil
}
