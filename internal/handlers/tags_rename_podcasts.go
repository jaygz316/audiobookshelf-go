package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"strings"

	"audiobookshelf/internal/utils"
)

// renameTagInPodcasts updates the tags column in the podcasts table for the renamed tag.
func renameTagInPodcasts(tx *sql.Tx, tagVal, newTagVal string) error {
	rows2, err := tx.Query("SELECT id, tags FROM podcasts WHERE tags IS NOT NULL")
	if err != nil {
		log.Errorf("[Rename Tag] Query podcasts failed: %v", err)
		return err
	}
	defer rows2.Close()

	var podcastUpdateIds []string
	var podcastUpdateArgs []interface{}
	var podcastUpdateCases []string

	for rows2.Next() {
		var id string
		var tagsStr sql.NullString
		if err := rows2.Scan(&id, &tagsStr); err != nil {
			log.Errorf("[Rename Tag] Scan podcast failed: %v", err)
			return err
		}
		if updated, changed := utils.ReplaceInJSONArray(tagsStr, tagVal, newTagVal); changed {
			podcastUpdateIds = append(podcastUpdateIds, id)
			podcastUpdateArgs = append(podcastUpdateArgs, id, updated)
			podcastUpdateCases = append(podcastUpdateCases, "WHEN ? THEN ?")
		}
	}
	if err := rows2.Err(); err != nil {
		log.Errorf("[Rename Tag] Podcasts iteration failed: %v", err)
		return err
	}
	rows2.Close() // Explicitly close before exec

	if len(podcastUpdateIds) > 0 {
		chunkSize := 1000
		for i := 0; i < len(podcastUpdateIds); i += chunkSize {
			end := i + chunkSize
			if end > len(podcastUpdateIds) {
				end = len(podcastUpdateIds)
			}
			chunkIds := podcastUpdateIds[i:end]
			chunkArgs := podcastUpdateArgs[i*2 : end*2]
			chunkCases := podcastUpdateCases[i:end]

			query := "UPDATE podcasts SET tags = CASE id " + strings.Join(chunkCases, " ") + " END WHERE id IN (?" + strings.Repeat(",?", len(chunkIds)-1) + ")"
			args := append(chunkArgs, make([]interface{}, len(chunkIds))...)
			for j, id := range chunkIds {
				args[len(chunkArgs)+j] = id
			}
			_, err = tx.Exec(query, args...)
			if err != nil {
				log.Errorf("[Rename Tag] Update podcast failed: %v", err)
				return err
			}
		}
	}
	return nil
}
