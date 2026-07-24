package socket

import (
	"database/sql"
	"fmt"
	"math/rand"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	_ "modernc.org/sqlite"
)

func setupStressTestDB(t *testing.T) *sql.DB {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "stress_test.db")
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode=WAL&_pragma=busy_timeout=5000&_pragma=synchronous=OFF", dbPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("Failed to open temp db: %v", err)
	}

	// Use multiple connections now that WAL mode is enabled
	db.SetMaxOpenConns(10)

	queries := []string{
		`CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT)`,
		`CREATE TABLE users (id TEXT PRIMARY KEY, username TEXT, type TEXT, isActive INTEGER, permissions TEXT, extraData TEXT, lastSeen TEXT, createdAt TEXT, updatedAt TEXT)`,
		`CREATE TABLE apiKeys (id TEXT PRIMARY KEY, isActive INTEGER, expiresAt TEXT, userId TEXT, name TEXT, createdAt TEXT)`,
		`CREATE TABLE libraries (id TEXT PRIMARY KEY, name TEXT, displayOrder INTEGER, icon TEXT, mediaType TEXT, provider TEXT, lastScan TEXT, lastScanVersion TEXT, settings TEXT, createdAt TEXT, updatedAt TEXT)`,
		`CREATE TABLE libraryFolders (id TEXT PRIMARY KEY, path TEXT, libraryId TEXT, createdAt TEXT, updatedAt TEXT)`,
		`CREATE TABLE libraryItems (id TEXT PRIMARY KEY, ino TEXT, libraryId TEXT, path TEXT, relPath TEXT, isFile INTEGER, mtime TEXT, ctime TEXT, birthtime TEXT, createdAt TEXT, updatedAt TEXT, isMissing INTEGER, isInvalid INTEGER, mediaType TEXT, mediaId TEXT, size INTEGER, libraryFolderId TEXT, authorNamesFirstLast TEXT, authorNamesLastFirst TEXT, title TEXT, titleIgnorePrefix TEXT)`,
		`CREATE TABLE books (id TEXT PRIMARY KEY, title TEXT, titleIgnorePrefix TEXT, subtitle TEXT, publishedYear TEXT, publishedDate TEXT, publisher TEXT, description TEXT, isbn TEXT, asin TEXT, language TEXT, explicit INTEGER, abridged INTEGER, coverPath TEXT, duration REAL, narrators BLOB, audioFiles BLOB, ebookFile BLOB, chapters BLOB, tags BLOB, genres BLOB)`,
		`CREATE TABLE podcasts (id TEXT PRIMARY KEY, title TEXT, titleIgnorePrefix TEXT, author TEXT, releaseDate TEXT, feedURL TEXT, imageURL TEXT, description TEXT, itunesPageURL TEXT, itunesId TEXT, itunesArtistId TEXT, language TEXT, podcastType TEXT, explicit INTEGER, autoDownloadEpisodes INTEGER, autoDownloadSchedule TEXT, lastEpisodeCheck TEXT, maxEpisodesToKeep INTEGER, maxNewEpisodesToDownload INTEGER, coverPath TEXT, tags BLOB, genres BLOB, numEpisodes INTEGER)`,
		`CREATE TABLE bookSeries (bookId TEXT, seriesId TEXT, sequence TEXT)`,
		`CREATE TABLE series (id TEXT PRIMARY KEY, name TEXT)`,
		`CREATE TABLE mediaProgresses (id TEXT PRIMARY KEY, userId TEXT, mediaItemId TEXT, isFinished INTEGER, currentTime REAL, updatedAt TEXT)`,
		`CREATE TABLE playbackSessions (id TEXT PRIMARY KEY, userId TEXT, mediaItemId TEXT, mediaItemType TEXT, startTime REAL, libraryId TEXT, extraData TEXT, createdAt TEXT, updatedAt TEXT)`,
		`CREATE TABLE podcastEpisodes (id TEXT PRIMARY KEY, podcastId TEXT, title TEXT, audioFile TEXT)`,
		`CREATE TABLE playlists (id TEXT PRIMARY KEY, name TEXT NOT NULL, description TEXT, createdAt TEXT, updatedAt TEXT, libraryId TEXT, userId TEXT)`,
		`CREATE TABLE playlistMediaItems (id TEXT PRIMARY KEY, mediaItemId TEXT, mediaItemType TEXT, "order" INTEGER, createdAt TEXT, playlistId TEXT)`,
	}

	for _, q := range queries {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("Failed to execute query %q: %v", q, err)
		}
	}

	_, err = db.Exec(`INSERT INTO settings (key, value) VALUES ('server-settings', '{"sortingIgnorePrefix":true,"tokenSecret":"test-secret-12345"}')`)
	if err != nil {
		t.Fatalf("Failed to insert settings: %v", err)
	}

	return db
}

func TestSocketStress(t *testing.T) {
	db := setupStressTestDB(t)
	defer db.Close()

	cachedSecret = "test-secret-12345"

	// Seed users with various roles
	insertTestUser(t, db, "admin-user", "admin-user", "admin", true, true, true, true, true, false, nil, nil)
	insertTestUser(t, db, "regular-user-1", "regular-user-1", "user", true, true, false, true, true, false, nil, nil)
	insertTestUser(t, db, "regular-user-2", "regular-user-2", "user", true, true, true, true, false, false, nil, []string{"TagA"})

	sa := NewAuthority(db)
	handler := InitSocketAuthority(sa)
	defer sa.Close()

	server := httptest.NewServer(handler)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	const concurrency = 15
	var wg sync.WaitGroup
	wg.Add(concurrency)

	// WaitGroup to ensure all clients are authenticated before starting broadcasts
	var authWg sync.WaitGroup
	authWg.Add(concurrency)

	// Channel to start active load test phase
	startLoadSignal := make(chan struct{})

	for i := 0; i < concurrency; i++ {
		time.Sleep(25 * time.Millisecond)
		go func(id int) {
			defer wg.Done()

			// Randomly assign a user
			var userID, username, uType string
			switch rand.Intn(3) {
			case 0:
				userID, username, uType = "admin-user", "admin-user", "admin"
			case 1:
				userID, username, uType = "regular-user-1", "regular-user-1", "user"
			default:
				userID, username, uType = "regular-user-2", "regular-user-2", "user"
			}

			// Establish connection
			u, err := url.Parse(wsURL)
			if err != nil {
				t.Errorf("[Client %d] Failed to parse URL: %v", id, err)
				authWg.Done()
				return
			}
			u.Path = "/socket.io/"
			u.RawQuery = "EIO=4&transport=websocket"

			c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
			if err != nil {
				t.Errorf("[Client %d] WebSocket connection failed: %v", id, err)
				authWg.Done()
				return
			}
			defer c.Close()

			// Engine.io Handshake
			_, _, err = c.ReadMessage()
			if err != nil {
				authWg.Done()
				return
			}
			_ = c.WriteMessage(websocket.TextMessage, []byte("40"))
			_, _, err = c.ReadMessage()
			if err != nil {
				authWg.Done()
				return
			}

			tc := newTestClient(c)

			// Auth
			token, err := generateTestToken(userID, username, uType, cachedSecret)
			if err != nil {
				t.Errorf("[Client %d] Failed to generate token: %v", id, err)
				authWg.Done()
				return
			}
			_ = c.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf(`42["auth",%q]`, token)))

			// Wait for init event robustly with 60s timeout
			foundInit := false
			timeout := time.After(60 * time.Second)
			for !foundInit {
				select {
				case ev := <-tc.Chan:
					if ev.Name == "init" {
						foundInit = true
					}
				case <-timeout:
					t.Errorf("[Client %d] Timeout waiting for init", id)
					authWg.Done()
					return
				}
			}

			// Signal authentication completed
			authWg.Done()

			// Wait for main load test phase to start
			<-startLoadSignal

			// Perform multiple operations
			for step := 0; step < 5; step++ {
				// 1. Ping
				_ = c.WriteMessage(websocket.TextMessage, []byte(`42["ping"]`))

				// 2. Cover Search
				reqID := fmt.Sprintf("req-%d-%d", id, step)
				coverSearchPayload := fmt.Sprintf(`42["search_covers",{"requestId":%q,"title":"Test Book"}]`, reqID)
				_ = c.WriteMessage(websocket.TextMessage, []byte(coverSearchPayload))

				// 3. Wait a tiny bit then Cancel Cover Search
				time.Sleep(10 * time.Millisecond)
				cancelPayload := fmt.Sprintf(`42["cancel_cover_search",%q]`, reqID)
				_ = c.WriteMessage(websocket.TextMessage, []byte(cancelPayload))

				// 4. Log listeners
				if uType == "admin" {
					_ = c.WriteMessage(websocket.TextMessage, []byte(`42["set_log_listener",3]`))
					time.Sleep(5 * time.Millisecond)
					_ = c.WriteMessage(websocket.TextMessage, []byte(`42["remove_log_listener"]`))
				}

				time.Sleep(10 * time.Millisecond)
			}

			// Drain remaining events or wait before disconnect
			time.Sleep(50 * time.Millisecond)
		}(i)
	}

	// Wait for all clients to connect and authenticate successfully
	authWg.Wait()

	// Start background broadcasts to create pressure on mutexes/maps
	stopBroadcasts := make(chan struct{})
	var broadcastWg sync.WaitGroup
	broadcastWg.Add(1)
	go func() {
		defer broadcastWg.Done()
		ticker := time.NewTicker(20 * time.Millisecond)
		defer ticker.Stop()

		itemCount := 0
		for {
			select {
			case <-stopBroadcasts:
				return
			case <-ticker.C:
				itemCount++
				// Random broadcasts
				switch rand.Intn(4) {
				case 0:
					sa.LibraryItemEmitter("item_update", map[string]interface{}{
						"libraryId": "lib-1",
						"media": map[string]interface{}{
							"explicit": rand.Float32() > 0.5,
							"tags":     []interface{}{"TagA", "TagB"},
						},
					})
				case 1:
					sa.BroadcastPlaybackSessionAdded("regular-user-1", fmt.Sprintf("sess-%d", itemCount))
				case 2:
					sa.BroadcastPlaybackSessionUpdated("regular-user-1", fmt.Sprintf("sess-%d", itemCount))
				default:
					sa.BroadcastPlaybackSessionRemoved("regular-user-1", fmt.Sprintf("sess-%d", itemCount))
				}
			}
		}
	}()

	// Signal workers to start the load test phase
	close(startLoadSignal)

	// Wait for workers to finish
	wg.Wait()

	// Stop background broadcasts
	close(stopBroadcasts)
	broadcastWg.Wait()
}
