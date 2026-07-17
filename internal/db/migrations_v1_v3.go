package db

import (
	"database/sql"
)

var migrationV1 = migration{
	version:     1,
	description: "Ensure apiKeys table exists and has name and createdAt columns",
	run: func(db *sql.DB) error {
		var exists int
		err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='apiKeys'").Scan(&exists)
		if err != nil {
			return err
		}
		if exists == 0 {
			_, err = db.Exec("CREATE TABLE apiKeys (id TEXT PRIMARY KEY, isActive INTEGER, expiresAt TEXT, userId TEXT, name TEXT, createdAt TEXT)")
			return err
		}
		rows, err := db.Query("PRAGMA table_info(apiKeys)")
		if err != nil {
			return err
		}
		defer rows.Close()
		hasName, hasCreatedAt := false, false
		for rows.Next() {
			var cid int
			var name, typeStr string
			var notnull int
			var dfltValue sql.NullString
			var pk int
			if err := rows.Scan(&cid, &name, &typeStr, &notnull, &dfltValue, &pk); err != nil {
				return err
			}
			if name == "name" {
				hasName = true
			}
			if name == "createdAt" {
				hasCreatedAt = true
			}
		}
		if !hasName {
			if _, err := db.Exec("ALTER TABLE apiKeys ADD COLUMN name TEXT"); err != nil {
				return err
			}
		}
		if !hasCreatedAt {
			if _, err := db.Exec("ALTER TABLE apiKeys ADD COLUMN createdAt TEXT"); err != nil {
				return err
			}
		}
		return nil
	},
}

var migrationV2 = migration{
	version:     2,
	description: "Ensure playbackSessions table has createdAt and updatedAt columns",
	run: func(db *sql.DB) error {
		var exists int
		err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='playbackSessions'").Scan(&exists)
		if err != nil {
			return err
		}
		if exists == 0 {
			return nil
		}
		rows, err := db.Query("PRAGMA table_info(playbackSessions)")
		if err != nil {
			return err
		}
		defer rows.Close()
		hasCreatedAt, hasUpdatedAt := false, false
		for rows.Next() {
			var cid int
			var name, typeStr string
			var notnull int
			var dfltValue sql.NullString
			var pk int
			if err := rows.Scan(&cid, &name, &typeStr, &notnull, &dfltValue, &pk); err != nil {
				return err
			}
			if name == "createdAt" {
				hasCreatedAt = true
			}
			if name == "updatedAt" {
				hasUpdatedAt = true
			}
		}
		if !hasCreatedAt {
			if _, err := db.Exec("ALTER TABLE playbackSessions ADD COLUMN createdAt TEXT"); err != nil {
				return err
			}
		}
		if !hasUpdatedAt {
			if _, err := db.Exec("ALTER TABLE playbackSessions ADD COLUMN updatedAt TEXT"); err != nil {
				return err
			}
		}
		return nil
	},
}

var migrationV3 = migration{
	version:     3,
	description: "Ensure books table has lockedFields column",
	run: func(db *sql.DB) error {
		var exists int
		err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='books'").Scan(&exists)
		if err != nil {
			return err
		}
		if exists == 0 {
			return nil
		}
		rows, err := db.Query("PRAGMA table_info(books)")
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
			if _, err := db.Exec("ALTER TABLE books ADD COLUMN lockedFields BLOB"); err != nil {
				return err
			}
		}
		return nil
	},
}
