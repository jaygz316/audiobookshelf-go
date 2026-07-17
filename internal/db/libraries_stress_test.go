package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math/rand"
	"path/filepath"
	"sync"
	"testing"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

func setupLibrariesStressDB(t *testing.T) *sql.DB {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := InitDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to initialize test DB: %v", err)
	}
	db.SetMaxOpenConns(1)
	return db
}

// TestLibraries_Create_Concurrency tests concurrent calls to CreateLibrary.
func TestLibraries_Create_Concurrency(t *testing.T) {
	db := setupLibrariesStressDB(t)
	defer db.Close()

	numWorkers := 20
	var wg sync.WaitGroup
	errCh := make(chan error, numWorkers)
	createdLibs := make(chan *LibraryJSON, numWorkers)

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			payload := &CreateLibraryPayload{
				Name:      fmt.Sprintf("Library %d", idx),
				MediaType: "book",
				Icon:      "book-icon",
				Provider:  "google",
				Settings: map[string]interface{}{
					"coverAspectRatio": float64(1),
					"disableWatcher":   false,
				},
				Folders: []CreateFolderPayload{
					{Path: fmt.Sprintf("/path/to/folder/%d", idx)},
				},
			}
			lib, err := CreateLibrary(db, payload)
			if err != nil {
				errCh <- fmt.Errorf("worker %d failed to create library: %w", idx, err)
				return
			}
			createdLibs <- lib
		}(i)
	}

	wg.Wait()
	close(errCh)
	close(createdLibs)

	for err := range errCh {
		t.Error(err)
	}

	var libs []*LibraryJSON
	for lib := range createdLibs {
		libs = append(libs, lib)
	}

	if len(libs) != numWorkers {
		t.Errorf("Expected %d libraries, got %d", numWorkers, len(libs))
	}

	// Verify they are all in the database and displayOrder is correct
	dbLibs, err := GetLibraries(db)
	if err != nil {
		t.Fatalf("Failed to get libraries: %v", err)
	}

	if len(dbLibs) != numWorkers {
		t.Errorf("Expected %d libraries in DB, got %d", numWorkers, len(dbLibs))
	}

	// Check displayOrder is sequential and unique
	orders := make(map[int]bool)
	for _, l := range dbLibs {
		if orders[l.DisplayOrder] {
			t.Errorf("Duplicate displayOrder: %d", l.DisplayOrder)
		}
		orders[l.DisplayOrder] = true
	}
}

// TestLibraries_Update_Concurrency tests concurrent calls to UpdateLibrary.
func TestLibraries_Update_Concurrency(t *testing.T) {
	db := setupLibrariesStressDB(t)
	defer db.Close()

	// Initial library setup
	payload := &CreateLibraryPayload{
		Name:      "Initial Name",
		MediaType: "book",
		Icon:      "icon",
		Provider:  "google",
		Folders: []CreateFolderPayload{
			{Path: "/initial/folder"},
		},
	}
	lib, err := CreateLibrary(db, payload)
	if err != nil {
		t.Fatalf("Failed to create initial library: %v", err)
	}

	numWorkers := 30
	var wg sync.WaitGroup
	errCh := make(chan error, numWorkers)

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			newName := fmt.Sprintf("Updated Name %d", idx)
			newProvider := fmt.Sprintf("provider-%d", idx)
			updatePayload := &UpdateLibraryPayload{
				Name:     &newName,
				Provider: &newProvider,
				Settings: map[string]interface{}{
					"disableWatcher: ": true,
				},
			}
			_, err := UpdateLibrary(db, lib.ID, updatePayload)
			if err != nil {
				errCh <- fmt.Errorf("worker %d failed to update library: %w", idx, err)
			}
		}(i)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Error(err)
	}

	// Verify the final state is updated
	updatedLib, err := GetLibraryByID(db, lib.ID)
	if err != nil {
		t.Fatalf("Failed to get library by ID: %v", err)
	}
	if updatedLib == nil {
		t.Fatalf("Library not found after updates")
	}
	t.Logf("Final updated library name: %s", updatedLib.Name)
}

// TestLibraries_Delete_Concurrency tests concurrent deletes.
func TestLibraries_Delete_Concurrency(t *testing.T) {
	db := setupLibrariesStressDB(t)
	defer db.Close()

	numLibs := 15
	var libIDs []string
	for i := 0; i < numLibs; i++ {
		payload := &CreateLibraryPayload{
			Name:      fmt.Sprintf("Lib to delete %d", i),
			MediaType: "book",
			Folders: []CreateFolderPayload{
				{Path: fmt.Sprintf("/delete/%d", i)},
			},
		}
		lib, err := CreateLibrary(db, payload)
		if err != nil {
			t.Fatalf("Failed to setup library %d: %v", i, err)
		}
		libIDs = append(libIDs, lib.ID)
	}

	var wg sync.WaitGroup
	errCh := make(chan error, numLibs)

	for _, id := range libIDs {
		wg.Add(1)
		go func(libID string) {
			defer wg.Done()
			_, err := DeleteLibrary(db, libID)
			if err != nil {
				errCh <- fmt.Errorf("failed to delete library %s: %w", libID, err)
			}
		}(id)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Error(err)
	}

	// Verify they are all deleted
	dbLibs, err := GetLibraries(db)
	if err != nil {
		t.Fatalf("Failed to get libraries: %v", err)
	}
	if len(dbLibs) != 0 {
		t.Errorf("Expected 0 libraries left, got %d", len(dbLibs))
	}
}

// TestLibraries_MixedOps_Concurrency runs a heavy mix of concurrent operations.
func TestLibraries_MixedOps_Concurrency(t *testing.T) {
	db := setupLibrariesStressDB(t)
	defer db.Close()

	// Setup initial libraries
	numInitial := 5
	var initialIDs []string
	for i := 0; i < numInitial; i++ {
		payload := &CreateLibraryPayload{
			Name:      fmt.Sprintf("Mixed Lib %d", i),
			MediaType: "book",
			Folders: []CreateFolderPayload{
				{Path: fmt.Sprintf("/mixed/%d", i)},
			},
		}
		lib, err := CreateLibrary(db, payload)
		if err != nil {
			t.Fatalf("Failed to set up: %v", err)
		}
		initialIDs = append(initialIDs, lib.ID)
	}

	numWorkers := 40
	var wg sync.WaitGroup
	errCh := make(chan error, numWorkers)

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			// Randomly perform different operations
			op := rand.Intn(5)
			switch op {
			case 0: // Read all
				_, err := GetLibraries(db)
				if err != nil {
					errCh <- fmt.Errorf("get libraries failed: %w", err)
				}
			case 1: // Read single
				id := initialIDs[rand.Intn(len(initialIDs))]
				_, err := GetLibraryByID(db, id)
				if err != nil {
					errCh <- fmt.Errorf("get library by id %s failed: %w", id, err)
				}
			case 2: // Create new
				payload := &CreateLibraryPayload{
					Name:      fmt.Sprintf("Dynamic Lib %d", idx),
					MediaType: "podcast",
				}
				_, err := CreateLibrary(db, payload)
				if err != nil {
					errCh <- fmt.Errorf("create dynamic library failed: %w", err)
				}
			case 3: // Update existing
				id := initialIDs[rand.Intn(len(initialIDs))]
				newName := fmt.Sprintf("Updated Dynamic Name %d", idx)
				updatePayload := &UpdateLibraryPayload{
					Name: &newName,
				}
				_, err := UpdateLibrary(db, id, updatePayload)
				if err != nil {
					errCh <- fmt.Errorf("update library %s failed: %w", id, err)
				}
			case 4: // Query stats
				id := initialIDs[rand.Intn(len(initialIDs))]
				_, err1 := GetBookLibraryStats(db, id)
				_, err2 := GetPodcastLibraryStats(db, id)
				if err1 != nil || err2 != nil {
					errCh <- fmt.Errorf("get stats for library %s failed: %v, %v", id, err1, err2)
				}
			}
		}(i)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Error(err)
	}
}

// TestLibraries_Adversarial_Inputs verifies edge cases and validation.
func TestLibraries_Adversarial_Inputs(t *testing.T) {
	db := setupLibrariesStressDB(t)
	defer db.Close()

	t.Run("NonExistentGet", func(t *testing.T) {
		lib, err := GetLibraryByID(db, "does-not-exist")
		if err != nil {
			t.Errorf("Expected nil error for non-existent library ID, got %v", err)
		}
		if lib != nil {
			t.Errorf("Expected nil library result, got %+v", lib)
		}
	})

	t.Run("NonExistentUpdate", func(t *testing.T) {
		newName := "New Name"
		payload := &UpdateLibraryPayload{
			Name: &newName,
		}
		_, err := UpdateLibrary(db, "does-not-exist", payload)
		if err == nil {
			t.Error("Expected error when updating non-existent library, got nil")
		} else if err.Error() != "library not found" {
			t.Errorf("Expected 'library not found' error, got: %v", err)
		}
	})

	t.Run("NonExistentDelete", func(t *testing.T) {
		_, err := DeleteLibrary(db, "does-not-exist")
		if err == nil {
			t.Error("Expected error when deleting non-existent library, got nil")
		} else if err.Error() != "library not found" {
			t.Errorf("Expected 'library not found' error, got: %v", err)
		}
	})

	t.Run("InvalidSettingsValidation", func(t *testing.T) {
		// Valid settings input should succeed, but invalid settings should trigger error from mergeSettings
		payload := &CreateLibraryPayload{
			Name:      "Lib with Bad Settings",
			MediaType: "book",
			Settings: map[string]interface{}{
				"metadataPrecedence": "not-an-array", // expected []interface{} of strings
			},
		}
		_, err := CreateLibrary(db, payload)
		if err == nil {
			t.Error("Expected error when setting invalid settings field, got nil")
		}
	})

	t.Run("SQLInjectionDefense", func(t *testing.T) {
		// Attempt SQL injections in Name, Provider, MediaType, Icon, path
		payload := &CreateLibraryPayload{
			Name:      "Library name'; DROP TABLE libraries; --",
			MediaType: "book",
			Icon:      "icon'; SELECT * FROM users; --",
			Provider:  "provider'); --",
			Folders: []CreateFolderPayload{
				{Path: "path'; --"},
			},
		}
		lib, err := CreateLibrary(db, payload)
		if err != nil {
			t.Fatalf("Expected no error (SQL parameters should escape), got %v", err)
		}

		// Ensure library and folders were created with the literal injection strings, not executed
		if lib.Name != payload.Name {
			t.Errorf("Expected literal name %q, got %q", payload.Name, lib.Name)
		}

		var count int
		err = db.QueryRow("SELECT COUNT(*) FROM libraries WHERE name = ?", payload.Name).Scan(&count)
		if err != nil {
			t.Fatalf("Failed to query DB: %v", err)
		}
		if count != 1 {
			t.Errorf("Expected 1 library with literal injection name, got %d", count)
		}
	})
}

// TestLibraries_BookStats verifies correctness of GetBookLibraryStats calculation.
func TestLibraries_BookStats(t *testing.T) {
	db := setupLibrariesStressDB(t)
	defer db.Close()

	// Create library
	payload := &CreateLibraryPayload{
		Name:      "Stats Book Lib",
		MediaType: "book",
	}
	lib, err := CreateLibrary(db, payload)
	if err != nil {
		t.Fatalf("Failed to create library: %v", err)
	}

	// Stats on empty library
	stats, err := GetBookLibraryStats(db, lib.ID)
	if err != nil {
		t.Fatalf("Failed to get book stats: %v", err)
	}
	if stats.TotalItems != 0 || stats.TotalSize != 0 || stats.TotalDuration != 0 {
		t.Errorf("Expected zero values for empty library, got %+v", stats)
	}

	// Setup authors and books
	authorID1 := uuid.New().String()
	_, _ = db.Exec("INSERT INTO authors (id, libraryId, name) VALUES (?, ?, 'Author One')", authorID1, lib.ID)
	authorID2 := uuid.New().String()
	_, _ = db.Exec("INSERT INTO authors (id, libraryId, name) VALUES (?, ?, 'Author Two')", authorID2, lib.ID)

	// Book 1: Size = 1000, Duration = 3600, 2 audiofiles, Genre = Fiction, Author = Author One
	bookID1 := uuid.New().String()
	_, _ = db.Exec("INSERT INTO books (id, title, duration, genres, audioFiles) VALUES (?, 'Book One', 3600, '[\"Fiction\"]', '[\"file1.mp3\", \"file2.mp3\"]')", bookID1)
	_, _ = db.Exec("INSERT INTO bookAuthors (bookId, authorId) VALUES (?, ?)", bookID1, authorID1)
	_, _ = db.Exec("INSERT INTO libraryItems (id, libraryId, mediaType, mediaId, size, title) VALUES (?, ?, 'book', ?, 1000, 'Item One')", uuid.New().String(), lib.ID, bookID1)

	// Book 2: Size = 2000, Duration = 7200, 1 audiofile, Genre = Fiction, Drama, Author = Author Two
	bookID2 := uuid.New().String()
	_, _ = db.Exec("INSERT INTO books (id, title, duration, genres, audioFiles) VALUES (?, 'Book Two', 7200, '[\"Fiction\", \"Drama\"]', '[\"file3.mp3\"]')", bookID2)
	_, _ = db.Exec("INSERT INTO bookAuthors (bookId, authorId) VALUES (?, ?)", bookID2, authorID2)
	_, _ = db.Exec("INSERT INTO libraryItems (id, libraryId, mediaType, mediaId, size, title) VALUES (?, ?, 'book', ?, 2000, 'Item Two')", uuid.New().String(), lib.ID, bookID2)

	// Query stats
	stats, err = GetBookLibraryStats(db, lib.ID)
	if err != nil {
		t.Fatalf("Failed to get book stats: %v", err)
	}

	if stats.TotalItems != 2 {
		t.Errorf("Expected 2 items, got %d", stats.TotalItems)
	}
	if stats.TotalSize != 3000 {
		t.Errorf("Expected 3000 total size, got %d", stats.TotalSize)
	}
	if stats.TotalDuration != 10800 {
		t.Errorf("Expected 10800 total duration, got %f", stats.TotalDuration)
	}
	if stats.NumAudioFiles != 3 {
		t.Errorf("Expected 3 audio files, got %d", stats.NumAudioFiles)
	}
	if stats.TotalAuthors != 2 {
		t.Errorf("Expected 2 authors, got %d", stats.TotalAuthors)
	}

	// Verify genres list sorted by count descending, genre ascending
	// "Fiction" has 2 books, "Drama" has 1 book.
	if len(stats.GenresWithCount) != 2 {
		t.Errorf("Expected 2 genres, got %d", len(stats.GenresWithCount))
	} else {
		if stats.GenresWithCount[0].Genre != "Fiction" || stats.GenresWithCount[0].Count != 2 {
			t.Errorf("Expected first genre to be Fiction with count 2, got: %+v", stats.GenresWithCount[0])
		}
		if stats.GenresWithCount[1].Genre != "Drama" || stats.GenresWithCount[1].Count != 1 {
			t.Errorf("Expected second genre to be Drama with count 1, got: %+v", stats.GenresWithCount[1])
		}
	}
}

// TestLibraries_PodcastStats verifies correctness of GetPodcastLibraryStats calculation.
func TestLibraries_PodcastStats(t *testing.T) {
	db := setupLibrariesStressDB(t)
	defer db.Close()

	// Create library
	payload := &CreateLibraryPayload{
		Name:      "Stats Podcast Lib",
		MediaType: "podcast",
	}
	lib, err := CreateLibrary(db, payload)
	if err != nil {
		t.Fatalf("Failed to create library: %v", err)
	}

	// Stats on empty library
	stats, err := GetPodcastLibraryStats(db, lib.ID)
	if err != nil {
		t.Fatalf("Failed to get podcast stats: %v", err)
	}
	if stats.TotalItems != 0 || stats.TotalSize != 0 || stats.TotalDuration != 0 {
		t.Errorf("Expected zero values for empty library, got %+v", stats)
	}

	// Setup podcast and episodes
	podcastID1 := uuid.New().String()
	_, _ = db.Exec("INSERT INTO podcasts (id, title, genres) VALUES (?, 'Podcast One', '[\"Tech\"]')", podcastID1)
	_, _ = db.Exec("INSERT INTO libraryItems (id, libraryId, mediaType, mediaId, size, title) VALUES (?, ?, 'podcast', ?, 500, 'Item Podcast')", uuid.New().String(), lib.ID, podcastID1)

	// Episode 1: duration = 1800
	_, _ = db.Exec("INSERT INTO podcastEpisodes (id, podcastId, title, audioFile) VALUES (?, ?, 'Episode 1', '{\"duration\": 1800}')", uuid.New().String(), podcastID1)
	// Episode 2: duration = 2400
	_, _ = db.Exec("INSERT INTO podcastEpisodes (id, podcastId, title, audioFile) VALUES (?, ?, 'Episode 2', '{\"duration\": 2400}')", uuid.New().String(), podcastID1)

	// Query stats
	stats, err = GetPodcastLibraryStats(db, lib.ID)
	if err != nil {
		t.Fatalf("Failed to get podcast stats: %v", err)
	}

	if stats.TotalItems != 1 {
		t.Errorf("Expected 1 item, got %d", stats.TotalItems)
	}
	if stats.TotalSize != 500 {
		t.Errorf("Expected 500 total size, got %d", stats.TotalSize)
	}
	if stats.TotalDuration != 4200 {
		t.Errorf("Expected 4200 total duration, got %f", stats.TotalDuration)
	}
	if stats.NumAudioFiles != 2 {
		t.Errorf("Expected 2 audio files, got %d", stats.NumAudioFiles)
	}
	if len(stats.GenresWithCount) != 1 || stats.GenresWithCount[0].Genre != "Tech" || stats.GenresWithCount[0].Count != 1 {
		t.Errorf("Expected Tech genre with count 1, got %+v", stats.GenresWithCount)
	}
}

// TestLibraries_Update_Folders validates path additions, deletions, updates.
func TestLibraries_Update_Folders(t *testing.T) {
	db := setupLibrariesStressDB(t)
	defer db.Close()

	// Initial library setup with folders
	payload := &CreateLibraryPayload{
		Name:      "Folder Update Lib",
		MediaType: "book",
		Folders: []CreateFolderPayload{
			{Path: "/initial/folder1"},
			{Path: "/initial/folder2"},
		},
	}
	lib, err := CreateLibrary(db, payload)
	if err != nil {
		t.Fatalf("Failed to create initial library: %v", err)
	}

	if len(lib.Folders) != 2 {
		t.Fatalf("Expected 2 initial folders, got %d", len(lib.Folders))
	}

	// Setup library items for the folders to test deletion cascades
	folder1ID := lib.Folders[0].ID
	folder2ID := lib.Folders[1].ID

	bookID := uuid.New().String()
	_, _ = db.Exec("INSERT INTO books (id, title) VALUES (?, 'Book')", bookID)
	// Item under folder 2
	_, _ = db.Exec("INSERT INTO libraryItems (id, libraryId, mediaType, mediaId, libraryFolderId, title) VALUES ('item-f2', ?, 'book', ?, ?, 'Item')", lib.ID, bookID, folder2ID)

	// Update library: delete folder 2, keep folder 1 (updated path), add folder 3
	newPath1 := "/updated/folder1"
	updatePayload := &UpdateLibraryPayload{
		Folders: []UpdateFolderPayload{
			{ID: folder1ID, Path: newPath1},
			{Path: "/added/folder3"},
		},
	}

	updatedLib, err := UpdateLibrary(db, lib.ID, updatePayload)
	if err != nil {
		t.Fatalf("UpdateLibrary failed: %v", err)
	}

	if len(updatedLib.Folders) != 2 {
		t.Errorf("Expected 2 folders after update, got %d", len(updatedLib.Folders))
	}

	// Verify paths and existence of folders
	var f1Path, f3Path string
	for _, f := range updatedLib.Folders {
		if f.ID == folder1ID {
			f1Path = f.FullPath
		} else {
			f3Path = f.FullPath
		}
	}

	if f1Path != newPath1 {
		t.Errorf("Expected folder 1 path to be %q, got %q", newPath1, f1Path)
	}
	if f3Path != "/added/folder3" {
		t.Errorf("Expected folder 3 path to be /added/folder3, got %q", f3Path)
	}

	// Verify library item under deleted folder 2 is deleted
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM libraryItems WHERE id = 'item-f2'").Scan(&count)
	if err != nil {
		t.Fatalf("Failed to query libraryItems: %v", err)
	}
	if count != 0 {
		t.Errorf("Expected library item under folder 2 to be cascade-deleted, got count = %d", count)
	}
}

// TestLibraries_DefaultSettingsValidation validates defaults and partial merges.
func TestLibraries_DefaultSettingsValidation(t *testing.T) {
	db := setupLibrariesStressDB(t)
	defer db.Close()

	// 1. Book defaults
	payloadBook := &CreateLibraryPayload{
		Name:      "Default Book Lib",
		MediaType: "book",
	}
	libBook, err := CreateLibrary(db, payloadBook)
	if err != nil {
		t.Fatalf("Failed to create book library: %v", err)
	}

	var bookSettings map[string]interface{}
	if err := json.Unmarshal(libBook.Settings, &bookSettings); err != nil {
		t.Fatalf("Failed to parse settings: %v", err)
	}

	if bookSettings["coverAspectRatio"] != float64(1) || bookSettings["disableWatcher"] != false {
		t.Errorf("Unexpected default book settings: %+v", bookSettings)
	}

	// 2. Podcast defaults
	payloadPodcast := &CreateLibraryPayload{
		Name:      "Default Podcast Lib",
		MediaType: "podcast",
	}
	libPodcast, err := CreateLibrary(db, payloadPodcast)
	if err != nil {
		t.Fatalf("Failed to create podcast library: %v", err)
	}

	var podcastSettings map[string]interface{}
	if err := json.Unmarshal(libPodcast.Settings, &podcastSettings); err != nil {
		t.Fatalf("Failed to parse settings: %v", err)
	}

	if podcastSettings["podcastSearchRegion"] != "us" || podcastSettings["coverAspectRatio"] != float64(1) {
		t.Errorf("Unexpected default podcast settings: %+v", podcastSettings)
	}
}
