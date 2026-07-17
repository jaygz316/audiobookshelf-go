package db

import (
	"database/sql"
	"encoding/json"
)

func fetchPodcastMinified(db *sql.DB, mediaID string, size int64) (*PodcastMinifiedJSON, error) {
	var pTitle, pTitleIgnorePrefix, pAuthor, pReleaseDate, pFeedURL, pImageURL, pDescription, pItunesPageURL, pItunesID, pItunesArtistID, pLanguage, pPodcastType, pCoverPath string
	var pExplicit, pAutoDownloadEpisodes, pMaxEpisodesToKeep, pMaxNewEpisodesToDownload, pNumEpisodes, pSkipIntroDuration, pSkipOutroDuration, pAutoDeletePlayed int
	var pTags, pGenres, pLockedFields []byte
	var pAutoDownloadSchedule sql.NullString
	var pLastEpisodeCheck sql.NullInt64

	err := db.QueryRow(`
		SELECT title, titleIgnorePrefix, author, releaseDate, feedURL, imageURL, description, itunesPageURL, itunesId, itunesArtistId, language, podcastType, explicit, autoDownloadEpisodes, maxEpisodesToKeep, maxNewEpisodesToDownload, coverPath, tags, genres, numEpisodes, lockedFields, autoDownloadSchedule, lastEpisodeCheck, autoDeletePlayed, skipIntroDuration, skipOutroDuration
		FROM podcasts WHERE id = ?
	`, mediaID).Scan(
		&pTitle, &pTitleIgnorePrefix, &pAuthor, &pReleaseDate, &pFeedURL, &pImageURL, &pDescription, &pItunesPageURL, &pItunesID, &pItunesArtistID, &pLanguage, &pPodcastType, &pExplicit, &pAutoDownloadEpisodes, &pMaxEpisodesToKeep, &pMaxNewEpisodesToDownload, &pCoverPath, &pTags, &pGenres, &pNumEpisodes, &pLockedFields,
		&pAutoDownloadSchedule, &pLastEpisodeCheck, &pAutoDeletePlayed, &pSkipIntroDuration, &pSkipOutroDuration,
	)
	if err != nil {
		return nil, err
	}

	var tags []string
	_ = json.Unmarshal(pTags, &tags)
	var genres []string
	_ = json.Unmarshal(pGenres, &genres)
	var lockedFields []string
	if len(pLockedFields) > 0 {
		_ = json.Unmarshal(pLockedFields, &lockedFields)
	}
	if lockedFields == nil {
		lockedFields = []string{}
	}

	var episodes []interface{}
	erows, err4 := db.Query("SELECT id, title, audioFile, pubDate, description, season, episode, episodeType, enclosureURL, publishedAt FROM podcastEpisodes WHERE podcastId = ?", mediaID)
	if err4 == nil {
		defer erows.Close()
		for erows.Next() {
			var epId, epTitle string
			var epAudioFile []byte
			var epPubDate, epDesc, epSeason, epEpisode, epEpType, epEncURL, epPubAt sql.NullString
			if err := erows.Scan(&epId, &epTitle, &epAudioFile, &epPubDate, &epDesc, &epSeason, &epEpisode, &epEpType, &epEncURL, &epPubAt); err == nil {
				var audioFile interface{}
				if len(epAudioFile) > 0 {
					_ = json.Unmarshal(epAudioFile, &audioFile)
				}
				epMap := map[string]interface{}{
					"id":        epId,
					"title":     epTitle,
					"audioFile": audioFile,
				}
				if epPubDate.Valid {
					epMap["pubDate"] = epPubDate.String
				}
				if epDesc.Valid {
					epMap["description"] = epDesc.String
				}
				if epSeason.Valid {
					epMap["season"] = epSeason.String
				}
				if epEpisode.Valid {
					epMap["episode"] = epEpisode.String
				}
				if epEpType.Valid {
					epMap["episodeType"] = epEpType.String
				}
				if epEncURL.Valid {
					epMap["enclosureUrl"] = epEncURL.String
				}
				if epPubAt.Valid {
					epMap["publishedAt"] = epPubAt.String
				}
				episodes = append(episodes, epMap)
			}
		}
	}

	var lastCheck *int64
	if pLastEpisodeCheck.Valid {
		lastCheck = &pLastEpisodeCheck.Int64
	}
	var schedule *string
	if pAutoDownloadSchedule.Valid {
		schedule = &pAutoDownloadSchedule.String
	}

	podcastMin := &PodcastMinifiedJSON{
		ID:                       mediaID,
		CoverPath:                nullIfEmpty(pCoverPath),
		Tags:                     tags,
		NumEpisodes:              pNumEpisodes,
		AutoDownloadEpisodes:     pAutoDownloadEpisodes != 0,
		AutoDownloadSchedule:     schedule,
		LastEpisodeCheck:         lastCheck,
		MaxEpisodesToKeep:        pMaxEpisodesToKeep,
		MaxNewEpisodesToDownload: pMaxNewEpisodesToDownload,
		AutoDeletePlayed:         pAutoDeletePlayed != 0,
		SkipIntroDuration:        pSkipIntroDuration,
		SkipOutroDuration:        pSkipOutroDuration,
		Size:                     size,
		Episodes:                 episodes,
		Metadata: &PodcastMetadataMin{
			Title:             pTitle,
			TitleIgnorePrefix: pTitleIgnorePrefix,
			Author:            nullIfEmpty(pAuthor),
			Description:       nullIfEmpty(pDescription),
			ReleaseDate:       nullIfEmpty(pReleaseDate),
			Genres:            genres,
			FeedURL:           nullIfEmpty(pFeedURL),
			ImageURL:          nullIfEmpty(pImageURL),
			ItunesPageURL:     nullIfEmpty(pItunesPageURL),
			ItunesID:          nullIfEmpty(pItunesID),
			ItunesArtistID:    nullIfEmpty(pItunesArtistID),
			Explicit:          pExplicit != 0,
			Language:          nullIfEmpty(pLanguage),
			Type:              nullIfEmpty(pPodcastType),
			LockedFields:      lockedFields,
		},
	}
	return podcastMin, nil
}
