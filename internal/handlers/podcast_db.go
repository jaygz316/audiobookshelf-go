package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
)

func fetchPodcastEpisodesList(ctx context.Context, db *sql.DB, podcastID string) ([]map[string]interface{}, error) {
	hasPubDate := hasColumn(ctx, db, "podcastEpisodes", "pubDate")
	hasDesc := hasColumn(ctx, db, "podcastEpisodes", "description")
	hasSeason := hasColumn(ctx, db, "podcastEpisodes", "season")
	hasEp := hasColumn(ctx, db, "podcastEpisodes", "episode")
	hasEpType := hasColumn(ctx, db, "podcastEpisodes", "episodeType")
	hasEnclosureURL := hasColumn(ctx, db, "podcastEpisodes", "enclosureURL")

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
	if hasEnclosureURL {
		epQuery += ", enclosureURL"
	}
	epQuery += " FROM podcastEpisodes WHERE podcastId = ?"

	rows, err := db.QueryContext(ctx, epQuery, podcastID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var episodes []map[string]interface{}
	for rows.Next() {
		var epID, epTitle, audioFileStr string
		var pubDateVal, descVal, seasonVal, epVal, epTypeVal, encURLVal sql.NullString

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
		if hasEnclosureURL {
			dest = append(dest, &encURLVal)
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
			if hasEnclosureURL && encURLVal.Valid {
				epMap["enclosureUrl"] = encURLVal.String
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
	return episodes, nil
}
