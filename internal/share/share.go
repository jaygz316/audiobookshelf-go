package share

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"time"

	"golang.org/x/crypto/bcrypt"

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

// CreateShare saves a new share link entry in the database.
func (m *ShareManager) CreateShare(ctx context.Context, s *ShareLink) error {
	if s.ID == "" {
		return fmt.Errorf("share link ID cannot be empty")
	}

	now := time.Now()
	if s.CreatedAt.IsZero() {
		s.CreatedAt = now
	}
	if s.UpdatedAt.IsZero() {
		s.UpdatedAt = now
	}

	var expiresAtVal interface{}
	if !s.ExpiresAt.IsZero() {
		expiresAtVal = timeToDBStr(s.ExpiresAt)
	}

	isDownloadableInt := 0
	if s.IsDownloadable {
		isDownloadableInt = 1
	}

	embeddableInt := 0
	if s.Embeddable {
		embeddableInt = 1
	}

	query := `
		INSERT INTO shares (id, libraryItemId, createdBy, expiresAt, isDownloadable, pash, createdAt, updatedAt, maxDownloads, downloadsCount, embeddable)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := m.db.ExecContext(ctx, query,
		s.ID,
		s.LibraryItemID,
		s.CreatedBy,
		expiresAtVal,
		isDownloadableInt,
		s.PasswordHash,
		timeToDBStr(s.CreatedAt),
		timeToDBStr(s.UpdatedAt),
		s.MaxDownloads,
		s.DownloadsCount,
		embeddableInt,
	)
	if err != nil {
		return fmt.Errorf("failed to insert share link: %w", err)
	}

	return nil
}

// GetShare retrieves a share link by ID, returning nil if expired or not found.
// PORT: Checking if the share has expired, delete it and return nil if it is.
func (m *ShareManager) GetShare(ctx context.Context, id string) (*ShareLink, error) {
	query := `
		SELECT id, libraryItemId, createdBy, expiresAt, isDownloadable, pash, createdAt, updatedAt, maxDownloads, downloadsCount, embeddable
		FROM shares
		WHERE id = ?
	`
	row := m.db.QueryRowContext(ctx, query, id)

	var s ShareLink
	var expiresAtStr sql.NullString
	var createdAtStr, updatedAtStr string
	var isDownloadableInt, embeddableInt int

	err := row.Scan(&s.ID, &s.LibraryItemID, &s.CreatedBy, &expiresAtStr, &isDownloadableInt, &s.PasswordHash, &createdAtStr, &updatedAtStr, &s.MaxDownloads, &s.DownloadsCount, &embeddableInt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan share link: %w", err)
	}

	s.IsDownloadable = isDownloadableInt != 0
	s.Embeddable = embeddableInt != 0
	s.HasPassword = s.PasswordHash != ""

	s.CreatedAt, err = parseTimeStr(createdAtStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse createdAt timestamp: %w", err)
	}

	s.UpdatedAt, err = parseTimeStr(updatedAtStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse updatedAt timestamp: %w", err)
	}

	if expiresAtStr.Valid && expiresAtStr.String != "" {
		s.ExpiresAt, err = parseTimeStr(expiresAtStr.String)
		if err != nil {
			return nil, fmt.Errorf("failed to parse expiresAt timestamp: %w", err)
		}

		if !s.ExpiresAt.IsZero() && time.Now().After(s.ExpiresAt) {
			// PORT: Automatically delete expired share link upon fetch
			if err := m.DeleteShare(ctx, id); err != nil {
				log.Printf("[Share] Failed to delete expired share link %s: %v", id, err)
			}
			return nil, nil
		}
	}

	return &s, nil
}

// GetShares retrieves all share links.
func (m *ShareManager) GetShares(ctx context.Context) ([]*ShareLink, error) {
	query := `
		SELECT id, libraryItemId, createdBy, expiresAt, isDownloadable, pash, createdAt, updatedAt, maxDownloads, downloadsCount, embeddable
		FROM shares
		ORDER BY createdAt DESC
	`
	rows, err := m.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query share links: %w", err)
	}
	defer rows.Close()

	var list []*ShareLink
	for rows.Next() {
		var s ShareLink
		var expiresAtStr sql.NullString
		var createdAtStr, updatedAtStr string
		var isDownloadableInt, embeddableInt int

		err := rows.Scan(&s.ID, &s.LibraryItemID, &s.CreatedBy, &expiresAtStr, &isDownloadableInt, &s.PasswordHash, &createdAtStr, &updatedAtStr, &s.MaxDownloads, &s.DownloadsCount, &embeddableInt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan share link row: %w", err)
		}

		s.IsDownloadable = isDownloadableInt != 0
		s.Embeddable = embeddableInt != 0
		s.HasPassword = s.PasswordHash != ""
		s.CreatedAt, _ = parseTimeStr(createdAtStr)
		s.UpdatedAt, _ = parseTimeStr(updatedAtStr)
		if expiresAtStr.Valid && expiresAtStr.String != "" {
			s.ExpiresAt, _ = parseTimeStr(expiresAtStr.String)
		}

		list = append(list, &s)
	}
	return list, nil
}

// IncrementDownloadsCount increments the downloads count of a share link.
func (m *ShareManager) IncrementDownloadsCount(ctx context.Context, id string) error {
	query := `UPDATE shares SET downloadsCount = downloadsCount + 1 WHERE id = ?`
	_, err := m.db.ExecContext(ctx, query, id)
	return err
}

// ValidateSharePassword matches a plaintext password with the hashed credentials.
func (m *ShareManager) ValidateSharePassword(ctx context.Context, id, password string) (bool, error) {
	s, err := m.GetShare(ctx, id)
	if err != nil {
		return false, fmt.Errorf("failed to retrieve share link for validation: %w", err)
	}
	if s == nil {
		return false, nil
	}

	if s.PasswordHash == "" {
		// No password protection configured
		return true, nil
	}

	err = bcrypt.CompareHashAndPassword([]byte(s.PasswordHash), []byte(password))
	if err != nil {
		if err == bcrypt.ErrMismatchedHashAndPassword {
			return false, nil
		}
		return false, fmt.Errorf("failed to validate password: %w", err)
	}

	return true, nil
}

// DeleteShare removes the share link from database.
func (m *ShareManager) DeleteShare(ctx context.Context, id string) error {
	query := `DELETE FROM shares WHERE id = ?`
	_, err := m.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete share link: %w", err)
	}
	return nil
}

// Helper functions for formatting and parsing SQLite datetime strings

func timeToDBStr(t time.Time) string {
	return t.UTC().Format("2006-01-02 15:04:05.000 +00:00")
}

func parseTimeStr(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}

	layouts := []string{
		"2006-01-02 15:04:05.000 +00:00",
		"2006-01-02T15:04:05.000Z",
		"2006-01-02 15:04:05.000000 +00:00",
		"2006-01-02 15:04:05.000",
		"2006-01-02 15:04:05",
		time.RFC3339,
	}

	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC(), nil
		}
	}

	// Fallback to millisecond unix timestamp parsing
	if val, err := strconv.ParseInt(s, 10, 64); err == nil {
		return time.Unix(0, val*int64(time.Millisecond)).UTC(), nil
	}

	return time.Time{}, fmt.Errorf("failed to parse datetime: %s", s)
}
