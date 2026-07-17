package share

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	log "audiobookshelf/internal/logger"
)

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
