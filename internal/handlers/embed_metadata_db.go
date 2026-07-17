package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

type ChapterInfo struct {
	ID    int     `json:"id"`
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Title string  `json:"title"`
}

type bookEmbedMetadata struct {
	MediaID       string
	MediaType     string
	AuthorName    string
	Title         string
	Subtitle      string
	PublishedYear string
	PublishedDate string
	Publisher     string
	Description   string
	CoverPath     string
	Narrators     []string
	AudioFiles    []map[string]interface{}
	Chapters      []ChapterInfo
	Genres        []string
	Tags          []string
}

func getBookMetadataForEmbedding(db *sql.DB, itemID string) (*bookEmbedMetadata, error) {
	var mediaID, mediaType, authorName string
	err := db.QueryRow("SELECT mediaId, mediaType, authorNamesFirstLast FROM libraryItems WHERE id = ?", itemID).Scan(&mediaID, &mediaType, &authorName)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, err
		}
		return nil, fmt.Errorf("DB error: %v", err)
	}

	if mediaType != "book" {
		return nil, errors.New("Only books support metadata tag embedding")
	}

	// Retrieve all metadata for the book
	var (
		title, subtitle, publishedYear, publishedDate, publisher, description, coverPath string
		bNarrators, bAudioFiles, bChapters, bGenres, bTags                               []byte
	)

	err = db.QueryRow(`
		SELECT title, subtitle, publishedYear, publishedDate, publisher, description, coverPath, narrators, audioFiles, chapters, genres, tags
		FROM books WHERE id = ?`, mediaID).Scan(
		&title, &subtitle, &publishedYear, &publishedDate, &publisher, &description, &coverPath,
		&bNarrators, &bAudioFiles, &bChapters, &bGenres, &bTags,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, err
		}
		return nil, fmt.Errorf("DB error querying book: %v", err)
	}

	var narrators []string
	_ = json.Unmarshal(bNarrators, &narrators)

	var audioFiles []map[string]interface{}
	_ = json.Unmarshal(bAudioFiles, &audioFiles)

	var chapters []ChapterInfo
	_ = json.Unmarshal(bChapters, &chapters)

	var genres []string
	_ = json.Unmarshal(bGenres, &genres)

	var tags []string
	_ = json.Unmarshal(bTags, &tags)

	if len(audioFiles) == 0 {
		return nil, errors.New("No audio files found for this library item")
	}

	return &bookEmbedMetadata{
		MediaID:       mediaID,
		MediaType:     mediaType,
		AuthorName:    authorName,
		Title:         title,
		Subtitle:      subtitle,
		PublishedYear: publishedYear,
		PublishedDate: publishedDate,
		Publisher:     publisher,
		Description:   description,
		CoverPath:     coverPath,
		Narrators:     narrators,
		AudioFiles:    audioFiles,
		Chapters:      chapters,
		Genres:        genres,
		Tags:          tags,
	}, nil
}
