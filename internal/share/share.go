package share

import (
	"database/sql"
	"time"

	log "audiobookshelf/internal/logger"
)

// ShareLink represents a public sharing link.
type ShareLink struct {
	ID             string    `json:"id"`
	LibraryItemID  string    `json:"libraryItemId"`
	CreatedBy      string    `json:"createdBy"`
	ExpiresAt      time.Time `json:"expiresAt"`
	IsDownloadable bool      `json:"isDownloadable"`
	PasswordHash   string    `json:"-"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
	MaxDownloads   int       `json:"maxDownloads"`
	DownloadsCount int       `json:"downloadsCount"`
	Embeddable     bool      `json:"embeddable"`
	HasPassword    bool      `json:"hasPassword"`
}

// ShareManager handles persistence and validation of public share links.
type ShareManager struct {
	db *sql.DB
}

// NewShareManager constructs a share manager.
// PORT: Automatically initialize the "shares" table if it does not exist in SQLite database.
func NewShareManager(db *sql.DB) *ShareManager {
	query := `
	CREATE TABLE IF NOT EXISTS shares (
		id TEXT PRIMARY KEY,
		libraryItemId TEXT,
		createdBy TEXT,
		expiresAt TEXT,
		isDownloadable INTEGER,
		pash TEXT,
		createdAt TEXT,
		updatedAt TEXT,
		maxDownloads INTEGER DEFAULT 0,
		downloadsCount INTEGER DEFAULT 0,
		embeddable INTEGER DEFAULT 0
	);`
	if _, err := db.Exec(query); err != nil {
		log.Printf("[Share] Failed to initialize shares table: %v", err)
	}

	// Gracefully handle existing SQLite databases by ensuring the columns exist
	_, _ = db.Exec("ALTER TABLE shares ADD COLUMN maxDownloads INTEGER DEFAULT 0")
	_, _ = db.Exec("ALTER TABLE shares ADD COLUMN downloadsCount INTEGER DEFAULT 0")
	_, _ = db.Exec("ALTER TABLE shares ADD COLUMN embeddable INTEGER DEFAULT 0")

	return &ShareManager{db: db}
}
