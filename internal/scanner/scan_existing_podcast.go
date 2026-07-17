package scanner

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	log "audiobookshelf/internal/logger"
)

func applyLockedPodcastFields(tx *sql.Tx, mediaID string, meta *GroupMetadata, prefixes []string) (string, string, string, error) {
	var pLockedFields []byte
	var dbTitle, dbAuthor, dbDescription, dbLanguage, dbCoverPath sql.NullString
	var dbTags, dbGenres []byte

	_ = tx.QueryRow(`
		SELECT title, author, description, language, coverPath, tags, genres, lockedFields
		FROM podcasts WHERE id = ?
	`, mediaID).Scan(
		&dbTitle, &dbAuthor, &dbDescription, &dbLanguage, &dbCoverPath, &dbTags, &dbGenres, &pLockedFields,
	)

	var lockedFields []string
	if len(pLockedFields) > 0 {
		_ = json.Unmarshal(pLockedFields, &lockedFields)
	}

	isLocked := func(field string) bool {
		for _, f := range lockedFields {
			if f == field {
				return true
			}
		}
		return false
	}

	var title, titleIgnorePrefix, author string
	if isLocked("title") && dbTitle.String != "" {
		title = dbTitle.String
		titleIgnorePrefix = getTitleIgnorePrefixGo(title, prefixes)
	}
	var defaultAuthor string
	if len(meta.Authors) > 0 {
		defaultAuthor = meta.Authors[0]
	}
	author = defaultAuthor
	setIfLocked(isLocked("author") || isLocked("authors"), &author, dbAuthor)
	setIfLocked(isLocked("description"), &meta.Description, dbDescription)
	setIfLocked(isLocked("language"), &meta.Language, dbLanguage)
	setIfLocked(isLocked("cover") || isLocked("coverPath"), &meta.CoverPath, dbCoverPath)
	unmarshalIfLocked(isLocked("tags"), &meta.Tags, dbTags)
	unmarshalIfLocked(isLocked("genres"), &meta.Genres, dbGenres)

	return title, titleIgnorePrefix, author, nil
}

func updateExistingPodcast(tx *sql.Tx, mediaID, title, titleIgnorePrefix, itemPath string, meta *GroupMetadata, nowStr string) error {
	var author string
	prefixes := getSortingPrefixesTx(tx)
	lockedTitle, lockedTitleIgnorePrefix, lockedAuthor, err := applyLockedPodcastFields(tx, mediaID, meta, prefixes)
	if err == nil {
		if lockedTitle != "" {
			title = lockedTitle
			titleIgnorePrefix = lockedTitleIgnorePrefix
		}
		author = lockedAuthor
	} else {
		if len(meta.Authors) > 0 {
			author = meta.Authors[0]
		}
	}

	tagsJSON, _ := json.Marshal(meta.Tags)
	genresJSON, _ := json.Marshal(meta.Genres)

	cols := getTableColumnsTx(tx, "podcasts")
	var setStmts []string
	var args []interface{}

	addCol := func(name string, val interface{}) {
		if cols[name] {
			setStmts = append(setStmts, fmt.Sprintf("%s = ?", name))
			args = append(args, val)
		}
	}

	addCol("title", title)
	addCol("titleIgnorePrefix", titleIgnorePrefix)
	addCol("author", author)
	addCol("releaseDate", meta.PublishedDate)
	addCol("description", meta.Description)
	addCol("language", meta.Language)
	addCol("coverPath", meta.CoverPath)
	addCol("tags", tagsJSON)
	addCol("genres", genresJSON)
	addCol("numEpisodes", len(meta.AudioFiles))
	addCol("updatedAt", nowStr)

	args = append(args, mediaID)
	query := fmt.Sprintf("UPDATE podcasts SET %s WHERE id = ?", strings.Join(setStmts, ", "))
	log.Printf("[Scanner] [%s] scanExistingLibraryItem: Updating podcasts table", itemPath)
	_, err = tx.Exec(query, args...)
	if err != nil {
		return err
	}

	log.Printf("[Scanner] [%s] scanExistingLibraryItem: Updating podcast episodes", itemPath)
	if tableExistsTx(tx, "podcastEpisodes") {
		_, _ = tx.Exec("DELETE FROM podcastEpisodes WHERE podcastId = ?", mediaID)
	}
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
