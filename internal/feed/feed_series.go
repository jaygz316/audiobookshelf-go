package feed

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"
)

func (m *FeedManager) buildSeriesChannel(ctx context.Context, itemID string, hostPrefix string, feedBaseURL string, feedChannel *channel) error {
	var seriesName string
	var seriesDesc sql.NullString
	err := m.db.QueryRowContext(ctx, "SELECT name, description FROM series WHERE id = ?", itemID).Scan(&seriesName, &seriesDesc)
	if err != nil {
		return err
	}

	feedChannel.Title = seriesName
	if seriesDesc.Valid {
		feedChannel.Description = seriesDesc.String
		feedChannel.ITunesSummary = &cdata{Value: seriesDesc.String}
	}
	feedChannel.SiteURL = hostPrefix + "/series/" + itemID
	feedChannel.Image = &image{
		URL:   feedBaseURL + "/cover",
		Title: seriesName,
		Link:  feedChannel.SiteURL,
	}
	feedChannel.ITunesImage = &itunesImage{Href: feedBaseURL + "/cover"}

	// Query all books in the series
	rows, err := m.db.QueryContext(ctx, `
		SELECT b.id, b.title, b.description, b.explicit, b.audioFiles, b.chapters, li.createdAt, bs.sequence
		FROM bookSeries bs
		JOIN books b ON bs.bookId = b.id
		JOIN libraryItems li ON li.mediaId = b.id AND li.mediaType = 'book'
		WHERE bs.seriesId = ?
		ORDER BY CAST(bs.sequence AS REAL) ASC, bs.sequence ASC
	`, itemID)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var mediaItemID string
		var title, desc sql.NullString
		var explicit int
		var audioFilesJSON, chaptersJSON, liCreatedAtStr, sequence string
		if err := rows.Scan(&mediaItemID, &title, &desc, &explicit, &audioFilesJSON, &chaptersJSON, &liCreatedAtStr, &sequence); err == nil {
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
				itemID, // seriesId
				mediaItemID,
				false, // not direct book
				sequence,
			)
			feedChannel.Items = append(feedChannel.Items, bookItems...)
		}
	}

	return rows.Err()
}
