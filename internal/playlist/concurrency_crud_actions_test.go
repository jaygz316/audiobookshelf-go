package playlist

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math/rand"
	"testing"
	"time"
)

func runCRUDAction0(t *testing.T, mgr *PlaylistManager, ctx context.Context, userID string, numBooks int, workerID, op int, r *rand.Rand, errCh chan<- error) {
	pName := fmt.Sprintf("Playlist %d-%d", workerID, op)
	p := &Playlist{
		Name:    pName,
		UserID:  userID,
		ItemIDs: []string{fmt.Sprintf("book-%d", r.Intn(numBooks)), fmt.Sprintf("book-%d", r.Intn(numBooks))},
	}
	if err := mgr.CreatePlaylist(ctx, p); err != nil {
		errCh <- fmt.Errorf("worker %d failed CreatePlaylist: %w", workerID, err)
		return
	}

	// Verify read
	ret, err := mgr.GetPlaylist(ctx, p.ID)
	if err != nil {
		errCh <- fmt.Errorf("worker %d failed GetPlaylist: %w", workerID, err)
	} else if ret.Name != pName {
		errCh <- fmt.Errorf("worker %d got wrong playlist name %q, expected %q", workerID, ret.Name, pName)
	}

	// Update
	ret.Name = pName + " Updated"
	ret.ItemIDs = append(ret.ItemIDs, fmt.Sprintf("book-%d", r.Intn(numBooks)))
	if err := mgr.UpdatePlaylist(ctx, ret); err != nil {
		errCh <- fmt.Errorf("worker %d failed UpdatePlaylist: %w", workerID, err)
	}

	// Delete half the time
	if r.Float32() < 0.5 {
		if err := mgr.DeletePlaylist(ctx, p.ID); err != nil {
			errCh <- fmt.Errorf("worker %d failed DeletePlaylist: %w", workerID, err)
		}
	}
}

func runCRUDAction1(t *testing.T, mgr *PlaylistManager, ctx context.Context, libraryID string, numBooks int, workerID, op int, r *rand.Rand, errCh chan<- error) {
	cName := fmt.Sprintf("Collection %d-%d", workerID, op)
	c := &Collection{
		Name:         cName,
		Description:  "Static collection description",
		LibraryID:    libraryID,
		DisplayOrder: r.Intn(100),
		ItemIDs:      []string{fmt.Sprintf("book-%d", r.Intn(numBooks))},
	}
	if err := mgr.CreateCollection(ctx, c); err != nil {
		errCh <- fmt.Errorf("worker %d failed CreateCollection static: %w", workerID, err)
		return
	}

	// Read
	ret, err := mgr.GetCollection(ctx, c.ID)
	if err != nil {
		errCh <- fmt.Errorf("worker %d failed GetCollection static: %w", workerID, err)
	} else if ret.Name != cName {
		errCh <- fmt.Errorf("worker %d got wrong collection name %q, expected %q", workerID, ret.Name, cName)
	}

	// Update
	ret.Name = cName + " Updated"
	ret.ItemIDs = append(ret.ItemIDs, fmt.Sprintf("book-%d", r.Intn(numBooks)))
	if err := mgr.UpdateCollection(ctx, ret); err != nil {
		errCh <- fmt.Errorf("worker %d failed UpdateCollection static: %w", workerID, err)
	}

	// Delete half the time
	if r.Float32() < 0.5 {
		if err := mgr.DeleteCollection(ctx, c.ID); err != nil {
			errCh <- fmt.Errorf("worker %d failed DeleteCollection static: %w", workerID, err)
		}
	}
}

func runCRUDAction2(t *testing.T, mgr *PlaylistManager, ctx context.Context, libraryID string, workerID, op int, errCh chan<- error) {
	cName := fmt.Sprintf("Smart Collection %d-%d", workerID, op)
	rules := SmartCollectionRules{
		Genres: []string{"CommonGenre"},
	}
	rulesBytes, _ := json.Marshal(rules)
	c := &Collection{
		Name:      cName,
		LibraryID: libraryID,
		IsSmart:   true,
		Rules:     string(rulesBytes),
	}
	if err := mgr.CreateCollection(ctx, c); err != nil {
		errCh <- fmt.Errorf("worker %d failed CreateCollection smart: %w", workerID, err)
		return
	}

	// Get & resolve
	ret, err := mgr.GetCollection(ctx, c.ID)
	if err != nil {
		errCh <- fmt.Errorf("worker %d failed GetCollection smart: %w", workerID, err)
	}

	// Update rules
	newRules := SmartCollectionRules{
		Genres:  []string{"CommonGenre"},
		Authors: []string{"Author Name 0"},
	}
	newRulesBytes, _ := json.Marshal(newRules)
	ret.Rules = string(newRulesBytes)
	if err := mgr.UpdateCollection(ctx, ret); err != nil {
		errCh <- fmt.Errorf("worker %d failed UpdateCollection smart: %w", workerID, err)
	}

	// Delete
	if err := mgr.DeleteCollection(ctx, c.ID); err != nil {
		errCh <- fmt.Errorf("worker %d failed DeleteCollection smart: %w", workerID, err)
	}
}

func runCRUDAction3(t *testing.T, mgr *PlaylistManager, ctx context.Context, userID, libraryID string, workerID int, errCh chan<- error) {
	// Verify concurrent reads don't block
	rows1, err := mgr.db.QueryContext(ctx, "SELECT id FROM playlists WHERE userId = ?", userID)
	if err != nil {
		errCh <- fmt.Errorf("worker %d failed concurrent query playlists: %w", workerID, err)
	} else {
		rows1.Close()
	}

	rows2, err := mgr.db.QueryContext(ctx, "SELECT id FROM collections WHERE libraryId = ?", libraryID)
	if err != nil {
		errCh <- fmt.Errorf("worker %d failed concurrent query collections: %w", workerID, err)
	} else {
		rows2.Close()
	}
}

func runCRUDAction4(t *testing.T, db *sql.DB, ctx context.Context, userID string, workerID int) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Logf("Expected concurrency conflict: worker %d failed to begin tx: %v", workerID, err)
		return
	}
	var count int
	err = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM playlists WHERE userId = ?", userID).Scan(&count)
	if err != nil {
		tx.Rollback()
		t.Logf("Expected concurrency conflict: worker %d failed to query in tx: %v", workerID, err)
		return
	}

	// Perform a dummy update to escalate lock
	_, err = tx.ExecContext(ctx, "UPDATE playlists SET updatedAt = ? WHERE userId = ?", time.Now().Format("2006-01-02 15:04:05.000"), userID)
	if err != nil {
		tx.Rollback()
		t.Logf("Expected concurrency conflict: worker %d failed to update in tx: %v", workerID, err)
		return
	}
	tx.Commit()
}

func runCRUDAction5(t *testing.T, mgr *PlaylistManager, ctx context.Context, libraryID string, workerID int, errCh chan<- error) {
	rules := SmartCollectionRules{
		Tags: []string{"Tag-0"},
	}
	rulesBytes, _ := json.Marshal(rules)
	_, err := mgr.ResolveSmartCollectionItems(ctx, libraryID, string(rulesBytes))
	if err != nil {
		errCh <- fmt.Errorf("worker %d failed ResolveSmartCollectionItems: %w", workerID, err)
	}
}
