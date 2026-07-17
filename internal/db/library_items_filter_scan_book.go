package db

import (
	"database/sql"
	"encoding/json"
)

func scanFilteredBookItem(rows *sql.Rows, libraryID string) (*LibraryItemMinifiedJSON, *BookMinifiedJSON, string, error) {
	var id string
	var ino, path, relPath, mediaType, mediaID, libraryFolderID sql.NullString
	var isFileVal, isMissingVal, isInvalidVal sql.NullInt64
	var mtimeStr, ctimeStr, birthtimeStr, createdAtStr, updatedAtStr sql.NullString
	var size sql.NullInt64

	var authorNamesFirstLast, authorNamesLastFirst sql.NullString
	var bID, bTitle string
	var bTitleIgnorePrefix sql.NullString
	var bSubtitle, bPublishedYear, bPublishedDate, bPublisher, bDescription, bIsbn, bAsin, bLanguage, bCoverPath sql.NullString
	var bExplicit, bAbridged sql.NullInt64
	var bDuration sql.NullFloat64
	var bNarrators, bAudioFiles, bEbookFile, bChapters, bTags, bGenres []byte
	var bLockedFields []byte

	err := rows.Scan(
		&id, &ino, &path, &relPath, &isFileVal, &mtimeStr, &ctimeStr, &birthtimeStr, &createdAtStr, &updatedAtStr, &isMissingVal, &isInvalidVal, &mediaType, &mediaID, &size, &libraryFolderID, &authorNamesFirstLast, &authorNamesLastFirst,
		&bID, &bTitle, &bTitleIgnorePrefix, &bSubtitle, &bPublishedYear, &bPublishedDate, &bPublisher, &bDescription, &bIsbn, &bAsin, &bLanguage, &bExplicit, &bAbridged, &bCoverPath, &bDuration, &bNarrators, &bAudioFiles, &bEbookFile, &bChapters, &bTags, &bGenres, &bLockedFields,
	)
	if err != nil {
		return nil, nil, "", err
	}

	var tags []string
	if len(bTags) > 0 {
		json.Unmarshal(bTags, &tags)
	}
	var genres []string
	if len(bGenres) > 0 {
		json.Unmarshal(bGenres, &genres)
	}
	var audioFiles []struct {
		Exclude  bool `json:"exclude"`
		Metadata struct {
			Size int64 `json:"size"`
		} `json:"metadata"`
	}
	if len(bAudioFiles) > 0 {
		json.Unmarshal(bAudioFiles, &audioFiles)
	}
	var ebook struct {
		EbookFormat string `json:"ebookFormat"`
		Metadata    struct {
			Size int64 `json:"size"`
		} `json:"metadata"`
	}
	if len(bEbookFile) > 0 {
		json.Unmarshal(bEbookFile, &ebook)
	}
	var chapters []interface{}
	if len(bChapters) > 0 {
		json.Unmarshal(bChapters, &chapters)
	}

	numTracks := 0
	for _, af := range audioFiles {
		if !af.Exclude {
			numTracks++
		}
	}

	var ebookFormat *string
	if ebook.EbookFormat != "" {
		val := ebook.EbookFormat
		ebookFormat = &val
	}

	var cover *string
	if bCoverPath.Valid && bCoverPath.String != "" {
		cover = &bCoverPath.String
	}

	var subtitleVal *string
	if bSubtitle.Valid {
		subtitleVal = &bSubtitle.String
	}
	var publishedYearVal *string
	if bPublishedYear.Valid {
		publishedYearVal = &bPublishedYear.String
	}
	var publishedDateVal *string
	if bPublishedDate.Valid {
		publishedDateVal = &bPublishedDate.String
	}
	var publisherVal *string
	if bPublisher.Valid {
		publisherVal = &bPublisher.String
	}
	var descriptionVal *string
	if bDescription.Valid {
		descriptionVal = &bDescription.String
	}
	var isbnVal *string
	if bIsbn.Valid {
		isbnVal = &bIsbn.String
	}
	var asinVal *string
	if bAsin.Valid {
		asinVal = &bAsin.String
	}
	var languageVal *string
	if bLanguage.Valid {
		languageVal = &bLanguage.String
	}

	var calculatedSize int64
	for _, af := range audioFiles {
		calculatedSize += af.Metadata.Size
	}
	calculatedSize += ebook.Metadata.Size
	if calculatedSize == 0 {
		calculatedSize = size.Int64
	}

	var lockedFields []string
	if len(bLockedFields) > 0 {
		json.Unmarshal(bLockedFields, &lockedFields)
	}

	bookMin := &BookMinifiedJSON{
		ID:            bID,
		CoverPath:     cover,
		Tags:          tags,
		NumTracks:     numTracks,
		NumAudioFiles: len(audioFiles),
		NumChapters:   len(chapters),
		Duration:      bDuration.Float64,
		Size:          calculatedSize,
		EbookFormat:   ebookFormat,
		Metadata: &BookMetadataMinified{
			Title:             bTitle,
			TitleIgnorePrefix: bTitleIgnorePrefix.String,
			Subtitle:          subtitleVal,
			AuthorName:        authorNamesFirstLast.String,
			AuthorNameLF:      authorNamesLastFirst.String,
			NarratorName:      jsonArrayToCommaString(bNarrators),
			SeriesName:        "", // Filled later
			Genres:            genres,
			PublishedYear:     publishedYearVal,
			PublishedDate:     publishedDateVal,
			Publisher:         publisherVal,
			Description:       descriptionVal,
			Isbn:              isbnVal,
			Asin:              asinVal,
			Language:          languageVal,
			Explicit:          bExplicit.Valid && bExplicit.Int64 != 0,
			Abridged:          bAbridged.Valid && bAbridged.Int64 != 0,
			LockedFields:      lockedFields,
		},
	}

	liMin := &LibraryItemMinifiedJSON{
		ID:          id,
		Ino:         ino.String,
		LibraryID:   libraryID,
		FolderID:    libraryFolderID.String,
		Path:        path.String,
		RelPath:     relPath.String,
		IsFile:      isFileVal.Valid && isFileVal.Int64 != 0,
		MtimeMs:     parseEpochMillis(mtimeStr.String),
		CtimeMs:     parseEpochMillis(ctimeStr.String),
		BirthtimeMs: parseEpochMillis(birthtimeStr.String),
		AddedAt:     parseEpochMillis(createdAtStr.String),
		UpdatedAt:   parseEpochMillis(updatedAtStr.String),
		IsMissing:   isMissingVal.Valid && isMissingVal.Int64 != 0,
		IsInvalid:   isInvalidVal.Valid && isInvalidVal.Int64 != 0,
		MediaType:   mediaType.String,
		Media:       bookMin,
		NumFiles:    len(audioFiles) + len(chapters),
		Size:        calculatedSize,
	}

	return liMin, bookMin, bID, nil
}
