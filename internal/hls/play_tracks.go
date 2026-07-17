package hls

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
)

func getPodcastEpisodeTracks(ctx context.Context, db *sql.DB, mediaItemID string, sessionID string) (audioTracks []map[string]interface{}, displayTitle string, displayAuthor string, err error) {
	var audioFileJSONStr string
	var epTitle string
	err = db.QueryRowContext(ctx, `SELECT title, audioFile FROM podcastEpisodes WHERE id = ?`, mediaItemID).Scan(&epTitle, &audioFileJSONStr)
	if err != nil {
		return nil, "", "", err
	}
	displayTitle = epTitle

	var podcastID string
	_ = db.QueryRowContext(ctx, `SELECT podcastId FROM podcastEpisodes WHERE id = ?`, mediaItemID).Scan(&podcastID)
	if podcastID != "" {
		var podAuthor string
		_ = db.QueryRowContext(ctx, `SELECT author FROM podcasts WHERE id = ?`, podcastID).Scan(&podAuthor)
		displayAuthor = podAuthor
	}

	type AudioFileStruct struct {
		Duration float64 `json:"duration"`
		Codec    string  `json:"codec"`
		MimeType string  `json:"mimeType"`
		Metadata struct {
			Path string `json:"path"`
		} `json:"metadata"`
	}
	var audioFile AudioFileStruct
	if err := json.Unmarshal([]byte(audioFileJSONStr), &audioFile); err == nil {
		audioTracks = append(audioTracks, map[string]interface{}{
			"index":       0,
			"startOffset": 0.0,
			"duration":    audioFile.Duration,
			"title":       epTitle,
			"contentUrl":  fmt.Sprintf("/hls/%s/output.m3u8", sessionID),
			"mimeType":    audioFile.MimeType,
			"metadata": map[string]interface{}{
				"path": audioFile.Metadata.Path,
			},
		})
	}
	return audioTracks, displayTitle, displayAuthor, nil
}

func getBookTracks(ctx context.Context, db *sql.DB, mediaItemID string, sessionID string) (audioTracks []map[string]interface{}, displayTitle string, displayAuthor string, err error) {
	var bTitle string
	err = db.QueryRowContext(ctx, `SELECT title FROM books WHERE id = ?`, mediaItemID).Scan(&bTitle)
	if err != nil {
		return nil, "", "", err
	}
	displayTitle = bTitle

	// Get book authors
	var authorNames []string
	rows, errAuthors := db.QueryContext(ctx, "SELECT name FROM authors WHERE id IN (SELECT authorId FROM bookAuthors WHERE bookId = ?)", mediaItemID)
	if errAuthors == nil {
		defer rows.Close()
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err == nil {
				authorNames = append(authorNames, name)
			}
		}
	}
	displayAuthor = strings.Join(authorNames, ", ")

	var audioFilesJSONStr string
	err = db.QueryRowContext(ctx, `SELECT audioFiles FROM books WHERE id = ?`, mediaItemID).Scan(&audioFilesJSONStr)
	if err == nil {
		type AudioFileJSON struct {
			Index    int     `json:"index"`
			Exclude  bool    `json:"exclude"`
			Duration float64 `json:"duration"`
			Codec    string  `json:"codec"`
			MimeType string  `json:"mimeType"`
			Metadata struct {
				Path     string `json:"path"`
				Filename string `json:"filename"`
				Size     int64  `json:"size"`
			} `json:"metadata"`
		}
		var audioFiles []AudioFileJSON
		if err := json.Unmarshal([]byte(audioFilesJSONStr), &audioFiles); err == nil {
			var currentOffset float64 = 0.0
			for _, af := range audioFiles {
				if !af.Exclude {
					audioTracks = append(audioTracks, map[string]interface{}{
						"index":       af.Index,
						"startOffset": currentOffset,
						"duration":    af.Duration,
						"title":       af.Metadata.Filename,
						"contentUrl":  fmt.Sprintf("/hls/%s/output.m3u8", sessionID),
						"mimeType":    af.MimeType,
						"metadata": map[string]interface{}{
							"path":     af.Metadata.Path,
							"filename": af.Metadata.Filename,
							"size":     af.Metadata.Size,
						},
					})
					currentOffset += af.Duration
				}
			}
		}
	}
	return audioTracks, displayTitle, displayAuthor, nil
}
