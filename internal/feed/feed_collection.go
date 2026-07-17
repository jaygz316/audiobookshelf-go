package feed

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"
)

func (m *FeedManager) buildCollectionChannel(ctx context.Context, itemID string, hostPrefix string, feedBaseURL string, feedChannel *channel) error {
	var collName string
	var collDesc sql.NullString
	err := m.db.QueryRowContext(ctx, "SELECT name, description FROM collections WHERE id = ?", itemID).Scan(&collName, &collDesc)
	if err != nil {
		return err
	}

	feedChannel.Title = collName
	if collDesc.Valid {
		feedChannel.Description = collDesc.String
		feedChannel.ITunesSummary = &cdata{Value: collDesc.String}
	}
	feedChannel.SiteURL = hostPrefix + "/collection/" + itemID
	feedChannel.Image = &image{
		URL:   feedBaseURL + "/cover",
		Title: collName,
		Link:  feedChannel.SiteURL,
	}
	feedChannel.ITunesImage = &itunesImage{Href: feedBaseURL + "/cover"}

	// Query all books in the collection
	rows, err := m.db.QueryContext(ctx, `
		SELECT b.id, b.title, b.description, b.explicit, b.audioFiles, b.chapters, li.createdAt
		FROM collectionBooks cb
		JOIN books b ON cb.bookId = b.id
		JOIN libraryItems li ON li.mediaId = b.id AND li.mediaType = 'book'
		WHERE cb.collectionId = ?
		ORDER BY cb."order" ASC
	`, itemID)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var mediaItemID string
		var title, desc sql.NullString
		var explicit int
		var audioFilesJSON, chaptersJSON, liCreatedAtStr string
		if err := rows.Scan(&mediaItemID, &title, &desc, &explicit, &audioFilesJSON, &chaptersJSON, &liCreatedAtStr); err == nil {
			var tracks []audiobookTrack
			_ = json.Unmarshal([]byte(audioFilesJSON), &tracks)
			var chapters []audiobookChapter
			_ = json.Unmarshal([]byte(chaptersJSON), &chapters)

			liCreatedAt, _ := time.Parse(time.RFC3339, liCreatedAtStr)
			if liCreatedAt.IsZero() {
				liCreatedAt = time.Now()
			}

			bookItems := buildBookItems(
				tracks,
				chapters,
				liCreatedAt,
				title.String,
				desc.String,
				explicit,
				hostPrefix,
				feedBaseURL,
				itemID, // collectionId
				mediaItemID,
				false, // not direct book
				"",    // no sequence
			)
			feedChannel.Items = append(feedChannel.Items, bookItems...)
		}
	}

	return rows.Err()
}
