package db

import (
	"database/sql"
	"fmt"
)

var migrationV8 = migration{
	version:     8,
	description: "Add metadata columns to podcastEpisodes table",
	run: func(db *sql.DB) error {
		var exists int
		err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='podcastEpisodes'").Scan(&exists)
		if err != nil {
			return err
		}
		if exists == 0 {
			return nil
		}
		rows, err := db.Query("PRAGMA table_info(podcastEpisodes)")
		if err != nil {
			return err
		}
		defer rows.Close()
		cols := map[string]bool{}
		for rows.Next() {
			var cid int
			var name, typeStr string
			var notnull int
			var dfltValue sql.NullString
			var pk int
			if err := rows.Scan(&cid, &name, &typeStr, &notnull, &dfltValue, &pk); err == nil {
				cols[name] = true
			}
		}
		addCols := []string{"pubDate", "description", "season", "episode", "episodeType", "enclosureURL", "publishedAt", "createdAt", "updatedAt"}
		for _, col := range addCols {
			if !cols[col] {
				if _, err := db.Exec(fmt.Sprintf("ALTER TABLE podcastEpisodes ADD COLUMN %s TEXT", col)); err != nil {
					return err
				}
			}
		}
		return nil
	},
}

var migrationV9 = migration{
	version:     9,
	description: "Add autoDeletePlayed column to podcasts table",
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
		hasAutoDeletePlayed := false
		for rows.Next() {
			var cid int
			var name, typeStr string
			var notnull int
			var dfltValue sql.NullString
			var pk int
			if err := rows.Scan(&cid, &name, &typeStr, &notnull, &dfltValue, &pk); err == nil {
				if name == "autoDeletePlayed" {
					hasAutoDeletePlayed = true
				}
			}
		}
		if !hasAutoDeletePlayed {
			if _, err := db.Exec("ALTER TABLE podcasts ADD COLUMN autoDeletePlayed INTEGER DEFAULT 0"); err != nil {
				return err
			}
		}
		return nil
	},
}

var migrationV10 = migration{
	version:     10,
	description: "Add skipIntroDuration and skipOutroDuration columns to podcasts table, and imageURL column to podcastEpisodes table",
	run: func(db *sql.DB) error {
		var exists int
		err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='podcasts'").Scan(&exists)
		if err != nil {
			return err
		}
		if exists > 0 {
			rows, err := db.Query("PRAGMA table_info(podcasts)")
			if err != nil {
				return err
			}
			cols := map[string]bool{}
			for rows.Next() {
				var cid int
				var name, typeStr string
				var notnull int
				var dfltValue sql.NullString
				var pk int
				if err := rows.Scan(&cid, &name, &typeStr, &notnull, &dfltValue, &pk); err == nil {
					cols[name] = true
				}
			}
			rows.Close()
			if !cols["skipIntroDuration"] {
				if _, err := db.Exec("ALTER TABLE podcasts ADD COLUMN skipIntroDuration INTEGER DEFAULT 0"); err != nil {
					return err
				}
			}
			if !cols["skipOutroDuration"] {
				if _, err := db.Exec("ALTER TABLE podcasts ADD COLUMN skipOutroDuration INTEGER DEFAULT 0"); err != nil {
					return err
				}
			}
		}

		err = db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='podcastEpisodes'").Scan(&exists)
		if err != nil {
			return err
		}
		if exists > 0 {
			rows, err := db.Query("PRAGMA table_info(podcastEpisodes)")
			if err != nil {
				return err
			}
			cols := map[string]bool{}
			for rows.Next() {
				var cid int
				var name, typeStr string
				var notnull int
				var dfltValue sql.NullString
				var pk int
				if err := rows.Scan(&cid, &name, &typeStr, &notnull, &dfltValue, &pk); err == nil {
					cols[name] = true
				}
			}
			rows.Close()
			if !cols["imageURL"] {
				if _, err := db.Exec("ALTER TABLE podcastEpisodes ADD COLUMN imageURL TEXT"); err != nil {
					return err
				}
			}
		}
		return nil
	},
}
