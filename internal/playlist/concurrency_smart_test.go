package playlist

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"testing"
)

func seedSmartCollectionBooks(t *testing.T, db *sql.DB, ctx context.Context, libraryID string, numBooks int) {
	genresList := []string{"Fantasy", "Sci-Fi", "Mystery", "Biography", "History", "Fiction", "Non-Fiction"}
	tagsList := []string{"TopPick", "Unread", "Audiobook", "BestOf2025", "Classic"}
	narratorsList := []string{"Stephen Fry", "Jim Dale", "Ray Porter", "Simon Vance", "Bahni Turpin"}
	authorsList := []string{"J.K. Rowling", "J.R.R. Tolkien", "Brandon Sanderson", "Stephen King", "Agatha Christie"}

	// Insert standard books
	for i := 1; i <= numBooks; i++ {
		bookID := fmt.Sprintf("b-%d", i)
		title := fmt.Sprintf("Book Volume %d", i)
		subtitle := fmt.Sprintf("Sub %d", i)
		description := fmt.Sprintf("Description text for book %d", i)

		// Deterministic assignments
		genre := genresList[i%len(genresList)]
		tag := tagsList[i%len(tagsList)]
		narrator := narratorsList[i%len(narratorsList)]
		author := authorsList[i%len(authorsList)]
		publishedYear := fmt.Sprintf("%d", 1950+(i%75)) // 1950 to 2024

		// Format JSON arrays
		genresJSON := fmt.Sprintf(`["%s"]`, genre)
		tagsJSON := fmt.Sprintf(`["%s"]`, tag)
		narratorsJSON := fmt.Sprintf(`["%s"]`, narrator)

		_, err := db.ExecContext(ctx, `
			INSERT INTO books (id, title, subtitle, description, genres, tags, narrators, publishedYear)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			bookID, title, subtitle, description, genresJSON, tagsJSON, narratorsJSON, publishedYear)
		if err != nil {
			t.Fatalf("failed to insert book %d: %v", i, err)
		}

		_, err = db.ExecContext(ctx, `
			INSERT INTO libraryItems (id, libraryId, mediaId, mediaType, isMissing, isInvalid)
			VALUES (?, ?, ?, 'book', 0, 0)`,
			fmt.Sprintf("li-%d", i), libraryID, bookID)
		if err != nil {
			t.Fatalf("failed to insert library item %d: %v", i, err)
		}

		// Insert author relation
		authorID := fmt.Sprintf("auth-%s", author)
		_, _ = db.ExecContext(ctx, "INSERT OR IGNORE INTO authors (id, name) VALUES (?, ?)", authorID, author)
		_, _ = db.ExecContext(ctx, "INSERT INTO bookAuthors (bookId, authorId) VALUES (?, ?)", bookID, authorID)
	}

	// 2. Insert adversarial/malformed items to test robustness of query builder
	// Book with NULL genres
	_, _ = db.ExecContext(ctx, `INSERT INTO books (id, title, genres) VALUES ('adv-1', 'Null Genres Book', NULL)`)
	_, _ = db.ExecContext(ctx, `INSERT INTO libraryItems (id, libraryId, mediaId, mediaType) VALUES ('li-adv-1', ?, 'adv-1', 'book')`, libraryID)

	// Book with empty string genres
	_, _ = db.ExecContext(ctx, `INSERT INTO books (id, title, genres) VALUES ('adv-2', 'Empty String Genres Book', '')`)
	_, _ = db.ExecContext(ctx, `INSERT INTO libraryItems (id, libraryId, mediaId, mediaType) VALUES ('li-adv-2', ?, 'adv-2', 'book')`, libraryID)

	// Book with invalid JSON string genres
	_, _ = db.ExecContext(ctx, `INSERT INTO books (id, title, genres) VALUES ('adv-3', 'Malformed JSON Genres Book', 'not-a-json-array')`)
	_, _ = db.ExecContext(ctx, `INSERT INTO libraryItems (id, libraryId, mediaId, mediaType) VALUES ('li-adv-3', ?, 'adv-3', 'book')`, libraryID)
}

// TestSmartCollection_StressAndRobustness builds a large, diverse dataset of books and runs complex queries, case-insensitivity tests, and adversarial malformed columns.
func TestSmartCollection_StressAndRobustness(t *testing.T) {
	db, _ := setupRealStressDB(t, 1)
	defer db.Close()

	mgr := NewPlaylistManager(db)
	ctx := context.Background()
	libraryID := "lib-robust"

	seedSmartCollectionBooks(t, db, ctx, libraryID, 1000)

	// 3. Define and run test queries
	t.Run("Single Genre Case Insensitive Match", func(t *testing.T) {
		rules := SmartCollectionRules{
			Genres: []string{"fAnTaSy"}, // mixed case
		}
		rulesBytes, _ := json.Marshal(rules)
		items, err := mgr.ResolveSmartCollectionItems(ctx, libraryID, string(rulesBytes))
		if err != nil {
			t.Fatalf("failed to resolve smart collection: %v", err)
		}

		// Expected matching count: 1000 / 7 (~142)
		if len(items) == 0 {
			t.Errorf("expected matched items, got 0")
		}
		t.Logf("Fantasy match found %d books", len(items))
	})

	t.Run("Multiple Combined Rules", func(t *testing.T) {
		// Genre = Sci-Fi, Author = J.R.R. Tolkien, Tag = Unread
		rules := SmartCollectionRules{
			Genres:  []string{"Sci-Fi"},
			Authors: []string{"J.R.R. Tolkien"},
			Tags:    []string{"Unread"},
		}
		rulesBytes, _ := json.Marshal(rules)
		items, err := mgr.ResolveSmartCollectionItems(ctx, libraryID, string(rulesBytes))
		if err != nil {
			t.Fatalf("failed to resolve: %v", err)
		}
		t.Logf("Combined query matched %d books", len(items))
	})

	t.Run("Search Term Matching Title", func(t *testing.T) {
		rules := SmartCollectionRules{
			Search: "Volume 500", // matches exactly book 500
		}
		rulesBytes, _ := json.Marshal(rules)
		items, err := mgr.ResolveSmartCollectionItems(ctx, libraryID, string(rulesBytes))
		if err != nil {
			t.Fatalf("failed: %v", err)
		}
		if len(items) != 1 || items[0] != "b-500" {
			t.Errorf("expected matches to contain only b-500, got %v", items)
		}
	})

	t.Run("Search Term Matching Author Case Insensitive", func(t *testing.T) {
		rules := SmartCollectionRules{
			Search: "rOwLiNg",
		}
		rulesBytes, _ := json.Marshal(rules)
		items, err := mgr.ResolveSmartCollectionItems(ctx, libraryID, string(rulesBytes))
		if err != nil {
			t.Fatalf("failed: %v", err)
		}
		// Rowling books should be matched via author search
		if len(items) == 0 {
			t.Errorf("expected Rowling books to match, got 0")
		}
		t.Logf("Search matching author Rowling found %d books", len(items))
	})

	t.Run("Empty and Spaces Rules", func(t *testing.T) {
		rules := SmartCollectionRules{
			Genres: []string{" ", ""},
			Tags:   []string{"   "},
		}
		rulesBytes, _ := json.Marshal(rules)
		items, err := mgr.ResolveSmartCollectionItems(ctx, libraryID, string(rulesBytes))
		if err != nil {
			t.Fatalf("failed: %v", err)
		}
		// Since filters are empty after trimming, all 1,000 standard books + adversarial books should match
		if len(items) < 1000 {
			t.Errorf("expected all books to match when rules are effectively empty, got %d", len(items))
		}
	})

	t.Run("Malformed JSON Query Rules", func(t *testing.T) {
		_, err := mgr.ResolveSmartCollectionItems(ctx, libraryID, "{invalid-json}")
		if err == nil {
			t.Errorf("expected error on invalid rules JSON, got nil")
		}
	})

	t.Run("Robustness Against Malformed Database Fields", func(t *testing.T) {
		// When query is genres = Sci-Fi, it should evaluate correctly even if some rows contain non-JSON values
		rules := SmartCollectionRules{
			Genres: []string{"Sci-Fi"},
		}
		rulesBytes, _ := json.Marshal(rules)
		_, err := mgr.ResolveSmartCollectionItems(ctx, libraryID, string(rulesBytes))

		// If SQLite fails with malformed JSON on 'not-a-json-array', this will return an error!
		if err != nil {
			t.Errorf("Smart collection resolution failed when DB contains malformed JSON columns: %v", err)
		}
	})
}
