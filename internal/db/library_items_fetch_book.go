package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"audiobookshelf/internal/utils"
)

func fetchBookMinified(db *sql.DB, mediaID string, size int64) (*BookMinifiedJSON, error) {
	var bTitle, bTitleIgnorePrefix, bSubtitle, bPublishedYear, bPublishedDate, bPublisher, bDescription, bIsbn, bAsin, bLanguage, bCoverPath string
	var bDuration float64
	var bNarrators, bAudioFiles, bEbookFile, bChapters, bTags, bGenres, bLockedFields []byte
	var bExplicit, bAbridged int

	err := db.QueryRow(`
		SELECT title, titleIgnorePrefix, subtitle, publishedYear, publishedDate, publisher, description, isbn, asin, language, explicit, abridged, coverPath, duration, narrators, audioFiles, ebookFile, chapters, tags, genres, lockedFields
		FROM books WHERE id = ?
	`, mediaID).Scan(
		&bTitle, &bTitleIgnorePrefix, &bSubtitle, &bPublishedYear, &bPublishedDate, &bPublisher, &bDescription, &bIsbn, &bAsin, &bLanguage, &bExplicit, &bAbridged, &bCoverPath, &bDuration, &bNarrators, &bAudioFiles, &bEbookFile, &bChapters, &bTags, &bGenres, &bLockedFields,
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
	var chapters []interface{}
	_ = json.Unmarshal(bChapters, &chapters)
	var narratorNames []string
	_ = json.Unmarshal(bNarrators, &narratorNames)
	var lockedFields []string
	if len(bLockedFields) > 0 {
		_ = json.Unmarshal(bLockedFields, &lockedFields)
	}
	if lockedFields == nil {
		lockedFields = []string{}
	}

	var authorNames []string
	rows, err2 := db.Query("SELECT name FROM authors WHERE id IN (SELECT authorId FROM bookAuthors WHERE bookId = ?)", mediaID)
	if err2 == nil {
		defer rows.Close()
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err == nil {
				authorNames = append(authorNames, name)
			}
		}
	}

	var seriesList []*BookSeriesMinifiedJSON
	var seriesNames []string
	srows, err3 := db.Query("SELECT s.id, s.name, bs.sequence FROM series s JOIN bookSeries bs ON s.id = bs.seriesId WHERE bs.bookId = ?", mediaID)
	if err3 == nil {
		defer srows.Close()
		for srows.Next() {
			var sid, name string
			var sequence sql.NullString
			if err := srows.Scan(&sid, &name, &sequence); err == nil {
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
		}
	}

	var firstSeq *string
	if len(seriesList) > 0 && seriesList[0].Sequence != "" {
		firstSeq = &seriesList[0].Sequence
	}

	var ebookFormat *string
	if len(bEbookFile) > 0 {
		var eb struct {
			EbookFormat string `json:"ebookFormat"`
		}
		if jsonUnmarshalSafe(bEbookFile, &eb) && eb.EbookFormat != "" {
			ebookFormat = &eb.EbookFormat
		}
	}

	authorName := strings.Join(authorNames, ", ")
	seriesName := strings.Join(seriesNames, ", ")
	narratorName := strings.Join(narratorNames, ", ")

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
		AudioFiles:    audioFiles,
		Metadata: &BookMetadataMinified{
			Title:             bTitle,
			TitleIgnorePrefix: bTitleIgnorePrefix,
			Subtitle:          nullIfEmpty(bSubtitle),
			AuthorName:        authorName,
			AuthorNameLF:      utils.NameToLastFirst(authorName),
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
			LockedFields:      lockedFields,
		},
	}
	return bookMin, nil
}
