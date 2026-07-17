package podcast

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	log "audiobookshelf/internal/logger"
)

func (m *PodcastManager) syncPodcastEpisodes(ctx context.Context, p podcastInfo, feed *PodcastFeed) error {
	var libraryItemPath string
	var libraryItemID string
	err := m.db.QueryRowContext(ctx, "SELECT id, path FROM libraryItems WHERE mediaId = ? AND mediaType = 'podcast'", p.ID).Scan(&libraryItemID, &libraryItemPath)
	if err != nil {
		if err != sql.ErrNoRows {
			log.Printf("[Podcast] Failed to query library items for podcast %s: %v", p.ID, err)
		}
		libraryItemPath = ""
	}

	hasEnclosureURL := hasColumn(ctx, m.db, "podcastEpisodes", "enclosureURL")
	query := "SELECT title"
	if hasEnclosureURL {
		query += ", enclosureURL"
	}
	query += " FROM podcastEpisodes WHERE podcastId = ?"

	rows, err := m.db.QueryContext(ctx, query, p.ID)
	if err != nil {
		return fmt.Errorf("query existing episodes: %w", err)
	}
	defer rows.Close()

	existingEpisodes := make(map[string]bool)
	for rows.Next() {
		var title string
		var encURL sql.NullString
		if hasEnclosureURL {
			if err := rows.Scan(&title, &encURL); err != nil {
				return fmt.Errorf("scan existing episode details: %w", err)
			}
			if encURL.Valid && encURL.String != "" {
				existingEpisodes[encURL.String] = true
			}
		} else {
			if err := rows.Scan(&title); err != nil {
				return fmt.Errorf("scan existing episode title: %w", err)
			}
		}
		existingEpisodes[title] = true
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate existing episodes: %w", err)
	}

	newCount := 0
	for _, ep := range feed.Episodes {
		isNew := false
		if hasEnclosureURL && ep.EnclosureURL != "" {
			if !existingEpisodes[ep.EnclosureURL] && !existingEpisodes[ep.Title] {
				isNew = true
			}
		} else {
			if !existingEpisodes[ep.Title] {
				isNew = true
			}
		}

		if !isNew {
			continue
		}

		// Enforce max new episodes to download limit if configured
		if p.MaxNewEpisodesToDownload > 0 && newCount >= p.MaxNewEpisodesToDownload {
			break
		}

		downloadedPath := m.autoDownloadEpisode(ctx, p, ep, libraryItemPath)
		audioFileJSON := buildAudioFileJSON(downloadedPath, ep.Duration)

		epID := uuid.New().String()
		hasPubDate := hasColumn(ctx, m.db, "podcastEpisodes", "pubDate")
		hasDesc := hasColumn(ctx, m.db, "podcastEpisodes", "description")
		hasSeason := hasColumn(ctx, m.db, "podcastEpisodes", "season")
		hasEp := hasColumn(ctx, m.db, "podcastEpisodes", "episode")
		hasEpType := hasColumn(ctx, m.db, "podcastEpisodes", "episodeType")
		hasPublishedAt := hasColumn(ctx, m.db, "podcastEpisodes", "publishedAt")
		hasCreatedAt := hasColumn(ctx, m.db, "podcastEpisodes", "createdAt")
		hasUpdatedAt := hasColumn(ctx, m.db, "podcastEpisodes", "updatedAt")
		hasImageURL := hasColumn(ctx, m.db, "podcastEpisodes", "imageURL")

		cols := []string{"id", "podcastId", "title", "audioFile"}
		vals := []interface{}{epID, p.ID, ep.Title, audioFileJSON}

		if hasPubDate {
			cols = append(cols, "pubDate")
			vals = append(vals, ep.PublishedAt)
		}
		if hasDesc {
			cols = append(cols, "description")
			vals = append(vals, ep.Description)
		}
		if hasPublishedAt {
			cols = append(cols, "publishedAt")
			vals = append(vals, ep.PublishedAt)
		}
		if hasEnclosureURL {
			cols = append(cols, "enclosureURL")
			vals = append(vals, ep.EnclosureURL)
		}
		if hasSeason {
			cols = append(cols, "season")
			vals = append(vals, ep.Season)
		}
		if hasEp {
			cols = append(cols, "episode")
			vals = append(vals, ep.Episode)
		}
		if hasEpType {
			cols = append(cols, "episodeType")
			vals = append(vals, ep.EpisodeType)
		}
		if hasImageURL {
			cols = append(cols, "imageURL")
			vals = append(vals, ep.ImageURL)
		}
		if hasCreatedAt {
			cols = append(cols, "createdAt")
			vals = append(vals, time.Now().Format("2006-01-02 15:04:05.000"))
		}
		if hasUpdatedAt {
			cols = append(cols, "updatedAt")
			vals = append(vals, time.Now().Format("2006-01-02 15:04:05.000"))
		}

		placeholders := make([]string, len(cols))
		for i := range cols {
			placeholders[i] = "?"
		}

		insertSQL := fmt.Sprintf("INSERT INTO podcastEpisodes (%s) VALUES (%s)",
			strings.Join(cols, ", "),
			strings.Join(placeholders, ", "),
		)

		if _, err := m.db.ExecContext(ctx, insertSQL, vals...); err == nil {
			newCount++
		} else {
			log.Printf("[Podcast] Failed to insert episode %q: %v", ep.Title, err)
		}
	}

	if p.MaxEpisodesToKeep > 0 {
		if err := m.EnforceRetentionPolicy(ctx, p.ID, p.MaxEpisodesToKeep); err != nil {
			log.Printf("[Podcast] Failed to enforce retention policy: %v", err)
		}
	}

	return nil
}
