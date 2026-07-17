package playlist

import (
	"context"
	"testing"
)

func TestMediaItemTypeResolution(t *testing.T) {
	db := setupDB(t, true)
	defer db.Close()

	// Setup some test data in books and podcastEpisodes tables
	_, err := db.Exec("INSERT INTO books (id) VALUES ('book-1'), ('book-2')")
	if err != nil {
		t.Fatalf("failed to seed books: %v", err)
	}

	_, err = db.Exec("INSERT INTO podcastEpisodes (id) VALUES ('podcast-1'), ('podcast-2')")
	if err != nil {
		t.Fatalf("failed to seed podcastEpisodes: %v", err)
	}

	mgr := NewPlaylistManager(db)
	ctx := context.Background()

	// 1. Test Playlist Creation
	p := &Playlist{
		Name:    "Media Type Test Playlist",
		UserID:  "user-1",
		ItemIDs: []string{"book-1", "podcast-1", "unknown-item", "book-2", "podcast-2"},
	}

	if err := mgr.CreatePlaylist(ctx, p); err != nil {
		t.Fatalf("CreatePlaylist failed: %v", err)
	}

	// Query the DB directly to check what types were stored
	rows, err := db.QueryContext(ctx, "SELECT mediaItemId, mediaItemType, \"order\" FROM playlistMediaItems WHERE playlistId = ? ORDER BY \"order\" ASC", p.ID)
	if err != nil {
		t.Fatalf("failed to query playlistMediaItems: %v", err)
	}
	defer rows.Close()

	type itemTypeOrder struct {
		id       string
		itemType string
		order    int
	}
	var results []itemTypeOrder
	for rows.Next() {
		var id, itemType string
		var ord int
		if err := rows.Scan(&id, &itemType, &ord); err != nil {
			t.Fatalf("failed to scan: %v", err)
		}
		results = append(results, itemTypeOrder{id: id, itemType: itemType, order: ord})
	}

	expected := []itemTypeOrder{
		{id: "book-1", itemType: "book", order: 1},
		{id: "podcast-1", itemType: "podcastEpisode", order: 2},
		{id: "unknown-item", itemType: "book", order: 3}, // defaults to "book"
		{id: "book-2", itemType: "book", order: 4},
		{id: "podcast-2", itemType: "podcastEpisode", order: 5},
	}

	if len(results) != len(expected) {
		t.Fatalf("expected %d results, got %d", len(expected), len(results))
	}
	for i, v := range expected {
		if results[i] != v {
			t.Errorf("at index %d: expected %+v, got %+v", i, v, results[i])
		}
	}

	// 2. Test Playlist Update
	p.ItemIDs = []string{"podcast-2", "unknown-item", "book-1"}
	if err := mgr.UpdatePlaylist(ctx, p); err != nil {
		t.Fatalf("UpdatePlaylist failed: %v", err)
	}

	// Query DB again
	rows2, err := db.QueryContext(ctx, "SELECT mediaItemId, mediaItemType, \"order\" FROM playlistMediaItems WHERE playlistId = ? ORDER BY \"order\" ASC", p.ID)
	if err != nil {
		t.Fatalf("failed to query playlistMediaItems after update: %v", err)
	}
	defer rows2.Close()

	var results2 []itemTypeOrder
	for rows2.Next() {
		var id, itemType string
		var ord int
		if err := rows2.Scan(&id, &itemType, &ord); err != nil {
			t.Fatalf("failed to scan updated: %v", err)
		}
		results2 = append(results2, itemTypeOrder{id: id, itemType: itemType, order: ord})
	}

	expected2 := []itemTypeOrder{
		{id: "podcast-2", itemType: "podcastEpisode", order: 1},
		{id: "unknown-item", itemType: "book", order: 2}, // defaults to "book"
		{id: "book-1", itemType: "book", order: 3},
	}

	if len(results2) != len(expected2) {
		t.Fatalf("expected %d results, got %d", len(expected2), len(results2))
	}
	for i, v := range expected2 {
		if results2[i] != v {
			t.Errorf("at index %d: expected %+v, got %+v", i, v, results2[i])
		}
	}
}
