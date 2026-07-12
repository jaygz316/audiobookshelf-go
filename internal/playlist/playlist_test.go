package playlist

import (
	"context"
	"database/sql"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

var dbCounter int64

func setupDB(t *testing.T, withDisplayOrder bool) *sql.DB {
	id := atomic.AddInt64(&dbCounter, 1)
	dsn := fmt.Sprintf("file:memdb%d?mode=memory&cache=shared", id)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("failed to open in-memory sqlite db: %v", err)
	}
	db.SetMaxIdleConns(2)

	// Create tables
	queries := []string{
		`CREATE TABLE playlists (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			description TEXT,
			createdAt TEXT,
			updatedAt TEXT,
			libraryId TEXT,
			userId TEXT
		);`,
		`CREATE TABLE playlistMediaItems (
			id TEXT PRIMARY KEY,
			mediaItemId TEXT,
			mediaItemType TEXT,
			"order" INTEGER,
			createdAt TEXT,
			playlistId TEXT
		);`,
		`CREATE TABLE collectionBooks (
			id TEXT PRIMARY KEY,
			"order" INTEGER,
			createdAt TEXT,
			bookId TEXT,
			collectionId TEXT
		);`,
		`CREATE TABLE books (
			id TEXT PRIMARY KEY,
			title TEXT,
			subtitle TEXT,
			description TEXT,
			genres TEXT,
			tags TEXT,
			narrators TEXT,
			publishedYear TEXT
		);`,
		`CREATE TABLE podcastEpisodes (
			id TEXT PRIMARY KEY
		);`,
		`CREATE TABLE libraryItems (
			id TEXT PRIMARY KEY,
			libraryId TEXT,
			mediaId TEXT,
			mediaType TEXT,
			isMissing INTEGER DEFAULT 0,
			isInvalid INTEGER DEFAULT 0
		);`,
		`CREATE TABLE bookAuthors (
			bookId TEXT,
			authorId TEXT
		);`,
		`CREATE TABLE authors (
			id TEXT PRIMARY KEY,
			name TEXT
		);`,
		`CREATE TABLE bookSeries (
			bookId TEXT,
			seriesId TEXT
		);`,
		`CREATE TABLE series (
			id TEXT PRIMARY KEY,
			name TEXT
		);`,
	}

	for _, q := range queries {
		if _, err := db.Exec(q); err != nil {
			db.Close()
			t.Fatalf("failed to execute query %q: %v", q, err)
		}
	}

	// Create collections table depending on withDisplayOrder
	var createCollectionsQuery string
	if withDisplayOrder {
		createCollectionsQuery = `CREATE TABLE collections (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			description TEXT,
			createdAt TEXT,
			updatedAt TEXT,
			libraryId TEXT,
			displayOrder INTEGER,
			isSmart INTEGER DEFAULT 0,
			rules TEXT
		);`
	} else {
		createCollectionsQuery = `CREATE TABLE collections (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			description TEXT,
			createdAt TEXT,
			updatedAt TEXT,
			libraryId TEXT,
			isSmart INTEGER DEFAULT 0,
			rules TEXT
		);`
	}

	if _, err := db.Exec(createCollectionsQuery); err != nil {
		db.Close()
		t.Fatalf("failed to create collections table: %v", err)
	}

	return db
}

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

func TestTimeHelpers(t *testing.T) {
	// Test msToTimeStr and parseMsFromDBStr cycle
	nowMs := time.Now().UnixNano() / int64(time.Millisecond)
	timeStr := msToTimeStr(nowMs)
	parsedMs := parseMsFromDBStr(timeStr)

	// Since millisecond resolution is kept, nowMs and parsedMs should match.
	if nowMs != parsedMs {
		t.Errorf("expected original ms %d to match parsed ms %d (timeStr: %s)", nowMs, parsedMs, timeStr)
	}

	// Test invalid time strings return 0 without panicking
	if parsed := parseMsFromDBStr("invalid-time"); parsed != 0 {
		t.Errorf("expected 0 for invalid time, got %d", parsed)
	}
}

func TestSmartCollections(t *testing.T) {
	db := setupDB(t, true)
	defer db.Close()

	ctx := context.Background()
	m := NewPlaylistManager(db)

	// Insert test books
	_, err := db.ExecContext(ctx, `
		INSERT INTO books (id, title, subtitle, description, genres, tags, narrators, publishedYear) VALUES
		('book-1', 'Harry Potter and the Philosopher Stone', 'Book 1', 'A wizard boy', '["Fantasy", "Adventure"]', '["Fav", "Classic"]', '["Stephen Fry"]', '1997'),
		('book-2', 'The Hobbit', 'Pre-LOTR', 'A hobbit journey', '["Fantasy"]', '["Classic"]', '["Rob Inglis"]', '1937')
	`)
	if err != nil {
		t.Fatalf("failed to insert test books: %v", err)
	}

	// Insert libraryItems
	_, err = db.ExecContext(ctx, `
		INSERT INTO libraryItems (id, libraryId, mediaId, mediaType, isMissing, isInvalid) VALUES
		('li-1', 'lib-1', 'book-1', 'book', 0, 0),
		('li-2', 'lib-1', 'book-2', 'book', 0, 0)
	`)
	if err != nil {
		t.Fatalf("failed to insert test library items: %v", err)
	}

	// Insert author
	_, err = db.ExecContext(ctx, `
		INSERT INTO authors (id, name) VALUES ('auth-1', 'J.K. Rowling')
	`)
	if err != nil {
		t.Fatalf("failed to insert author: %v", err)
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO bookAuthors (bookId, authorId) VALUES ('book-1', 'auth-1')
	`)
	if err != nil {
		t.Fatalf("failed to insert bookAuthor relation: %v", err)
	}

	// Create a Smart Collection with Genre rule: Fantasy
	c1 := &Collection{
		ID:          "coll-smart-1",
		Name:        "Smart Fantasy",
		Description: "Rules-based",
		LibraryID:   "lib-1",
		IsSmart:     true,
		Rules:       `{"genres": ["fantasy"]}`, // Case-insensitive test
	}

	if err := m.CreateCollection(ctx, c1); err != nil {
		t.Fatalf("failed to create smart collection: %v", err)
	}

	// Retrieve the collection and verify items are dynamically resolved
	retrieved, err := m.GetCollection(ctx, c1.ID)
	if err != nil {
		t.Fatalf("failed to retrieve smart collection: %v", err)
	}

	if !retrieved.IsSmart {
		t.Errorf("expected collection to be smart")
	}

	// Both books have genre "Fantasy" (case-insensitive search matches both)
	if len(retrieved.ItemIDs) != 2 {
		t.Errorf("expected 2 resolved items, got %d: %v", len(retrieved.ItemIDs), retrieved.ItemIDs)
	}

	// Now update the rules to Genre "Adventure"
	retrieved.Rules = `{"genres": ["Adventure"]}`
	if err := m.UpdateCollection(ctx, retrieved); err != nil {
		t.Fatalf("failed to update smart collection rules: %v", err)
	}

	// Retrieve again and verify only book-1 is resolved
	retrieved2, err := m.GetCollection(ctx, c1.ID)
	if err != nil {
		t.Fatalf("failed to retrieve smart collection after update: %v", err)
	}

	if len(retrieved2.ItemIDs) != 1 || retrieved2.ItemIDs[0] != "book-1" {
		t.Errorf("expected only book-1 (Adventure), got %v", retrieved2.ItemIDs)
	}

	// Test case-insensitive Author Name matching
	retrieved2.Rules = `{"authors": ["j.k. rowling"]}`
	if err := m.UpdateCollection(ctx, retrieved2); err != nil {
		t.Fatalf("failed to update smart rules for author name: %v", err)
	}
	retrieved3, err := m.GetCollection(ctx, c1.ID)
	if err != nil {
		t.Fatalf("failed to retrieve after author name update: %v", err)
	}
	if len(retrieved3.ItemIDs) != 1 || retrieved3.ItemIDs[0] != "book-1" {
		t.Errorf("expected only book-1 for author name 'j.k. rowling', got %v", retrieved3.ItemIDs)
	}

	// Test Published Years filter
	retrieved3.Rules = `{"publishedYears": ["1937"]}`
	if err := m.UpdateCollection(ctx, retrieved3); err != nil {
		t.Fatalf("failed to update smart rules for published years: %v", err)
	}
	retrieved4, err := m.GetCollection(ctx, c1.ID)
	if err != nil {
		t.Fatalf("failed to retrieve after published years update: %v", err)
	}
	if len(retrieved4.ItemIDs) != 1 || retrieved4.ItemIDs[0] != "book-2" {
		t.Errorf("expected only book-2 for published year 1937, got %v", retrieved4.ItemIDs)
	}
}
