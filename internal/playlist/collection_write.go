package playlist

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// CreateCollection creates a new collection record in the database.
func (m *PlaylistManager) CreateCollection(ctx context.Context, c *Collection) error {
	if c.ID == "" {
		c.ID = uuid.New().String()
	}
	if c.CreatedAt == 0 {
		c.CreatedAt = time.Now().UnixNano() / int64(time.Millisecond)
	}
	if c.UpdatedAt == 0 {
		c.UpdatedAt = c.CreatedAt
	}

	createdAtStr := msToTimeStr(c.CreatedAt)
	updatedAtStr := msToTimeStr(c.UpdatedAt)

	// Check if "displayOrder" column exists in "collections" table dynamically
	hasDisplayOrder := hasColumn(ctx, m.db, "collections", "displayOrder")

	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	isSmartInt := 0
	if c.IsSmart {
		isSmartInt = 1
	}

	if hasDisplayOrder {
		_, err = tx.ExecContext(ctx, `
			INSERT INTO collections (id, name, description, createdAt, updatedAt, libraryId, displayOrder, isSmart, rules)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			c.ID, c.Name, c.Description, createdAtStr, updatedAtStr, c.LibraryID, c.DisplayOrder, isSmartInt, c.Rules)
	} else {
		_, err = tx.ExecContext(ctx, `
			INSERT INTO collections (id, name, description, createdAt, updatedAt, libraryId, isSmart, rules)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			c.ID, c.Name, c.Description, createdAtStr, updatedAtStr, c.LibraryID, isSmartInt, c.Rules)
	}
	if err != nil {
		return fmt.Errorf("failed to insert collection: %w", err)
	}

	// Insert items into collectionBooks if not smart
	if !c.IsSmart {
		for i, itemID := range c.ItemIDs {
			cbUUID := uuid.New().String()
			order := i + 1

			_, err = tx.ExecContext(ctx, `
				INSERT INTO collectionBooks (id, "order", createdAt, bookId, collectionId)
				VALUES (?, ?, ?, ?, ?)`,
				cbUUID, order, createdAtStr, itemID, c.ID)
			if err != nil {
				return fmt.Errorf("failed to insert collection book: %w", err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	return nil
}

// UpdateCollection updates an existing collection's details and replaces its books list.
func (m *PlaylistManager) UpdateCollection(ctx context.Context, c *Collection) error {
	c.UpdatedAt = time.Now().UnixNano() / int64(time.Millisecond)
	updatedAtStr := msToTimeStr(c.UpdatedAt)

	hasDisplayOrder := hasColumn(ctx, m.db, "collections", "displayOrder")

	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	isSmartInt := 0
	if c.IsSmart {
		isSmartInt = 1
	}

	if hasDisplayOrder {
		_, err = tx.ExecContext(ctx, `
			UPDATE collections SET name = ?, description = ?, libraryId = ?, displayOrder = ?, isSmart = ?, rules = ?, updatedAt = ? WHERE id = ?`,
			c.Name, c.Description, c.LibraryID, c.DisplayOrder, isSmartInt, c.Rules, updatedAtStr, c.ID)
	} else {
		_, err = tx.ExecContext(ctx, `
			UPDATE collections SET name = ?, description = ?, libraryId = ?, isSmart = ?, rules = ?, updatedAt = ? WHERE id = ?`,
			c.Name, c.Description, c.LibraryID, isSmartInt, c.Rules, updatedAtStr, c.ID)
	}
	if err != nil {
		return fmt.Errorf("failed to update collection: %w", err)
	}

	// Delete old items
	_, err = tx.ExecContext(ctx, `
		DELETE FROM collectionBooks WHERE collectionId = ?`, c.ID)
	if err != nil {
		return fmt.Errorf("failed to delete collection books: %w", err)
	}

	// Insert items into collectionBooks if not smart
	if !c.IsSmart {
		for i, itemID := range c.ItemIDs {
			cbUUID := uuid.New().String()
			order := i + 1

			_, err = tx.ExecContext(ctx, `
				INSERT INTO collectionBooks (id, "order", createdAt, bookId, collectionId)
				VALUES (?, ?, ?, ?, ?)`,
				cbUUID, order, updatedAtStr, itemID, c.ID)
			if err != nil {
				return fmt.Errorf("failed to insert collection book: %w", err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	return nil
}

// DeleteCollection removes a collection and its book relations from the database.
func (m *PlaylistManager) DeleteCollection(ctx context.Context, id string) error {
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Delete collection books
	_, err = tx.ExecContext(ctx, `
		DELETE FROM collectionBooks WHERE collectionId = ?`, id)
	if err != nil {
		return fmt.Errorf("failed to delete collection books: %w", err)
	}

	// Delete collection itself
	_, err = tx.ExecContext(ctx, `
		DELETE FROM collections WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("failed to delete collection: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	return nil
}
