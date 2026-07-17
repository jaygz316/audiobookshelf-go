package playlist

import (
	"context"
	"testing"
)

func TestDisplayOrderAndSorting(t *testing.T) {
	db := setupDB(t, true)
	defer db.Close()

	mgr := NewPlaylistManager(db)
	ctx := context.Background()

	// 1. Check playlist media items order on creation
	p := &Playlist{
		Name:    "Sort Test Playlist",
		UserID:  "user-1",
		ItemIDs: []string{"item-C", "item-A", "item-B"},
	}
	if err := mgr.CreatePlaylist(ctx, p); err != nil {
		t.Fatalf("CreatePlaylist failed: %v", err)
	}

	// Query DB directly to verify order values
	rows, err := db.QueryContext(ctx, "SELECT mediaItemId, \"order\" FROM playlistMediaItems WHERE playlistId = ? ORDER BY \"order\" ASC", p.ID)
	if err != nil {
		t.Fatalf("failed to query playlistMediaItems: %v", err)
	}
	defer rows.Close()

	type itemOrder struct {
		id    string
		order int
	}
	var orders []itemOrder
	for rows.Next() {
		var id string
		var ord int
		if err := rows.Scan(&id, &ord); err != nil {
			t.Fatalf("failed to scan item: %v", err)
		}
		orders = append(orders, itemOrder{id: id, order: ord})
	}

	expectedOrders := []itemOrder{
		{id: "item-C", order: 1},
		{id: "item-A", order: 2},
		{id: "item-B", order: 3},
	}
	if len(orders) != len(expectedOrders) {
		t.Fatalf("expected %d items, got %d", len(expectedOrders), len(orders))
	}
	for i, v := range expectedOrders {
		if orders[i] != v {
			t.Errorf("at index %d: expected %+v, got %+v", i, v, orders[i])
		}
	}

	// 2. Check playlist update reordering
	p.ItemIDs = []string{"item-B", "item-C"}
	if err := mgr.UpdatePlaylist(ctx, p); err != nil {
		t.Fatalf("UpdatePlaylist failed: %v", err)
	}

	// Query DB again
	rows2, err := db.QueryContext(ctx, "SELECT mediaItemId, \"order\" FROM playlistMediaItems WHERE playlistId = ? ORDER BY \"order\" ASC", p.ID)
	if err != nil {
		t.Fatalf("failed to query playlistMediaItems: %v", err)
	}
	defer rows2.Close()

	var orders2 []itemOrder
	for rows2.Next() {
		var id string
		var ord int
		if err := rows2.Scan(&id, &ord); err != nil {
			t.Fatalf("failed to scan item: %v", err)
		}
		orders2 = append(orders2, itemOrder{id: id, order: ord})
	}

	expectedOrders2 := []itemOrder{
		{id: "item-B", order: 1},
		{id: "item-C", order: 2},
	}
	if len(orders2) != len(expectedOrders2) {
		t.Fatalf("expected %d items, got %d", len(expectedOrders2), len(orders2))
	}
	for i, v := range expectedOrders2 {
		if orders2[i] != v {
			t.Errorf("at index %d: expected %+v, got %+v", i, v, orders2[i])
		}
	}

	// 3. Check collection books order on creation
	c := &Collection{
		Name:      "Sort Test Collection",
		LibraryID: "lib-1",
		ItemIDs:   []string{"book-Z", "book-X", "book-Y"},
	}
	if err := mgr.CreateCollection(ctx, c); err != nil {
		t.Fatalf("CreateCollection failed: %v", err)
	}

	// Query DB directly for collectionBooks order
	rows3, err := db.QueryContext(ctx, "SELECT bookId, \"order\" FROM collectionBooks WHERE collectionId = ? ORDER BY \"order\" ASC", c.ID)
	if err != nil {
		t.Fatalf("failed to query collectionBooks: %v", err)
	}
	defer rows3.Close()

	var cbOrders []itemOrder
	for rows3.Next() {
		var id string
		var ord int
		if err := rows3.Scan(&id, &ord); err != nil {
			t.Fatalf("failed to scan collectionBook: %v", err)
		}
		cbOrders = append(cbOrders, itemOrder{id: id, order: ord})
	}

	expectedCbOrders := []itemOrder{
		{id: "book-Z", order: 1},
		{id: "book-X", order: 2},
		{id: "book-Y", order: 3},
	}
	if len(cbOrders) != len(expectedCbOrders) {
		t.Fatalf("expected %d items, got %d", len(expectedCbOrders), len(cbOrders))
	}
	for i, v := range expectedCbOrders {
		if cbOrders[i] != v {
			t.Errorf("at index %d: expected %+v, got %+v", i, v, cbOrders[i])
		}
	}

	// 4. Check collection update reordering
	c.ItemIDs = []string{"book-Y", "book-Z"}
	if err := mgr.UpdateCollection(ctx, c); err != nil {
		t.Fatalf("UpdateCollection failed: %v", err)
	}

	// Query DB again for collectionBooks
	rows4, err := db.QueryContext(ctx, "SELECT bookId, \"order\" FROM collectionBooks WHERE collectionId = ? ORDER BY \"order\" ASC", c.ID)
	if err != nil {
		t.Fatalf("failed to query collectionBooks: %v", err)
	}
	defer rows4.Close()

	var cbOrders2 []itemOrder
	for rows4.Next() {
		var id string
		var ord int
		if err := rows4.Scan(&id, &ord); err != nil {
			t.Fatalf("failed to scan collectionBook: %v", err)
		}
		cbOrders2 = append(cbOrders2, itemOrder{id: id, order: ord})
	}

	expectedCbOrders2 := []itemOrder{
		{id: "book-Y", order: 1},
		{id: "book-Z", order: 2},
	}
	if len(cbOrders2) != len(expectedCbOrders2) {
		t.Fatalf("expected %d items, got %d", len(expectedCbOrders2), len(cbOrders2))
	}
	for i, v := range expectedCbOrders2 {
		if cbOrders2[i] != v {
			t.Errorf("at index %d: expected %+v, got %+v", i, v, cbOrders2[i])
		}
	}
}
