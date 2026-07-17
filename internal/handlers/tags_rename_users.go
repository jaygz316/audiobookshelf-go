package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"encoding/json"
	"strings"
)

// renameTagInUsers updates the itemTagsSelected permission for users when a tag is renamed.
func renameTagInUsers(tx *sql.Tx, tagVal, newTagVal string) error {
	rows3, err := tx.Query("SELECT id, permissions FROM users WHERE permissions IS NOT NULL")
	if err != nil {
		log.Errorf("[Rename Tag] Query users failed: %v", err)
		return err
	}
	defer rows3.Close()

	var userUpdateIds []string
	var userUpdateArgs []interface{}
	var userUpdateCases []string

	for rows3.Next() {
		var id string
		var permsStr sql.NullString
		if err := rows3.Scan(&id, &permsStr); err != nil {
			log.Errorf("[Rename Tag] Scan user failed: %v", err)
			return err
		}
		if permsStr.Valid && permsStr.String != "" {
			var perms map[string]interface{}
			if json.Unmarshal([]byte(permsStr.String), &perms) == nil {
				if tagsSel, ok := perms["itemTagsSelected"].([]interface{}); ok {
					changed := false
					newTagsSel := []interface{}{}
					for _, t := range tagsSel {
						if tStr, ok := t.(string); ok && tStr == tagVal {
							// Rename
							alreadyHasNew := false
							for _, existT := range tagsSel {
								if existTStr, ok := existT.(string); ok && existTStr == newTagVal {
									alreadyHasNew = true
									break
								}
							}
							if !alreadyHasNew {
								newTagsSel = append(newTagsSel, newTagVal)
							}
							changed = true
						} else {
							newTagsSel = append(newTagsSel, t)
						}
					}
					if changed {
						perms["itemTagsSelected"] = newTagsSel
						newPermsBytes, _ := json.Marshal(perms)
						userUpdateIds = append(userUpdateIds, id)
						userUpdateArgs = append(userUpdateArgs, id, string(newPermsBytes))
						userUpdateCases = append(userUpdateCases, "WHEN ? THEN ?")
					}
				}
			}
		}
	}
	if err := rows3.Err(); err != nil {
		log.Errorf("[Rename Tag] Users iteration failed: %v", err)
		return err
	}
	rows3.Close() // Explicitly close before exec

	if len(userUpdateIds) > 0 {
		chunkSize := 1000
		for i := 0; i < len(userUpdateIds); i += chunkSize {
			end := i + chunkSize
			if end > len(userUpdateIds) {
				end = len(userUpdateIds)
			}
			chunkIds := userUpdateIds[i:end]
			chunkArgs := userUpdateArgs[i*2 : end*2]
			chunkCases := userUpdateCases[i:end]

			query := "UPDATE users SET permissions = CASE id " + strings.Join(chunkCases, " ") + " END WHERE id IN (?" + strings.Repeat(",?", len(chunkIds)-1) + ")"
			args := append(chunkArgs, make([]interface{}, len(chunkIds))...)
			for j, id := range chunkIds {
				args[len(chunkArgs)+j] = id
			}
			_, err = tx.Exec(query, args...)
			if err != nil {
				log.Errorf("[Rename Tag] Update user failed: %v", err)
				return err
			}
		}
	}
	return nil
}
