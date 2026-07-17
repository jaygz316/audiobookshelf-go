package db

import (
	"database/sql"
	"encoding/base64"
	"fmt"
	"strings"
	"sync"
	"testing"

	"audiobookshelf/internal/core"
	_ "modernc.org/sqlite"
)

func setupStressTestDB(t *testing.T) *sql.DB {
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("Failed to open test DB: %v", err)
	}

	if err := bootstrapSchema(db); err != nil {
		db.Close()
		t.Fatalf("Failed to bootstrap schema: %v", err)
	}

	return db
}

// TestGetFilteredLibraryItems_Stress tests all sorting options for books and podcasts.
func TestGetFilteredLibraryItems_Stress_Sorting(t *testing.T) {
	db := setupStressTestDB(t)
	defer db.Close()

	// Insert test user, library, items
	_, _ = db.Exec(`INSERT INTO users (id, username, permissions) VALUES ('u1', 'user1', '[]')`)
	_, _ = db.Exec(`INSERT INTO libraries (id, name, mediaType) VALUES ('l_book', 'Books', 'book')`)
	_, _ = db.Exec(`INSERT INTO libraries (id, name, mediaType) VALUES ('l_podcast', 'Podcasts', 'podcast')`)

	// Books
	for i := 1; i <= 5; i++ {
		bID := fmt.Sprintf("b%d", i)
		liID := fmt.Sprintf("li_b%d", i)
		title := fmt.Sprintf("Book %d", i)
		pubYear := fmt.Sprintf("%d", 2010+i)
		duration := float64(i * 1000)
		size := int64(i * 100000)

		_, err := db.Exec(`INSERT INTO books (id, title, publishedYear, duration, tags, genres, narrators) 
			VALUES (?, ?, ?, ?, '["tag1"]', '["genre1"]', '["Narrator X"]')`, bID, title, pubYear, duration)
		if err != nil {
			t.Fatalf("Failed to insert book %s: %v", bID, err)
		}

		_, err = db.Exec(`INSERT INTO libraryItems (id, ino, libraryId, path, relPath, isFile, createdAt, updatedAt, mediaType, mediaId, size, title) 
			VALUES (?, 'ino1', 'l_book', 'path1', 'relpath1', 0, '2026-07-01', '2026-07-01', 'book', ?, ?, ?)`,
			liID, bID, size, title)
		if err != nil {
			t.Fatalf("Failed to insert library item %s: %v", liID, err)
		}

		// Also insert progress
		_, err = db.Exec(`INSERT INTO mediaProgresses (id, userId, mediaItemId, duration, currentTime, isFinished, updatedAt) 
			VALUES (?, 'u1', ?, ?, 10.0, 0, '2026-07-02')`, fmt.Sprintf("mp%d", i), bID, duration)
		if err != nil {
			t.Fatalf("Failed to insert progress: %v", err)
		}
	}

	// Podcasts
	for i := 1; i <= 3; i++ {
		pID := fmt.Sprintf("p%d", i)
		liID := fmt.Sprintf("li_p%d", i)
		title := fmt.Sprintf("Podcast %d", i)

		_, err := db.Exec(`INSERT INTO podcasts (id, title, author, numEpisodes, tags, genres) 
			VALUES (?, ?, 'Author X', ?, '[]', '[]')`, pID, title, i)
		if err != nil {
			t.Fatalf("Failed to insert podcast %s: %v", pID, err)
		}

		_, err = db.Exec(`INSERT INTO libraryItems (id, ino, libraryId, path, relPath, isFile, createdAt, updatedAt, mediaType, mediaId, size, title) 
			VALUES (?, 'ino2', 'l_podcast', 'path2', 'relpath2', 0, '2026-07-01', '2026-07-01', 'podcast', ?, 1000, ?)`,
			liID, pID, title)
		if err != nil {
			t.Fatalf("Failed to insert library item %s: %v", liID, err)
		}
	}

	bookSortOptions := []string{
		"addedAt", "size", "birthtimeMs", "mtimeMs",
		"media.duration", "media.metadata.publishedYear",
		"media.metadata.authorNameLF", "media.metadata.authorName",
		"media.metadata.title", "sequence", "progress",
		"random", "invalid_sort_option",
	}

	for _, sortBy := range bookSortOptions {
		for _, desc := range []bool{false, true} {
			t.Run(fmt.Sprintf("Book_Sort_%s_Desc_%t", sortBy, desc), func(t *testing.T) {
				opts := GetFilteredLibraryItemsOptions{
					LibraryID: "l_book",
					User:      &core.UserSession{ID: "u1", CanAccessExplicitContent: true, AccessAllTags: true},
					MediaType: "book",
					SortBy:    sortBy,
					SortDesc:  desc,
				}
				res, _, err := GetFilteredLibraryItems(db, opts)
				if err != nil {
					t.Fatalf("GetFilteredLibraryItems failed for book sort %s: %v", sortBy, err)
				}
				if len(res) == 0 {
					t.Errorf("Expected results, got 0")
				}
			})
		}
	}

	podcastSortOptions := []string{
		"addedAt", "size", "birthtimeMs", "mtimeMs",
		"media.metadata.author", "media.numTracks",
		"random", "invalid_sort_option_pod",
	}

	for _, sortBy := range podcastSortOptions {
		for _, desc := range []bool{false, true} {
			t.Run(fmt.Sprintf("Podcast_Sort_%s_Desc_%t", sortBy, desc), func(t *testing.T) {
				opts := GetFilteredLibraryItemsOptions{
					LibraryID: "l_podcast",
					User:      &core.UserSession{ID: "u1", CanAccessExplicitContent: true, AccessAllTags: true},
					MediaType: "podcast",
					SortBy:    sortBy,
					SortDesc:  desc,
				}
				res, _, err := GetFilteredLibraryItems(db, opts)
				if err != nil {
					t.Fatalf("GetFilteredLibraryItems failed for podcast sort %s: %v", sortBy, err)
				}
				if len(res) == 0 {
					t.Errorf("Expected results, got 0")
				}
			})
		}
	}
}

// TestGetFilteredLibraryItems_Stress_Filters tests different filter options.
func TestGetFilteredLibraryItems_Stress_Filters(t *testing.T) {
	db := setupStressTestDB(t)
	defer db.Close()

	// Insert test library and users
	_, _ = db.Exec(`INSERT INTO users (id, username, permissions) VALUES ('u1', 'user1', '[]')`)
	_, _ = db.Exec(`INSERT INTO libraries (id, name, mediaType) VALUES ('l_book', 'Books', 'book')`)

	// Authors and Series setup
	_, _ = db.Exec(`INSERT INTO authors (id, libraryId, name) VALUES ('a1', 'l_book', 'Author One')`)
	_, _ = db.Exec(`INSERT INTO series (id, libraryId, name) VALUES ('s1', 'l_book', 'Series One')`)

	// Books
	// Book 1: Author 1, Series 1, Genre Fiction, Tag Favorite, Language English, Publisher Acme, Year 2020, Duration 4000 (1h-5h), missing/invalid = 0
	_, _ = db.Exec(`INSERT INTO books (id, title, publishedYear, duration, tags, genres, language, publisher) 
		VALUES ('b1', 'Book 1', '2020', 4000.0, '["Favorite"]', '["Fiction"]', 'English', 'Acme')`)
	_, _ = db.Exec(`INSERT INTO libraryItems (id, libraryId, mediaType, mediaId, title, isMissing, isInvalid, libraryFolderId) 
		VALUES ('li1', 'l_book', 'book', 'b1', 'Book 1', 0, 0, 'f1')`)
	_, _ = db.Exec(`INSERT INTO bookAuthors (bookId, authorId) VALUES ('b1', 'a1')`)
	_, _ = db.Exec(`INSERT INTO bookSeries (bookId, seriesId, sequence) VALUES ('b1', 's1', '1')`)
	_, _ = db.Exec(`INSERT INTO mediaProgresses (id, userId, mediaItemId, duration, currentTime, isFinished, updatedAt) 
		VALUES ('prog1', 'u1', 'b1', 4000.0, 2000.0, 0, '2026-07-02')`) // in-progress

	// Book 2: No Series, Genre Science, Tag Sci-Fi, Language French, Publisher Beta, Year 2010, Duration 500 (<1h), missing = 1
	_, _ = db.Exec(`INSERT INTO books (id, title, publishedYear, duration, tags, genres, language, publisher) 
		VALUES ('b2', 'Book 2', '2010', 500.0, '["Sci-Fi"]', '["Science"]', 'French', 'Beta')`)
	_, _ = db.Exec(`INSERT INTO libraryItems (id, libraryId, mediaType, mediaId, title, isMissing, isInvalid, libraryFolderId) 
		VALUES ('li2', 'l_book', 'book', 'b2', 'Book 2', 1, 0, 'f2')`)
	_, _ = db.Exec(`INSERT INTO mediaProgresses (id, userId, mediaItemId, duration, currentTime, isFinished, updatedAt) 
		VALUES ('prog2', 'u1', 'b2', 500.0, 500.0, 1, '2026-07-02')`) // finished

	// Book 3: No progress (not started), invalid = 1
	_, _ = db.Exec(`INSERT INTO books (id, title, publishedYear, duration, tags, genres, language, publisher) 
		VALUES ('b3', 'Book 3', '1995', 50000.0, '[]', '[]', 'English', 'Acme')`)
	_, _ = db.Exec(`INSERT INTO libraryItems (id, libraryId, mediaType, mediaId, title, isMissing, isInvalid, libraryFolderId) 
		VALUES ('li3', 'l_book', 'book', 'b3', 'Book 3', 0, 1, 'f1')`)

	b64 := func(val string) string {
		return base64.StdEncoding.EncodeToString([]byte(val))
	}

	filterTests := []struct {
		name        string
		filterBy    string
		expectedIDs []string
	}{
		{"Filter Authors", "authors." + b64("a1"), []string{"li1"}},
		{"Filter Series (Specific)", "series." + b64("s1"), []string{"li1"}},
		{"Filter Series (None)", "series." + b64("no-series"), []string{"li2", "li3"}},
		{"Filter Genres", "genres." + b64("Fiction"), []string{"li1"}},
		{"Filter Tags", "tags." + b64("Sci-Fi"), []string{"li2"}},
		{"Filter Languages", "languages." + b64("French"), []string{"li2"}},
		{"Filter Publishers", "publishers." + b64("Acme"), []string{"li1", "li3"}},
		{"Filter Progress (In-Progress)", "progress.in-progress", []string{"li1"}},
		{"Filter Progress (Finished)", "progress.finished", []string{"li2"}},
		{"Filter Progress (Not-Started)", "progress.not-started", []string{"li3"}},
		{"Filter Missing/Invalid", "missing", []string{"li2", "li3"}},
		{"Filter Decades (2020)", "decades." + b64("2020"), []string{"li1"}},
		{"Filter Decades (2010)", "decades." + b64("2010"), []string{"li2"}},
		{"Filter Decades (Invalid Value)", "decades." + b64("abc"), []string{"li1", "li2", "li3"}}, // Invalid decade value should not filter out items
		{"Filter Years (2010)", "years." + b64("2010"), []string{"li2"}},
		{"Filter Duration (Under 1h)", "duration.under-1h", []string{"li2"}},
		{"Filter Duration (1h-5h)", "duration.1h-5h", []string{"li1"}},
		{"Filter Duration (Over 10h)", "duration.over-10h", []string{"li3"}},
		{"Filter Folder", "folder.f1", []string{"li1", "li3"}},
		{"Filter Unknown Group", "unknownGroup.val", []string{"li1", "li2", "li3"}}, // should ignore the filter
	}

	for _, tt := range filterTests {
		t.Run(tt.name, func(t *testing.T) {
			opts := GetFilteredLibraryItemsOptions{
				LibraryID: "l_book",
				User:      &core.UserSession{ID: "u1", CanAccessExplicitContent: true, AccessAllTags: true},
				MediaType: "book",
				FilterBy:  tt.filterBy,
			}
			res, _, err := GetFilteredLibraryItems(db, opts)
			if err != nil {
				t.Fatalf("GetFilteredLibraryItems failed for filter %s: %v", tt.filterBy, err)
			}

			if len(res) != len(tt.expectedIDs) {
				// Query and print mediaProgresses content
				prows, _ := db.Query("SELECT id, userId, mediaItemId, duration, currentTime, isFinished, updatedAt FROM mediaProgresses")
				t.Errorf("Expected %d results, got %d. Filter: %s", len(tt.expectedIDs), len(res), tt.filterBy)
				whereClause, wargs := buildFilteredItemsWhere(opts)
				selectQuery := buildFilteredItemsSelectQuery(opts, whereClause, false, &wargs)
				t.Logf("Constructed query: %s", selectQuery)
				t.Logf("Constructed args: %v", wargs)
				if prows != nil {
					defer prows.Close()
					for prows.Next() {
						var id, uid, mid, upd string
						var dur, ct float64
						var isFin int
						prows.Scan(&id, &uid, &mid, &dur, &ct, &isFin, &upd)
						t.Logf("mediaProgresses Row: id=%s, userId=%s, mediaItemId=%s, duration=%f, currentTime=%f, isFinished=%d, updatedAt=%s", id, uid, mid, dur, ct, isFin, upd)
					}
				}
				for i, item := range res {
					t.Logf("Result[%d]: ID=%s, MediaID=%s, Title=%s", i, item.ID, item.Media.(*BookMinifiedJSON).ID, item.Media.(*BookMinifiedJSON).Metadata.Title)
				}
				return
			}

			// Map IDs
			foundMap := make(map[string]bool)
			for _, item := range res {
				foundMap[item.ID] = true
			}

			for _, expectedID := range tt.expectedIDs {
				if !foundMap[expectedID] {
					t.Errorf("Expected item ID %s in results but not found. Filter: %s", expectedID, tt.filterBy)
				}
			}
		})
	}
}

// TestGetFilteredLibraryItems_Stress_Search checks robustness of search queries and SQL injection attempts.
func TestGetFilteredLibraryItems_Stress_Search(t *testing.T) {
	db := setupStressTestDB(t)
	defer db.Close()

	_, _ = db.Exec(`INSERT INTO users (id, username, permissions) VALUES ('u1', 'user1', '[]')`)
	_, _ = db.Exec(`INSERT INTO libraries (id, name, mediaType) VALUES ('l_book', 'Books', 'book')`)
	_, _ = db.Exec(`INSERT INTO books (id, title, publishedYear, duration, description, subtitle) 
		VALUES ('b1', 'Target Book Title', '2020', 1000.0, 'Some description text', 'Target Subtitle')`)
	_, _ = db.Exec(`INSERT INTO libraryItems (id, libraryId, mediaType, mediaId, title, authorNamesFirstLast) 
		VALUES ('li1', 'l_book', 'book', 'b1', 'Target Book Title', 'Author TargetName')`)

	searchTests := []struct {
		name        string
		search      string
		expectedLen int
	}{
		{"Search Match Title", "Title", 1},
		{"Search Match Description", "description", 1},
		{"Search Match Subtitle", "Subtitle", 1},
		{"Search Match Author", "TargetName", 1},
		{"Search No Match", "NonExistentWord", 0},
		{"Search Special Characters", "!!! @@@ ###", 0},
		{"Search SQL Injection Attempt 1", "' OR 1=1 --", 0},
		{"Search SQL Injection Attempt 2", "\"; DROP TABLE libraryItems; --", 0},
		{"Search Very Long String", strings.Repeat("A", 1000), 0},
	}

	for _, tt := range searchTests {
		t.Run(tt.name, func(t *testing.T) {
			opts := GetFilteredLibraryItemsOptions{
				LibraryID: "l_book",
				User:      &core.UserSession{ID: "u1", CanAccessExplicitContent: true, AccessAllTags: true},
				MediaType: "book",
				Search:    tt.search,
			}
			res, _, err := GetFilteredLibraryItems(db, opts)
			if err != nil {
				t.Fatalf("Search failed: %v", err)
			}
			if len(res) != tt.expectedLen {
				t.Errorf("Expected length %d, got %d for search: %q", tt.expectedLen, len(res), tt.search)
			}
		})
	}
}

// TestGetFilteredLibraryItems_Stress_Pagination verifies limits, offsets, page numbers.
func TestGetFilteredLibraryItems_Stress_Pagination(t *testing.T) {
	db := setupStressTestDB(t)
	defer db.Close()

	_, _ = db.Exec(`INSERT INTO users (id, username, permissions) VALUES ('u1', 'user1', '[]')`)
	_, _ = db.Exec(`INSERT INTO libraries (id, name, mediaType) VALUES ('l_book', 'Books', 'book')`)

	// Insert 10 items
	for i := 1; i <= 10; i++ {
		bID := fmt.Sprintf("b%d", i)
		liID := fmt.Sprintf("li%d", i)
		title := fmt.Sprintf("Book %d", i)
		_, _ = db.Exec(`INSERT INTO books (id, title) VALUES (?, ?)`, bID, title)
		// Set createdTime or title for alphabetical/default sorting consistency
		_, _ = db.Exec(`INSERT INTO libraryItems (id, libraryId, mediaType, mediaId, title) VALUES (?, 'l_book', 'book', ?, ?)`, liID, bID, title)
	}

	paginationTests := []struct {
		name        string
		limit       int
		page        int
		expectedLen int
		total       int
	}{
		{"Limit 5 Page 0", 5, 0, 5, 10},
		{"Limit 5 Page 1", 5, 1, 5, 10},
		{"Limit 5 Page 2", 5, 2, 0, 10},
		{"Limit 20 Page 0", 20, 0, 10, 10},
		{"Limit 0 Page 0 (No Pagination)", 0, 0, 10, 10},
		{"Limit -5 Page -1 (Negative)", -5, -1, 10, 10}, // Negatives should disable/be ignored
	}

	for _, tt := range paginationTests {
		t.Run(tt.name, func(t *testing.T) {
			opts := GetFilteredLibraryItemsOptions{
				LibraryID: "l_book",
				User:      &core.UserSession{ID: "u1", CanAccessExplicitContent: true, AccessAllTags: true},
				MediaType: "book",
				Limit:     tt.limit,
				Page:      tt.page,
				SortBy:    "media.metadata.title",
				SortDesc:  false,
			}
			res, total, err := GetFilteredLibraryItems(db, opts)
			if err != nil {
				t.Fatalf("Pagination failed: %v", err)
			}
			if total != tt.total {
				t.Errorf("Expected total count %d, got %d", tt.total, total)
			}
			if len(res) != tt.expectedLen {
				t.Errorf("Expected returned count %d, got %d", tt.expectedLen, len(res))
			}
		})
	}
}

// TestGetFilteredLibraryItems_Stress_UserPermissions tests tag exclusion and explicit content permissions.
func TestGetFilteredLibraryItems_Stress_UserPermissions(t *testing.T) {
	db := setupStressTestDB(t)
	defer db.Close()

	_, _ = db.Exec(`INSERT INTO users (id, username, permissions) VALUES ('u1', 'user1', '[]')`)
	_, _ = db.Exec(`INSERT INTO libraries (id, name, mediaType) VALUES ('l_book', 'Books', 'book')`)

	// Book 1: explicit = 0, tags = ["kids", "safe"]
	_, _ = db.Exec(`INSERT INTO books (id, title, explicit, tags) VALUES ('b1', 'Book 1', 0, '["kids", "safe"]')`)
	_, _ = db.Exec(`INSERT INTO libraryItems (id, libraryId, mediaType, mediaId, title) VALUES ('li1', 'l_book', 'book', 'b1', 'Book 1')`)

	// Book 2: explicit = 1, tags = ["adult", "dark"]
	_, _ = db.Exec(`INSERT INTO books (id, title, explicit, tags) VALUES ('b2', 'Book 2', 1, '["adult", "dark"]')`)
	_, _ = db.Exec(`INSERT INTO libraryItems (id, libraryId, mediaType, mediaId, title) VALUES ('li2', 'l_book', 'book', 'b2', 'Book 2')`)

	// Book 3: explicit = 0, tags = ["teen"]
	_, _ = db.Exec(`INSERT INTO books (id, title, explicit, tags) VALUES ('b3', 'Book 3', 0, '["teen"]')`)
	_, _ = db.Exec(`INSERT INTO libraryItems (id, libraryId, mediaType, mediaId, title) VALUES ('li3', 'l_book', 'book', 'b3', 'Book 3')`)

	permissionTests := []struct {
		name        string
		user        *core.UserSession
		expectedIDs []string
	}{
		{
			name:        "Access explicit, access all tags",
			user:        &core.UserSession{ID: "u1", CanAccessExplicitContent: true, AccessAllTags: true},
			expectedIDs: []string{"li1", "li2", "li3"},
		},
		{
			name:        "No explicit access, access all tags",
			user:        &core.UserSession{ID: "u1", CanAccessExplicitContent: false, AccessAllTags: true},
			expectedIDs: []string{"li1", "li3"},
		},
		{
			name:        "Access explicit, select specific tags accessible: kids",
			user:        &core.UserSession{ID: "u1", CanAccessExplicitContent: true, AccessAllTags: false, ItemTagsSelected: []string{"kids"}},
			expectedIDs: []string{"li1"},
		},
		{
			name:        "Access explicit, select specific tags not accessible: kids (meaning kids tag items are excluded)",
			user:        &core.UserSession{ID: "u1", CanAccessExplicitContent: true, AccessAllTags: false, ItemTagsSelected: []string{"kids"}, SelectedTagsNotAccessible: true},
			expectedIDs: []string{"li2", "li3"},
		},
	}

	for _, tt := range permissionTests {
		t.Run(tt.name, func(t *testing.T) {
			opts := GetFilteredLibraryItemsOptions{
				LibraryID: "l_book",
				User:      tt.user,
				MediaType: "book",
			}
			res, _, err := GetFilteredLibraryItems(db, opts)
			if err != nil {
				t.Fatalf("Permissions test failed: %v", err)
			}

			if len(res) != len(tt.expectedIDs) {
				t.Errorf("Expected %d results, got %d", len(tt.expectedIDs), len(res))
				return
			}

			foundMap := make(map[string]bool)
			for _, item := range res {
				foundMap[item.ID] = true
			}

			for _, expectedID := range tt.expectedIDs {
				if !foundMap[expectedID] {
					t.Errorf("Expected item ID %s in results but not found", expectedID)
				}
			}
		})
	}
}

// TestGetFilteredLibraryItems_Stress_Concurrency runs multiple queries concurrently to ensure thread safety.
func TestGetFilteredLibraryItems_Stress_Concurrency(t *testing.T) {
	db := setupStressTestDB(t)
	defer db.Close()

	_, _ = db.Exec(`INSERT INTO users (id, username, permissions) VALUES ('u1', 'user1', '[]')`)
	_, _ = db.Exec(`INSERT INTO libraries (id, name, mediaType) VALUES ('l_book', 'Books', 'book')`)
	_, _ = db.Exec(`INSERT INTO books (id, title) VALUES ('b1', 'Book 1')`)
	_, _ = db.Exec(`INSERT INTO libraryItems (id, libraryId, mediaType, mediaId, title) VALUES ('li1', 'l_book', 'book', 'b1', 'Book 1')`)

	numWorkers := 20
	numQueriesPerWorker := 50
	var wg sync.WaitGroup

	opts := GetFilteredLibraryItemsOptions{
		LibraryID: "l_book",
		User:      &core.UserSession{ID: "u1", CanAccessExplicitContent: true, AccessAllTags: true},
		MediaType: "book",
	}

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for q := 0; q < numQueriesPerWorker; q++ {
				_, _, err := GetFilteredLibraryItems(db, opts)
				if err != nil {
					t.Errorf("Worker %d, query %d failed: %v", workerID, q, err)
					return
				}
			}
		}(i)
	}

	wg.Wait()
}

// TestGetFilteredLibraryItems_Stress_NilGracefulness tests calling the functions with nil DB or invalid arguments.
func TestGetFilteredLibraryItems_Stress_NilGracefulness(t *testing.T) {
	// 1. GetLibraryItemDownloadInfo
	_, err := GetLibraryItemDownloadInfo(nil, "some-id")
	if err == nil {
		t.Error("Expected error with nil db in GetLibraryItemDownloadInfo, got nil")
	}

	// 2. GetCoverPath
	_, err = GetCoverPath(nil, "some-id")
	if err == nil {
		t.Error("Expected error with nil db in GetCoverPath, got nil")
	}

	// 3. GetLibraryItemMinifiedByID
	db := setupStressTestDB(t)
	defer db.Close()

	_, err = GetLibraryItemMinifiedByID(db, "non-existent")
	if err == nil {
		t.Error("Expected sql.ErrNoRows error for non-existent item, got nil")
	}
}
