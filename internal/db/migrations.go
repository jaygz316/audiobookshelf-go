package db

import (
	"database/sql"
	"fmt"

	log "audiobookshelf/internal/logger"
)

type migration struct {
	version     int
	description string
	run         func(db *sql.DB) error
}

var dbMigrations = []migration{
	migrationV1,
	migrationV2,
	migrationV3,
	migrationV4,
	migrationV5,
	migrationV6,
	migrationV7,
	migrationV8,
	migrationV9,
	migrationV10,
}

func migrateDatabase(db *sql.DB) error {
	var currentVersion int
	err := db.QueryRow("PRAGMA user_version").Scan(&currentVersion)
	if err != nil {
		return fmt.Errorf("failed to read database version: %w", err)
	}

	for _, m := range dbMigrations {
		if m.version > currentVersion {
			log.Infof("[DB] Running migration version %d: %s", m.version, m.description)
			if err := m.run(db); err != nil {
				return fmt.Errorf("failed running migration %d (%s): %w", m.version, m.description, err)
			}
			_, err = db.Exec(fmt.Sprintf("PRAGMA user_version = %d", m.version))
			if err != nil {
				return fmt.Errorf("failed setting database version to %d: %w", m.version, err)
			}
			log.Infof("[DB] Successfully migrated to version %d", m.version)
		}
	}
	return nil
}
