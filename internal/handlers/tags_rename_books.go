package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"strings"

	"audiobookshelf/internal/utils"
)

// renameTagInBooks updates the tags column in the books table for the renamed tag.
func renameTagInBooks(tx *sql.Tx, tagVal, newTagVal string) error {
	rows, err := tx.Query("SELECT id, tags FROM books WHERE tags IS NOT NULL")
	if err != nil {
		log.Errorf("[Rename Tag] Query books failed: %v", err)
		return err
	}
	defer rows.Close()

	var bookUpdateIds []string
	var bookUpdateArgs []interface{}
	var bookUpdateCases []string

	for rows.Next() {
		var id string
		var tagsStr sql.NullString
		if err := rows.Scan(&id, &tagsStr); err != nil {
			log.Errorf("[Rename Tag] Scan book failed: %v", err)
			return err
		}
		if updated, changed := utils.ReplaceInJSONArray(tagsStr, tagVal, newTagVal); changed {
			bookUpdateIds = append(bookUpdateIds, id)
			bookUpdateArgs = append(bookUpdateArgs, id, updated)
			bookUpdateCases = append(bookUpdateCases, "WHEN ? THEN ?")
		}
	}
	if err := rows.Err(); err != nil {
		log.Errorf("[Rename Tag] Books iteration failed: %v", err)
		return err
	}
	rows.Close() // Explicitly close before exec

	if len(bookUpdateIds) > 0 {
		chunkSize := 1000
		for i := 0; i < len(bookUpdateIds); i += chunkSize {
			end := i + chunkSize
			if end > len(bookUpdateIds) {
				end = len(bookUpdateIds)
			}
			chunkIds := bookUpdateIds[i:end]
			chunkArgs := bookUpdateArgs[i*2 : end*2]
			chunkCases := bookUpdateCases[i:end]

			query := "UPDATE books SET tags = CASE id " + strings.Join(chunkCases, " ") + " END WHERE id IN (?" + strings.Repeat(",?", len(chunkIds)-1) + ")"
			args := append(chunkArgs, make([]interface{}, len(chunkIds))...)
			for j, id := range chunkIds {
				args[len(chunkArgs)+j] = id
			}
			_, err = tx.Exec(query, args...)
			if err != nil {
				log.Errorf("[Rename Tag] Update book failed: %v", err)
				return err
			}
		}
	}
	return nil
}
