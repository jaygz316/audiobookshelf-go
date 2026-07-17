package playlist

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"
)

func TestCollectionOperations(t *testing.T) {
	for _, withDisplayOrder := range []bool{true, false} {
		t.Run(fmt.Sprintf("withDisplayOrder=%v", withDisplayOrder), func(t *testing.T) {
			db := setupDB(t, withDisplayOrder)
			defer db.Close()

			mgr := NewPlaylistManager(db)
			ctx := context.Background()

			// 1. Create a collection
			c := &Collection{
				Name:         "Test Collection",
				Description:  "This is a description",
				LibraryID:    "lib-123",
				DisplayOrder: 42,
				ItemIDs:      []string{"book-a", "book-b"},
			}

			err := mgr.CreateCollection(ctx, c)
			if err != nil {
				t.Fatalf("CreateCollection failed: %v", err)
			}

			if c.ID == "" {
				t.Errorf("expected ID to be generated")
			}
			if c.CreatedAt == 0 || c.UpdatedAt == 0 {
				t.Errorf("expected timestamps to be populated")
			}

			// 2. Get the collection
			retrieved, err := mgr.GetCollection(ctx, c.ID)
			if err != nil {
				t.Fatalf("GetCollection failed: %v", err)
			}

			if retrieved.ID != c.ID {
				t.Errorf("expected ID %q, got %q", c.ID, retrieved.ID)
			}
			if retrieved.Name != c.Name {
				t.Errorf("expected Name %q, got %q", c.Name, retrieved.Name)
			}
			if retrieved.Description != c.Description {
				t.Errorf("expected Description %q, got %q", c.Description, retrieved.Description)
			}
			if retrieved.LibraryID != c.LibraryID {
				t.Errorf("expected LibraryID %q, got %q", c.LibraryID, retrieved.LibraryID)
			}
			if withDisplayOrder {
				if retrieved.DisplayOrder != c.DisplayOrder {
					t.Errorf("expected DisplayOrder %d, got %d", c.DisplayOrder, retrieved.DisplayOrder)
				}
			} else {
				if retrieved.DisplayOrder != 0 {
					t.Errorf("expected DisplayOrder 0 when column not exists, got %d", retrieved.DisplayOrder)
				}
			}
			if len(retrieved.ItemIDs) != 2 || retrieved.ItemIDs[0] != "book-a" || retrieved.ItemIDs[1] != "book-b" {
				t.Errorf("expected ItemIDs %+v, got %+v", c.ItemIDs, retrieved.ItemIDs)
			}
			if retrieved.CreatedAt != c.CreatedAt {
				t.Errorf("expected CreatedAt %d, got %d", c.CreatedAt, retrieved.CreatedAt)
			}
			if retrieved.UpdatedAt != c.UpdatedAt {
				t.Errorf("expected UpdatedAt %d, got %d", c.UpdatedAt, retrieved.UpdatedAt)
			}

			// 3. Update the collection
			originalCreatedAt := c.CreatedAt
			originalUpdatedAt := c.UpdatedAt
			time.Sleep(2 * time.Millisecond)

			retrieved.Name = "Updated Collection Name"
			retrieved.Description = "Updated description"
			retrieved.DisplayOrder = 100
			retrieved.ItemIDs = []string{"book-b", "book-c"}

			err = mgr.UpdateCollection(ctx, retrieved)
			if err != nil {
				t.Fatalf("UpdateCollection failed: %v", err)
			}

			updated, err := mgr.GetCollection(ctx, c.ID)
			if err != nil {
				t.Fatalf("GetCollection failed: %v", err)
			}

			if updated.Name != "Updated Collection Name" {
				t.Errorf("expected Name to be updated, got %q", updated.Name)
			}
			if updated.Description != "Updated description" {
				t.Errorf("expected Description to be updated, got %q", updated.Description)
			}
			if withDisplayOrder {
				if updated.DisplayOrder != 100 {
					t.Errorf("expected DisplayOrder updated to 100, got %d", updated.DisplayOrder)
				}
			} else {
				if updated.DisplayOrder != 0 {
					t.Errorf("expected DisplayOrder to remain 0, got %d", updated.DisplayOrder)
				}
			}
			if len(updated.ItemIDs) != 2 || updated.ItemIDs[0] != "book-b" || updated.ItemIDs[1] != "book-c" {
				t.Errorf("expected ItemIDs updated to %+v, got %+v", []string{"book-b", "book-c"}, updated.ItemIDs)
			}
			if updated.CreatedAt != originalCreatedAt {
				t.Errorf("expected CreatedAt to be preserved as %d, got %d", originalCreatedAt, updated.CreatedAt)
			}
			if updated.UpdatedAt <= originalUpdatedAt {
				t.Errorf("expected UpdatedAt to be updated (got %d), should be > %d", updated.UpdatedAt, originalUpdatedAt)
			}

			// 4. Delete the collection
			err = mgr.DeleteCollection(ctx, c.ID)
			if err != nil {
				t.Fatalf("DeleteCollection failed: %v", err)
			}

			_, err = mgr.GetCollection(ctx, c.ID)
			if err != sql.ErrNoRows {
				t.Errorf("expected sql.ErrNoRows, got %v", err)
			}

			// Confirm items deleted from collectionBooks
			var count int
			err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM collectionBooks WHERE collectionId = ?", c.ID).Scan(&count)
			if err != nil {
				t.Fatalf("failed to count collectionBooks: %v", err)
			}
			if count != 0 {
				t.Errorf("expected collectionBooks to be deleted, got %d remaining", count)
			}
		})
	}
}
