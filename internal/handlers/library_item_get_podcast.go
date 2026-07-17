package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"encoding/json"
	"net/http"

	"audiobookshelf/internal/core"
	"audiobookshelf/internal/utils"
)

func getPodcastMediaDetails(w http.ResponseWriter, r *http.Request, db *sql.DB, mediaID string, user *core.UserSession, payload map[string]interface{}) bool {
	var pTitle, pAuthor, pDescription, pLanguage, pPodcastType, pCoverPath sql.NullString
	var pExplicit sql.NullInt64
	var pTags, pGenres, pLockedFields []byte
	var autoDownloadVal, maxKeepVal, maxNewVal, autoDeleteVal, skipIntroVal, skipOutroVal int
	var scheduleVal sql.NullString

	err := db.QueryRow(`
		SELECT title, author, description, language, podcastType, explicit, coverPath, tags, genres, lockedFields,
		       autoDownloadEpisodes, autoDownloadSchedule, maxEpisodesToKeep, maxNewEpisodesToDownload, autoDeletePlayed,
		       skipIntroDuration, skipOutroDuration
		FROM podcasts WHERE id = ?
	`, mediaID).Scan(
		&pTitle, &pAuthor, &pDescription, &pLanguage, &pPodcastType, &pExplicit, &pCoverPath, &pTags, &pGenres, &pLockedFields,
		&autoDownloadVal, &scheduleVal, &maxKeepVal, &maxNewVal, &autoDeleteVal, &skipIntroVal, &skipOutroVal,
	)
	if err != nil {
		log.Warnf("[Go Warning] Failed to scan podcast with id %s: %v", mediaID, err)
		return true
	}

	var tags []string
	_ = json.Unmarshal(pTags, &tags)
	if !user.IsAdminOrUp() {
		var explicit = pExplicit.Valid && pExplicit.Int64 != 0
		if explicit && !user.CanAccessExplicitContent {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return false
		}
		if !user.CheckCanAccessLibraryItemWithTags(tags) {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return false
		}
	}
	var genres []string
	_ = json.Unmarshal(pGenres, &genres)
	var lockedFields []string
	if len(pLockedFields) > 0 {
		_ = json.Unmarshal(pLockedFields, &lockedFields)
	}
	if lockedFields == nil {
		lockedFields = []string{}
	}

	episodes := getPodcastEpisodes(r, db, mediaID)

	payload["media"] = map[string]interface{}{
		"id":                       mediaID,
		"coverPath":                utils.NullIfEmpty(pCoverPath.String),
		"tags":                     tags,
		"episodes":                 episodes,
		"autoDownloadEpisodes":     autoDownloadVal == 1,
		"autoDownloadSchedule":     scheduleVal.String,
		"maxEpisodesToKeep":        maxKeepVal,
		"maxNewEpisodesToDownload": maxNewVal,
		"autoDeletePlayed":         autoDeleteVal == 1,
		"skipIntroDuration":        skipIntroVal,
		"skipOutroDuration":        skipOutroVal,
		"metadata": map[string]interface{}{
			"title":                    pTitle.String,
			"author":                   pAuthor.String,
			"description":              utils.NullIfEmpty(pDescription.String),
			"language":                 utils.NullIfEmpty(pLanguage.String),
			"podcastType":              utils.NullIfEmpty(pPodcastType.String),
			"explicit":                 pExplicit.Valid && pExplicit.Int64 != 0,
			"genres":                   genres,
			"lockedFields":             lockedFields,
			"autoDownloadEpisodes":     autoDownloadVal == 1,
			"autoDownloadSchedule":     scheduleVal.String,
			"maxEpisodesToKeep":        maxKeepVal,
			"maxNewEpisodesToDownload": maxNewVal,
			"autoDeletePlayed":         autoDeleteVal == 1,
			"skipIntroDuration":        skipIntroVal,
			"skipOutroDuration":        skipOutroVal,
		},
	}
	return true
}

func getPodcastEpisodes(r *http.Request, db *sql.DB, mediaID string) []map[string]interface{} {
	hasPubDate := hasColumn(r.Context(), db, "podcastEpisodes", "pubDate")
	hasDesc := hasColumn(r.Context(), db, "podcastEpisodes", "description")
	hasSeason := hasColumn(r.Context(), db, "podcastEpisodes", "season")
	hasEp := hasColumn(r.Context(), db, "podcastEpisodes", "episode")
	hasEpType := hasColumn(r.Context(), db, "podcastEpisodes", "episodeType")

	epQuery := "SELECT id, title, audioFile"
	if hasPubDate {
		epQuery += ", pubDate"
	}
	if hasDesc {
		epQuery += ", description"
	}
	if hasSeason {
		epQuery += ", season"
	}
	if hasEp {
		epQuery += ", episode"
	}
	if hasEpType {
		epQuery += ", episodeType"
	}
	epQuery += " FROM podcastEpisodes WHERE podcastId = ?"

	rows, err := db.Query(epQuery, mediaID)
	var episodes []map[string]interface{}
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var epID, epTitle, audioFileStr string
			var pubDateVal, descVal, seasonVal, epVal, epTypeVal sql.NullString

			dest := []interface{}{&epID, &epTitle, &audioFileStr}
			if hasPubDate {
				dest = append(dest, &pubDateVal)
			}
			if hasDesc {
				dest = append(dest, &descVal)
			}
			if hasSeason {
				dest = append(dest, &seasonVal)
			}
			if hasEp {
				dest = append(dest, &epVal)
			}
			if hasEpType {
				dest = append(dest, &epTypeVal)
			}

			if err := rows.Scan(dest...); err == nil {
				var af map[string]interface{}
				_ = json.Unmarshal([]byte(audioFileStr), &af)

				epMap := map[string]interface{}{
					"id":        epID,
					"title":     epTitle,
					"audioFile": af,
				}
				if hasPubDate && pubDateVal.Valid {
					epMap["pubDate"] = pubDateVal.String
				}
				if hasDesc && descVal.Valid {
					epMap["description"] = descVal.String
				}
				if hasSeason && seasonVal.Valid {
					epMap["season"] = seasonVal.String
				}
				if hasEp && epVal.Valid {
					epMap["episode"] = epVal.String
				}
				if hasEpType && epTypeVal.Valid {
					epMap["episodeType"] = epTypeVal.String
				}

				if af != nil {
					if dur, ok := af["duration"]; ok {
						epMap["duration"] = dur
					}
					if meta, ok := af["metadata"].(map[string]interface{}); ok {
						if sz, ok := meta["size"]; ok {
							epMap["size"] = sz
						}
					}
				}

				episodes = append(episodes, epMap)
			}
		}
	}
	return episodes
}
