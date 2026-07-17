package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
)

func updatePodcastDetails(r *http.Request, tx *sql.Tx, itemID, mediaID, nowStr string, prefixes []string, payload *updateItemPayload) error {
	tagsJSON, _ := json.Marshal(payload.Tags)
	genresJSON, _ := json.Marshal(payload.Genres)
	lockedFieldsJSON, _ := json.Marshal(payload.LockedFields)
	var author string
	if len(payload.Authors) > 0 {
		author = payload.Authors[0]
	}

	titleIgnorePrefix := getTitleIgnorePrefixGo(payload.Title, prefixes)

	var currAutoDownload, currMaxKeep, currMaxNew, currAutoDelete, currSkipIntro, currSkipOutro int
	var currSchedule sql.NullString
	_ = tx.QueryRowContext(r.Context(), "SELECT autoDownloadEpisodes, maxEpisodesToKeep, maxNewEpisodesToDownload, autoDeletePlayed, autoDownloadSchedule, skipIntroDuration, skipOutroDuration FROM podcasts WHERE id = ?", mediaID).Scan(&currAutoDownload, &currMaxKeep, &currMaxNew, &currAutoDelete, &currSchedule, &currSkipIntro, &currSkipOutro)

	autoDownloadVal := currAutoDownload
	if payload.AutoDownloadEpisodes != nil {
		autoDownloadVal = boolToInt(*payload.AutoDownloadEpisodes)
	}
	maxKeepVal := currMaxKeep
	if payload.MaxEpisodesToKeep != nil {
		maxKeepVal = *payload.MaxEpisodesToKeep
	}
	maxNewVal := currMaxNew
	if payload.MaxNewEpisodesToDownload != nil {
		maxNewVal = *payload.MaxNewEpisodesToDownload
	}
	autoDeleteVal := currAutoDelete
	if payload.AutoDeletePlayed != nil {
		autoDeleteVal = boolToInt(*payload.AutoDeletePlayed)
	}
	scheduleVal := currSchedule.String
	if payload.AutoDownloadSchedule != nil {
		scheduleVal = *payload.AutoDownloadSchedule
	}
	skipIntroVal := currSkipIntro
	if payload.SkipIntroDuration != nil {
		skipIntroVal = *payload.SkipIntroDuration
	}
	skipOutroVal := currSkipOutro
	if payload.SkipOutroDuration != nil {
		skipOutroVal = *payload.SkipOutroDuration
	}

	_, err := tx.Exec(`
		UPDATE podcasts
		SET title = ?, titleIgnorePrefix = ?, author = ?, description = ?, language = ?, explicit = ?, tags = ?, genres = ?, lockedFields = ?,
		    autoDownloadEpisodes = ?, maxEpisodesToKeep = ?, maxNewEpisodesToDownload = ?, autoDeletePlayed = ?, autoDownloadSchedule = ?,
		    skipIntroDuration = ?, skipOutroDuration = ?
		WHERE id = ?
	`, payload.Title, titleIgnorePrefix, author, payload.Description, payload.Language, boolToInt(payload.Explicit), tagsJSON, genresJSON, lockedFieldsJSON,
		autoDownloadVal, maxKeepVal, maxNewVal, autoDeleteVal, scheduleVal, skipIntroVal, skipOutroVal, mediaID)
	if err != nil {
		return err
	}

	_, err = tx.Exec(`
		UPDATE libraryItems
		SET title = ?, titleIgnorePrefix = ?, authorNamesFirstLast = ?, authorNamesLastFirst = ?, updatedAt = ?
		WHERE id = ?
	`, payload.Title, titleIgnorePrefix, author, author, nowStr, itemID)
	return err
}
