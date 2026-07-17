package db

import (
	"database/sql"
	"fmt"
	"os"
	"strconv"
	"time"

	log "audiobookshelf/internal/logger"
)

// InitDB initializes the SQLite database at dbPath, creating it and bootstrapping the schema if needed.
func InitDB(dbPath string) (*sql.DB, error) {
	isNew := false
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		isNew = true
		log.Infof("[DB] Database file not found, creating new database at %s", dbPath)
	}

	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode=WAL&_pragma=busy_timeout=5000", dbPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}

	// Configure DB Connection Pooling for optimized SQLite WAL concurrency with environment overrides
	maxOpen := 25
	maxIdle := 10
	maxLifetime := time.Hour
	maxIdleTime := 30 * time.Minute

	if val := os.Getenv("DB_MAX_OPEN_CONNS"); val != "" {
		if i, err := strconv.Atoi(val); err == nil && i > 0 {
			maxOpen = i
		}
	}
	if val := os.Getenv("DB_MAX_IDLE_CONNS"); val != "" {
		if i, err := strconv.Atoi(val); err == nil && i >= 0 {
			maxIdle = i
		}
	}
	if val := os.Getenv("DB_CONN_MAX_LIFETIME"); val != "" {
		if d, err := time.ParseDuration(val); err == nil && d > 0 {
			maxLifetime = d
		}
	}
	if val := os.Getenv("DB_CONN_MAX_IDLE_TIME"); val != "" {
		if d, err := time.ParseDuration(val); err == nil && d > 0 {
			maxIdleTime = d
		}
	}

	db.SetMaxOpenConns(maxOpen)
	db.SetMaxIdleConns(maxIdle)
	db.SetConnMaxLifetime(maxLifetime)
	db.SetConnMaxIdleTime(maxIdleTime)

	if isNew {
		if err := bootstrapSchema(db); err != nil {
			db.Close()
			return nil, fmt.Errorf("failed to bootstrap schema: %w", err)
		}
		log.Info("[DB] Schema bootstrapped successfully")
	} else {
		if err := migrateDatabase(db); err != nil {
			db.Close()
			return nil, fmt.Errorf("failed to migrate database: %w", err)
		}
	}

	return db, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
