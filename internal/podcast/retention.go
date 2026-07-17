package podcast

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	log "audiobookshelf/internal/logger"
)

// EnforceRetentionPolicy deletes the oldest downloaded episode files for a podcast if the number of downloaded episodes exceeds maxKeep.
func (m *PodcastManager) EnforceRetentionPolicy(ctx context.Context, podcastID string, maxKeep int) error {
	lk := m.getLock(podcastID)
	lk.Lock()
	defer lk.Unlock()

	if maxKeep <= 0 {
		return nil
	}

	// Fetch all downloaded episodes for this podcast, ordered by pubDate/publishedAt/createdAt descending
	// We want to keep the newest ones, and delete the oldest ones.
	type episodeDownload struct {
		ID        string
		Title     string
		AudioFile string
	}

	rows, err := m.db.QueryContext(ctx, `
		SELECT id, title, audioFile
		FROM podcastEpisodes
		WHERE podcastId = ? AND audioFile IS NOT NULL AND audioFile != '{}' AND audioFile != ''
		ORDER BY COALESCE(pubDate, publishedAt, createdAt, '') DESC, COALESCE(publishedAt, '') DESC, COALESCE(createdAt, '') DESC
	`, podcastID)
	if err != nil {
		return fmt.Errorf("query downloaded episodes for retention: %w", err)
	}
	defer rows.Close()

	var downloads []episodeDownload
	for rows.Next() {
		var ed episodeDownload
		if err := rows.Scan(&ed.ID, &ed.Title, &ed.AudioFile); err != nil {
			return fmt.Errorf("scan episode download: %w", err)
		}

		// Verify it actually has a file path in metadata
		var af struct {
			Metadata struct {
				Path string `json:"path"`
			} `json:"metadata"`
		}
		if err := json.Unmarshal([]byte(ed.AudioFile), &af); err == nil && af.Metadata.Path != "" {
			downloads = append(downloads, ed)
		}
	}

	if len(downloads) <= maxKeep {
		return nil
	}

	log.Printf("[Podcast] Retention policy: %d downloaded episodes found, max is %d. Deleting oldest %d episodes.", len(downloads), maxKeep, len(downloads)-maxKeep)

	// Delete from index maxKeep onwards
	for i := maxKeep; i < len(downloads); i++ {
		ed := downloads[i]
		var af struct {
			Metadata struct {
				Path string `json:"path"`
			} `json:"metadata"`
		}
		_ = json.Unmarshal([]byte(ed.AudioFile), &af)
		filePath := af.Metadata.Path

		if filePath != "" {
			if _, err := os.Stat(filePath); err == nil {
				log.Printf("[Podcast] Retention policy deleting: %s", filePath)
				if err := os.Remove(filePath); err != nil {
					log.Printf("[Podcast] Failed to delete file %s: %v", filePath, err)
				}
			}
		}

		// Update DB
		_, err = m.db.ExecContext(ctx, "UPDATE podcastEpisodes SET audioFile = '{}' WHERE id = ?", ed.ID)
		if err != nil {
			log.Printf("[Podcast] Failed to clear audioFile in DB for episode %s: %v", ed.ID, err)
		}
	}

	return nil
}
