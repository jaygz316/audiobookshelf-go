package feed

import (
	"context"
	"strings"
	"sync"
	"testing"
)

func TestCheckUserAccess_Concurrency(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Insert several users
	insertUser(t, db, "admin_user", "admin", "admin", 1, "")
	insertUser(t, db, "inactive_user", "inactive", "user", 0, `{"accessAllLibraries": true}`)
	insertUser(t, db, "no_perms_user", "noperms", "user", 1, "")
	insertUser(t, db, "all_libs_user", "alllibs", "user", 1, `{"accessAllLibraries": true}`)
	insertUser(t, db, "spec_lib_user", "speclib", "user", 1, `{"librariesAccessible": ["lib1"]}`)

	manager := NewFeedManager(db, t.TempDir())

	ctx := context.Background()

	const goroutines = 100
	const iterations = 50

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				// 1. Admin user should have access
				ok, err := manager.checkUserAccess(ctx, "admin_user", "lib1")
				if err != nil || !ok {
					t.Errorf("worker %d: expected admin to have access, err: %v, ok: %t", workerID, err, ok)
				}

				// 2. Inactive user should fail with inactive error
				ok, err = manager.checkUserAccess(ctx, "inactive_user", "lib1")
				if err == nil || !strings.Contains(err.Error(), "user is inactive") || ok {
					t.Errorf("worker %d: expected inactive user to fail, err: %v, ok: %t", workerID, err, ok)
				}

				// 3. User with no permissions should not have access
				ok, err = manager.checkUserAccess(ctx, "no_perms_user", "lib1")
				if err != nil || ok {
					t.Errorf("worker %d: expected no_perms_user to have no access, err: %v, ok: %t", workerID, err, ok)
				}

				// 4. User with accessAllLibraries should have access
				ok, err = manager.checkUserAccess(ctx, "all_libs_user", "lib1")
				if err != nil || !ok {
					t.Errorf("worker %d: expected all_libs_user to have access, err: %v, ok: %t", workerID, err, ok)
				}

				// 5. User with specific library access (lib1) should have access to lib1, but not lib2
				ok, err = manager.checkUserAccess(ctx, "spec_lib_user", "lib1")
				if err != nil || !ok {
					t.Errorf("worker %d: expected spec_lib_user to have access to lib1, err: %v, ok: %t", workerID, err, ok)
				}

				ok, err = manager.checkUserAccess(ctx, "spec_lib_user", "lib2")
				if err != nil || ok {
					t.Errorf("worker %d: expected spec_lib_user to not have access to lib2, err: %v, ok: %t", workerID, err, ok)
				}

				// 6. Non-existent user should fail with user not found
				ok, err = manager.checkUserAccess(ctx, "non_existent_user", "lib1")
				if err == nil || !strings.Contains(err.Error(), "user not found") || ok {
					t.Errorf("worker %d: expected non-existent user to fail, err: %v, ok: %t", workerID, err, ok)
				}
			}
		}(i)
	}

	wg.Wait()
}
