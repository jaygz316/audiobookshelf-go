package playlist

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"
)

// TestPlaylist_Concurrency_MaxOpen1 runs concurrent operations using MaxOpenConns = 1.
// Since Go serializes operations through 1 connection, there should be zero database lock/busy errors.
func TestPlaylist_Concurrency_MaxOpen1(t *testing.T) {
	db, _ := setupRealStressDB(t, 1)
	defer db.Close()

	runConcurrentCRUD(t, db, "user-1", "lib-1")
}

// TestPlaylist_Concurrency_MaxOpen5 runs concurrent operations using MaxOpenConns = 5.
// This tests how SQLite's WAL mode and busy_timeout handle concurrent writers when multiple
// database connections are active in the pool.
func TestPlaylist_Concurrency_MaxOpen5(t *testing.T) {
	db, _ := setupRealStressDB(t, 5)
	defer db.Close()

	runConcurrentCRUD(t, db, "user-2", "lib-2")
}

func runConcurrentCRUD(t *testing.T, db *sql.DB, userID, libraryID string) {
	mgr := NewPlaylistManager(db)
	ctx := context.Background()

	// Seed some books, authors, and library items for smart collection resolution
	numBooks := 20
	for i := 0; i < numBooks; i++ {
		bookID := fmt.Sprintf("book-%d", i)
		title := fmt.Sprintf("Book Title %d", i)
		genres := fmt.Sprintf(`["Genre-%d", "CommonGenre"]`, i%3)
		tags := fmt.Sprintf(`["Tag-%d"]`, i%4)
		narrators := fmt.Sprintf(`["Narrator-%d"]`, i%5)
		publishedYear := fmt.Sprintf("%d", 2000+i)

		_, err := db.ExecContext(ctx, `
			INSERT INTO books (id, title, subtitle, description, genres, tags, narrators, publishedYear)
			VALUES (?, ?, 'Subtitle', 'Description of book', ?, ?, ?, ?)`,
			bookID, title, genres, tags, narrators, publishedYear)
		if err != nil {
			t.Fatalf("failed to insert book: %v", err)
		}

		_, err = db.ExecContext(ctx, `
			INSERT INTO libraryItems (id, libraryId, mediaId, mediaType, isMissing, isInvalid)
			VALUES (?, ?, ?, 'book', 0, 0)`,
			fmt.Sprintf("li-%d", i), libraryID, bookID)
		if err != nil {
			t.Fatalf("failed to insert libraryItem: %v", err)
		}

		// Insert author
		authorID := fmt.Sprintf("author-%d", i%2)
		authorName := fmt.Sprintf("Author Name %d", i%2)
		_, _ = db.ExecContext(ctx, "INSERT OR IGNORE INTO authors (id, name) VALUES (?, ?)", authorID, authorName)
		_, _ = db.ExecContext(ctx, "INSERT INTO bookAuthors (bookId, authorId) VALUES (?, ?)", bookID, authorID)
	}

	numWorkers := 30
	var wg sync.WaitGroup
	errCh := make(chan error, numWorkers*10)

	// We will launch concurrent workers doing operations on the same user/library
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			r := rand.New(rand.NewSource(time.Now().UnixNano() + int64(workerID)))

			for op := 0; op < 10; op++ {
				action := r.Intn(6)
				switch action {
				case 0:
					runCRUDAction0(t, mgr, ctx, userID, numBooks, workerID, op, r, errCh)
				case 1:
					runCRUDAction1(t, mgr, ctx, libraryID, numBooks, workerID, op, r, errCh)
				case 2:
					runCRUDAction2(t, mgr, ctx, libraryID, workerID, op, errCh)
				case 3:
					runCRUDAction3(t, mgr, ctx, userID, libraryID, workerID, errCh)
				case 4:
					runCRUDAction4(t, db, ctx, userID, workerID)
				case 5:
					runCRUDAction5(t, mgr, ctx, libraryID, workerID, errCh)
				}
			}
		}(w)
	}

	wg.Wait()
	close(errCh)

	var errs []error
	for err := range errCh {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		t.Errorf("Encountered %d errors during concurrency stress test:", len(errs))
		for _, err := range errs {
			t.Errorf("Error: %v", err)
		}
	}
}
