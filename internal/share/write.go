package share

import (
	"context"
	"fmt"
	"time"
)

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

// IncrementDownloadsCount increments the downloads count of a share link.
func (m *ShareManager) IncrementDownloadsCount(ctx context.Context, id string) error {
	query := `UPDATE shares SET downloadsCount = downloadsCount + 1 WHERE id = ?`
	_, err := m.db.ExecContext(ctx, query, id)
	return err
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
