package playlist

import (
	"context"
	"testing"
)

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
