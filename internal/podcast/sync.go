package podcast

import (
	"context"
	"database/sql"
	"fmt"

	log "audiobookshelf/internal/logger"
)

// podcastInfo holds SQLite metadata retrieved for a podcast during sync.
type podcastInfo struct {
	ID                       string
	Title                    string
	FeedURL                  string
	AutoDownload             int
	MaxEpisodesToKeep        int
	MaxNewEpisodesToDownload int
	AutoDeletePlayed         int
}

// SyncAllFeeds queries all podcasts from database and checks them for updates.
func (m *PodcastManager) SyncAllFeeds(ctx context.Context) error {
	rows, err := m.db.QueryContext(ctx, "SELECT id, title, feedURL, autoDownloadEpisodes, maxEpisodesToKeep, maxNewEpisodesToDownload, autoDeletePlayed FROM podcasts")
	if err != nil {
		return fmt.Errorf("query podcasts: %w", err)
	}
	defer rows.Close()

	var podcasts []podcastInfo
	for rows.Next() {
		var p podcastInfo
		var feedURL sql.NullString
		var autoDownload sql.NullInt64
		var maxKeep sql.NullInt64
		var maxDownload sql.NullInt64
		var autoDelete sql.NullInt64
		if err := rows.Scan(&p.ID, &p.Title, &feedURL, &autoDownload, &maxKeep, &maxDownload, &autoDelete); err != nil {
			return fmt.Errorf("scan podcast: %w", err)
		}
		p.FeedURL = feedURL.String
		p.AutoDownload = int(autoDownload.Int64)
		p.MaxEpisodesToKeep = int(maxKeep.Int64)
		p.MaxNewEpisodesToDownload = int(maxDownload.Int64)
		p.AutoDeletePlayed = int(autoDelete.Int64)
		podcasts = append(podcasts, p)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("rows error: %w", err)
	}

	for _, p := range podcasts {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if p.FeedURL == "" {
			continue
		}

		func() {
			lk := m.getLock(p.ID)
			lk.Lock()
			defer lk.Unlock()

			feed, err := m.FetchFeed(ctx, p.FeedURL)
			if err != nil {
				log.Printf("[Podcast] Failed to fetch feed for %q (%s): %v", p.Title, p.FeedURL, err)
				return
			}

			if err := m.syncPodcastEpisodes(ctx, p, feed); err != nil {
				log.Printf("[Podcast] Failed to sync episodes for %q: %v", p.Title, err)
				return
			}
		}()
	}

	return nil
}

// SyncFeed syncs the feed and downloads episodes for a single podcast by its ID.
func (m *PodcastManager) SyncFeed(ctx context.Context, podcastID string) error {
	lk := m.getLock(podcastID)
	lk.Lock()
	defer lk.Unlock()

	var p podcastInfo
	var feedURL sql.NullString
	var autoDownload sql.NullInt64
	var maxKeep sql.NullInt64
	var maxDownload sql.NullInt64
	var autoDelete sql.NullInt64

	err := m.db.QueryRowContext(ctx, `
		SELECT id, title, feedURL, autoDownloadEpisodes, maxEpisodesToKeep, maxNewEpisodesToDownload, autoDeletePlayed
		FROM podcasts
		WHERE id = ?
	`, podcastID).Scan(&p.ID, &p.Title, &feedURL, &autoDownload, &maxKeep, &maxDownload, &autoDelete)
	if err != nil {
		return err
	}

	p.FeedURL = feedURL.String
	p.AutoDownload = int(autoDownload.Int64)
	p.MaxEpisodesToKeep = int(maxKeep.Int64)
	p.MaxNewEpisodesToDownload = int(maxDownload.Int64)
	p.AutoDeletePlayed = int(autoDelete.Int64)

	if p.FeedURL == "" {
		return fmt.Errorf("no feed URL configured")
	}

	feed, err := m.FetchFeed(ctx, p.FeedURL)
	if err != nil {
		return err
	}

	return m.syncPodcastEpisodes(ctx, p, feed)
}
