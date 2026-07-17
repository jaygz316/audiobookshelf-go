package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
)

func getAudioFilesInfo(db *sql.DB, id string) ([]AudioFileInfo, error) {
	// 1. Try to find the ID in libraryItems
	var mediaId, mediaType string
	err := db.QueryRow("SELECT mediaId, mediaType FROM libraryItems WHERE id = ?", id).Scan(&mediaId, &mediaType)
	if err == nil {
		if mediaType == "book" {
			return getBookAudioInfo(db, mediaId)
		} else if mediaType == "podcast" {
			// Find first episode's audio path for the podcast
			var firstEpId string
			err = db.QueryRow("SELECT id FROM podcastEpisodes WHERE podcastId = ? LIMIT 1", mediaId).Scan(&firstEpId)
			if err == nil {
				return getPodcastEpisodeAudioInfo(db, firstEpId)
			}
		}
	}

	// 2. Try to find the ID in books (directly)
	var bookExists int
	errBook := db.QueryRow("SELECT 1 FROM books WHERE id = ?", id).Scan(&bookExists)
	if errBook == nil && bookExists == 1 {
		return getBookAudioInfo(db, id)
	}

	// 3. Try to find the ID in podcastEpisodes (directly)
	var epExists int
	errEp := db.QueryRow("SELECT 1 FROM podcastEpisodes WHERE id = ?", id).Scan(&epExists)
	if errEp == nil && epExists == 1 {
		return getPodcastEpisodeAudioInfo(db, id)
	}

	return nil, fmt.Errorf("item not found or has no audio files")
}

func getBookAudioInfo(db *sql.DB, bookID string) ([]AudioFileInfo, error) {
	var audioFilesJSONStr string
	err := db.QueryRow("SELECT audioFiles FROM books WHERE id = ?", bookID).Scan(&audioFilesJSONStr)
	if err != nil {
		return nil, err
	}
	type AudioFileJSON struct {
		Exclude  bool    `json:"exclude"`
		Duration float64 `json:"duration"`
		Metadata struct {
			Path string `json:"path"`
		} `json:"metadata"`
	}
	var audioFiles []AudioFileJSON
	if err := json.Unmarshal([]byte(audioFilesJSONStr), &audioFiles); err != nil {
		return nil, err
	}
	var infos []AudioFileInfo
	for _, af := range audioFiles {
		if !af.Exclude && af.Metadata.Path != "" {
			infos = append(infos, AudioFileInfo{
				Path:     af.Metadata.Path,
				Duration: af.Duration,
			})
		}
	}
	return infos, nil
}

func getPodcastEpisodeAudioInfo(db *sql.DB, epID string) ([]AudioFileInfo, error) {
	var audioFileJSONStr string
	err := db.QueryRow("SELECT audioFile FROM podcastEpisodes WHERE id = ?", epID).Scan(&audioFileJSONStr)
	if err != nil {
		return nil, err
	}
	type AudioFileStruct struct {
		Duration float64 `json:"duration"`
		Metadata struct {
			Path string `json:"path"`
		} `json:"metadata"`
	}
	var audioFile AudioFileStruct
	if err := json.Unmarshal([]byte(audioFileJSONStr), &audioFile); err != nil {
		return nil, err
	}
	if audioFile.Metadata.Path != "" {
		return []AudioFileInfo{{
			Path:     audioFile.Metadata.Path,
			Duration: audioFile.Duration,
		}}, nil
	}
	return nil, fmt.Errorf("no audio path found for podcast episode")
}
