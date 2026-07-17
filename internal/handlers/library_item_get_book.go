package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"

	"audiobookshelf/internal/core"
	"audiobookshelf/internal/utils"
)

func getBookMediaDetails(w http.ResponseWriter, r *http.Request, db *sql.DB, mediaID, libraryID, itemID, ino string, size int64, user *core.UserSession, payload map[string]interface{}) bool {
	var bTitle string
	var bTitleIgnorePrefix, bSubtitle, bPublishedYear, bPublishedDate, bPublisher, bDescription, bIsbn, bAsin, bLanguage, bCoverPath sql.NullString
	var bDuration float64
	var bNarrators, bAudioFiles, bEbookFile, bChapters, bTags, bGenres, bLockedFields []byte
	var bExplicit, bAbridged sql.NullInt64

	err := db.QueryRow(`
		SELECT title, titleIgnorePrefix, subtitle, publishedYear, publishedDate, publisher, description, isbn, asin, language, explicit, abridged, coverPath, duration, narrators, audioFiles, ebookFile, chapters, tags, genres, lockedFields
		FROM books WHERE id = ?
	`, mediaID).Scan(
		&bTitle, &bTitleIgnorePrefix, &bSubtitle, &bPublishedYear, &bPublishedDate, &bPublisher, &bDescription, &bIsbn, &bAsin, &bLanguage, &bExplicit, &bAbridged, &bCoverPath, &bDuration, &bNarrators, &bAudioFiles, &bEbookFile, &bChapters, &bTags, &bGenres, &bLockedFields,
	)
	if err != nil {
		log.Warnf("[Go Warning] Failed to scan book with id %s: %v", mediaID, err)
		return true
	}

	var tags []string
	_ = json.Unmarshal(bTags, &tags)
	if !user.IsAdminOrUp() {
		var explicit = bExplicit.Valid && bExplicit.Int64 != 0
		if explicit && !user.CanAccessExplicitContent {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return false
		}
		if !user.CheckCanAccessLibraryItemWithTags(tags) {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return false
		}
	}
	var genres []string
	_ = json.Unmarshal(bGenres, &genres)
	var audioFiles []map[string]interface{}
	_ = json.Unmarshal(bAudioFiles, &audioFiles)
	var ebook interface{}
	_ = json.Unmarshal(bEbookFile, &ebook)
	var chapters []interface{}
	_ = json.Unmarshal(bChapters, &chapters)
	var lockedFields []string
	if len(bLockedFields) > 0 {
		_ = json.Unmarshal(bLockedFields, &lockedFields)
	}
	if lockedFields == nil {
		lockedFields = []string{}
	}

	var narratorNames []string
	_ = json.Unmarshal(bNarrators, &narratorNames)

	authorsList, authorNames, _ := getAuthorsList(db, mediaID)
	seriesList, seriesNames, _ := getSeriesList(db, mediaID)

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

	tracks, _ := processAudiobookTracks(bAudioFiles)

	payload["media"] = map[string]interface{}{
		"id":            mediaID,
		"coverPath":     utils.NullIfEmpty(bCoverPath.String),
		"tags":          tags,
		"numTracks":     len(tracks),
		"numAudioFiles": len(audioFiles),
		"numChapters":   len(chapters),
		"duration":      bDuration,
		"size":          size,
		"ebookFormat":   ebookFormat,
		"audioFiles":    audioFiles,
		"tracks":        tracks,
		"ebookFile":     ebook,
		"chapters":      chapters,
		"metadata": map[string]interface{}{
			"title":             bTitle,
			"titleIgnorePrefix": bTitleIgnorePrefix.String,
			"subtitle":          utils.NullIfEmpty(bSubtitle.String),
			"authors":           authorsList,
			"authorName":        authorName,
			"authorNameLF":      utils.NameToLastFirst(authorName),
			"narrators":         narratorNames,
			"narratorName":      narratorName,
			"series":            seriesList,
			"seriesName":        seriesName,
			"genres":            genres,
			"publishedYear":     utils.NullIfEmpty(bPublishedYear.String),
			"publishedDate":     utils.NullIfEmpty(bPublishedDate.String),
			"publisher":         utils.NullIfEmpty(bPublisher.String),
			"description":       utils.NullIfEmpty(bDescription.String),
			"isbn":              utils.NullIfEmpty(bIsbn.String),
			"asin":              utils.NullIfEmpty(bAsin.String),
			"language":          utils.NullIfEmpty(bLanguage.String),
			"explicit":          bExplicit.Valid && bExplicit.Int64 != 0,
			"abridged":          bAbridged.Valid && bAbridged.Int64 != 0,
			"lockedFields":      lockedFields,
		},
	}

	payload["libraryFiles"] = buildLibraryFilesForBook(bEbookFile, audioFiles, ino)
	payload["otherVersions"] = getOtherBookVersions(db, libraryID, itemID, bTitle)

	return true
}
