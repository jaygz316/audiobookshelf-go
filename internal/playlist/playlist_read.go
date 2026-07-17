package playlist

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// Helper: query media item types from books or podcastEpisodes
func (m *PlaylistManager) getMediaItemTypes(ctx context.Context, tx *sql.Tx, itemIDs []string) (map[string]string, error) {
	if len(itemIDs) == 0 {
		return make(map[string]string), nil
	}

	placeholders := make([]string, len(itemIDs))
	args := make([]interface{}, len(itemIDs))
	for i, id := range itemIDs {
		placeholders[i] = "?"
		args[i] = id
	}

	inClause := strings.Join(placeholders, ",")
	query := fmt.Sprintf(
		"SELECT id, 'book' FROM books WHERE id IN (%s) UNION ALL SELECT id, 'podcastEpisode' FROM podcastEpisodes WHERE id IN (%s)",
		inClause, inClause,
	)
	doubleArgs := append(args, args...)

	rows, err := tx.QueryContext(ctx, query, doubleArgs...)
	if err != nil {
		return nil, fmt.Errorf("failed to query media item types: %w", err)
	}
	defer rows.Close()

	typeMap := make(map[string]string)
	for rows.Next() {
		var id, itemType string
		if err := rows.Scan(&id, &itemType); err != nil {
			return nil, fmt.Errorf("failed to scan media item type: %w", err)
		}
		typeMap[id] = itemType
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error during media item types iteration: %w", err)
	}
	return typeMap, nil
}

// GetPlaylist retrieves a playlist and its associated media item IDs by ID from the database.
func (m *PlaylistManager) GetPlaylist(ctx context.Context, id string) (*Playlist, error) {
	var userID, name, createdAtStr, updatedAtStr string
	err := m.db.QueryRowContext(ctx, `
		SELECT userId, name, createdAt, updatedAt FROM playlists WHERE id = ?`, id).
		Scan(&userID, &name, &createdAtStr, &updatedAtStr)
	if err == sql.ErrNoRows {
		return nil, sql.ErrNoRows
	} else if err != nil {
		return nil, fmt.Errorf("failed to query playlist: %w", err)
	}

	p := &Playlist{
		ID:        id,
		UserID:    userID,
		Name:      name,
		CreatedAt: parseMsFromDBStr(createdAtStr),
		UpdatedAt: parseMsFromDBStr(updatedAtStr),
		ItemIDs:   []string{},
	}

	rows, err := m.db.QueryContext(ctx, `
		SELECT mediaItemId FROM playlistMediaItems WHERE playlistId = ? ORDER BY "order" ASC`, id)
	if err != nil {
		return nil, fmt.Errorf("failed to query playlist media items: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var itemID string
		if err := rows.Scan(&itemID); err != nil {
			return nil, fmt.Errorf("failed to scan playlist media item: %w", err)
		}
		p.ItemIDs = append(p.ItemIDs, itemID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error during playlist media items iteration: %w", err)
	}

	return p, nil
}
