package playlist

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"testing"
)

// TestPlaylist_ConcurrentWritesSameItem_MaxOpen5 tests concurrent updates on the exact same playlist using MaxOpenConns = 5.
func TestPlaylist_ConcurrentWritesSameItem_MaxOpen5(t *testing.T) {
	db, _ := setupRealStressDB(t, 5)
	defer db.Close()
	runConcurrentWritesSameItem(t, db)
}

// TestPlaylist_ConcurrentWritesSameItem_MaxOpen1 tests concurrent updates on the exact same playlist using MaxOpenConns = 1.
func TestPlaylist_ConcurrentWritesSameItem_MaxOpen1(t *testing.T) {
	db, _ := setupRealStressDB(t, 1)
	defer db.Close()
	runConcurrentWritesSameItem(t, db)
}

func runConcurrentWritesSameItem(t *testing.T, db *sql.DB) {
	mgr := NewPlaylistManager(db)
	ctx := context.Background()

	// Create single shared playlist
	p := &Playlist{
		Name:    "Shared Playlist",
		UserID:  "user-shared",
		ItemIDs: []string{"book-1"},
	}
	if err := mgr.CreatePlaylist(ctx, p); err != nil {
		t.Fatalf("failed to create shared playlist: %v", err)
	}

	numWorkers := 20
	var wg sync.WaitGroup
	errCh := make(chan error, numWorkers)

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for step := 0; step < 5; step++ {
				// Retrieve the playlist
				ret, err := mgr.GetPlaylist(ctx, p.ID)
				if err != nil {
					errCh <- fmt.Errorf("worker %d failed to get playlist: %w", workerID, err)
					return
				}

				// Modify and update
				ret.Name = fmt.Sprintf("Shared Playlist (worker %d, step %d)", workerID, step)
				if err := mgr.UpdatePlaylist(ctx, ret); err != nil {
					errCh <- fmt.Errorf("worker %d failed to update playlist: %w", workerID, err)
					return
				}
			}
		}(i)
	}

	wg.Wait()
	close(errCh)

	var errs []error
	for err := range errCh {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		t.Errorf("Encountered %d errors during concurrent writes on the same item:", len(errs))
		for _, err := range errs {
			t.Errorf("Error: %v", err)
		}
	}
}

// TestPlaylist_ConcurrentCreateUpdate_MaxOpen5 tests concurrent CreatePlaylist and UpdatePlaylist operations using MaxOpenConns = 5.
func TestPlaylist_ConcurrentCreateUpdate_MaxOpen5(t *testing.T) {
	db, _ := setupRealStressDB(t, 5)
	defer db.Close()
	runConcurrentCreateUpdate(t, db)
}

// TestPlaylist_ConcurrentCreateUpdate_MaxOpen1 tests concurrent CreatePlaylist and UpdatePlaylist operations using MaxOpenConns = 1.
func TestPlaylist_ConcurrentCreateUpdate_MaxOpen1(t *testing.T) {
	db, _ := setupRealStressDB(t, 1)
	defer db.Close()
	runConcurrentCreateUpdate(t, db)
}

func runConcurrentCreateUpdate(t *testing.T, db *sql.DB) {
	mgr := NewPlaylistManager(db)
	ctx := context.Background()

	// Seed some books and library items
	_, _ = db.ExecContext(ctx, "INSERT INTO books (id, title) VALUES ('book-1', 'Book One'), ('book-2', 'Book Two')")
	_, _ = db.ExecContext(ctx, "INSERT INTO libraryItems (id, libraryId, mediaId, mediaType) VALUES ('li-1', 'lib-1', 'book-1', 'book'), ('li-2', 'lib-1', 'book-2', 'book')")

	numWorkers := 30
	var wg sync.WaitGroup
	errCh := make(chan error, numWorkers*10)

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for step := 0; step < 5; step++ {
				// Create
				p := &Playlist{
					Name:    fmt.Sprintf("Playlist %d-%d", workerID, step),
					UserID:  fmt.Sprintf("user-%d", workerID),
					ItemIDs: []string{"book-1", "book-2"},
				}
				if err := mgr.CreatePlaylist(ctx, p); err != nil {
					errCh <- fmt.Errorf("worker %d failed CreatePlaylist: %w", workerID, err)
					continue
				}

				// Update
				p.Name = p.Name + " Updated"
				p.ItemIDs = []string{"book-2"}
				if err := mgr.UpdatePlaylist(ctx, p); err != nil {
					errCh <- fmt.Errorf("worker %d failed UpdatePlaylist: %w", workerID, err)
				}
			}
		}(i)
	}

	wg.Wait()
	close(errCh)

	var errs []error
	for err := range errCh {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		t.Errorf("Encountered %d errors during concurrent Create/Update operations:", len(errs))
		for _, err := range errs {
			t.Errorf("Error: %v", err)
		}
	}
}
