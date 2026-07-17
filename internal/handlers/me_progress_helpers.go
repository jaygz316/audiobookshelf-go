package handlers

import (
	"database/sql"
	"encoding/json"

	idb "audiobookshelf/internal/db"
)

// scanMediaProgressRows scans multiple database rows into a slice of maps used by client API
func scanMediaProgressRows(rows *sql.Rows) ([]map[string]interface{}, error) {
	var results []map[string]interface{}
	for rows.Next() {
		var id, userId, mediaItemId, mediaItemType string
		var duration, currentTime float64
		var isFinishedInt, hideFromContinueListeningInt int
		var ebookLocation, finishedAt, extraData, createdAt, updatedAt, podcastId sql.NullString
		var ebookProgress sql.NullFloat64

		err := rows.Scan(&id, &userId, &mediaItemId, &mediaItemType, &duration, &currentTime, &isFinishedInt, &hideFromContinueListeningInt, &ebookLocation, &ebookProgress, &finishedAt, &extraData, &createdAt, &updatedAt, &podcastId)
		if err != nil {
			return nil, err
		}

		var extra map[string]interface{}
		if extraData.Valid && extraData.String != "" {
			json.Unmarshal([]byte(extraData.String), &extra)
		}
		if extra == nil {
			extra = make(map[string]interface{})
		}

		libItemID, _ := extra["libraryItemId"].(string)
		progressVal := 0.0
		if duration > 0 {
			progressVal = currentTime / duration
			if progressVal > 1.0 {
				progressVal = 1.0
			}
		}

		updatedAtMs := idb.ParseTimeStr(updatedAt.String)
		createdAtMs := idb.ParseTimeStr(createdAt.String)
		var finishedAtMs *int64
		if finishedAt.Valid && finishedAt.String != "" {
			val := idb.ParseTimeStr(finishedAt.String)
			finishedAtMs = &val
		}

		var episodeID *string
		if mediaItemType == "podcastEpisode" {
			val := mediaItemId
			episodeID = &val
		}

		var ebookLoc *string
		if ebookLocation.Valid {
			val := ebookLocation.String
			ebookLoc = &val
		}
		var ebookProgVal *float64
		if ebookProgress.Valid {
			val := ebookProgress.Float64
			ebookProgVal = &val
		}

		results = append(results, map[string]interface{}{
			"id":                        id,
			"userId":                    userId,
			"libraryItemId":             libItemID,
			"episodeId":                 episodeID,
			"mediaItemId":               mediaItemId,
			"mediaItemType":             mediaItemType,
			"duration":                  duration,
			"progress":                  progressVal,
			"currentTime":               currentTime,
			"isFinished":                isFinishedInt != 0,
			"hideFromContinueListening": hideFromContinueListeningInt != 0,
			"ebookLocation":             ebookLoc,
			"ebookProgress":             ebookProgVal,
			"lastUpdate":                updatedAtMs,
			"startedAt":                 createdAtMs,
			"finishedAt":                finishedAtMs,
		})
	}
	return results, nil
}

// scanMediaProgress scans a database row into a map format used by client API
func scanMediaProgress(row *sql.Row) (map[string]interface{}, error) {
	var id, userId, mediaItemId, mediaItemType string
	var duration, currentTime float64
	var isFinishedInt, hideFromContinueListeningInt int
	var ebookLocation, finishedAt, extraData, createdAt, updatedAt, podcastId sql.NullString
	var ebookProgress sql.NullFloat64

	err := row.Scan(&id, &userId, &mediaItemId, &mediaItemType, &duration, &currentTime, &isFinishedInt, &hideFromContinueListeningInt, &ebookLocation, &ebookProgress, &finishedAt, &extraData, &createdAt, &updatedAt, &podcastId)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var extra map[string]interface{}
	if extraData.Valid && extraData.String != "" {
		json.Unmarshal([]byte(extraData.String), &extra)
	}
	if extra == nil {
		extra = make(map[string]interface{})
	}

	libItemID, _ := extra["libraryItemId"].(string)
	progressVal := 0.0
	if duration > 0 {
		progressVal = currentTime / duration
		if progressVal > 1.0 {
			progressVal = 1.0
		}
	}

	updatedAtMs := idb.ParseTimeStr(updatedAt.String)
	createdAtMs := idb.ParseTimeStr(createdAt.String)
	var finishedAtMs *int64
	if finishedAt.Valid && finishedAt.String != "" {
		val := idb.ParseTimeStr(finishedAt.String)
		finishedAtMs = &val
	}

	var episodeID *string
	if mediaItemType == "podcastEpisode" {
		val := mediaItemId
		episodeID = &val
	}

	var ebookLoc *string
	if ebookLocation.Valid {
		val := ebookLocation.String
		ebookLoc = &val
	}
	var ebookProgVal *float64
	if ebookProgress.Valid {
		val := ebookProgress.Float64
		ebookProgVal = &val
	}

	return map[string]interface{}{
		"id":                        id,
		"userId":                    userId,
		"libraryItemId":             libItemID,
		"episodeId":                 episodeID,
		"mediaItemId":               mediaItemId,
		"mediaItemType":             mediaItemType,
		"duration":                  duration,
		"progress":                  progressVal,
		"currentTime":               currentTime,
		"isFinished":                isFinishedInt != 0,
		"hideFromContinueListening": hideFromContinueListeningInt != 0,
		"ebookLocation":             ebookLoc,
		"ebookProgress":             ebookProgVal,
		"lastUpdate":                updatedAtMs,
		"startedAt":                 createdAtMs,
		"finishedAt":                finishedAtMs,
	}, nil
}
