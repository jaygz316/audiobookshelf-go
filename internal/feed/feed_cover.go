package feed

import (
	"database/sql"
	"net/http"

	"audiobookshelf/internal/utils"
)

// Cover serving
func (m *FeedManager) serveFeedCover(w http.ResponseWriter, r *http.Request, itemID string, entityType string) {
	ctx := r.Context()

	var coverPath string
	if entityType == "playlist" {
		// PORT: Legacy behavior resolves playlist cover using the first available item cover
		rows, err := m.db.QueryContext(ctx, `
			SELECT mediaItemId, mediaItemType 
			FROM playlistMediaItems 
			WHERE playlistId = ? 
			ORDER BY "order" ASC
		`, itemID)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var mediaItemID, mediaItemType string
				if err := rows.Scan(&mediaItemID, &mediaItemType); err == nil {
					if mediaItemType == "book" {
						var bookCover sql.NullString
						if err := m.db.QueryRowContext(ctx, "SELECT coverPath FROM books WHERE id = ?", mediaItemID).Scan(&bookCover); err == nil && bookCover.Valid && bookCover.String != "" {
							coverPath = bookCover.String
							break
						}
					} else if mediaItemType == "podcastEpisode" {
						var podcastCover sql.NullString
						if err := m.db.QueryRowContext(ctx, `
							SELECT p.coverPath FROM podcasts p
							JOIN podcastEpisodes pe ON pe.podcastId = p.id
							WHERE pe.id = ?
						`, mediaItemID).Scan(&podcastCover); err == nil && podcastCover.Valid && podcastCover.String != "" {
							coverPath = podcastCover.String
							break
						}
					}
				}
			}
			_ = rows.Err()
		}
	} else if entityType == "collection" {
		// Query first available book in collection
		rows, err := m.db.QueryContext(ctx, `
			SELECT b.coverPath
			FROM collectionBooks cb
			JOIN books b ON cb.bookId = b.id
			WHERE cb.collectionId = ?
			ORDER BY cb."order" ASC
		`, itemID)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var bCover sql.NullString
				if err := rows.Scan(&bCover); err == nil && bCover.Valid && bCover.String != "" {
					coverPath = bCover.String
					break
				}
			}
			_ = rows.Err()
		}
	} else if entityType == "series" {
		// Query first available book in series
		rows, err := m.db.QueryContext(ctx, `
			SELECT b.coverPath
			FROM bookSeries bs
			JOIN books b ON bs.bookId = b.id
			WHERE bs.seriesId = ?
			ORDER BY CAST(bs.sequence AS REAL) ASC, bs.sequence ASC
		`, itemID)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var bCover sql.NullString
				if err := rows.Scan(&bCover); err == nil && bCover.Valid && bCover.String != "" {
					coverPath = bCover.String
					break
				}
			}
			_ = rows.Err()
		}
	} else {
		var mediaID string = itemID
		var mediaType string = entityType

		// If itemID is a libraryItem ID, resolve to mediaID
		var mID string
		if m.db.QueryRowContext(ctx, "SELECT mediaId FROM libraryItems WHERE id = ?", itemID).Scan(&mID) == nil {
			mediaID = mID
		}

		var cp sql.NullString
		if mediaType == "book" {
			_ = m.db.QueryRowContext(ctx, "SELECT coverPath FROM books WHERE id = ?", mediaID).Scan(&cp)
		} else if mediaType == "podcast" {
			_ = m.db.QueryRowContext(ctx, "SELECT coverPath FROM podcasts WHERE id = ?", mediaID).Scan(&cp)
		}
		if cp.Valid {
			coverPath = cp.String
		}
	}

	if coverPath == "" {
		http.NotFound(w, r)
		return
	}

	if !utils.IsSafeFilePath(m.db, m.metadataPath, coverPath) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	http.ServeFile(w, r, coverPath)
}
