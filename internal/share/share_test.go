package share

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite"
)

func setupTestDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	return db
}

func TestNewShareManager(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Constructing ShareManager should execute CREATE TABLE query
	sm := NewShareManager(db)
	if sm == nil {
		t.Fatal("expected non-nil ShareManager")
	}

	// Verify table "shares" exists
	var name string
	err := db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='shares'").Scan(&name)
	if err != nil {
		t.Fatalf("failed to find 'shares' table: %v", err)
	}
	if name != "shares" {
		t.Errorf("expected table 'shares', got %q", name)
	}
}

func TestCreateShare(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	sm := NewShareManager(db)
	ctx := context.Background()

	t.Run("valid share", func(t *testing.T) {
		s := &ShareLink{
			ID:             "share-1",
			LibraryItemID:  "item-1",
			CreatedBy:      "user-1",
			IsDownloadable: true,
			PasswordHash:   "some-hash",
		}

		err := sm.CreateShare(ctx, s)
		if err != nil {
			t.Fatalf("unexpected error creating share: %v", err)
		}

		// Read back directly to verify timestamps are populated
		if s.CreatedAt.IsZero() {
			t.Error("expected CreatedAt to be non-zero")
		}
		if s.UpdatedAt.IsZero() {
			t.Error("expected UpdatedAt to be non-zero")
		}
	})

	t.Run("empty ID should fail", func(t *testing.T) {
		s := &ShareLink{
			ID:            "",
			LibraryItemID: "item-1",
		}
		err := sm.CreateShare(ctx, s)
		if err == nil {
			t.Error("expected error due to empty share ID, got nil")
		}
	})
}

func TestGetShare(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	sm := NewShareManager(db)
	ctx := context.Background()

	// Seed a share
	now := time.Now().UTC().Truncate(time.Millisecond)
	expires := now.Add(1 * time.Hour)
	s := &ShareLink{
		ID:             "share-1",
		LibraryItemID:  "item-1",
		CreatedBy:      "user-1",
		IsDownloadable: true,
		ExpiresAt:      expires,
		PasswordHash:   "hash-123",
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	err := sm.CreateShare(ctx, s)
	if err != nil {
		t.Fatalf("failed to seed share: %v", err)
	}

	t.Run("retrieve existing", func(t *testing.T) {
		retrieved, err := sm.GetShare(ctx, "share-1")
		if err != nil {
			t.Fatalf("unexpected error retrieving share: %v", err)
		}
		if retrieved == nil {
			t.Fatal("expected share to be found")
		}

		if retrieved.ID != s.ID {
			t.Errorf("expected ID %q, got %q", s.ID, retrieved.ID)
		}
		if retrieved.LibraryItemID != s.LibraryItemID {
			t.Errorf("expected LibraryItemID %q, got %q", s.LibraryItemID, retrieved.LibraryItemID)
		}
		if retrieved.CreatedBy != s.CreatedBy {
			t.Errorf("expected CreatedBy %q, got %q", s.CreatedBy, retrieved.CreatedBy)
		}
		if retrieved.IsDownloadable != s.IsDownloadable {
			t.Errorf("expected IsDownloadable %v, got %v", s.IsDownloadable, retrieved.IsDownloadable)
		}
		if retrieved.PasswordHash != s.PasswordHash {
			t.Errorf("expected PasswordHash %q, got %q", s.PasswordHash, retrieved.PasswordHash)
		}
		// Compare times, allowing for tiny string formatting parsing differences if any
		if !retrieved.ExpiresAt.Equal(s.ExpiresAt) {
			t.Errorf("expected ExpiresAt %v, got %v", s.ExpiresAt, retrieved.ExpiresAt)
		}
		if !retrieved.CreatedAt.Equal(s.CreatedAt) {
			t.Errorf("expected CreatedAt %v, got %v", s.CreatedAt, retrieved.CreatedAt)
		}
		if !retrieved.UpdatedAt.Equal(s.UpdatedAt) {
			t.Errorf("expected UpdatedAt %v, got %v", s.UpdatedAt, retrieved.UpdatedAt)
		}
	})

	t.Run("retrieve non-existent", func(t *testing.T) {
		retrieved, err := sm.GetShare(ctx, "share-nonexistent")
		if err != nil {
			t.Fatalf("unexpected error retrieving non-existent share: %v", err)
		}
		if retrieved != nil {
			t.Errorf("expected nil share, got %v", retrieved)
		}
	})
}

func TestValidateSharePassword(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	sm := NewShareManager(db)
	ctx := context.Background()

	// 1. Share with no password configured (empty hash)
	sNoPass := &ShareLink{
		ID:            "share-nopass",
		LibraryItemID: "item-1",
		PasswordHash:  "",
	}
	if err := sm.CreateShare(ctx, sNoPass); err != nil {
		t.Fatalf("failed to create share: %v", err)
	}

	// 2. Share with bcrypt password configured
	password := "secure-password"
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}

	sWithPass := &ShareLink{
		ID:            "share-pass",
		LibraryItemID: "item-2",
		PasswordHash:  string(hashed),
	}
	if err := sm.CreateShare(ctx, sWithPass); err != nil {
		t.Fatalf("failed to create share: %v", err)
	}

	tests := []struct {
		name       string
		shareID    string
		password   string
		wantResult bool
	}{
		{
			name:       "no password share - correct validation",
			shareID:    "share-nopass",
			password:   "",
			wantResult: true,
		},
		{
			name:       "no password share - any password passes",
			shareID:    "share-nopass",
			password:   "anything",
			wantResult: true,
		},
		{
			name:       "password share - correct password",
			shareID:    "share-pass",
			password:   "secure-password",
			wantResult: true,
		},
		{
			name:       "password share - incorrect password",
			shareID:    "share-pass",
			password:   "wrong-password",
			wantResult: false,
		},
		{
			name:       "non-existent share - returns false",
			shareID:    "share-nonexistent",
			password:   "whatever",
			wantResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := sm.ValidateSharePassword(ctx, tt.shareID, tt.password)
			if err != nil {
				t.Fatalf("unexpected error validating password: %v", err)
			}
			if got != tt.wantResult {
				t.Errorf("ValidateSharePassword() = %v, want %v", got, tt.wantResult)
			}
		})
	}
}

func TestDeleteShare(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	sm := NewShareManager(db)
	ctx := context.Background()

	s := &ShareLink{
		ID:            "share-to-delete",
		LibraryItemID: "item-1",
	}
	if err := sm.CreateShare(ctx, s); err != nil {
		t.Fatalf("failed to create share: %v", err)
	}

	// Verify it exists first
	got, err := sm.GetShare(ctx, "share-to-delete")
	if err != nil || got == nil {
		t.Fatalf("expected share to exist, got error: %v, share: %v", err, got)
	}

	// Delete it
	if err := sm.DeleteShare(ctx, "share-to-delete"); err != nil {
		t.Fatalf("unexpected error deleting share: %v", err)
	}

	// Verify it is gone
	got2, err := sm.GetShare(ctx, "share-to-delete")
	if err != nil {
		t.Fatalf("unexpected error fetching deleted share: %v", err)
	}
	if got2 != nil {
		t.Error("expected share to be deleted, but it was found")
	}

	// Verify database row is gone
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM shares WHERE id = 'share-to-delete'").Scan(&count)
	if err != nil {
		t.Fatalf("failed to query shares: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 rows in database, got %d", count)
	}
}

func TestExpiredShareAutoCleanup(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	sm := NewShareManager(db)
	ctx := context.Background()

	t.Run("expired share gets deleted on get", func(t *testing.T) {
		// Expires 10 seconds ago
		expiredTime := time.Now().Add(-10 * time.Second).Truncate(time.Millisecond)
		s := &ShareLink{
			ID:            "expired-share",
			LibraryItemID: "item-1",
			ExpiresAt:     expiredTime,
		}
		if err := sm.CreateShare(ctx, s); err != nil {
			t.Fatalf("failed to create share: %v", err)
		}

		// Retrieve it - should return nil, nil and trigger deletion
		retrieved, err := sm.GetShare(ctx, "expired-share")
		if err != nil {
			t.Fatalf("unexpected error retrieving share: %v", err)
		}
		if retrieved != nil {
			t.Fatalf("expected nil for expired share, got %+v", retrieved)
		}

		// Check the database directly to confirm the row was actually deleted
		var count int
		err = db.QueryRow("SELECT COUNT(*) FROM shares WHERE id = 'expired-share'").Scan(&count)
		if err != nil {
			t.Fatalf("failed to query database: %v", err)
		}
		if count != 0 {
			t.Error("expected expired share row to be deleted from database, but it still exists")
		}
	})

	t.Run("active share does not get deleted on get", func(t *testing.T) {
		// Expires in 1 hour
		activeTime := time.Now().Add(1 * time.Hour).Truncate(time.Millisecond)
		s := &ShareLink{
			ID:            "active-share",
			LibraryItemID: "item-2",
			ExpiresAt:     activeTime,
		}
		if err := sm.CreateShare(ctx, s); err != nil {
			t.Fatalf("failed to create share: %v", err)
		}

		// Retrieve it - should return the share
		retrieved, err := sm.GetShare(ctx, "active-share")
		if err != nil {
			t.Fatalf("unexpected error retrieving share: %v", err)
		}
		if retrieved == nil {
			t.Fatal("expected non-nil for active share")
		}

		// Check the database directly to confirm row still exists
		var count int
		err = db.QueryRow("SELECT COUNT(*) FROM shares WHERE id = 'active-share'").Scan(&count)
		if err != nil {
			t.Fatalf("failed to query database: %v", err)
		}
		if count != 1 {
			t.Errorf("expected active share row to exist in database, got count %d", count)
		}
	})
}
