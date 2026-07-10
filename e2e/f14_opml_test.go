package e2e

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite"
)

func TestF14OPMLImportExport(t *testing.T) {
	t.Setenv("BYPASS_SAFEURL", "true")
	h := NewTestHarness()
	if err := h.Start(); err != nil {
		t.Fatalf("Failed to start harness: %v", err)
	}
	defer func() {
		if t.Failed() {
			logPath := filepath.Join(h.MetadataDir, "server.log")
			if content, err := os.ReadFile(logPath); err == nil {
				t.Logf("--- SERVER LOG ---\n%s\n------------------", string(content))
			} else {
				t.Logf("Failed to read server.log: %v", err)
			}
		}
		h.Stop()
	}()

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("Failed to create cookie jar: %v", err)
	}
	client := &http.Client{Jar: jar}

	// 1. Setup Admin Root and Login
	initPayload := map[string]interface{}{
		"newRoot": map[string]string{
			"username": "rootadmin",
			"password": "supersecurepassword123",
		},
	}
	initBody, _ := json.Marshal(initPayload)
	resp, err := client.Post(h.BaseURL+"/init", "application/json", bytes.NewReader(initBody))
	if err != nil {
		t.Fatalf("Failed to initialize root: %v", err)
	}
	resp.Body.Close()

	// login
	loginPayload := map[string]string{
		"username": "rootadmin",
		"password": "supersecurepassword123",
	}
	loginBody, _ := json.Marshal(loginPayload)
	resp, err = client.Post(h.BaseURL+"/login", "application/json", bytes.NewReader(loginBody))
	if err != nil {
		t.Fatalf("Failed to login admin: %v", err)
	}
	var adminResp map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&adminResp)
	resp.Body.Close()
	adminToken := adminResp["user"].(map[string]interface{})["accessToken"].(string)

	// Create podcast library
	createPayload := map[string]interface{}{
		"name":      "E2E Podcast Library",
		"mediaType": "podcast",
		"folders": []map[string]string{
			{"path": t.TempDir()},
		},
	}
	createBody, _ := json.Marshal(createPayload)
	req, _ := http.NewRequest("POST", h.BaseURL+"/api/libraries", bytes.NewReader(createBody))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("Failed to create library: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Create library status: %d", resp.StatusCode)
	}
	var createdLib map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&createdLib)
	resp.Body.Close()
	libraryID := createdLib["id"].(string)

	// 2. Start a mock RSS feed server
	rssContent := `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>E2E Mock Podcast Feed</title>
    <author>E2E Author</author>
    <description>E2E Description</description>
    <item>
      <title>E2E Episode 1</title>
      <description>E2E Episode 1 Description</description>
      <enclosure url="http://example.com/e2e-ep1.mp3" type="audio/mpeg" />
      <pubDate>Fri, 10 Jul 2026 12:00:00 GMT</pubDate>
    </item>
  </channel>
</rss>`

	mockRSS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		w.Write([]byte(rssContent))
	}))
	defer mockRSS.Close()

	// 3. Test: Parse OPML via POST /api/podcasts/opml/parse
	var parsedFeedURL string
	t.Run("POST /api/podcasts/opml/parse - Valid OPML", func(t *testing.T) {
		opmlText := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<opml version="1.0">
  <head>
    <title>Podcast Subscriptions</title>
  </head>
  <body>
    <outline text="E2E Mock Podcast Feed" type="rss" xmlUrl="%s" htmlUrl="" />
  </body>
</opml>`, mockRSS.URL)

		reqPayload := map[string]string{
			"opmlText": opmlText,
		}
		reqBody, _ := json.Marshal(reqPayload)
		req, _ = http.NewRequest("POST", h.BaseURL+"/api/podcasts/opml/parse", bytes.NewReader(reqBody))
		req.Header.Set("Authorization", "Bearer "+adminToken)
		req.Header.Set("Content-Type", "application/json")

		resp, err = client.Do(req)
		if err != nil {
			t.Fatalf("Parse OPML failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Expected status 200, got %d", resp.StatusCode)
		}

		var parseResp struct {
			Feeds []map[string]string `json:"feeds"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&parseResp); err != nil {
			t.Fatalf("Failed to decode parse response: %v", err)
		}

		if len(parseResp.Feeds) != 1 {
			t.Fatalf("Expected 1 parsed feed, got %d", len(parseResp.Feeds))
		}

		feed := parseResp.Feeds[0]
		urlKey := "xmlUrl"
		if _, ok := feed[urlKey]; !ok {
			urlKey = "feedUrl"
		}
		parsedFeedURL = feed[urlKey]
		if parsedFeedURL != mockRSS.URL {
			t.Errorf("Expected feed XML URL %q, got %q", mockRSS.URL, parsedFeedURL)
		}
	})

	// 4. Test: Import Podcast via POST /api/podcasts/opml/create
	t.Run("POST /api/podcasts/opml/create - Create Podcasts", func(t *testing.T) {
		if parsedFeedURL == "" {
			t.Skip("Parsed feed URL is empty")
		}

		createPayload := map[string]interface{}{
			"feeds":                []string{parsedFeedURL},
			"libraryId":            libraryID,
			"autoDownloadEpisodes": false,
		}
		createBody, _ := json.Marshal(createPayload)
		req, _ = http.NewRequest("POST", h.BaseURL+"/api/podcasts/opml/create", bytes.NewReader(createBody))
		req.Header.Set("Authorization", "Bearer "+adminToken)
		req.Header.Set("Content-Type", "application/json")

		resp, err = client.Do(req)
		if err != nil {
			t.Fatalf("Create podcast from OPML failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Expected status 200, got %d", resp.StatusCode)
		}

		// Wait and poll for podcast to be parsed and created in the DB
		db, err := sql.Open("sqlite", h.DBPath)
		if err != nil {
			t.Fatalf("Failed to open DB: %v", err)
		}
		defer db.Close()

		podcastCreated := false
		for attempt := 0; attempt < 30; attempt++ {
			time.Sleep(150 * time.Millisecond)
			var count int
			err = db.QueryRow("SELECT COUNT(*) FROM podcasts WHERE feedURL = ?", parsedFeedURL).Scan(&count)
			if err == nil && count > 0 {
				podcastCreated = true
				break
			}
		}

		if !podcastCreated {
			t.Fatalf("Podcast was not created in the database after polling")
		}

		// Verify podcast info
		var title, author, description string
		err = db.QueryRow("SELECT title, author, description FROM podcasts WHERE feedURL = ?", parsedFeedURL).
			Scan(&title, &author, &description)
		if err != nil {
			t.Fatalf("Failed to query podcast from DB: %v", err)
		}

		if title != "E2E Mock Podcast Feed" {
			t.Errorf("Expected title 'E2E Mock Podcast Feed', got %q", title)
		}
		if author != "E2E Author" {
			t.Errorf("Expected author 'E2E Author', got %q", author)
		}
		if !strings.Contains(description, "E2E Description") {
			t.Errorf("Expected description to contain 'E2E Description', got %q", description)
		}
	})

	// 5. Test: Export OPML via GET /api/libraries/:id/opml
	t.Run("GET /api/libraries/:id/opml - Export OPML", func(t *testing.T) {
		req, _ = http.NewRequest("GET", h.BaseURL+"/api/libraries/"+libraryID+"/opml", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)

		resp, err = client.Do(req)
		if err != nil {
			t.Fatalf("Export OPML failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Expected status 200, got %d", resp.StatusCode)
		}

		var buf bytes.Buffer
		buf.ReadFrom(resp.Body)
		opmlContent := buf.String()

		if !strings.Contains(opmlContent, "<opml") {
			t.Errorf("Expected OPML content to contain '<opml', got %q", opmlContent)
		}
		if !strings.Contains(opmlContent, "E2E Mock Podcast Feed") {
			t.Errorf("Expected OPML content to contain podcast title, got %q", opmlContent)
		}
		if !strings.Contains(opmlContent, parsedFeedURL) {
			t.Errorf("Expected OPML content to contain feed URL %q, got %q", parsedFeedURL, opmlContent)
		}
	})

	// 6. Access Control Checks
	t.Run("Access Control - Restricted User Blocked", func(t *testing.T) {
		db, err := sql.Open("sqlite", h.DBPath)
		if err != nil {
			t.Fatalf("Failed to open DB: %v", err)
		}
		defer db.Close()

		hashedPash, _ := bcrypt.GenerateFromPassword([]byte("userpass123"), 8)
		userID := uuid.New().String()
		permsJSON := `{"download":true,"accessExplicitContent":false,"accessAllLibraries":true,"accessAllTags":true}`
		_, err = db.Exec(`INSERT INTO users (id, username, type, pash, isActive, permissions, extraData, bookmarks, createdAt, updatedAt)
			VALUES (?, 'standard_user', 'user', ?, 1, ?, '{}', '[]', datetime('now'), datetime('now'))`,
			userID, string(hashedPash), permsJSON)
		if err != nil {
			t.Fatalf("Failed to insert standard user: %v", err)
		}

		// Login standard user
		uLoginPayload := map[string]string{
			"username": "standard_user",
			"password": "userpass123",
		}
		uLoginBody, _ := json.Marshal(uLoginPayload)
		respU, err := client.Post(h.BaseURL+"/login", "application/json", bytes.NewReader(uLoginBody))
		if err != nil {
			t.Fatalf("Failed to login standard user: %v", err)
		}
		var uResp map[string]interface{}
		json.NewDecoder(respU.Body).Decode(&uResp)
		respU.Body.Close()
		userToken := uResp["user"].(map[string]interface{})["accessToken"].(string)

		// A. Parse OPML should return Forbidden for standard user
		reqPayload := map[string]string{
			"opmlText": "<opml></opml>",
		}
		reqBody, _ := json.Marshal(reqPayload)
		req, _ = http.NewRequest("POST", h.BaseURL+"/api/podcasts/opml/parse", bytes.NewReader(reqBody))
		req.Header.Set("Authorization", "Bearer "+userToken)
		req.Header.Set("Content-Type", "application/json")
		resp, _ = client.Do(req)
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("Expected parse OPML to return 403 Forbidden for non-admin, got %d", resp.StatusCode)
		}
		resp.Body.Close()

		// B. Create Podcast should return Forbidden for standard user
		createPayload := map[string]interface{}{
			"feeds":     []string{parsedFeedURL},
			"libraryId": libraryID,
		}
		createBody, _ := json.Marshal(createPayload)
		req, _ = http.NewRequest("POST", h.BaseURL+"/api/podcasts/opml/create", bytes.NewReader(createBody))
		req.Header.Set("Authorization", "Bearer "+userToken)
		req.Header.Set("Content-Type", "application/json")
		resp, _ = client.Do(req)
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("Expected create OPML podcasts to return 403 Forbidden for non-admin, got %d", resp.StatusCode)
		}
		resp.Body.Close()
	})
}
