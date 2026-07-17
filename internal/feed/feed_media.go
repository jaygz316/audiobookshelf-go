package feed

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"
)

func (m *FeedManager) buildLibraryItemChannel(ctx context.Context, itemID string, hostPrefix string, feedBaseURL string, feedChannel *channel) error {
	var mediaType, mediaID, liCreatedAtStr string
	err := m.db.QueryRowContext(ctx, "SELECT mediaType, mediaId, createdAt FROM libraryItems WHERE id = ?", itemID).Scan(&mediaType, &mediaID, &liCreatedAtStr)
	if err != nil {
		return err
	}

	liCreatedAt, _ := time.Parse(time.RFC3339, liCreatedAtStr)
	if liCreatedAt.IsZero() {
		liCreatedAt = time.Now()
	}

	if mediaType == "podcast" {
		var title, author, description, language, podcastType sql.NullString
		var explicit int
		err := m.db.QueryRowContext(ctx, `
			SELECT title, author, description, language, podcastType, explicit 
			FROM podcasts WHERE id = ?
		`, mediaID).Scan(&title, &author, &description, &language, &podcastType, &explicit)
		if err != nil {
			return err
		}

		feedChannel.Title = title.String
		feedChannel.Description = description.String
		if description.Valid && description.String != "" {
			feedChannel.ITunesSummary = &cdata{Value: description.String}
		}
		if language.Valid && language.String != "" {
			feedChannel.Language = language.String
		}
		if podcastType.Valid && podcastType.String != "" {
			feedChannel.ITunesType = podcastType.String
		}
		feedChannel.SiteURL = hostPrefix + "/item/" + itemID
		feedChannel.Image = &image{
			URL:   feedBaseURL + "/cover",
			Title: title.String,
			Link:  feedChannel.SiteURL,
		}
		feedChannel.ITunesImage = &itunesImage{Href: feedBaseURL + "/cover"}
		feedChannel.ITunesAuthor = author.String
		if explicit != 0 {
			feedChannel.ITunesExplicit = "yes"
		} else {
			feedChannel.ITunesExplicit = "no"
		}

		eps, err := queryPodcastEpisodes(ctx, m.db, mediaID)
		if err == nil {
			sortPodcastEpisodes(eps, feedChannel.ITunesType == "episodic")

			for _, ep := range eps {
				itemVal := buildPodcastEpisodeItem(ep, author.String, explicit, hostPrefix, feedBaseURL)
				feedChannel.Items = append(feedChannel.Items, itemVal)
			}
		}
	} else if mediaType == "book" {
		var title, description, language sql.NullString
		var explicit int
		var audioFilesJSON, chaptersJSON string
		err := m.db.QueryRowContext(ctx, `
			SELECT title, description, language, explicit, audioFiles, chapters 
			FROM books WHERE id = ?
		`, mediaID).Scan(&title, &description, &language, &explicit, &audioFilesJSON, &chaptersJSON)
		if err != nil {
			return err
		}

		var authorName sql.NullString
		_ = m.db.QueryRowContext(ctx, "SELECT authorNamesFirstLast FROM libraryItems WHERE id = ?", itemID).Scan(&authorName)

		feedChannel.Title = title.String
		feedChannel.Description = description.String
		if description.Valid && description.String != "" {
			feedChannel.ITunesSummary = &cdata{Value: description.String}
		}
		if language.Valid && language.String != "" {
			feedChannel.Language = language.String
		}
		feedChannel.SiteURL = hostPrefix + "/item/" + itemID
		feedChannel.Image = &image{
			URL:   feedBaseURL + "/cover",
			Title: title.String,
			Link:  feedChannel.SiteURL,
		}
		feedChannel.ITunesImage = &itunesImage{Href: feedBaseURL + "/cover"}
		feedChannel.ITunesAuthor = authorName.String
		if explicit != 0 {
			feedChannel.ITunesExplicit = "yes"
		} else {
			feedChannel.ITunesExplicit = "no"
		}

		var tracks []audiobookTrack
		_ = json.Unmarshal([]byte(audioFilesJSON), &tracks)
		var chapters []audiobookChapter
		_ = json.Unmarshal([]byte(chaptersJSON), &chapters)

		bookItems := buildBookItems(
			tracks,
			chapters,
			liCreatedAt,
			title.String,
			description.String,
			explicit,
			hostPrefix,
			feedBaseURL,
			itemID, // book's libraryItem ID
			mediaID,
			true, // direct book
			"",   // no sequence
		)
		feedChannel.Items = append(feedChannel.Items, bookItems...)
	}
	return nil
}
