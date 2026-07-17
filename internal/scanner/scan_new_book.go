package scanner

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	log "audiobookshelf/internal/logger"
)

func insertNewBook(tx *sql.Tx, mediaID, title, titleIgnorePrefix, libraryID, itemPath string, meta *GroupMetadata, nowStr string) error {
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
	_, err := tx.Exec(query, args...)
	if err != nil {
		return err
	}

	associateBookAuthors(tx, mediaID, meta.Authors, libraryID, itemPath)
	associateBookSeries(tx, mediaID, meta.SeriesName, meta.SeriesSequence, libraryID, itemPath)

	return nil
}

func associateBookAuthors(tx *sql.Tx, mediaID string, authors []string, libraryID, itemPath string) {
	log.Printf("[Scanner] [%s] scanNewLibraryItem: Inserting authors", itemPath)
	for _, author := range authors {
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
}

func associateBookSeries(tx *sql.Tx, mediaID, seriesName, sequence, libraryID, itemPath string) {
	if seriesName != "" {
		log.Printf("[Scanner] [%s] scanNewLibraryItem: Inserting series", itemPath)
		seriesID := uuidStr()
		_ = insertSeries(tx, seriesID, seriesName, libraryID)

		var existingSeriesID string
		_ = tx.QueryRow("SELECT id FROM series WHERE name = ? AND libraryId = ?", seriesName, libraryID).Scan(&existingSeriesID)
		if existingSeriesID != "" {
			seriesID = existingSeriesID
		}
		_ = insertBookSeries(tx, mediaID, seriesID, sequence)
	}
}
