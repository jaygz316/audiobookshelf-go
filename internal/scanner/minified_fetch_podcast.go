package scanner

import (
	"database/sql"
	"encoding/json"
)

func getPodcastMinified(db *sql.DB, mediaID string, size int64) (*PodcastMinifiedJSON, error) {
	var pTitle, pTitleIgnorePrefix, pAuthor, pReleaseDate, pFeedURL, pImageURL, pDescription, pItunesPageURL, pItunesID, pItunesArtistID, pLanguage, pPodcastType, pCoverPath string
	var pExplicit, pAutoDownloadEpisodes, pMaxEpisodesToKeep, pMaxNewEpisodesToDownload, pNumEpisodes int
	var pTags, pGenres []byte

	err := db.QueryRow(`
		SELECT title, titleIgnorePrefix, author, releaseDate, feedURL, imageURL, description, itunesPageURL, itunesId, itunesArtistId, language, podcastType, explicit, autoDownloadEpisodes, maxEpisodesToKeep, maxNewEpisodesToDownload, coverPath, tags, genres, numEpisodes
		FROM podcasts WHERE id = ?
	`, mediaID).Scan(
		&pTitle, &pTitleIgnorePrefix, &pAuthor, &pReleaseDate, &pFeedURL, &pImageURL, &pDescription, &pItunesPageURL, &pItunesID, &pItunesArtistID, &pLanguage, &pPodcastType, &pExplicit, &pAutoDownloadEpisodes, &pMaxEpisodesToKeep, &pMaxNewEpisodesToDownload, &pCoverPath, &pTags, &pGenres, &pNumEpisodes,
	)
	if err != nil {
		return nil, err
	}

	var tags []string
	_ = json.Unmarshal(pTags, &tags)
	var genres []string
	_ = json.Unmarshal(pGenres, &genres)

	podcastMin := &PodcastMinifiedJSON{
		ID:                       mediaID,
		CoverPath:                nullIfEmpty(pCoverPath),
		Tags:                     tags,
		NumEpisodes:              pNumEpisodes,
		AutoDownloadEpisodes:     pAutoDownloadEpisodes != 0,
		MaxEpisodesToKeep:        pMaxEpisodesToKeep,
		MaxNewEpisodesToDownload: pMaxNewEpisodesToDownload,
		Size:                     size,
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
		},
	}
	return podcastMin, nil
}
