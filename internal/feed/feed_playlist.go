package feed

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"
)

func (m *FeedManager) buildPlaylistChannel(ctx context.Context, itemID string, hostPrefix string, feedBaseURL string, feedChannel *channel) error {
	var playlistName string
	var playlistDesc sql.NullString
	err := m.db.QueryRowContext(ctx, "SELECT name, description FROM playlists WHERE id = ?", itemID).Scan(&playlistName, &playlistDesc)
	if err != nil {
		return err
	}

	feedChannel.Title = playlistName
	if playlistDesc.Valid {
		feedChannel.Description = playlistDesc.String
		feedChannel.ITunesSummary = &cdata{Value: playlistDesc.String}
	}
	feedChannel.SiteURL = hostPrefix + "/playlist/" + itemID
	feedChannel.Image = &image{
		URL:   feedBaseURL + "/cover",
		Title: playlistName,
		Link:  feedChannel.SiteURL,
	}
	feedChannel.ITunesImage = &itunesImage{Href: feedBaseURL + "/cover"}

	rows, err := m.db.QueryContext(ctx, `
		SELECT mediaItemId, mediaItemType 
		FROM playlistMediaItems 
		WHERE playlistId = ? 
		ORDER BY "order" ASC
	`, itemID)
	if err != nil {
		return err
	}

	type playlistItem struct {
		mediaItemID   string
		mediaItemType string
	}
	var items []playlistItem
	var bookIDs []interface{}
	var epIDs []interface{}

	defer rows.Close()
	for rows.Next() {
		var it playlistItem
		if err := rows.Scan(&it.mediaItemID, &it.mediaItemType); err == nil {
			items = append(items, it)
			if it.mediaItemType == "book" {
				bookIDs = append(bookIDs, it.mediaItemID)
			} else if it.mediaItemType == "podcastEpisode" {
				epIDs = append(epIDs, it.mediaItemID)
			}
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	type bookData struct {
		title       sql.NullString
		desc        sql.NullString
		explicit    int
		audioFiles  string
		chapters    string
		liCreatedAt string
	}
	bookMap := make(map[string]bookData)

	if len(bookIDs) > 0 {
		query := `
			SELECT b.id, b.title, b.description, b.explicit, b.audioFiles, b.chapters, li.createdAt
			FROM books b
			JOIN libraryItems li ON li.mediaId = b.id AND li.mediaType = 'book'
			WHERE b.id IN (?` + strings.Repeat(",?", len(bookIDs)-1) + `)
		`
		bookRows, err := m.db.QueryContext(ctx, query, bookIDs...)
		if err == nil {
			defer bookRows.Close()
			for bookRows.Next() {
				var id string
				var bd bookData
				if err := bookRows.Scan(&id, &bd.title, &bd.desc, &bd.explicit, &bd.audioFiles, &bd.chapters, &bd.liCreatedAt); err == nil {
					bookMap[id] = bd
				}
			}
		}
	}

	type epPodcastData struct {
		ep       *podcastEpData
		author   sql.NullString
		explicit int
	}
	epMap := make(map[string]epPodcastData)

	if len(epIDs) > 0 {
		for _, epID := range epIDs {
			idStr := epID.(string)
			ep, err := queryPodcastEpisode(ctx, m.db, idStr)
			if err == nil {
				epMap[idStr] = epPodcastData{ep: ep}
			}
		}

		query := `
			SELECT pe.id, p.author, p.explicit
			FROM podcasts p
			JOIN podcastEpisodes pe ON pe.podcastId = p.id
			WHERE pe.id IN (?` + strings.Repeat(",?", len(epIDs)-1) + `)
		`
		pRows, err := m.db.QueryContext(ctx, query, epIDs...)
		if err == nil {
			defer pRows.Close()
			for pRows.Next() {
				var epID string
				var author sql.NullString
				var explicit int
				if err := pRows.Scan(&epID, &author, &explicit); err == nil {
					if data, ok := epMap[epID]; ok {
						data.author = author
						data.explicit = explicit
						epMap[epID] = data
					}
				}
			}
		}
	}

	for _, it := range items {
		mediaItemID := it.mediaItemID
		mediaItemType := it.mediaItemType

		if mediaItemType == "podcastEpisode" {
			if epData, ok := epMap[mediaItemID]; ok {
				ep := epData.ep
				itemVal := buildPodcastEpisodeItem(ep, epData.author.String, epData.explicit, hostPrefix, feedBaseURL)
				feedChannel.Items = append(feedChannel.Items, itemVal)
			}
		} else if mediaItemType == "book" {
			if bd, ok := bookMap[mediaItemID]; ok {
				var tracks []audiobookTrack
				_ = json.Unmarshal([]byte(bd.audioFiles), &tracks)
				var chapters []audiobookChapter
				_ = json.Unmarshal([]byte(bd.chapters), &chapters)

				liCreatedAt, _ := time.Parse(time.RFC3339, bd.liCreatedAt)
				if liCreatedAt.IsZero() {
					liCreatedAt = time.Now()
				}

				bookItems := buildBookItems(
					tracks,
					chapters,
					liCreatedAt,
					bd.title.String,
					bd.desc.String,
					bd.explicit,
					hostPrefix,
					feedBaseURL,
					itemID, // playlistId
					mediaItemID,
					false, // not direct book
					"",    // no sequence
				)
				feedChannel.Items = append(feedChannel.Items, bookItems...)
			}
		}
	}

	return nil
}
