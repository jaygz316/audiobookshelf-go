package feed

import (
	"context"
	"database/sql"
	"fmt"
)

func queryPodcastEpisode(ctx context.Context, db *sql.DB, epID string) (*podcastEpData, error) {
	hasPubDate := hasColumn(ctx, db, "podcastEpisodes", "pubDate")
	hasDesc := hasColumn(ctx, db, "podcastEpisodes", "description")
	hasSeason := hasColumn(ctx, db, "podcastEpisodes", "season")
	hasEp := hasColumn(ctx, db, "podcastEpisodes", "episode")
	hasEpType := hasColumn(ctx, db, "podcastEpisodes", "episodeType")

	query := "SELECT id, title, audioFile"
	if hasPubDate {
		query += ", pubDate"
	}
	if hasDesc {
		query += ", description"
	}
	if hasSeason {
		query += ", season"
	}
	if hasEp {
		query += ", episode"
	}
	if hasEpType {
		query += ", episodeType"
	}
	query += " FROM podcastEpisodes WHERE id = ?"

	row := db.QueryRowContext(ctx, query, epID)

	var id, title, audioFileStr string
	dest := []interface{}{&id, &title, &audioFileStr}

	var pubDateVal, descVal, seasonVal, epVal, epTypeVal sql.NullString
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

	if err := row.Scan(dest...); err != nil {
		return nil, fmt.Errorf("scan podcast episode: %w", err)
	}

	ep := &podcastEpData{
		ID:        id,
		Title:     title,
		AudioFile: audioFileStr,
	}
	if pubDateVal.Valid {
		ep.PubDate = pubDateVal.String
	}
	if descVal.Valid {
		ep.Description = descVal.String
	}
	if seasonVal.Valid {
		ep.Season = seasonVal.String
	}
	if epVal.Valid {
		ep.Episode = epVal.String
	}
	if epTypeVal.Valid {
		ep.EpisodeType = epTypeVal.String
	}
	return ep, nil
}

func queryPodcastEpisodes(ctx context.Context, db *sql.DB, podcastID string) ([]*podcastEpData, error) {
	hasPubDate := hasColumn(ctx, db, "podcastEpisodes", "pubDate")
	hasDesc := hasColumn(ctx, db, "podcastEpisodes", "description")
	hasSeason := hasColumn(ctx, db, "podcastEpisodes", "season")
	hasEp := hasColumn(ctx, db, "podcastEpisodes", "episode")
	hasEpType := hasColumn(ctx, db, "podcastEpisodes", "episodeType")

	query := "SELECT id, title, audioFile"
	if hasPubDate {
		query += ", pubDate"
	}
	if hasDesc {
		query += ", description"
	}
	if hasSeason {
		query += ", season"
	}
	if hasEp {
		query += ", episode"
	}
	if hasEpType {
		query += ", episodeType"
	}
	query += " FROM podcastEpisodes WHERE podcastId = ?"

	rows, err := db.QueryContext(ctx, query, podcastID)
	if err != nil {
		return nil, fmt.Errorf("query podcast episodes: %w", err)
	}
	defer rows.Close()

	var eps []*podcastEpData
	for rows.Next() {
		var id, title, audioFileStr string
		dest := []interface{}{&id, &title, &audioFileStr}
		var pubDateVal, descVal, seasonVal, epVal, epTypeVal sql.NullString
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

		if err := rows.Scan(dest...); err != nil {
			return nil, fmt.Errorf("scan podcast episode row: %w", err)
		}

		ep := &podcastEpData{
			ID:        id,
			Title:     title,
			AudioFile: audioFileStr,
		}
		if pubDateVal.Valid {
			ep.PubDate = pubDateVal.String
		}
		if descVal.Valid {
			ep.Description = descVal.String
		}
		if seasonVal.Valid {
			ep.Season = seasonVal.String
		}
		if epVal.Valid {
			ep.Episode = epVal.String
		}
		if epTypeVal.Valid {
			ep.EpisodeType = epTypeVal.String
		}
		eps = append(eps, ep)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("podcast episodes rows error: %w", err)
	}
	return eps, nil
}
