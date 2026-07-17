package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestOPDS_Adversarial_Permissions(t *testing.T) {
	db := prepareOPDSTestDB(t)
	defer db.Close()

	// Insert a restricted user who can ONLY access items with the tag "OnlyThisTag"
	hashed, err := bcrypt.GenerateFromPassword([]byte("mypassword"), 8)
	if err != nil {
		t.Fatalf("Failed to hash password: %v", err)
	}

	// We insert user-restricted.
	// We want to test two restrictions:
	// 1. Tag restrictions: accessAllTags = false, itemTagsSelected = ["OnlyThisTag"]
	// 2. Explicit restrictions: accessExplicitContent = false
	// We will also insert another explicit book (book-explicit) which has explicit = 1.
	_, err = db.Exec(`
		INSERT INTO users (id, username, type, pash, isActive, permissions) 
		VALUES ('user-restricted', 'restricted-user', 'user', ?, 1, ?)
	`, string(hashed), `{"accessAllLibraries": true, "accessAllTags": false, "itemTagsSelected": ["OnlyThisTag"], "selectedTagsNotAccessible": false, "accessExplicitContent": false}`)
	if err != nil {
		t.Fatalf("Failed to insert restricted user: %v", err)
	}

	// We also insert the playlist for this restricted user since playlists are user-specific
	_, err = db.Exec(`
		INSERT INTO playlists (id, name, description, libraryId, userId) 
		VALUES ('play-restricted', 'Restricted Playlist', 'My Playlist', 'lib-1', 'user-restricted')
	`)
	if err != nil {
		t.Fatalf("Failed to insert playlist: %v", err)
	}

	_, err = db.Exec(`
		INSERT INTO playlistMediaItems (id, mediaItemId, mediaItemType, "order", playlistId) 
		VALUES ('pmi-restricted', 'item-1', 'book', 1, 'play-restricted')
	`)
	if err != nil {
		t.Fatalf("Failed to insert playlist item: %v", err)
	}

	handler := AuthMiddleware(db, "mysecret", ServeOPDS(db))

	// ---- CONTROL TEST: GET ALL ITEMS ----
	// The "/all" endpoint uses idb.GetFilteredLibraryItems, which checks permissions.
	// Since "item-1" has no tags, the restricted user should NOT see it.
	reqAll := httptest.NewRequest("GET", "/opds/v1.2/libraries/lib-1/all", nil)
	reqAll.SetBasicAuth("restricted-user", "mypassword")
	rrAll := httptest.NewRecorder()
	handler.ServeHTTP(rrAll, reqAll)

	if rrAll.Code != http.StatusOK {
		t.Fatalf("Control: Expected status 200, got %d", rrAll.Code)
	}
	bodyAll := rrAll.Body.String()
	if strings.Contains(bodyAll, "Test Book") {
		t.Logf("Control passed: Restricted book was correctly filtered out of /all feed.")
	} else {
		t.Logf("Control passed: /all feed did not contain restricted book.")
	}

	// ---- TEST CASE 1: AUTHOR ITEMS ----
	// The author endpoint should NOT expose the restricted book.
	reqAuthor := httptest.NewRequest("GET", "/opds/v1.2/libraries/lib-1/authors/author-1", nil)
	reqAuthor.SetBasicAuth("restricted-user", "mypassword")
	rrAuthor := httptest.NewRecorder()
	handler.ServeHTTP(rrAuthor, reqAuthor)

	if rrAuthor.Code != http.StatusOK {
		t.Fatalf("Expected status 200 from authors endpoint, got %d", rrAuthor.Code)
	}
	bodyAuthor := rrAuthor.Body.String()
	if strings.Contains(bodyAuthor, "Test Book") {
		t.Errorf("SECURITY VULNERABILITY: Restricted book was exposed in author items feed for restricted-user!")
	}

	// ---- TEST CASE 2: SERIES ITEMS ----
	// The series endpoint should NOT expose the restricted book.
	reqSeries := httptest.NewRequest("GET", "/opds/v1.2/libraries/lib-1/series/series-1", nil)
	reqSeries.SetBasicAuth("restricted-user", "mypassword")
	rrSeries := httptest.NewRecorder()
	handler.ServeHTTP(rrSeries, reqSeries)

	if rrSeries.Code != http.StatusOK {
		t.Fatalf("Expected status 200 from series endpoint, got %d", rrSeries.Code)
	}
	bodySeries := rrSeries.Body.String()
	if strings.Contains(bodySeries, "Test Book") {
		t.Errorf("SECURITY VULNERABILITY: Restricted book was exposed in series items feed for restricted-user!")
	}

	// ---- TEST CASE 3: COLLECTION ITEMS ----
	// The collection endpoint should NOT expose the restricted book.
	reqCollection := httptest.NewRequest("GET", "/opds/v1.2/libraries/lib-1/collections/coll-1", nil)
	reqCollection.SetBasicAuth("restricted-user", "mypassword")
	rrCollection := httptest.NewRecorder()
	handler.ServeHTTP(rrCollection, reqCollection)

	if rrCollection.Code != http.StatusOK {
		t.Fatalf("Expected status 200 from collections endpoint, got %d", rrCollection.Code)
	}
	bodyCollection := rrCollection.Body.String()
	if strings.Contains(bodyCollection, "Test Book") {
		t.Errorf("SECURITY VULNERABILITY: Restricted book was exposed in collection items feed for restricted-user!")
	}

	// ---- TEST CASE 4: PLAYLIST ITEMS ----
	// The playlist endpoint should NOT expose the restricted book.
	reqPlaylist := httptest.NewRequest("GET", "/opds/v1.2/libraries/lib-1/playlists/play-restricted", nil)
	reqPlaylist.SetBasicAuth("restricted-user", "mypassword")
	rrPlaylist := httptest.NewRecorder()
	handler.ServeHTTP(rrPlaylist, reqPlaylist)

	if rrPlaylist.Code != http.StatusOK {
		t.Fatalf("Expected status 200 from playlists endpoint, got %d", rrPlaylist.Code)
	}
	bodyPlaylist := rrPlaylist.Body.String()
	if strings.Contains(bodyPlaylist, "Test Book") {
		t.Errorf("SECURITY VULNERABILITY: Restricted book was exposed in playlist items feed for restricted-user!")
	}
}
