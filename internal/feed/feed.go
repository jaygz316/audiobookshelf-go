package feed

import (
	"database/sql"
)

type FeedManager struct {
	db           *sql.DB
	metadataPath string
}

// NewFeedManager constructs an XML Feed manager.
func NewFeedManager(db *sql.DB, metadataPath string) *FeedManager {
	return &FeedManager{db: db, metadataPath: metadataPath}
}
