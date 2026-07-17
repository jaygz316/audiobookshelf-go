package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	log "audiobookshelf/internal/logger"
	"github.com/google/uuid"
)

// resolveMediaItemFromPath parses the subPath and queries database to resolve the IDs and type.
func resolveMediaItemFromPath(ctx context.Context, db *sql.DB, subPath string) (libraryItemID, episodeID, mediaItemID, mediaItemType string, podcastID sql.NullString, status int, err error) {
	parts := strings.Split(subPath, "/")
	if len(parts) == 0 || parts[0] == "" {
		return "", "", "", "", sql.NullString{}, http.StatusBadRequest, fmt.Errorf("bad request")
	}

	libraryItemID = parts[0]
	if len(parts) > 1 {
		episodeID = parts[1]
	}

	if episodeID != "" {
		err = db.QueryRowContext(ctx, "SELECT id, podcastId FROM podcastEpisodes WHERE id = ?", episodeID).Scan(&mediaItemID, &podcastID)
		if err == sql.ErrNoRows {
			return libraryItemID, episodeID, "", "", sql.NullString{}, http.StatusNotFound, fmt.Errorf("episode not found")
		} else if err != nil {
			return libraryItemID, episodeID, "", "", sql.NullString{}, http.StatusInternalServerError, err
		}
		mediaItemType = "podcastEpisode"
	} else {
		var mediaType string
		err = db.QueryRowContext(ctx, "SELECT mediaId, mediaType FROM libraryItems WHERE id = ? OR mediaId = ?", libraryItemID, libraryItemID).Scan(&mediaItemID, &mediaType)
		if err == sql.ErrNoRows {
			return libraryItemID, episodeID, "", "", sql.NullString{}, http.StatusNotFound, fmt.Errorf("library item not found")
		} else if err != nil {
			return libraryItemID, episodeID, "", "", sql.NullString{}, http.StatusInternalServerError, err
		}
		if mediaType != "book" {
			return libraryItemID, episodeID, "", "", sql.NullString{}, http.StatusBadRequest, fmt.Errorf("library item is not a book")
		}
		mediaItemType = "book"
	}
	return libraryItemID, episodeID, mediaItemID, mediaItemType, podcastID, 0, nil
}

// queryMediaProgress queries mediaProgresses and scans fields.
func queryMediaProgress(ctx context.Context, db *sql.DB, userID, mediaItemID string) (progressID string, currDuration, currCurrentTime float64, currIsFinished, currHideFromContinueListening int, currEbookLocation, currFinishedAt, currExtraData, currCreatedAt, currUpdatedAt sql.NullString, currEbookProgress sql.NullFloat64, exists bool, err error) {
	err = db.QueryRowContext(ctx, `SELECT id, duration, currentTime, isFinished, hideFromContinueListening, ebookLocation, ebookProgress, finishedAt, extraData, createdAt, updatedAt 
		FROM mediaProgresses WHERE userId = ? AND mediaItemId = ?`, userID, mediaItemID).
		Scan(&progressID, &currDuration, &currCurrentTime, &currIsFinished, &currHideFromContinueListening, &currEbookLocation, &currEbookProgress, &currFinishedAt, &currExtraData, &currCreatedAt, &currUpdatedAt)

	exists = true
	if err == sql.ErrNoRows {
		exists = false
		err = nil
	}
	return
}

// saveMediaProgress executes UPDATE or INSERT for media progress depending on whether it exists.
func saveMediaProgress(ctx context.Context, db *sql.DB, exists bool, progressID, userID, mediaItemID, mediaItemType string, durationVal, currentTimeVal float64, isFinishedVal, hideFromContinueListeningVal bool, ebookLocationNullable interface{}, ebookProgressVal float64, finishedAtNullable interface{}, extraBytes []byte, podcastID sql.NullString, nowStr, updatedAtStr string) error {
	var err error
	if exists {
		_, err = db.ExecContext(ctx, `UPDATE mediaProgresses SET duration = ?, currentTime = ?, isFinished = ?, hideFromContinueListening = ?, ebookLocation = ?, ebookProgress = ?, finishedAt = ?, extraData = ?, updatedAt = ? WHERE id = ?`,
			durationVal, currentTimeVal, func() int {
				if isFinishedVal {
					return 1
				}
				return 0
			}(), func() int {
				if hideFromContinueListeningVal {
					return 1
				}
				return 0
			}(), ebookLocationNullable, ebookProgressVal, finishedAtNullable, string(extraBytes), updatedAtStr, progressID)
	} else {
		progressID = uuid.New().String()
		_, err = db.ExecContext(ctx, `INSERT INTO mediaProgresses (id, userId, mediaItemId, mediaItemType, duration, currentTime, isFinished, hideFromContinueListening, ebookLocation, ebookProgress, finishedAt, extraData, podcastId, createdAt, updatedAt) 
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			progressID, userID, mediaItemID, mediaItemType, durationVal, currentTimeVal, func() int {
				if isFinishedVal {
					return 1
				}
				return 0
			}(), func() int {
				if hideFromContinueListeningVal {
					return 1
				}
				return 0
			}(), ebookLocationNullable, ebookProgressVal, finishedAtNullable, string(extraBytes), podcastID, nowStr, updatedAtStr)
	}
	return err
}

// handleAutoDeletePlayedEpisode automatically deletes an episode file if autoDeletePlayed is enabled.
func handleAutoDeletePlayedEpisode(ctx context.Context, db *sql.DB, mediaItemID string) {
	var pID string
	var autoDeletePlayedVal int
	var audioFileStr string
	errDb := db.QueryRowContext(ctx, `
		SELECT p.id, p.autoDeletePlayed, pe.audioFile
		FROM podcastEpisodes pe
		JOIN podcasts p ON pe.podcastId = p.id
		WHERE pe.id = ?
	`, mediaItemID).Scan(&pID, &autoDeletePlayedVal, &audioFileStr)
	if errDb == nil && autoDeletePlayedVal == 1 && audioFileStr != "" && audioFileStr != "{}" {
		var af struct {
			Metadata struct {
				Path string `json:"path"`
			} `json:"metadata"`
		}
		if json.Unmarshal([]byte(audioFileStr), &af) == nil && af.Metadata.Path != "" {
			if _, err := os.Stat(af.Metadata.Path); err == nil {
				log.Infof("[AutoDeletePlayed] Deleting played episode file: %s", af.Metadata.Path)
				if err := os.Remove(af.Metadata.Path); err == nil {
					_, _ = db.ExecContext(ctx, "UPDATE podcastEpisodes SET audioFile = '{}' WHERE id = ?", mediaItemID)
				} else {
					log.Errorf("[AutoDeletePlayed] Failed to delete file %s: %v", af.Metadata.Path, err)
				}
			}
		}
	}
}
