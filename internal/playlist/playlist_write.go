package playlist

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// CreatePlaylist creates a new playlist record in the database along with its media items.
func (m *PlaylistManager) CreatePlaylist(ctx context.Context, p *Playlist) error {
	if p.ID == "" {
		p.ID = uuid.New().String()
	}
	if p.CreatedAt == 0 {
		p.CreatedAt = time.Now().UnixNano() / int64(time.Millisecond)
	}
	if p.UpdatedAt == 0 {
		p.UpdatedAt = p.CreatedAt
	}

	createdAtStr := msToTimeStr(p.CreatedAt)
	updatedAtStr := msToTimeStr(p.UpdatedAt)

	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Insert into playlists. libraryId is NULL, description is NULL.
	_, err = tx.ExecContext(ctx, `
		INSERT INTO playlists (id, name, description, createdAt, updatedAt, libraryId, userId)
		VALUES (?, ?, NULL, ?, ?, NULL, ?)`,
		p.ID, p.Name, createdAtStr, updatedAtStr, p.UserID)
	if err != nil {
		return fmt.Errorf("failed to insert playlist: %w", err)
	}

	// Lookup media item types
	typeMap, err := m.getMediaItemTypes(ctx, tx, p.ItemIDs)
	if err != nil {
		return fmt.Errorf("failed to get media item types: %w", err)
	}

	// Insert items
	for i, itemID := range p.ItemIDs {
		itemType := typeMap[itemID]
		if itemType == "" {
			itemType = "book" // Default to book if not found
		}
		itemUUID := uuid.New().String()
		order := i + 1

		_, err = tx.ExecContext(ctx, `
			INSERT INTO playlistMediaItems (id, mediaItemId, mediaItemType, "order", createdAt, playlistId)
			VALUES (?, ?, ?, ?, ?, ?)`,
			itemUUID, itemID, itemType, order, createdAtStr, p.ID)
		if err != nil {
			return fmt.Errorf("failed to insert playlist media item: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	return nil
}

// UpdatePlaylist updates an existing playlist's details and replaces its items with a new set of media items.
func (m *PlaylistManager) UpdatePlaylist(ctx context.Context, p *Playlist) error {
	p.UpdatedAt = time.Now().UnixNano() / int64(time.Millisecond)
	updatedAtStr := msToTimeStr(p.UpdatedAt)

	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Update playlist name and updatedAt
	_, err = tx.ExecContext(ctx, `
		UPDATE playlists SET name = ?, updatedAt = ? WHERE id = ?`,
		p.Name, updatedAtStr, p.ID)
	if err != nil {
		return fmt.Errorf("failed to update playlist: %w", err)
	}

	// Delete old items
	_, err = tx.ExecContext(ctx, `
		DELETE FROM playlistMediaItems WHERE playlistId = ?`, p.ID)
	if err != nil {
		return fmt.Errorf("failed to delete playlist media items: %w", err)
	}

	// Lookup media item types
	typeMap, err := m.getMediaItemTypes(ctx, tx, p.ItemIDs)
	if err != nil {
		return fmt.Errorf("failed to get media item types: %w", err)
	}

	// Insert items with sequential order
	for i, itemID := range p.ItemIDs {
		itemType := typeMap[itemID]
		if itemType == "" {
			itemType = "book"
		}
		itemUUID := uuid.New().String()
		order := i + 1

		_, err = tx.ExecContext(ctx, `
			INSERT INTO playlistMediaItems (id, mediaItemId, mediaItemType, "order", createdAt, playlistId)
			VALUES (?, ?, ?, ?, ?, ?)`,
			itemUUID, itemID, itemType, order, updatedAtStr, p.ID)
		if err != nil {
			return fmt.Errorf("failed to insert playlist media item: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	return nil
}

// DeletePlaylist removes a playlist and its associated items from the database.
func (m *PlaylistManager) DeletePlaylist(ctx context.Context, id string) error {
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Delete playlist items
	_, err = tx.ExecContext(ctx, `
		DELETE FROM playlistMediaItems WHERE playlistId = ?`, id)
	if err != nil {
		return fmt.Errorf("failed to delete playlist media items: %w", err)
	}

	// Delete playlist itself
	_, err = tx.ExecContext(ctx, `
		DELETE FROM playlists WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("failed to delete playlist: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	return nil
}
