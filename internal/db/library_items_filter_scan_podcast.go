package db

import (
	"database/sql"
	"encoding/json"
	"time"
)

func scanFilteredPodcastItem(rows *sql.Rows, libraryID string) (*LibraryItemMinifiedJSON, error) {
	var id string
	var ino, path, relPath, mediaType, mediaID, libraryFolderID sql.NullString
	var isFileVal, isMissingVal, isInvalidVal sql.NullInt64
	var mtimeStr, ctimeStr, birthtimeStr, createdAtStr, updatedAtStr sql.NullString
	var size sql.NullInt64

	var pID, pTitle string
	var pTitleIgnorePrefix sql.NullString
	var pAuthor, pReleaseDate, pFeedURL, pImageURL, pDescription, pItunesPageURL, pItunesID, pItunesArtistID, pLanguage, pPodcastType, pAutoDownloadSchedule, pCoverPath sql.NullString
	var pExplicit, pAutoDownloadEpisodes, pMaxEpisodesToKeep, pMaxNewEpisodesToDownload, pNumEpisodes, pSkipIntroDuration, pSkipOutroDuration sql.NullInt64
	var pLastEpisodeCheck sql.NullString
	var pTags, pGenres []byte
	var pAutoDeletePlayed sql.NullInt64
	var pLockedFields []byte

	err := rows.Scan(
		&id, &ino, &path, &relPath, &isFileVal, &mtimeStr, &ctimeStr, &birthtimeStr, &createdAtStr, &updatedAtStr, &isMissingVal, &isInvalidVal, &mediaType, &mediaID, &size, &libraryFolderID,
		&pID, &pTitle, &pTitleIgnorePrefix, &pAuthor, &pReleaseDate, &pFeedURL, &pImageURL, &pDescription, &pItunesPageURL, &pItunesID, &pItunesArtistID, &pLanguage, &pPodcastType, &pExplicit, &pAutoDownloadEpisodes, &pAutoDownloadSchedule, &pLastEpisodeCheck, &pMaxEpisodesToKeep, &pMaxNewEpisodesToDownload, &pCoverPath, &pTags, &pGenres, &pNumEpisodes, &pSkipIntroDuration, &pSkipOutroDuration, &pAutoDeletePlayed, &pLockedFields,
	)
	if err != nil {
		return nil, err
	}

	var tags []string
	if len(pTags) > 0 {
		json.Unmarshal(pTags, &tags)
	}
	var genres []string
	if len(pGenres) > 0 {
		json.Unmarshal(pGenres, &genres)
	}

	var cover *string
	if pCoverPath.Valid && pCoverPath.String != "" {
		cover = &pCoverPath.String
	}

	var authorVal *string
	if pAuthor.Valid {
		authorVal = &pAuthor.String
	}
	var descriptionVal *string
	if pDescription.Valid {
		descriptionVal = &pDescription.String
	}
	var releaseDateVal *string
	if pReleaseDate.Valid {
		releaseDateVal = &pReleaseDate.String
	}
	var feedURLVal *string
	if pFeedURL.Valid {
		feedURLVal = &pFeedURL.String
	}
	var imageURLVal *string
	if pImageURL.Valid {
		imageURLVal = &pImageURL.String
	}
	var itunesPageURLVal *string
	if pItunesPageURL.Valid {
		itunesPageURLVal = &pItunesPageURL.String
	}
	var itunesIDVal *string
	if pItunesID.Valid {
		itunesIDVal = &pItunesID.String
	}
	var itunesArtistIDVal *string
	if pItunesArtistID.Valid {
		itunesArtistIDVal = &pItunesArtistID.String
	}
	var languageVal *string
	if pLanguage.Valid {
		languageVal = &pLanguage.String
	}
	var podcastTypeVal *string
	if pPodcastType.Valid {
		podcastTypeVal = &pPodcastType.String
	}
	var autoDownloadScheduleVal *string
	if pAutoDownloadSchedule.Valid {
		autoDownloadScheduleVal = &pAutoDownloadSchedule.String
	}

	var lastEpisodeCheckVal *int64
	if pLastEpisodeCheck.Valid && pLastEpisodeCheck.String != "" {
		t, err := ParseSQLiteTime(pLastEpisodeCheck.String)
		if err == nil {
			val := t.UnixNano() / int64(time.Millisecond)
			lastEpisodeCheckVal = &val
		}
	}

	var lockedFields []string
	if len(pLockedFields) > 0 {
		json.Unmarshal(pLockedFields, &lockedFields)
	}

	podcastMin := &PodcastMinifiedJSON{
		ID:                       pID,
		CoverPath:                cover,
		Tags:                     tags,
		NumEpisodes:              int(pNumEpisodes.Int64),
		AutoDownloadEpisodes:     pAutoDownloadEpisodes.Valid && pAutoDownloadEpisodes.Int64 != 0,
		AutoDownloadSchedule:     autoDownloadScheduleVal,
		LastEpisodeCheck:         lastEpisodeCheckVal,
		MaxEpisodesToKeep:        int(pMaxEpisodesToKeep.Int64),
		MaxNewEpisodesToDownload: int(pMaxNewEpisodesToDownload.Int64),
		AutoDeletePlayed:         pAutoDeletePlayed.Valid && pAutoDeletePlayed.Int64 != 0,
		SkipIntroDuration:        int(pSkipIntroDuration.Int64),
		SkipOutroDuration:        int(pSkipOutroDuration.Int64),
		Size:                     size.Int64,
		Metadata: &PodcastMetadataMin{
			Title:             pTitle,
			TitleIgnorePrefix: pTitleIgnorePrefix.String,
			Author:            authorVal,
			Description:       descriptionVal,
			ReleaseDate:       releaseDateVal,
			Genres:            genres,
			FeedURL:           feedURLVal,
			ImageURL:          imageURLVal,
			ItunesPageURL:     itunesPageURLVal,
			ItunesID:          itunesIDVal,
			ItunesArtistID:    itunesArtistIDVal,
			Explicit:          pExplicit.Valid && pExplicit.Int64 != 0,
			Language:          languageVal,
			Type:              podcastTypeVal,
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
		Media:       podcastMin,
		NumFiles:    int(pNumEpisodes.Int64),
		Size:        size.Int64,
	}

	return liMin, nil
}
