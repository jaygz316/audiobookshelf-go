package hls

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"audiobookshelf/internal/core"
	isocket "audiobookshelf/internal/socket"

	_ "modernc.org/sqlite"
)

func setupIntegrationTestDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite", "file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("Failed to open memory db: %v", err)
	}

	queries := []string{
		`CREATE TABLE libraryItems (id TEXT PRIMARY KEY, mediaId TEXT, mediaType TEXT, libraryId TEXT)`,
		`CREATE TABLE playbackSessions (id TEXT PRIMARY KEY, userId TEXT, mediaItemId TEXT, mediaItemType TEXT, startTime REAL, libraryId TEXT, extraData TEXT, createdAt TEXT, updatedAt TEXT)`,
		`CREATE TABLE books (id TEXT PRIMARY KEY, title TEXT, audioFiles BLOB)`,
		`CREATE TABLE podcastEpisodes (id TEXT PRIMARY KEY, podcastId TEXT, title TEXT, audioFile TEXT)`,
		`CREATE TABLE authors (id TEXT PRIMARY KEY, name TEXT)`,
		`CREATE TABLE bookAuthors (bookId TEXT, authorId TEXT)`,
		`CREATE TABLE podcasts (id TEXT PRIMARY KEY, author TEXT)`,
	}

	for _, q := range queries {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("Failed to execute query %q: %v", q, err)
		}
	}
	return db
}

func TestHLSPlaybackFlow_IntegrationAndConcurrency(t *testing.T) {
	db := setupIntegrationTestDB(t)
	defer db.Close()

	// Seed Book Data
	bookID := "book-abc"
	libItemID := "item-abc"
	userID := "user-normal-1"

	_, err := db.Exec(`INSERT INTO libraryItems (id, mediaId, mediaType, libraryId) VALUES (?, ?, 'book', 'lib-1')`, libItemID, bookID)
	if err != nil {
		t.Fatalf("Failed to seed libraryItems: %v", err)
	}

	audioFilesJSON := `[
		{"index":0, "exclude":false, "duration":10.5, "codec":"mp3", "mimeType":"audio/mpeg", "metadata":{"path":"track0.mp3", "filename":"track0.mp3", "size":1000}},
		{"index":1, "exclude":false, "duration":20.0, "codec":"mp3", "mimeType":"audio/mpeg", "metadata":{"path":"track1.mp3", "filename":"track1.mp3", "size":2000}}
	]`
	_, err = db.Exec(`INSERT INTO books (id, title, audioFiles) VALUES (?, 'Concurrency Test Book', ?)`, bookID, audioFilesJSON)
	if err != nil {
		t.Fatalf("Failed to seed books: %v", err)
	}

	_, err = db.Exec(`INSERT INTO authors (id, name) VALUES ('auth-1', 'Jane Doe')`)
	if err != nil {
		t.Fatalf("Failed to seed authors: %v", err)
	}
	_, err = db.Exec(`INSERT INTO bookAuthors (bookId, authorId) VALUES (?, 'auth-1')`, bookID)
	if err != nil {
		t.Fatalf("Failed to seed bookAuthors: %v", err)
	}

	sm := NewStreamManager()
	defer sm.Close()

	// Step 1: Create Playback Session using HandlePlayItem
	userSess := &core.UserSession{
		ID:       userID,
		Username: "normaluser",
		Type:     "user",
		IsActive: true,
	}

	playReqBody := `{"startTime": 5.0}`
	req := httptest.NewRequest("POST", "/api/items/"+libItemID+"/play", strings.NewReader(playReqBody))
	req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, userSess))
	rr := httptest.NewRecorder()

	playHandler := HandlePlayItem(db, sm)
	playHandler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("HandlePlayItem returned status %d: %s", rr.Code, rr.Body.String())
	}

	var playResp map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &playResp); err != nil {
		t.Fatalf("Failed to parse playback session response: %v", err)
	}

	sessionID, ok := playResp["id"].(string)
	if !ok || sessionID == "" {
		t.Fatalf("Expected valid session id in response, got %v", playResp)
	}

	displayTitle := playResp["displayTitle"].(string)
	if displayTitle != "Concurrency Test Book" {
		t.Errorf("Expected displayTitle 'Concurrency Test Book', got %s", displayTitle)
	}

	displayAuthor := playResp["displayAuthor"].(string)
	if displayAuthor != "Jane Doe" {
		t.Errorf("Expected displayAuthor 'Jane Doe', got %s", displayAuthor)
	}

	// Step 2: Setup Temp Directory for Mock Stream files
	tempDir, err := os.MkdirTemp("", "hls-flow-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	mockPlaylistContent := `#EXTM3U
#EXT-X-VERSION:3
#EXT-X-TARGETDURATION:6
#EXTINF:6.0,
output-0.ts
#EXTINF:4.5,
output-1.ts
#EXT-X-ENDLIST`
	mockSegmentContent := []byte("mock-mpegts-data")

	playlistPath := filepath.Join(tempDir, "output.m3u8")
	segment0Path := filepath.Join(tempDir, "output-0.ts")

	if err := os.WriteFile(playlistPath, []byte(mockPlaylistContent), 0644); err != nil {
		t.Fatalf("Failed to write mock playlist: %v", err)
	}
	if err := os.WriteFile(segment0Path, mockSegmentContent, 0644); err != nil {
		t.Fatalf("Failed to write mock segment: %v", err)
	}

	// Step 3: Populate Mock Stream in StreamManager to skip FFmpeg startup
	stream := &Stream{
		ID:                  sessionID,
		UserID:              userID,
		LibraryItemID:       libItemID,
		StreamPath:          tempDir,
		PlaylistPath:        playlistPath,
		SegmentsCreated:     make(map[int]bool),
		isTranscodeComplete: true,
	}
	sm.AddStream(stream)

	// Step 4: Verify ServeHLS serves playlist and segment correctly
	serveHandler := ServeHLS(db, tempDir, sm, isocket.GlobalAuth)

	// 4A: Get Playlist (without token)
	reqPlaylist := httptest.NewRequest("GET", "/hls/"+sessionID+"/output.m3u8", nil)
	reqPlaylist = reqPlaylist.WithContext(context.WithValue(reqPlaylist.Context(), core.UserContextKey, userSess))
	rrPlaylist := httptest.NewRecorder()
	serveHandler.ServeHTTP(rrPlaylist, reqPlaylist)

	if rrPlaylist.Code != http.StatusOK {
		t.Errorf("GET playlist returned status %d: %s", rrPlaylist.Code, rrPlaylist.Body.String())
	}
	playlistBody := rrPlaylist.Body.String()
	if !strings.Contains(playlistBody, "output-0.ts") {
		t.Errorf("Playlist body does not contain expected segment path: %s", playlistBody)
	}

	// 4B: Get Playlist (with token query parameter)
	reqPlaylistWithToken := httptest.NewRequest("GET", "/hls/"+sessionID+"/output.m3u8?token=super-secret-token", nil)
	reqPlaylistWithToken = reqPlaylistWithToken.WithContext(context.WithValue(reqPlaylistWithToken.Context(), core.UserContextKey, userSess))
	rrPlaylistWithToken := httptest.NewRecorder()
	serveHandler.ServeHTTP(rrPlaylistWithToken, reqPlaylistWithToken)

	if rrPlaylistWithToken.Code != http.StatusOK {
		t.Errorf("GET playlist with token returned status %d", rrPlaylistWithToken.Code)
	}
	playlistBodyWithToken := rrPlaylistWithToken.Body.String()
	if !strings.Contains(playlistBodyWithToken, "output-0.ts?token=super-secret-token") {
		t.Errorf("Playlist body does not have appended token: %s", playlistBodyWithToken)
	}

	// 4C: Get Segment
	reqSegment := httptest.NewRequest("GET", "/hls/"+sessionID+"/output-0.ts", nil)
	reqSegment = reqSegment.WithContext(context.WithValue(reqSegment.Context(), core.UserContextKey, userSess))
	rrSegment := httptest.NewRecorder()
	serveHandler.ServeHTTP(rrSegment, reqSegment)

	if rrSegment.Code != http.StatusOK {
		t.Errorf("GET segment returned status %d", rrSegment.Code)
	}
	if rrSegment.Body.String() != "mock-mpegts-data" {
		t.Errorf("Segment body mismatch, got %s", rrSegment.Body.String())
	}

	// Step 5: Test Authorization Controls
	// 5A: Different normal user should get Forbidden (403)
	otherUserSess := &core.UserSession{
		ID:       "user-different-id",
		Username: "otheruser",
		Type:     "user",
		IsActive: true,
	}
	reqForbidden := httptest.NewRequest("GET", "/hls/"+sessionID+"/output.m3u8", nil)
	reqForbidden = reqForbidden.WithContext(context.WithValue(reqForbidden.Context(), core.UserContextKey, otherUserSess))
	rrForbidden := httptest.NewRecorder()
	serveHandler.ServeHTTP(rrForbidden, reqForbidden)

	if rrForbidden.Code != http.StatusForbidden {
		t.Errorf("Expected 403 Forbidden for unauthorized user access, got %d", rrForbidden.Code)
	}

	// 5B: Root/Admin user should bypass and get OK (200)
	adminUserSess := &core.UserSession{
		ID:       "admin-user-id",
		Username: "adminuser",
		Type:     "admin",
		IsActive: true,
	}
	reqAdminBypass := httptest.NewRequest("GET", "/hls/"+sessionID+"/output.m3u8", nil)
	reqAdminBypass = reqAdminBypass.WithContext(context.WithValue(reqAdminBypass.Context(), core.UserContextKey, adminUserSess))
	rrAdminBypass := httptest.NewRecorder()
	serveHandler.ServeHTTP(rrAdminBypass, reqAdminBypass)

	if rrAdminBypass.Code != http.StatusOK {
		t.Errorf("Expected admin to bypass ownership check and get 200 OK, got %d", rrAdminBypass.Code)
	}

	// Step 6: Concurrency Stress Test
	// Spawn many goroutines hitting ServeHLS and HandlePlayItem simultaneously
	var wg sync.WaitGroup
	concurrencyLimit := 40
	wg.Add(concurrencyLimit)

	for i := 0; i < concurrencyLimit; i++ {
		go func(idx int) {
			defer wg.Done()

			// Perform HandlePlayItem
			playReqBodyInner := `{"startTime": 0.0}`
			reqPlayInner := httptest.NewRequest("POST", "/api/items/"+libItemID+"/play", strings.NewReader(playReqBodyInner))
			reqPlayInner = reqPlayInner.WithContext(context.WithValue(reqPlayInner.Context(), core.UserContextKey, userSess))
			rrPlayInner := httptest.NewRecorder()
			playHandler.ServeHTTP(rrPlayInner, reqPlayInner)

			if rrPlayInner.Code != http.StatusOK {
				t.Errorf("Concurrent play handler failed: %d", rrPlayInner.Code)
			}

			// Perform ServeHLS playlist retrieval
			reqPlaylistInner := httptest.NewRequest("GET", "/hls/"+sessionID+"/output.m3u8", nil)
			reqPlaylistInner = reqPlaylistInner.WithContext(context.WithValue(reqPlaylistInner.Context(), core.UserContextKey, userSess))
			rrPlaylistInner := httptest.NewRecorder()
			serveHandler.ServeHTTP(rrPlaylistInner, reqPlaylistInner)

			if rrPlaylistInner.Code != http.StatusOK {
				t.Errorf("Concurrent ServeHLS playlist failed: %d", rrPlaylistInner.Code)
			}

			// Perform ServeHLS segment retrieval
			reqSegmentInner := httptest.NewRequest("GET", "/hls/"+sessionID+"/output-0.ts", nil)
			reqSegmentInner = reqSegmentInner.WithContext(context.WithValue(reqSegmentInner.Context(), core.UserContextKey, userSess))
			rrSegmentInner := httptest.NewRecorder()
			serveHandler.ServeHTTP(rrSegmentInner, reqSegmentInner)

			if rrSegmentInner.Code != http.StatusOK {
				t.Errorf("Concurrent ServeHLS segment failed: %d", rrSegmentInner.Code)
			}
		}(i)
	}

	wg.Wait()
}
