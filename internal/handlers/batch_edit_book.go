package handlers

import (
	"database/sql"
	"encoding/json"
	"strings"

	idb "audiobookshelf/internal/db"
	log "audiobookshelf/internal/logger"
	iscanner "audiobookshelf/internal/scanner"
	"audiobookshelf/internal/utils"
)

func batchUpdateBook(tx *sql.Tx, itemID string, mediaID string, libraryID string, mediaPayload BatchUpdateMediaPayload, prefixes []string, nowStr string) (bool, error) {
	var currentTitle, currentSubtitle, currentPublishedYear, currentPublishedDate, currentPublisher, currentDescription, currentIsbn, currentAsin, currentLanguage, currentNarratorsRaw, currentTagsRaw, currentGenresRaw string
	var currentExplicitVal, currentAbridgedVal int
	err := tx.QueryRow(`
		SELECT COALESCE(title, ''), COALESCE(subtitle, ''), COALESCE(publishedYear, ''), COALESCE(publishedDate, ''), COALESCE(publisher, ''), COALESCE(description, ''), COALESCE(isbn, ''), COALESCE(asin, ''), COALESCE(language, ''), explicit, abridged, COALESCE(narrators, '[]'), COALESCE(tags, '[]'), COALESCE(genres, '[]')
		FROM books WHERE id = ?
	`, mediaID).Scan(&currentTitle, &currentSubtitle, &currentPublishedYear, &currentPublishedDate, &currentPublisher, &currentDescription, &currentIsbn, &currentAsin, &currentLanguage, &currentExplicitVal, &currentAbridgedVal, &currentNarratorsRaw, &currentTagsRaw, &currentGenresRaw)
	if err != nil {
		log.Errorf("[Go] Batch edit: book media %s not found: %v", mediaID, err)
		return false, nil // skip on not found to preserve original logic (continue)
	}

	var currentNarrators, currentTags, currentGenres []string
	_ = json.Unmarshal([]byte(currentNarratorsRaw), &currentNarrators)
	_ = json.Unmarshal([]byte(currentTagsRaw), &currentTags)
	_ = json.Unmarshal([]byte(currentGenresRaw), &currentGenres)

	title := currentTitle
	if mediaPayload.Title != nil {
		title = *mediaPayload.Title
	}
	subtitle := currentSubtitle
	if mediaPayload.Subtitle != nil {
		subtitle = *mediaPayload.Subtitle
	}
	publishedYear := currentPublishedYear
	if mediaPayload.PublishedYear != nil {
		publishedYear = *mediaPayload.PublishedYear
	}
	publishedDate := currentPublishedDate
	if mediaPayload.PublishedDate != nil {
		publishedDate = *mediaPayload.PublishedDate
	}
	publisher := currentPublisher
	if mediaPayload.Publisher != nil {
		publisher = *mediaPayload.Publisher
	}
	description := currentDescription
	if mediaPayload.Description != nil {
		description = *mediaPayload.Description
	}
	isbn := currentIsbn
	if mediaPayload.Isbn != nil {
		isbn = *mediaPayload.Isbn
	}
	asin := currentAsin
	if mediaPayload.Asin != nil {
		asin = *mediaPayload.Asin
	}
	language := currentLanguage
	if mediaPayload.Language != nil {
		language = *mediaPayload.Language
	}
	explicit := currentExplicitVal != 0
	if mediaPayload.Explicit != nil {
		explicit = *mediaPayload.Explicit
	}
	abridged := currentAbridgedVal != 0
	if mediaPayload.Abridged != nil {
		abridged = *mediaPayload.Abridged
	}

	narrators := currentNarrators
	if mediaPayload.Narrators != nil {
		narrators = *mediaPayload.Narrators
	}
	tags := currentTags
	if mediaPayload.Tags != nil {
		tags = *mediaPayload.Tags
	}
	genres := currentGenres
	if mediaPayload.Genres != nil {
		genres = *mediaPayload.Genres
	}

	titleIgnorePrefix := getTitleIgnorePrefixGo(title, prefixes)
	narratorsJSON, _ := json.Marshal(narrators)
	tagsJSON, _ := json.Marshal(tags)
	genresJSON, _ := json.Marshal(genres)

	_, err = tx.Exec(`
		UPDATE books
		SET title = ?, titleIgnorePrefix = ?, subtitle = ?, publishedYear = ?, publishedDate = ?, publisher = ?, description = ?, isbn = ?, asin = ?, language = ?, explicit = ?, abridged = ?, narrators = ?, tags = ?, genres = ?
		WHERE id = ?
	`, title, titleIgnorePrefix, subtitle, publishedYear, publishedDate, publisher, description, isbn, asin, language, boolToInt(explicit), boolToInt(abridged), narratorsJSON, tagsJSON, genresJSON, mediaID)
	if err != nil {
		return false, err
	}

	// Authors
	var authorNames []string
	if mediaPayload.Authors != nil {
		authorNames = *mediaPayload.Authors
		if idb.TableExistsTx(tx, "bookAuthors") {
			_, _ = tx.Exec("DELETE FROM bookAuthors WHERE bookId = ?", mediaID)
		}
		for _, author := range authorNames {
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
	} else {
		rows, err := tx.Query(`
			SELECT COALESCE(a.name, '') FROM authors a
			JOIN bookAuthors ba ON a.id = ba.authorId
			WHERE ba.bookId = ?
		`, mediaID)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var name string
				if errScan := rows.Scan(&name); errScan == nil {
					authorNames = append(authorNames, name)
				}
			}
		}
	}

	authorNamesFirstLast := strings.Join(authorNames, ", ")
	var lfs []string
	for _, a := range authorNames {
		lfs = append(lfs, utils.NameToLastFirst(a))
	}
	authorNamesLastFirst := strings.Join(lfs, ", ")

	// Series
	if mediaPayload.SeriesName != nil {
		seriesName := *mediaPayload.SeriesName
		seriesSeq := ""
		if mediaPayload.SeriesSequence != nil {
			seriesSeq = *mediaPayload.SeriesSequence
		} else {
			_ = tx.QueryRow("SELECT COALESCE(sequence, '') FROM bookSeries WHERE bookId = ?", mediaID).Scan(&seriesSeq)
		}

		if idb.TableExistsTx(tx, "bookSeries") {
			_, _ = tx.Exec("DELETE FROM bookSeries WHERE bookId = ?", mediaID)
		}
		if seriesName != "" {
			seriesID := utils.UUIDStr()
			_ = iscanner.InsertSeries(tx, seriesID, seriesName, libraryID)

			var existingSeriesID string
			_ = tx.QueryRow("SELECT id FROM series WHERE name = ? AND libraryId = ?", seriesName, libraryID).Scan(&existingSeriesID)
			if existingSeriesID != "" {
				seriesID = existingSeriesID
			}
			_ = iscanner.InsertBookSeries(tx, mediaID, seriesID, seriesSeq)
		}
	}

	_, err = tx.Exec(`
		UPDATE libraryItems
		SET title = ?, titleIgnorePrefix = ?, authorNamesFirstLast = ?, authorNamesLastFirst = ?, updatedAt = ?
		WHERE id = ?
	`, title, titleIgnorePrefix, authorNamesFirstLast, authorNamesLastFirst, nowStr, itemID)
	if err != nil {
		return false, err
	}

	return true, nil
}
