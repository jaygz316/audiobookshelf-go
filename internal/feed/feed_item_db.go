package feed

import (
	"context"
	"database/sql"
	"encoding/json"
)

func (m *FeedManager) getPlaylistItemPath(ctx context.Context, itemID, episodeID string) (string, string, error) {
	rows, err := m.db.QueryContext(ctx, `
		SELECT p.mediaItemId, p.mediaItemType, b.audioFiles
		FROM playlistMediaItems p
		LEFT JOIN books b ON p.mediaItemId = b.id AND p.mediaItemType = 'book'
		WHERE p.playlistId = ?
		ORDER BY p."order" ASC
	`, itemID)
	if err != nil {
		return "", "", err
	}
	defer rows.Close()
	for rows.Next() {
		var mediaItemID, mediaItemType string
		var audioFilesJSON sql.NullString
		if err := rows.Scan(&mediaItemID, &mediaItemType, &audioFilesJSON); err == nil {
			if mediaItemType == "podcastEpisode" && mediaItemID == episodeID {
				var audioFileJSON string
				err := m.db.QueryRowContext(ctx, "SELECT audioFile FROM podcastEpisodes WHERE id = ?", episodeID).Scan(&audioFileJSON)
				if err == nil {
					var af audioFile
					if json.Unmarshal([]byte(audioFileJSON), &af) == nil {
						return af.Metadata.Path, af.MimeType, nil
					}
				}
			} else if mediaItemType == "book" && audioFilesJSON.Valid {
				var tracks []audiobookTrack
				if json.Unmarshal([]byte(audioFilesJSON.String), &tracks) == nil {
					for _, t := range tracks {
						if t.Exclude {
							continue
						}
						// PORT: Deterministic MD5 hash to uniquely identify tracks without database state
						trackID := computeMD5(itemID + "_" + mediaItemID + "_" + t.Metadata.Path)
						if trackID == episodeID {
							return t.Metadata.Path, t.MimeType, nil
						}
					}
				}
			}
		}
	}
	return "", "", rows.Err()
}

func (m *FeedManager) getCollectionItemPath(ctx context.Context, itemID, episodeID string) (string, string, error) {
	rows, err := m.db.QueryContext(ctx, `
		SELECT b.id, b.audioFiles
		FROM collectionBooks cb
		JOIN books b ON cb.bookId = b.id
		WHERE cb.collectionId = ?
		ORDER BY cb."order" ASC
	`, itemID)
	if err != nil {
		return "", "", err
	}
	defer rows.Close()
	for rows.Next() {
		var mediaItemID string
		var audioFilesJSON sql.NullString
		if err := rows.Scan(&mediaItemID, &audioFilesJSON); err == nil && audioFilesJSON.Valid {
			var tracks []audiobookTrack
			if json.Unmarshal([]byte(audioFilesJSON.String), &tracks) == nil {
				for _, t := range tracks {
					if t.Exclude {
						continue
					}
					trackID := computeMD5(itemID + "_" + mediaItemID + "_" + t.Metadata.Path)
					if trackID == episodeID {
						return t.Metadata.Path, t.MimeType, nil
					}
				}
			}
		}
	}
	return "", "", rows.Err()
}

func (m *FeedManager) getSeriesItemPath(ctx context.Context, itemID, episodeID string) (string, string, error) {
	rows, err := m.db.QueryContext(ctx, `
		SELECT b.id, b.audioFiles
		FROM bookSeries bs
		JOIN books b ON bs.bookId = b.id
		WHERE bs.seriesId = ?
		ORDER BY CAST(bs.sequence AS REAL) ASC, bs.sequence ASC
	`, itemID)
	if err != nil {
		return "", "", err
	}
	defer rows.Close()
	for rows.Next() {
		var mediaItemID string
		var audioFilesJSON sql.NullString
		if err := rows.Scan(&mediaItemID, &audioFilesJSON); err == nil && audioFilesJSON.Valid {
			var tracks []audiobookTrack
			if json.Unmarshal([]byte(audioFilesJSON.String), &tracks) == nil {
				for _, t := range tracks {
					if t.Exclude {
						continue
					}
					trackID := computeMD5(itemID + "_" + mediaItemID + "_" + t.Metadata.Path)
					if trackID == episodeID {
						return t.Metadata.Path, t.MimeType, nil
					}
				}
			}
		}
	}
	return "", "", rows.Err()
}

func (m *FeedManager) getLibraryItemPath(ctx context.Context, itemID, episodeID, entityType string) (string, string, error) {
	var mediaID string = itemID
	var mediaType string = entityType

	// If itemID is a libraryItem ID, resolve to mediaID
	var mID string
	if m.db.QueryRowContext(ctx, "SELECT mediaId FROM libraryItems WHERE id = ?", itemID).Scan(&mID) == nil {
		mediaID = mID
	}

	if mediaType == "podcast" {
		var audioFileJSON string
		err := m.db.QueryRowContext(ctx, `
			SELECT audioFile FROM podcastEpisodes 
			WHERE id = ? AND podcastId = ?
		`, episodeID, mediaID).Scan(&audioFileJSON)
		if err != nil {
			return "", "", err
		}
		var af audioFile
		if err := json.Unmarshal([]byte(audioFileJSON), &af); err != nil {
			return "", "", err
		}
		return af.Metadata.Path, af.MimeType, nil
	} else if mediaType == "book" {
		var audioFilesJSON string
		err := m.db.QueryRowContext(ctx, "SELECT audioFiles FROM books WHERE id = ?", mediaID).Scan(&audioFilesJSON)
		if err != nil {
			return "", "", err
		}
		var tracks []audiobookTrack
		if err := json.Unmarshal([]byte(audioFilesJSON), &tracks); err != nil {
			return "", "", err
		}
		for _, t := range tracks {
			if t.Exclude {
				continue
			}
			trackID := computeMD5(t.Metadata.Path)
			if trackID == episodeID {
				return t.Metadata.Path, t.MimeType, nil
			}
		}
	}
	return "", "", nil
}
