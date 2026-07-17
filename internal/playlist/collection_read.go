package playlist

import (
	"context"
	"database/sql"
	"fmt"
)

// GetCollection retrieves a collection and its associated book IDs by ID from the database.
func (m *PlaylistManager) GetCollection(ctx context.Context, id string) (*Collection, error) {
	var name, description, libraryID, createdAtStr, updatedAtStr string
	var displayOrder int
	var isSmart sql.NullInt64
	var rules sql.NullString

	hasDisplayOrder := hasColumn(ctx, m.db, "collections", "displayOrder")

	var err error
	if hasDisplayOrder {
		err = m.db.QueryRowContext(ctx, `
			SELECT name, description, libraryId, displayOrder, createdAt, updatedAt, isSmart, rules FROM collections WHERE id = ?`, id).
			Scan(&name, &description, &libraryID, &displayOrder, &createdAtStr, &updatedAtStr, &isSmart, &rules)
	} else {
		err = m.db.QueryRowContext(ctx, `
			SELECT name, description, libraryId, createdAt, updatedAt, isSmart, rules FROM collections WHERE id = ?`, id).
			Scan(&name, &description, &libraryID, &createdAtStr, &updatedAtStr, &isSmart, &rules)
	}

	if err == sql.ErrNoRows {
		return nil, sql.ErrNoRows
	} else if err != nil {
		return nil, fmt.Errorf("failed to query collection: %w", err)
	}

	c := &Collection{
		ID:           id,
		Name:         name,
		Description:  description,
		LibraryID:    libraryID,
		DisplayOrder: displayOrder,
		IsSmart:      isSmart.Int64 != 0,
		Rules:        rules.String,
		CreatedAt:    parseMsFromDBStr(createdAtStr),
		UpdatedAt:    parseMsFromDBStr(updatedAtStr),
		ItemIDs:      []string{},
	}

	if c.IsSmart {
		dynamicIDs, err := m.ResolveSmartCollectionItems(ctx, c.LibraryID, c.Rules)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve smart collection items: %w", err)
		}
		c.ItemIDs = dynamicIDs
	} else {
		// Get collection books in display order
		rows, err := m.db.QueryContext(ctx, `
			SELECT bookId FROM collectionBooks WHERE collectionId = ? ORDER BY "order" ASC`, id)
		if err != nil {
			return nil, fmt.Errorf("failed to query collection books: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var bookID string
			if err := rows.Scan(&bookID); err != nil {
				return nil, fmt.Errorf("failed to scan collection book: %w", err)
			}
			c.ItemIDs = append(c.ItemIDs, bookID)
		}
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("error during collection books iteration: %w", err)
		}
	}

	return c, nil
}
