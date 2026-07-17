package scanner

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	log "audiobookshelf/internal/logger"
)

func insertNewPodcast(tx *sql.Tx, mediaID, title, titleIgnorePrefix, itemPath string, meta *GroupMetadata, nowStr string) error {
	tagsJSON, _ := json.Marshal(meta.Tags)
	genresJSON, _ := json.Marshal(meta.Genres)
	var author string
	if len(meta.Authors) > 0 {
		author = meta.Authors[0]
	}

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

	addCol("id", mediaID)
	addCol("title", title)
	addCol("titleIgnorePrefix", titleIgnorePrefix)
	addCol("author", author)
	addCol("releaseDate", meta.PublishedDate)
	addCol("feedURL", "")
	addCol("imageURL", "")
	addCol("description", meta.Description)
	addCol("itunesPageURL", "")
	addCol("itunesId", "")
	addCol("itunesArtistId", "")
	addCol("language", meta.Language)
	addCol("podcastType", "")
	addCol("explicit", 0)
	addCol("autoDownloadEpisodes", 0)
	addCol("autoDownloadSchedule", "")
	addCol("lastEpisodeCheck", "")
	addCol("maxEpisodesToKeep", 0)
	addCol("maxNewEpisodesToDownload", 0)
	addCol("coverPath", meta.CoverPath)
	addCol("tags", tagsJSON)
	addCol("genres", genresJSON)
	addCol("numEpisodes", len(meta.AudioFiles))
	addCol("createdAt", nowStr)
	addCol("updatedAt", nowStr)

	query := fmt.Sprintf("INSERT INTO podcasts (%s) VALUES (%s)", strings.Join(colNames, ", "), strings.Join(placeholders, ", "))
	log.Printf("[Scanner] [%s] scanNewLibraryItem: Inserting into podcasts table", itemPath)
	_, err := tx.Exec(query, args...)
	if err != nil {
		return err
	}

	log.Printf("[Scanner] [%s] scanNewLibraryItem: Inserting podcast episodes", itemPath)
	for _, ep := range meta.PodcastEpisodes {
		audioFileJSON, _ := json.Marshal(ep.AudioFile)

		colsEp := getTableColumnsTx(tx, "podcastEpisodes")
		var colNamesEp []string
		var placeholdersEp []string
		var argsEp []interface{}

		addColEp := func(name string, val interface{}) {
			if colsEp[name] {
				colNamesEp = append(colNamesEp, name)
				placeholdersEp = append(placeholdersEp, "?")
				argsEp = append(argsEp, val)
			}
		}

		addColEp("id", ep.ID)
		addColEp("podcastId", mediaID)
		addColEp("title", ep.Title)
		addColEp("audioFile", string(audioFileJSON))
		addColEp("createdAt", nowStr)
		addColEp("updatedAt", nowStr)

		qEp := fmt.Sprintf("INSERT INTO podcastEpisodes (%s) VALUES (%s)", strings.Join(colNamesEp, ", "), strings.Join(placeholdersEp, ", "))
		_, err = tx.Exec(qEp, argsEp...)
		if err != nil {
			return err
		}
	}

	return nil
}
