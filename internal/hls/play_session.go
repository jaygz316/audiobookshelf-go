package hls

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/google/uuid"
)

func resolveMediaItemAndCreateSession(ctx context.Context, db *sql.DB, itemID string, episodeID string, userID string, startTime float64) (sessionID string, mediaItemID string, mediaItemType string, resolvedLibraryID sql.NullString, err error) {
	mediaItemID = itemID
	mediaItemType = "book"

	// Check if itemID exists in libraryItems
	var liMediaID, liMediaType, liLibraryID string
	err = db.QueryRowContext(ctx, "SELECT mediaId, mediaType, libraryId FROM libraryItems WHERE id = ?", itemID).Scan(&liMediaID, &liMediaType, &liLibraryID)
	if err == nil {
		resolvedLibraryID.Valid = true
		resolvedLibraryID.String = liLibraryID
		if liMediaType == "book" {
			mediaItemID = liMediaID
			mediaItemType = "book"
		} else if liMediaType == "podcast" {
			if episodeID != "" {
				mediaItemID = episodeID
				mediaItemType = "podcastEpisode"
			} else {
				// If a podcast, get the first episode
				var epID string
				errEp := db.QueryRowContext(ctx, "SELECT id FROM podcastEpisodes WHERE podcastId = ? LIMIT 1", liMediaID).Scan(&epID)
				if errEp == nil {
					mediaItemID = epID
					mediaItemType = "podcastEpisode"
				} else {
					mediaItemID = liMediaID
					mediaItemType = "podcast"
				}
			}
		}
	} else {
		// Not in libraryItems directly. Check if it's a book ID in books
		var bookExists int
		errBook := db.QueryRowContext(ctx, "SELECT 1 FROM books WHERE id = ?", itemID).Scan(&bookExists)
		if errBook == nil && bookExists == 1 {
			mediaItemID = itemID
			mediaItemType = "book"
			_ = db.QueryRowContext(ctx, "SELECT libraryId FROM libraryItems WHERE mediaId = ? AND mediaType = 'book'", itemID).Scan(&resolvedLibraryID)
		} else {
			// Check if it's a podcastEpisode ID in podcastEpisodes
			var podcastID string
			errEp := db.QueryRowContext(ctx, "SELECT podcastId FROM podcastEpisodes WHERE id = ?", itemID).Scan(&podcastID)
			if errEp == nil {
				mediaItemID = itemID
				mediaItemType = "podcastEpisode"
				_ = db.QueryRowContext(ctx, "SELECT libraryId FROM libraryItems WHERE mediaId = ? AND mediaType = 'podcast'", podcastID).Scan(&resolvedLibraryID)
			}
		}
	}

	sessionID = uuid.New().String()
	_, _ = db.ExecContext(ctx, "DELETE FROM playbackSessions WHERE userId = ? AND mediaItemId = ?", userID, mediaItemID)

	extraData := fmt.Sprintf(`{"libraryItemId": %q}`, itemID)
	query := `INSERT INTO playbackSessions (id, userId, mediaItemId, mediaItemType, startTime, libraryId, extraData, createdAt, updatedAt) VALUES (?, ?, ?, ?, ?, ?, ?, datetime('now'), datetime('now'))`
	_, err = db.ExecContext(ctx, query, sessionID, userID, mediaItemID, mediaItemType, startTime, resolvedLibraryID, extraData)
	if err != nil {
		return "", "", "", resolvedLibraryID, fmt.Errorf("failed to insert session: %w", err)
	}

	return sessionID, mediaItemID, mediaItemType, resolvedLibraryID, nil
}
