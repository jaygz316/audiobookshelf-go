package handlers

import (
	"database/sql"
	"encoding/json"

	log "audiobookshelf/internal/logger"
)

func batchUpdatePodcast(tx *sql.Tx, itemID string, mediaID string, mediaPayload BatchUpdateMediaPayload, prefixes []string, nowStr string) (bool, error) {
	var currentTitle, currentAuthor, currentDescription, currentLanguage, currentTagsRaw, currentGenresRaw string
	var currentExplicitVal int
	err := tx.QueryRow(`
		SELECT COALESCE(title, ''), COALESCE(author, ''), COALESCE(description, ''), COALESCE(language, ''), explicit, COALESCE(tags, '[]'), COALESCE(genres, '[]')
		FROM podcasts WHERE id = ?
	`, mediaID).Scan(&currentTitle, &currentAuthor, &currentDescription, &currentLanguage, &currentExplicitVal, &currentTagsRaw, &currentGenresRaw)
	if err != nil {
		log.Errorf("[Go] Batch edit: podcast media %s not found: %v", mediaID, err)
		return false, nil // skip on not found to preserve original logic (continue)
	}

	var currentTags, currentGenres []string
	_ = json.Unmarshal([]byte(currentTagsRaw), &currentTags)
	_ = json.Unmarshal([]byte(currentGenresRaw), &currentGenres)

	title := currentTitle
	if mediaPayload.Title != nil {
		title = *mediaPayload.Title
	}
	description := currentDescription
	if mediaPayload.Description != nil {
		description = *mediaPayload.Description
	}
	language := currentLanguage
	if mediaPayload.Language != nil {
		language = *mediaPayload.Language
	}
	explicit := currentExplicitVal != 0
	if mediaPayload.Explicit != nil {
		explicit = *mediaPayload.Explicit
	}
	tags := currentTags
	if mediaPayload.Tags != nil {
		tags = *mediaPayload.Tags
	}
	genres := currentGenres
	if mediaPayload.Genres != nil {
		genres = *mediaPayload.Genres
	}

	author := currentAuthor
	if mediaPayload.Authors != nil && len(*mediaPayload.Authors) > 0 {
		author = (*mediaPayload.Authors)[0]
	}

	titleIgnorePrefix := getTitleIgnorePrefixGo(title, prefixes)
	tagsJSON, _ := json.Marshal(tags)
	genresJSON, _ := json.Marshal(genres)

	_, err = tx.Exec(`
		UPDATE podcasts
		SET title = ?, titleIgnorePrefix = ?, author = ?, description = ?, language = ?, explicit = ?, tags = ?, genres = ?
		WHERE id = ?
	`, title, titleIgnorePrefix, author, description, language, boolToInt(explicit), tagsJSON, genresJSON, mediaID)
	if err != nil {
		return false, err
	}

	_, err = tx.Exec(`
		UPDATE libraryItems
		SET title = ?, titleIgnorePrefix = ?, authorNamesFirstLast = ?, authorNamesLastFirst = ?, updatedAt = ?
		WHERE id = ?
	`, title, titleIgnorePrefix, author, author, nowStr, itemID)
	if err != nil {
		return false, err
	}

	return true, nil
}
