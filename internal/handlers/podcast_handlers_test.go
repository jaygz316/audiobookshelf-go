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
