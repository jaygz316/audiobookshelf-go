package scanner

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	log "audiobookshelf/internal/logger"
)

func getBookMinified(db *sql.DB, mediaID string, size int64) (*BookMinifiedJSON, error) {
	var bTitle, bTitleIgnorePrefix, bSubtitle, bPublishedYear, bPublishedDate, bPublisher, bDescription, bIsbn, bAsin, bLanguage, bCoverPath string
	var bDuration float64
	var bNarrators, bAudioFiles, bEbookFile, bChapters, bTags, bGenres []byte
	var bExplicit, bAbridged int

	err := db.QueryRow(`
		SELECT title, titleIgnorePrefix, subtitle, publishedYear, publishedDate, publisher, description, isbn, asin, language, explicit, abridged, coverPath, duration, narrators, audioFiles, ebookFile, chapters, tags, genres
		FROM books WHERE id = ?
	`, mediaID).Scan(
		&bTitle, &bTitleIgnorePrefix, &bSubtitle, &bPublishedYear, &bPublishedDate, &bPublisher, &bDescription, &bIsbn, &bAsin, &bLanguage, &bExplicit, &bAbridged, &bCoverPath, &bDuration, &bNarrators, &bAudioFiles, &bEbookFile, &bChapters, &bTags, &bGenres,
	)
	if err != nil {
		return nil, err
	}

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
	return bookMin, nil
}
