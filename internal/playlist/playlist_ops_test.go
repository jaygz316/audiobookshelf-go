package playlist

import (
	"context"
	"database/sql"
	"testing"
	"time"
)

func TestPlaylistOperations(t *testing.T) {
	db := setupDB(t, true)
	defer db.Close()

	mgr := NewPlaylistManager(db)
	ctx := context.Background()

	// 1. Create a playlist
	p := &Playlist{
		Name:    "Test Playlist",
		UserID:  "user-123",
		ItemIDs: []string{"item-1", "item-2"},
	}

	err := mgr.CreatePlaylist(ctx, p)
	if err != nil {
		t.Fatalf("CreatePlaylist failed: %v", err)
	}

	if p.ID == "" {
		t.Errorf("expected p.ID to be generated, got empty string")
	}
	if p.CreatedAt == 0 || p.UpdatedAt == 0 {
		t.Errorf("expected timestamps to be populated, got CreatedAt=%d, UpdatedAt=%d", p.CreatedAt, p.UpdatedAt)
	}

	// 2. Get the playlist and assert fields
	retrieved, err := mgr.GetPlaylist(ctx, p.ID)
	if err != nil {
		t.Fatalf("GetPlaylist failed: %v", err)
	}

	if retrieved.ID != p.ID {
		t.Errorf("expected ID %q, got %q", p.ID, retrieved.ID)
	}
	if retrieved.UserID != p.UserID {
		t.Errorf("expected UserID %q, got %q", p.UserID, retrieved.UserID)
	}
	if retrieved.Name != p.Name {
		t.Errorf("expected Name %q, got %q", p.Name, retrieved.Name)
	}
	if len(retrieved.ItemIDs) != 2 || retrieved.ItemIDs[0] != "item-1" || retrieved.ItemIDs[1] != "item-2" {
		t.Errorf("expected ItemIDs %+v, got %+v", p.ItemIDs, retrieved.ItemIDs)
	}
	if retrieved.CreatedAt != p.CreatedAt {
		t.Errorf("expected CreatedAt %d, got %d", p.CreatedAt, retrieved.CreatedAt)
	}
	if retrieved.UpdatedAt != p.UpdatedAt {
		t.Errorf("expected UpdatedAt %d, got %d", p.UpdatedAt, retrieved.UpdatedAt)
	}

	// Check underlying DB for NULL columns as noted in PORT: libraryId is NULL, description is NULL.
	var dbLibraryId, dbDescription sql.NullString
	err = db.QueryRowContext(ctx, "SELECT libraryId, description FROM playlists WHERE id = ?", p.ID).Scan(&dbLibraryId, &dbDescription)
	if err != nil {
		t.Fatalf("failed to query playlists table directly: %v", err)
	}
	if dbLibraryId.Valid {
		t.Errorf("expected libraryId in DB to be NULL, got %q", dbLibraryId.String)
	}
	if dbDescription.Valid {
		t.Errorf("expected description in DB to be NULL, got %q", dbDescription.String)
	}

	// 3. Update the playlist
	originalCreatedAt := p.CreatedAt
	originalUpdatedAt := p.UpdatedAt
	time.Sleep(2 * time.Millisecond) // ensure updatedAt changes

	retrieved.Name = "Updated Playlist Name"
	retrieved.ItemIDs = []string{"item-2", "item-3"} // change items and order

	err = mgr.UpdatePlaylist(ctx, retrieved)
	if err != nil {
		t.Fatalf("UpdatePlaylist failed: %v", err)
	}

	// Fetch again
	updated, err := mgr.GetPlaylist(ctx, p.ID)
	if err != nil {
		t.Fatalf("GetPlaylist failed after update: %v", err)
	}

	if updated.Name != "Updated Playlist Name" {
		t.Errorf("expected name to be updated, got %q", updated.Name)
	}
	if len(updated.ItemIDs) != 2 || updated.ItemIDs[0] != "item-2" || updated.ItemIDs[1] != "item-3" {
		t.Errorf("expected updated ItemIDs %+v, got %+v", []string{"item-2", "item-3"}, updated.ItemIDs)
	}
	if updated.CreatedAt != originalCreatedAt {
		t.Errorf("expected CreatedAt to be preserved as %d, got %d", originalCreatedAt, updated.CreatedAt)
	}
	if updated.UpdatedAt <= originalUpdatedAt {
		t.Errorf("expected UpdatedAt to be updated (got %d), should be > %d", updated.UpdatedAt, originalUpdatedAt)
	}

	// 4. Delete the playlist
	err = mgr.DeletePlaylist(ctx, p.ID)
	if err != nil {
		t.Fatalf("DeletePlaylist failed: %v", err)
	}

	// Confirm deleted from playlists table
	_, err = mgr.GetPlaylist(ctx, p.ID)
	if err != sql.ErrNoRows {
		t.Errorf("expected sql.ErrNoRows, got %v", err)
	}

	// Confirm items deleted from playlistMediaItems table
	var count int
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM playlistMediaItems WHERE playlistId = ?", p.ID).Scan(&count)
	if err != nil {
		t.Fatalf("failed to count playlistMediaItems directly: %v", err)
	}
	if count != 0 {
		t.Errorf("expected playlistMediaItems to be deleted, found %d remaining", count)
	}
}
