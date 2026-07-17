package db

import (
	"database/sql"
)

var migrationV4 = migration{
	version:     4,
	description: "Ensure podcasts table has lockedFields column",
	run: func(db *sql.DB) error {
		var exists int
		err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='podcasts'").Scan(&exists)
		if err != nil {
			return err
		}
		if exists == 0 {
			return nil
		}
		rows, err := db.Query("PRAGMA table_info(podcasts)")
		if err != nil {
			return err
		}
		defer rows.Close()
		hasLockedFields := false
		for rows.Next() {
			var cid int
			var name, typeStr string
			var notnull int
			var dfltValue sql.NullString
			var pk int
			if err := rows.Scan(&cid, &name, &typeStr, &notnull, &dfltValue, &pk); err == nil {
				if name == "lockedFields" {
					hasLockedFields = true
				}
			}
		}
		if !hasLockedFields {
			if _, err := db.Exec("ALTER TABLE podcasts ADD COLUMN lockedFields BLOB"); err != nil {
				return err
			}
		}
		return nil
	},
}

var migrationV5 = migration{
	version:     5,
	description: "Ensure customMetadataProviders table exists",
	run: func(db *sql.DB) error {
		var exists int
		err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='customMetadataProviders'").Scan(&exists)
		if err != nil {
			return err
		}
		if exists == 0 {
			_, err = db.Exec("CREATE TABLE customMetadataProviders (id TEXT PRIMARY KEY, name TEXT, mediaType TEXT, url TEXT, authHeaderValue TEXT, extraData TEXT, createdAt INTEGER, updatedAt INTEGER)")
			return err
		}
		return nil
	},
}

var migrationV6 = migration{
	version:     6,
	description: "Ensure collections table has isSmart and rules columns",
	run: func(db *sql.DB) error {
		var exists int
		err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='collections'").Scan(&exists)
		if err != nil {
			return err
		}
		if exists == 0 {
			return nil
		}
		rows, err := db.Query("PRAGMA table_info(collections)")
		if err != nil {
			return err
		}
		defer rows.Close()
		hasIsSmart, hasRules := false, false
		for rows.Next() {
			var cid int
			var name, typeStr string
			var notnull int
			var dfltValue sql.NullString
			var pk int
			if err := rows.Scan(&cid, &name, &typeStr, &notnull, &dfltValue, &pk); err == nil {
				if name == "isSmart" {
					hasIsSmart = true
				}
				if name == "rules" {
					hasRules = true
				}
			}
		}
		if !hasIsSmart {
			if _, err := db.Exec("ALTER TABLE collections ADD COLUMN isSmart INTEGER DEFAULT 0"); err != nil {
				return err
			}
		}
		if !hasRules {
			if _, err := db.Exec("ALTER TABLE collections ADD COLUMN rules TEXT"); err != nil {
				return err
			}
		}
		return nil
	},
}
