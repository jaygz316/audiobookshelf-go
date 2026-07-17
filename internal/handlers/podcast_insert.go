package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"audiobookshelf/internal/podcast"

	"github.com/google/uuid"
)

type PodcastDbModel struct {
	ID                   string
	Title                string
	Author               string
	FeedURL              string
	Description          string
	Language             string
	Explicit             bool
	AutoDownloadEpisodes bool
	Genres               []string
}

func dbInsertPodcast(ctx context.Context, db *sql.DB, p *PodcastDbModel, libraryItemID, libraryFolderID, libraryID, podcastPath string, episodes []*podcast.PodcastEpisode) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback()

	cols := getTableColumnsTx(tx, "podcasts")
	var colNames []string
	var placeholders []string
	var args []interface{}

	addCol := func(name string, val interface{}) {
		if cols[name] {
			colNames = append(colNames, name)
			placeholders = append(placeholders, "?")
			args = append(args, val)
		}
	}

	nowStr := time.Now().Format("2006-01-02T15:04:05.000Z")

	addCol("id", p.ID)
	addCol("title", p.Title)
	addCol("titleIgnorePrefix", p.Title)
	addCol("author", p.Author)
	addCol("feedURL", p.FeedURL)
	addCol("description", p.Description)
	addCol("language", p.Language)
	addCol("explicit", explicitInt(p.Explicit))
	addCol("autoDownloadEpisodes", boolToInt(p.AutoDownloadEpisodes))
	addCol("createdAt", nowStr)
	addCol("updatedAt", nowStr)
	genresJSON, _ := json.Marshal(p.Genres)
	addCol("genres", genresJSON)
	tagsJSON, _ := json.Marshal([]string{})
	addCol("tags", tagsJSON)

	query := fmt.Sprintf("INSERT INTO podcasts (%s) VALUES (%s)", strings.Join(colNames, ", "), strings.Join(placeholders, ", "))
	_, err = tx.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("failed to insert podcast: %w", err)
	}

	colsLi := getTableColumnsTx(tx, "libraryItems")
	colNames = nil
	placeholders = nil
	args = nil

	addColLi := func(name string, val interface{}) {
		if colsLi[name] {
			colNames = append(colNames, name)
			placeholders = append(placeholders, "?")
			args = append(args, val)
		}
	}

	addColLi("id", libraryItemID)
	addColLi("libraryId", libraryID)
	addColLi("libraryFolderId", libraryFolderID)
	addColLi("path", podcastPath)
	folderPath := filepath.Dir(podcastPath)
	relPath, _ := filepath.Rel(folderPath, podcastPath)
	addColLi("relPath", relPath)
	addColLi("isFile", 0)
	addColLi("createdAt", nowStr)
	addColLi("updatedAt", nowStr)
	addColLi("isMissing", 0)
	addColLi("isInvalid", 0)
	addColLi("mediaType", "podcast")
	addColLi("mediaId", p.ID)
	addColLi("title", p.Title)
	addColLi("titleIgnorePrefix", p.Title)

	queryLi := fmt.Sprintf("INSERT INTO libraryItems (%s) VALUES (%s)", strings.Join(colNames, ", "), strings.Join(placeholders, ", "))
	_, err = tx.ExecContext(ctx, queryLi, args...)
	if err != nil {
		return fmt.Errorf("failed to insert library item: %w", err)
	}

	if len(episodes) > 0 {
		for _, ep := range episodes {
			epID := uuid.New().String()
			colsEp := getTableColumnsTx(tx, "podcastEpisodes")
			colNames = nil
			placeholders = nil
			args = nil

			addColEp := func(name string, val interface{}) {
				if colsEp[name] {
					colNames = append(colNames, name)
					placeholders = append(placeholders, "?")
					args = append(args, val)
				}
			}

			addColEp("id", epID)
			addColEp("podcastId", p.ID)
			addColEp("title", ep.Title)
			addColEp("description", ep.Description)
			addColEp("enclosureURL", ep.EnclosureURL)
			addColEp("pubDate", ep.PublishedAt)
			addColEp("publishedAt", ep.PublishedAt)
			addColEp("createdAt", nowStr)
			addColEp("updatedAt", nowStr)

			audioFileJSON, _ := json.Marshal(map[string]interface{}{
				"duration": ep.Duration,
			})
			addColEp("audioFile", audioFileJSON)

			queryEp := fmt.Sprintf("INSERT INTO podcastEpisodes (%s) VALUES (%s)", strings.Join(colNames, ", "), strings.Join(placeholders, ", "))
			_, err = tx.ExecContext(ctx, queryEp, args...)
			if err != nil {
				return fmt.Errorf("failed to insert podcast episode: %w", err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	return nil
}
