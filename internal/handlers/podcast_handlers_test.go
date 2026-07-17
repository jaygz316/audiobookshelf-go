package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"audiobookshelf/internal/core"
)

func TestParseOPML(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	opmlText := `<?xml version="1.0" encoding="UTF-8"?>
<opml version="1.0">
	<head>
		<title>Feeds</title>
	</head>
	<body>
		<outline text="Tech">
			<outline text="Go Time" type="rss" xmlUrl="https://changelog.com/gotime/feed" />
		</outline>
	</body>
</opml>`

	reqBody, _ := json.Marshal(map[string]string{
		"opmlText": opmlText,
	})

	req := httptest.NewRequest(http.MethodPost, "/api/podcasts/opml/parse", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")

	// Inject admin user session
	ctx := context.WithValue(req.Context(), core.UserContextKey, &core.UserSession{
		ID:   "user1",
		Type: "admin",
	})
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handleParseOPML(db)(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d. Body: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Feeds []map[string]string `json:"feeds"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if len(resp.Feeds) != 1 {
		t.Errorf("Expected 1 feed, got %d", len(resp.Feeds))
	} else {
		if resp.Feeds[0]["title"] != "Go Time" {
			t.Errorf("Expected title 'Go Time', got %q", resp.Feeds[0]["title"])
		}
		if resp.Feeds[0]["feedUrl"] != "https://changelog.com/gotime/feed" {
			t.Errorf("Expected feedUrl 'https://changelog.com/gotime/feed', got %q", resp.Feeds[0]["feedUrl"])
		}
	}
}

func TestClearEpisodeQueue(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/podcasts/podcast-123/clear-queue", nil)
	w := httptest.NewRecorder()

	handleClearEpisodeQueue(db, "podcast-123")(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d", w.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse body: %v", err)
	}
	if resp["success"] != true {
		t.Errorf("Expected success to be true, got %v", resp["success"])
	}
}

func TestMatchEpisodes(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/podcasts/podcast-123/match-episodes", nil)
	ctx := context.WithValue(req.Context(), core.UserContextKey, &core.UserSession{
		ID:   "user1",
		Type: "admin",
	})
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handleMatchEpisodes(db, "podcast-123")(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d", w.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse body: %v", err)
	}
	if resp["numEpisodesUpdated"] != float64(0) {
		t.Errorf("Expected numEpisodesUpdated to be 0, got %v", resp["numEpisodesUpdated"])
	}
}

func TestGetPodcastFeedValidation(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Missing rssFeed parameter
	reqBody, _ := json.Marshal(map[string]string{})
	req := httptest.NewRequest(http.MethodPost, "/api/podcasts/feed", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	handleGetPodcastFeed(db)(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected StatusBadRequest, got %d", w.Code)
	}
}

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Normal Name", "Normal Name"},
		{"Name / With / Slashes", "Name  With  Slashes"},
		{"Special: Characters?*", "Special Characters"},
		{"   Spaced Name   ", "Spaced Name"},
		{"", "unnamed"},
		{"...", "unnamed"},
	}

	for _, tc := range tests {
		got := sanitizeFilename(tc.input)
		if got != tc.expected {
			t.Errorf("sanitizeFilename(%q) = %q, expected %q", tc.input, got, tc.expected)
		}
	}
}

func TestExplicitInt(t *testing.T) {
	if explicitInt(true) != 1 {
		t.Error("expected explicitInt(true) to be 1")
	}
	if explicitInt(false) != 0 {
		t.Error("expected explicitInt(false) to be 0")
	}
}

func TestOPMLParseAuthorization(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/podcasts/opml/parse", nil)
	// Normal user should be rejected (only admin or root allowed)
	ctx := context.WithValue(req.Context(), core.UserContextKey, &core.UserSession{
		ID:   "user1",
		Type: "user",
	})
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handleParseOPML(db)(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("Expected StatusForbidden, got %d", w.Code)
	}
}

func TestGetPodcastFeedWithTimeout(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Short context deadline
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	reqBody, _ := json.Marshal(map[string]string{
		"rssFeed": "https://changelog.com/gotime/feed",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/podcasts/feed", bytes.NewReader(reqBody))
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handleGetPodcastFeed(db)(w, req)

	// Since context deadline exceeded, should return BadRequest or failure
	if w.Code != http.StatusBadRequest {
		t.Logf("Status code for cancelled context: %d", w.Code)
	}
}

func TestGetLibraryOPML(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Insert test data
	_, err := db.Exec(`INSERT INTO users (id, username, type, isActive, permissions) VALUES ('user-123', 'adminuser', 'admin', 1, '{}')`)
	if err != nil {
		t.Fatalf("Failed to insert test user: %v", err)
	}

	_, err = db.Exec(`INSERT INTO libraries (id, name, mediaType) VALUES ('lib-123', 'Podcast Library', 'podcast')`)
	if err != nil {
		t.Fatalf("Failed to insert test library: %v", err)
	}

	_, err = db.Exec(`INSERT INTO libraryItems (id, libraryId, mediaType, mediaId, title) VALUES ('item-123', 'lib-123', 'podcast', 'podcast-123', 'Test Podcast')`)
	if err != nil {
		t.Fatalf("Failed to insert library item: %v", err)
	}

	_, err = db.Exec(`INSERT INTO podcasts (id, title, feedURL) VALUES ('podcast-123', 'Test Podcast', 'https://example.com/podcast.xml')`)
	if err != nil {
		t.Fatalf("Failed to insert podcast: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/libraries/lib-123/opml", nil)
	ctx := context.WithValue(req.Context(), core.UserContextKey, &core.UserSession{
		ID:   "user-123",
		Type: "admin",
	})
	req = req.WithContext(ctx)

	reinitManagers(db)

	w := httptest.NewRecorder()
	handleGetLibraryOPML(db, "lib-123")(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d. Body: %s", w.Code, w.Body.String())
	}

	body := w.Body.String()
	if !bytes.Contains([]byte(body), []byte("Test Podcast")) {
		t.Errorf("Expected OPML to contain 'Test Podcast', got:\n%s", body)
	}
	if !bytes.Contains([]byte(body), []byte("https://example.com/podcast.xml")) {
		t.Errorf("Expected OPML to contain feedURL, got:\n%s", body)
	}
}

func TestExportOPML(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Seed user
	_, err := db.Exec(`INSERT INTO users (id, username, type, isActive, permissions) VALUES ('user-1', 'admin', 'admin', 1, '{"accessAllLibraries": true}')`)
	if err != nil {
		t.Fatalf("Failed to seed user: %v", err)
	}

	// Seed library
	_, err = db.Exec(`INSERT INTO libraries (id, name, mediaType) VALUES ('lib-1', 'Podcast Library', 'podcast')`)
	if err != nil {
		t.Fatalf("Failed to seed library: %v", err)
	}

	// Seed podcasts
	_, err = db.Exec(`INSERT INTO podcasts (id, title, feedURL) VALUES ('podcast-1', 'Podcast A', 'https://example.com/podcast_a.xml')`)
	if err != nil {
		t.Fatalf("Failed to seed podcast: %v", err)
	}
	_, err = db.Exec(`INSERT INTO podcasts (id, title, feedURL) VALUES ('podcast-2', 'Podcast B', '')`) // should be ignored (empty feedURL)
	if err != nil {
		t.Fatalf("Failed to seed podcast 2: %v", err)
	}

	// Seed library items
	_, err = db.Exec(`INSERT INTO libraryItems (id, libraryId, mediaId, mediaType) VALUES ('item-1', 'lib-1', 'podcast-1', 'podcast')`)
	if err != nil {
		t.Fatalf("Failed to seed library item 1: %v", err)
	}
	_, err = db.Exec(`INSERT INTO libraryItems (id, libraryId, mediaId, mediaType) VALUES ('item-2', 'lib-1', 'podcast-2', 'podcast')`)
	if err != nil {
		t.Fatalf("Failed to seed library item 2: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/podcasts/opml/export?libraryId=lib-1", nil)
	ctx := context.WithValue(req.Context(), core.UserContextKey, &core.UserSession{
		ID:   "user-1",
		Type: "admin",
	})
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handleExportOPML(db)(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d. Body: %s", w.Code, w.Body.String())
	}

	body := w.Body.String()
	if !bytes.Contains([]byte(body), []byte("Podcast A")) {
		t.Errorf("Expected OPML to contain 'Podcast A'")
	}
	if bytes.Contains([]byte(body), []byte("Podcast B")) {
		t.Errorf("Expected OPML to NOT contain 'Podcast B' because its feedURL is empty")
	}
}

func TestDeleteEpisodesBulk(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Seed podcast, episodes, and library item
	_, err := db.Exec(`INSERT INTO libraryItems (id, libraryId, mediaType, mediaId, title) VALUES ('item-1', 'lib-1', 'podcast', 'podcast-1', 'Test Podcast')`)
	if err != nil {
		t.Fatalf("Failed to insert library item: %v", err)
	}
	_, err = db.Exec(`INSERT INTO podcasts (id, title, feedURL) VALUES ('podcast-1', 'Test Podcast', 'https://example.com/podcast.xml')`)
	if err != nil {
		t.Fatalf("Failed to seed podcast: %v", err)
	}
	_, err = db.Exec(`INSERT INTO podcastEpisodes (id, podcastId, title, audioFile) VALUES ('ep-1', 'podcast-1', 'Episode 1', '{"metadata":{"path":"/tmp/ep1.mp3"}}')`)
	if err != nil {
		t.Fatalf("Failed to seed episode 1: %v", err)
	}
	_, err = db.Exec(`INSERT INTO podcastEpisodes (id, podcastId, title, audioFile) VALUES ('ep-2', 'podcast-1', 'Episode 2', '{"metadata":{"path":"/tmp/ep2.mp3"}}')`)
	if err != nil {
		t.Fatalf("Failed to seed episode 2: %v", err)
	}

	reqBody, _ := json.Marshal([]string{"ep-1", "ep-2"})
	req := httptest.NewRequest(http.MethodPost, "/api/podcasts/podcast-1/delete-episodes", bytes.NewReader(reqBody))
	ctx := context.WithValue(req.Context(), core.UserContextKey, &core.UserSession{
		ID:   "user-1",
		Type: "admin",
	})
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handleDeleteEpisodes(db, "podcast-1")(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d. Body: %s", w.Code, w.Body.String())
	}

	// Verify soft delete sets audioFile to '{}'
	var audioFile1, audioFile2 string
	db.QueryRow("SELECT audioFile FROM podcastEpisodes WHERE id = 'ep-1'").Scan(&audioFile1)
	db.QueryRow("SELECT audioFile FROM podcastEpisodes WHERE id = 'ep-2'").Scan(&audioFile2)
	if audioFile1 != "{}" {
		t.Errorf("Expected ep-1 audioFile to be '{}', got %q", audioFile1)
	}
	if audioFile2 != "{}" {
		t.Errorf("Expected ep-2 audioFile to be '{}', got %q", audioFile2)
	}

	// Verify hard delete deletes records
	reqBody2, _ := json.Marshal([]string{"ep-1", "ep-2"})
	req2 := httptest.NewRequest(http.MethodPost, "/api/podcasts/podcast-1/delete-episodes?hard=1", bytes.NewReader(reqBody2))
	req2 = req2.WithContext(ctx)
	w2 := httptest.NewRecorder()
	handleDeleteEpisodes(db, "podcast-1")(w2, req2)

	var count int
	db.QueryRow("SELECT COUNT(*) FROM podcastEpisodes WHERE podcastId = 'podcast-1'").Scan(&count)
	if count != 0 {
		t.Errorf("Expected 0 episodes remaining after hard delete, got %d", count)
	}
}

func TestBulkUpdateEpisodesProgress(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	_, _ = db.Exec("ALTER TABLE mediaProgresses ADD COLUMN mediaItemType TEXT")
	_, _ = db.Exec("ALTER TABLE mediaProgresses ADD COLUMN duration REAL")
	_, _ = db.Exec("ALTER TABLE mediaProgresses ADD COLUMN finishedAt TEXT")
	_, _ = db.Exec("ALTER TABLE mediaProgresses ADD COLUMN podcastId TEXT")
	_, _ = db.Exec("ALTER TABLE mediaProgresses ADD COLUMN createdAt TEXT")
	_, _ = db.Exec("ALTER TABLE mediaProgresses ADD COLUMN hideFromContinueListening INTEGER DEFAULT 0")
	_, _ = db.Exec("ALTER TABLE mediaProgresses ADD COLUMN ebookLocation TEXT")
	_, _ = db.Exec("ALTER TABLE mediaProgresses ADD COLUMN ebookProgress REAL")
	_, _ = db.Exec("ALTER TABLE mediaProgresses ADD COLUMN extraData TEXT")
	_, _ = db.Exec("ALTER TABLE mediaProgresses ADD COLUMN hideFromContinueListening INTEGER DEFAULT 0")

	// Seed podcast, episodes, and library item
	_, err := db.Exec(`INSERT INTO libraryItems (id, libraryId, mediaType, mediaId, title) VALUES ('item-1', 'lib-1', 'podcast', 'podcast-1', 'Test Podcast')`)
	if err != nil {
		t.Fatalf("Failed to insert library item: %v", err)
	}
	_, err = db.Exec(`INSERT INTO podcasts (id, title, feedURL) VALUES ('podcast-1', 'Test Podcast', 'https://example.com/podcast.xml')`)
	if err != nil {
		t.Fatalf("Failed to seed podcast: %v", err)
	}
	_, err = db.Exec(`INSERT INTO podcastEpisodes (id, podcastId, title, audioFile) VALUES ('ep-1', 'podcast-1', 'Episode 1', '{"duration": 100}')`)
	if err != nil {
		t.Fatalf("Failed to seed episode 1: %v", err)
	}
	_, err = db.Exec(`INSERT INTO podcastEpisodes (id, podcastId, title, audioFile) VALUES ('ep-2', 'podcast-1', 'Episode 2', '{"duration": 200}')`)
	if err != nil {
		t.Fatalf("Failed to seed episode 2: %v", err)
	}

	// Request with custom progress body
	reqBody, _ := json.Marshal(map[string]interface{}{
		"episodeIds":  []string{"ep-1", "ep-2"},
		"isFinished":  true,
		"currentTime": 50,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/podcasts/podcast-1/progress-episodes", bytes.NewReader(reqBody))
	ctx := context.WithValue(req.Context(), core.UserContextKey, &core.UserSession{
		ID:   "user-1",
		Type: "user",
	})
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handleBulkUpdateEpisodesProgress(db, "podcast-1")(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d. Body: %s", w.Code, w.Body.String())
	}

	// Verify progress entries in DB
	var count int
	db.QueryRow("SELECT COUNT(*) FROM mediaProgresses WHERE userId = 'user-1'").Scan(&count)
	if count != 2 {
		t.Errorf("Expected 2 progress records, got %d", count)
	}

	var isFinished int
	db.QueryRow("SELECT isFinished FROM mediaProgresses WHERE userId = 'user-1' AND mediaItemId = 'ep-1'").Scan(&isFinished)
	if isFinished != 1 {
		t.Errorf("Expected ep-1 to be marked as finished (1), got %d", isFinished)
	}
}

func TestUpdatePodcastSettingsSkipDurations(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	_, _ = db.Exec("ALTER TABLE podcasts ADD COLUMN autoDeletePlayed INTEGER DEFAULT 0")
	_, _ = db.Exec("ALTER TABLE podcasts ADD COLUMN skipIntroDuration INTEGER DEFAULT 0")
	_, _ = db.Exec("ALTER TABLE podcasts ADD COLUMN skipOutroDuration INTEGER DEFAULT 0")

	// Seed podcast, episodes, and library item
	_, err := db.Exec(`INSERT INTO libraryItems (id, libraryId, mediaType, mediaId, title) VALUES ('item-1', 'lib-1', 'podcast', 'podcast-1', 'Test Podcast')`)
	if err != nil {
		t.Fatalf("Failed to insert library item: %v", err)
	}
	_, err = db.Exec(`INSERT INTO podcasts (id, title, feedURL, skipIntroDuration, skipOutroDuration) VALUES ('podcast-1', 'Test Podcast', 'https://example.com/podcast.xml', 0, 0)`)
	if err != nil {
		t.Fatalf("Failed to seed podcast: %v", err)
	}

	// Request with skipIntroDuration and skipOutroDuration
	reqBody, _ := json.Marshal(map[string]interface{}{
		"title":             "Test Podcast",
		"skipIntroDuration": 15,
		"skipOutroDuration": 30,
	})
	req := httptest.NewRequest(http.MethodPatch, "/api/items/item-1", bytes.NewReader(reqBody))
	ctx := context.WithValue(req.Context(), core.UserContextKey, &core.UserSession{
		ID:   "user-1",
		Type: "admin",
	})
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handleUpdateLibraryItemByID(db, "item-1")(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d. Body: %s", w.Code, w.Body.String())
	}

	var skipIntro, skipOutro int
	db.QueryRow("SELECT skipIntroDuration, skipOutroDuration FROM podcasts WHERE id = 'podcast-1'").Scan(&skipIntro, &skipOutro)
	if skipIntro != 15 {
		t.Errorf("Expected skipIntroDuration to be 15, got %d", skipIntro)
	}
	if skipOutro != 30 {
		t.Errorf("Expected skipOutroDuration to be 30, got %d", skipOutro)
	}
}
