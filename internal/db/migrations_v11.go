package db

import (
	"database/sql"
	"fmt"
)

var migrationV11 = migration{
	version:     11,
	description: "Rename entityType to type in feeds table if it exists",
	run: func(db *sql.DB) error {
		var exists int
		err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='feeds'").Scan(&exists)
		if err != nil {
			return err
		}
		if exists == 0 {
			return nil
		}

		rows, err := db.Query("PRAGMA table_info(feeds)")
		if err != nil {
			return err
		}
		defer func() {
			_ = rows.Close()
		}()

		hasEntityType := false
		hasType := false
		for rows.Next() {
			var cid int
			var name, typeStr string
			var notnull int
			var dfltValue sql.NullString
			var pk int
			if err := rows.Scan(&cid, &name, &typeStr, &notnull, &dfltValue, &pk); err == nil {
				if name == "entityType" {
					hasEntityType = true
				}
				if name == "type" {
					hasType = true
				}
			}
		}

		if hasEntityType && !hasType {
			if _, err := db.Exec("ALTER TABLE feeds RENAME COLUMN entityType TO type"); err != nil {
				return fmt.Errorf("failed to rename entityType to type in feeds table: %w", err)
			}
		}
		return nil
	},
}
