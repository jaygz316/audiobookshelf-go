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

	_ "modernc.org/sqlite"
)

// setupAdmin performs the bootstrap /init and /login sequence to obtain an admin access token.
func setupAdmin(t *testing.T, h *TestHarness, client *http.Client) string {
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

	loginPayload := map[string]string{
		"username": "rootadmin",
		"password": "supersecurepassword123",
	}
	loginBody, _ := json.Marshal(loginPayload)
	resp, err = client.Post(h.BaseURL+"/login", "application/json", bytes.NewReader(loginBody))
	if err != nil {
		t.Fatalf("Failed to login admin: %v", err)
	}
	defer resp.Body.Close()

	var adminResp map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&adminResp); err != nil {
		t.Fatalf("Failed to decode login response: %v", err)
	}
	userObj, ok := adminResp["user"].(map[string]interface{})
	if !ok {
		t.Fatalf("Response missing 'user' object: %v", adminResp)
	}
	adminToken, ok := userObj["accessToken"].(string)
	if !ok || adminToken == "" {
		t.Fatalf("User object missing 'accessToken': %v", userObj)
	}
	return adminToken
}

// TestSettingsCORS verifies CORS checking dynamically sets Access-Control-Allow-Origin
// if the Origin header matches the configured allowedCorsOrigins.
func TestSettingsCORS(t *testing.T) {
	h := NewTestHarness()
	if err := h.Start(); err != nil {
		t.Fatalf("Failed to start harness: %v", err)
	}
	defer h.Stop()

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("Failed to create cookie jar: %v", err)
	}
	client := &http.Client{Jar: jar}

	adminToken := setupAdmin(t, h, client)

	// 1. By default, allowedCorsOrigins is empty.
	// Access-Control-Allow-Origin should NOT be returned for any Origin.
	req, _ := http.NewRequest("GET", h.BaseURL+"/status", nil)
	req.Header.Set("Origin", "http://example.com")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Failed to send GET /status: %v", err)
	}
	resp.Body.Close()
	if origin := resp.Header.Get("Access-Control-Allow-Origin"); origin != "" {
		t.Errorf("Expected Access-Control-Allow-Origin to be empty initially, got %q", origin)
	}

	// 2. Configure allowedCorsOrigins to "http://example.com,https://test.com"
	patchPayload := map[string]interface{}{
		"allowedCorsOrigins": "http://example.com,https://test.com",
	}
	patchBody, _ := json.Marshal(patchPayload)
	reqPatch, _ := http.NewRequest("PATCH", h.BaseURL+"/api/settings", bytes.NewReader(patchBody))
	reqPatch.Header.Set("Authorization", "Bearer "+adminToken)
	reqPatch.Header.Set("Content-Type", "application/json")
	respPatch, err := client.Do(reqPatch)
	if err != nil {
		t.Fatalf("Failed to update settings: %v", err)
	}
	respPatch.Body.Close()
	if respPatch.StatusCode != http.StatusOK {
		t.Fatalf("Expected PATCH /api/settings status 200, got %d", respPatch.StatusCode)
	}

	// 3. Verify Access-Control-Allow-Origin header behavior
	// Case A: Matches first allowed origin
	reqA, _ := http.NewRequest("GET", h.BaseURL+"/status", nil)
	reqA.Header.Set("Origin", "http://example.com")
	respA, err := client.Do(reqA)
	if err != nil {
		t.Fatalf("Failed to send GET /status: %v", err)
	}
	respA.Body.Close()
	if origin := respA.Header.Get("Access-Control-Allow-Origin"); origin != "http://example.com" {
		t.Errorf("Expected Access-Control-Allow-Origin to be %q, got %q", "http://example.com", origin)
	}

	// Case B: Matches second allowed origin
	reqB, _ := http.NewRequest("GET", h.BaseURL+"/status", nil)
	reqB.Header.Set("Origin", "https://test.com")
	respB, err := client.Do(reqB)
	if err != nil {
		t.Fatalf("Failed to send GET /status: %v", err)
	}
	respB.Body.Close()
	if origin := respB.Header.Get("Access-Control-Allow-Origin"); origin != "https://test.com" {
		t.Errorf("Expected Access-Control-Allow-Origin to be %q, got %q", "https://test.com", origin)
	}

	// Case C: Disallowed origin
	reqC, _ := http.NewRequest("GET", h.BaseURL+"/status", nil)
	reqC.Header.Set("Origin", "http://disallowed.com")
	respC, err := client.Do(reqC)
	if err != nil {
		t.Fatalf("Failed to send GET /status: %v", err)
	}
	respC.Body.Close()
	if origin := respC.Header.Get("Access-Control-Allow-Origin"); origin != "" {
		t.Errorf("Expected Access-Control-Allow-Origin to be empty for disallowed origin, got %q", origin)
	}
}

// TestSettingsIframe verifies that when allowIframe is false, GET /status and GET /
// return frame-blocking headers (X-Frame-Options: SAMEORIGIN & CSP frame-ancestors 'self').
// When allowIframe is true, these security headers/directives are omitted.
func TestSettingsIframe(t *testing.T) {
	h := NewTestHarness()
	if err := h.Start(); err != nil {
		t.Fatalf("Failed to start harness: %v", err)
	}
	defer h.Stop()

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("Failed to create cookie jar: %v", err)
	}
	client := &http.Client{Jar: jar}

	adminToken := setupAdmin(t, h, client)

	// helper to verify headers on /status and / (root index)
	checkHeaders := func(allowIframe bool) {
		endpoints := []string{"/status", "/"}
		for _, ep := range endpoints {
			req, _ := http.NewRequest("GET", h.BaseURL+ep, nil)
			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("Failed to GET %s: %v", ep, err)
			}
			resp.Body.Close()

			xFrame := resp.Header.Get("X-Frame-Options")
			csp := resp.Header.Get("Content-Security-Policy")

			if allowIframe {
				if xFrame != "" {
					t.Errorf("Expected X-Frame-Options to be empty when allowIframe is true for %s, got %q", ep, xFrame)
				}
				if strings.Contains(csp, "frame-ancestors") {
					t.Errorf("Expected CSP to not contain frame-ancestors when allowIframe is true for %s, got %q", ep, csp)
				}
			} else {
				if xFrame != "SAMEORIGIN" {
					t.Errorf("Expected X-Frame-Options: SAMEORIGIN when allowIframe is false for %s, got %q", ep, xFrame)
				}
				if !strings.Contains(csp, "frame-ancestors 'self'") {
					t.Errorf("Expected CSP to contain 'frame-ancestors \\'self\\'' when allowIframe is false for %s, got %q", ep, csp)
				}
			}
		}
	}

	// 1. By default, allowIframe should be false, so it should block iframes.
	checkHeaders(false)

	// 2. PATCH allowIframe = true
	patchPayload := map[string]interface{}{
		"allowIframe": true,
	}
	patchBody, _ := json.Marshal(patchPayload)
	reqPatch, _ := http.NewRequest("PATCH", h.BaseURL+"/api/settings", bytes.NewReader(patchBody))
	reqPatch.Header.Set("Authorization", "Bearer "+adminToken)
	reqPatch.Header.Set("Content-Type", "application/json")
	respPatch, err := client.Do(reqPatch)
	if err != nil {
		t.Fatalf("PATCH settings failed: %v", err)
	}
	respPatch.Body.Close()
	if respPatch.StatusCode != http.StatusOK {
		t.Fatalf("Expected PATCH settings status 200, got %d", respPatch.StatusCode)
	}

	// Verify headers are omitted
	checkHeaders(true)

	// 3. PATCH allowIframe = false
	patchPayload["allowIframe"] = false
	patchBody, _ = json.Marshal(patchPayload)
	reqPatch, _ = http.NewRequest("PATCH", h.BaseURL+"/api/settings", bytes.NewReader(patchBody))
	reqPatch.Header.Set("Authorization", "Bearer "+adminToken)
	reqPatch.Header.Set("Content-Type", "application/json")
	respPatch, err = client.Do(reqPatch)
	if err != nil {
		t.Fatalf("PATCH settings failed: %v", err)
	}
	respPatch.Body.Close()

	// Verify headers return
	checkHeaders(false)
}

// TestSettingsMetadataCover verifies the metadataCoverWithItem setting.
// When enabled, downloaded cover is saved in the item's library folder.
// When disabled, downloaded cover is saved in the metadata/items/{id} folder.
func TestSettingsMetadataCover(t *testing.T) {
	// Bypass safeurl loopback block in handlers package
	os.Setenv("BYPASS_SAFEURL", "true")
	defer os.Unsetenv("BYPASS_SAFEURL")

	h := NewTestHarness()
	if err := h.Start(); err != nil {
		t.Fatalf("Failed to start harness: %v", err)
	}
	defer h.Stop()

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("Failed to create cookie jar: %v", err)
	}
	client := &http.Client{Jar: jar}

	adminToken := setupAdmin(t, h, client)

	// Spin up a mock cover serving server
	mockCoverContent := []byte("mock-jpeg-cover-data")
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(mockCoverContent)
	}))
	defer mockServer.Close()

	// 1. Create a Library
	libraryPath := filepath.Join(h.ConfigDir, "libraryCover")
	createPayload := map[string]interface{}{
		"name":      "Library Cover",
		"mediaType": "book",
		"folders": []map[string]string{
			{"path": libraryPath},
		},
	}
	createBody, _ := json.Marshal(createPayload)
	reqLib, _ := http.NewRequest("POST", h.BaseURL+"/api/libraries", bytes.NewReader(createBody))
	reqLib.Header.Set("Authorization", "Bearer "+adminToken)
	reqLib.Header.Set("Content-Type", "application/json")
	respLib, err := client.Do(reqLib)
	if err != nil {
		t.Fatalf("Failed to create library: %v", err)
	}
	var createdLib map[string]interface{}
	json.NewDecoder(respLib.Body).Decode(&createdLib)
	respLib.Body.Close()
	libraryID := createdLib["id"].(string)

	// 2. Setup Library items (directories + files)
	// Item 1
	bookDir1 := filepath.Join(libraryPath, "Book1")
	if err := os.MkdirAll(bookDir1, 0755); err != nil {
		t.Fatalf("Failed to create book 1 directory: %v", err)
	}
	if err := GenerateMockAudio(filepath.Join(bookDir1, "track1.mp3"), "Book 1 Track", "Author C", "Book One", "1", "2026"); err != nil {
		t.Fatalf("Failed to generate mock audio: %v", err)
	}

	// Item 2
	bookDir2 := filepath.Join(libraryPath, "Book2")
	if err := os.MkdirAll(bookDir2, 0755); err != nil {
		t.Fatalf("Failed to create book 2 directory: %v", err)
	}
	if err := GenerateMockAudio(filepath.Join(bookDir2, "track1.mp3"), "Book 2 Track", "Author C", "Book Two", "1", "2026"); err != nil {
		t.Fatalf("Failed to generate mock audio: %v", err)
	}

	// Trigger Scan
	reqScan, _ := http.NewRequest("POST", h.BaseURL+"/api/libraries/"+libraryID+"/scan", nil)
	reqScan.Header.Set("Authorization", "Bearer "+adminToken)
	respScan, err := client.Do(reqScan)
	if err != nil {
		t.Fatalf("Scan request failed: %v", err)
	}
	respScan.Body.Close()

	// Wait for items to be scanned
	var items []interface{}
	for attempt := 0; attempt < 30; attempt++ {
		time.Sleep(150 * time.Millisecond)
		reqList, _ := http.NewRequest("GET", h.BaseURL+"/api/libraries/"+libraryID+"/items", nil)
		reqList.Header.Set("Authorization", "Bearer "+adminToken)
		respList, err := client.Do(reqList)
		if err == nil {
			var listResp map[string]interface{}
			json.NewDecoder(respList.Body).Decode(&listResp)
			respList.Body.Close()
			if listResp["results"] != nil {
				results := listResp["results"].([]interface{})
				if len(results) >= 2 {
					items = results
					break
				}
			}
		}
	}
	if len(items) < 2 {
		t.Fatalf("Expected at least 2 scanned items, got %d", len(items))
	}

	// Identify item IDs
	var itemID1, itemID2 string
	for _, it := range items {
		m := it.(map[string]interface{})
		relPath := m["relPath"].(string)
		if strings.Contains(relPath, "Book1") {
			itemID1 = m["id"].(string)
		} else if strings.Contains(relPath, "Book2") {
			itemID2 = m["id"].(string)
		}
	}
	if itemID1 == "" || itemID2 == "" {
		t.Fatalf("Could not locate items by relPath. Items: %v", items)
	}

	// Test case A: metadataCoverWithItem is ENABLED
	patchPayload := map[string]interface{}{
		"metadataCoverWithItem": true,
	}
	patchBody, _ := json.Marshal(patchPayload)
	reqPatch, _ := http.NewRequest("PATCH", h.BaseURL+"/api/settings", bytes.NewReader(patchBody))
	reqPatch.Header.Set("Authorization", "Bearer "+adminToken)
	reqPatch.Header.Set("Content-Type", "application/json")
	respPatch, err := client.Do(reqPatch)
	if err != nil {
		t.Fatalf("PATCH settings failed: %v", err)
	}
	respPatch.Body.Close()

	// Download cover for Book1
	coverPayload := map[string]string{
		"coverUrl": mockServer.URL,
	}
	coverBody, _ := json.Marshal(coverPayload)
	reqCover, _ := http.NewRequest("POST", h.BaseURL+fmt.Sprintf("/api/items/%s/cover-from-url", itemID1), bytes.NewReader(coverBody))
	reqCover.Header.Set("Authorization", "Bearer "+adminToken)
	reqCover.Header.Set("Content-Type", "application/json")
	respCover, err := client.Do(reqCover)
	if err != nil {
		t.Fatalf("POST cover-from-url failed: %v", err)
	}
	var coverResp1 map[string]interface{}
	json.NewDecoder(respCover.Body).Decode(&coverResp1)
	respCover.Body.Close()

	if respCover.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 OK for cover-from-url, got %d", respCover.StatusCode)
	}

	// Verify cover is stored in the library folder: bookDir1
	coverPathResult1, ok := coverResp1["coverPath"].(string)
	if !ok || coverPathResult1 == "" {
		t.Errorf("Expected coverPath in response: %v", coverResp1)
	}
	if !strings.HasPrefix(coverPathResult1, bookDir1) {
		t.Errorf("Expected cover to be stored inside library folder %q, got %q", bookDir1, coverPathResult1)
	}
	if _, err := os.Stat(coverPathResult1); err != nil {
		t.Errorf("Cover file does not exist on disk at %q: %v", coverPathResult1, err)
	}

	// Test case B: metadataCoverWithItem is DISABLED
	patchPayload["metadataCoverWithItem"] = false
	patchBody, _ = json.Marshal(patchPayload)
	reqPatch, _ = http.NewRequest("PATCH", h.BaseURL+"/api/settings", bytes.NewReader(patchBody))
	reqPatch.Header.Set("Authorization", "Bearer "+adminToken)
	reqPatch.Header.Set("Content-Type", "application/json")
	respPatch, err = client.Do(reqPatch)
	if err != nil {
		t.Fatalf("PATCH settings failed: %v", err)
	}
	respPatch.Body.Close()

	// Download cover for Book2
	reqCover2, _ := http.NewRequest("POST", h.BaseURL+fmt.Sprintf("/api/items/%s/cover-from-url", itemID2), bytes.NewReader(coverBody))
	reqCover2.Header.Set("Authorization", "Bearer "+adminToken)
	reqCover2.Header.Set("Content-Type", "application/json")
	respCover2, err := client.Do(reqCover2)
	if err != nil {
		t.Fatalf("POST cover-from-url failed: %v", err)
	}
	var coverResp2 map[string]interface{}
	json.NewDecoder(respCover2.Body).Decode(&coverResp2)
	respCover2.Body.Close()

	if respCover2.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 OK for cover-from-url, got %d", respCover2.StatusCode)
	}

	// Verify cover is stored in the internal metadata folder: metadata/items/{id}
	coverPathResult2, ok := coverResp2["coverPath"].(string)
	if !ok || coverPathResult2 == "" {
		t.Errorf("Expected coverPath in response: %v", coverResp2)
	}
	expectedMetadataDir := filepath.Join(h.MetadataDir, "items", itemID2)
	if !strings.HasPrefix(coverPathResult2, expectedMetadataDir) {
		t.Errorf("Expected cover to be stored inside metadata folder %q, got %q", expectedMetadataDir, coverPathResult2)
	}
	if _, err := os.Stat(coverPathResult2); err != nil {
		t.Errorf("Cover file does not exist on disk at %q: %v", coverPathResult2, err)
	}
}

// TestSettingsMetadataMarkdown verifies the metadataMarkdownWithItem setting.
// When enabled, PATCH /api/items/{id} writes/updates metadata.json in the item's library folder.
// When disabled, PATCH /api/items/{id} does not create/update metadata.json in the library folder.
func TestSettingsMetadataMarkdown(t *testing.T) {
	h := NewTestHarness()
	if err := h.Start(); err != nil {
		t.Fatalf("Failed to start harness: %v", err)
	}
	defer h.Stop()

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("Failed to create cookie jar: %v", err)
	}
	client := &http.Client{Jar: jar}

	adminToken := setupAdmin(t, h, client)

	// 1. Create Library
	libraryPath := filepath.Join(h.ConfigDir, "libraryMetadata")
	createPayload := map[string]interface{}{
		"name":      "Library Metadata",
		"mediaType": "book",
		"folders": []map[string]string{
			{"path": libraryPath},
		},
	}
	createBody, _ := json.Marshal(createPayload)
	reqLib, _ := http.NewRequest("POST", h.BaseURL+"/api/libraries", bytes.NewReader(createBody))
	reqLib.Header.Set("Authorization", "Bearer "+adminToken)
	reqLib.Header.Set("Content-Type", "application/json")
	respLib, err := client.Do(reqLib)
	if err != nil {
		t.Fatalf("Failed to create library: %v", err)
	}
	var createdLib map[string]interface{}
	json.NewDecoder(respLib.Body).Decode(&createdLib)
	respLib.Body.Close()
	libraryID := createdLib["id"].(string)

	// 2. Setup Book folder on disk
	bookDir := filepath.Join(libraryPath, "BookMetadata")
	if err := os.MkdirAll(bookDir, 0755); err != nil {
		t.Fatalf("Failed to create book metadata directory: %v", err)
	}
	if err := GenerateMockAudio(filepath.Join(bookDir, "track1.mp3"), "Original Title", "Original Author", "Original Album", "1", "2026"); err != nil {
		t.Fatalf("Failed to generate mock audio: %v", err)
	}

	// Trigger Scan
	reqScan, _ := http.NewRequest("POST", h.BaseURL+"/api/libraries/"+libraryID+"/scan", nil)
	reqScan.Header.Set("Authorization", "Bearer "+adminToken)
	respScan, err := client.Do(reqScan)
	if err != nil {
		t.Fatalf("Scan request failed: %v", err)
	}
	respScan.Body.Close()

	// Wait for item to be scanned
	var itemID string
	for attempt := 0; attempt < 20; attempt++ {
		time.Sleep(150 * time.Millisecond)
		reqList, _ := http.NewRequest("GET", h.BaseURL+"/api/libraries/"+libraryID+"/items", nil)
		reqList.Header.Set("Authorization", "Bearer "+adminToken)
		respList, err := client.Do(reqList)
		if err == nil {
			var listResp map[string]interface{}
			json.NewDecoder(respList.Body).Decode(&listResp)
			respList.Body.Close()
			if listResp["results"] != nil {
				results := listResp["results"].([]interface{})
				if len(results) > 0 {
					itemID = results[0].(map[string]interface{})["id"].(string)
					break
				}
			}
		}
	}
	if itemID == "" {
		t.Fatalf("Expected scanned item ID")
	}

	// Path to metadata.json in the library folder
	metadataJSONPath := filepath.Join(bookDir, "metadata.json")

	// Test case A: metadataMarkdownWithItem is ENABLED
	patchSettingsPayload := map[string]interface{}{
		"metadataMarkdownWithItem": true,
	}
	patchSettingsBody, _ := json.Marshal(patchSettingsPayload)
	reqPatchSettings, _ := http.NewRequest("PATCH", h.BaseURL+"/api/settings", bytes.NewReader(patchSettingsBody))
	reqPatchSettings.Header.Set("Authorization", "Bearer "+adminToken)
	reqPatchSettings.Header.Set("Content-Type", "application/json")
	respPatchSettings, err := client.Do(reqPatchSettings)
	if err != nil {
		t.Fatalf("PATCH settings failed: %v", err)
	}
	respPatchSettings.Body.Close()

	// Perform PATCH /api/items/{id} to update metadata
	updateItemPayload := map[string]interface{}{
		"title":         "Super Cool Title",
		"subtitle":      "Super Cool Subtitle",
		"authors":       []string{"Author One", "Author Two"},
		"narrators":     []string{"Narrator One"},
		"publisher":     "Fantastic Publisher",
		"publishedYear": "2026",
		"publishedDate": "2026-06-19",
		"description":   "The best description ever.",
		"isbn":          "1234567890123",
		"asin":          "B00TESTASIN",
		"language":      "en",
		"explicit":      true,
		"abridged":      false,
		"tags":          []string{"Sci-Fi", "Classic"},
		"genres":        []string{"Fiction"},
	}
	updateItemBody, _ := json.Marshal(updateItemPayload)
	reqUpdateItem, _ := http.NewRequest("PATCH", h.BaseURL+fmt.Sprintf("/api/items/%s", itemID), bytes.NewReader(updateItemBody))
	reqUpdateItem.Header.Set("Authorization", "Bearer "+adminToken)
	reqUpdateItem.Header.Set("Content-Type", "application/json")
	respUpdateItem, err := client.Do(reqUpdateItem)
	if err != nil {
		t.Fatalf("PATCH item failed: %v", err)
	}
	respUpdateItem.Body.Close()
	if respUpdateItem.StatusCode != http.StatusOK {
		t.Fatalf("Expected PATCH item status 200, got %d", respUpdateItem.StatusCode)
	}

	// Verify metadata.json was written/updated in the bookDir directory
	if _, err := os.Stat(metadataJSONPath); err != nil {
		t.Fatalf("Expected metadata.json to exist at %q, but got error: %v", metadataJSONPath, err)
	}

	// Read and verify metadata.json contents
	metaFileContent, err := os.ReadFile(metadataJSONPath)
	if err != nil {
		t.Fatalf("Failed to read metadata.json: %v", err)
	}
	var writtenMetadata map[string]interface{}
	if err := json.Unmarshal(metaFileContent, &writtenMetadata); err != nil {
		t.Fatalf("Failed to unmarshal written metadata: %v", err)
	}

	// Check matching fields
	if writtenMetadata["title"] != "Super Cool Title" {
		t.Errorf("Expected title 'Super Cool Title', got %v", writtenMetadata["title"])
	}
	if writtenMetadata["subtitle"] != "Super Cool Subtitle" {
		t.Errorf("Expected subtitle 'Super Cool Subtitle', got %v", writtenMetadata["subtitle"])
	}
	if writtenMetadata["publisher"] != "Fantastic Publisher" {
		t.Errorf("Expected publisher 'Fantastic Publisher', got %v", writtenMetadata["publisher"])
	}
	if writtenMetadata["publishedYear"] != "2026" {
		t.Errorf("Expected publishedYear '2026', got %v", writtenMetadata["publishedYear"])
	}
	if writtenMetadata["description"] != "The best description ever." {
		t.Errorf("Expected description 'The best description ever.', got %v", writtenMetadata["description"])
	}

	// Test case B: metadataMarkdownWithItem is DISABLED
	patchSettingsPayload["metadataMarkdownWithItem"] = false
	patchSettingsBody, _ = json.Marshal(patchSettingsPayload)
	reqPatchSettings, _ = http.NewRequest("PATCH", h.BaseURL+"/api/settings", bytes.NewReader(patchSettingsBody))
	reqPatchSettings.Header.Set("Authorization", "Bearer "+adminToken)
	reqPatchSettings.Header.Set("Content-Type", "application/json")
	respPatchSettings, err = client.Do(reqPatchSettings)
	if err != nil {
		t.Fatalf("PATCH settings failed: %v", err)
	}
	respPatchSettings.Body.Close()

	// Delete existing metadata.json
	if err := os.Remove(metadataJSONPath); err != nil {
		t.Fatalf("Failed to remove metadata.json: %v", err)
	}

	// Update item metadata again
	updateItemPayload["title"] = "Even Cooler Title"
	updateItemBody, _ = json.Marshal(updateItemPayload)
	reqUpdateItem, _ = http.NewRequest("PATCH", h.BaseURL+fmt.Sprintf("/api/items/%s", itemID), bytes.NewReader(updateItemBody))
	reqUpdateItem.Header.Set("Authorization", "Bearer "+adminToken)
	reqUpdateItem.Header.Set("Content-Type", "application/json")
	respUpdateItem, err = client.Do(reqUpdateItem)
	if err != nil {
		t.Fatalf("PATCH item failed: %v", err)
	}
	respUpdateItem.Body.Close()

	// Verify metadata.json was NOT re-created
	if _, err := os.Stat(metadataJSONPath); !os.IsNotExist(err) {
		t.Errorf("Expected metadata.json NOT to exist, but stat returned error: %v", err)
	}
}

// TestSettingsPersistence verifies that changing homePageBookshelfView, libraryBookshelfView,
// dateFormat, timeFormat, language, watchLibraryChanges, scannerParseSubtitles,
// scannerFindCovers, scannerCoverProvider, scannerPreferMatchedMetadata, chromecastEnabled
// via PATCH /api/settings correctly updates settings table (key 'server-settings') and retrieves them via GET /api/settings.
func TestSettingsPersistence(t *testing.T) {
	h := NewTestHarness()
	if err := h.Start(); err != nil {
		t.Fatalf("Failed to start harness: %v", err)
	}
	defer h.Stop()

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("Failed to create cookie jar: %v", err)
	}
	client := &http.Client{Jar: jar}

	adminToken := setupAdmin(t, h, client)

	// 1. Prepare PATCH settings payload
	targetSettings := map[string]interface{}{
		"homePageBookshelfView":        true,
		"libraryBookshelfView":         false,
		"dateFormat":                   "YYYY-MM-DD",
		"timeFormat":                   "HH:mm:ss",
		"language":                     "fr-fr",
		"watchLibraryChanges":          true,
		"scannerParseSubtitles":        true,
		"scannerFindCovers":            false,
		"scannerCoverProvider":         "audnexus",
		"scannerPreferMatchedMetadata": true,
		"chromecastEnabled":            true,
	}

	patchBody, _ := json.Marshal(targetSettings)
	reqPatch, _ := http.NewRequest("PATCH", h.BaseURL+"/api/settings", bytes.NewReader(patchBody))
	reqPatch.Header.Set("Authorization", "Bearer "+adminToken)
	reqPatch.Header.Set("Content-Type", "application/json")
	respPatch, err := client.Do(reqPatch)
	if err != nil {
		t.Fatalf("PATCH /api/settings failed: %v", err)
	}
	respPatch.Body.Close()
	if respPatch.StatusCode != http.StatusOK {
		t.Fatalf("Expected PATCH settings status 200, got %d", respPatch.StatusCode)
	}

	// 2. Perform GET /api/settings and verify values
	reqGet, _ := http.NewRequest("GET", h.BaseURL+"/api/settings", nil)
	reqGet.Header.Set("Authorization", "Bearer "+adminToken)
	respGet, err := client.Do(reqGet)
	if err != nil {
		t.Fatalf("GET /api/settings failed: %v", err)
	}
	defer respGet.Body.Close()
	if respGet.StatusCode != http.StatusOK {
		t.Fatalf("Expected GET settings status 200, got %d", respGet.StatusCode)
	}

	var getResp map[string]interface{}
	if err := json.NewDecoder(respGet.Body).Decode(&getResp); err != nil {
		t.Fatalf("Failed to decode GET settings response: %v", err)
	}

	for key, expectedVal := range targetSettings {
		actualVal, ok := getResp[key]
		if !ok {
			t.Errorf("GET /api/settings response missing key %q", key)
			continue
		}
		if actualVal != expectedVal {
			t.Errorf("GET /api/settings mismatch for key %q: expected %v, got %v", key, expectedVal, actualVal)
		}
	}

	// 3. Verify directly in the SQLite database to check true persistence (and verify DO NOT CHEAT)
	db, err := sql.Open("sqlite", h.DBPath)
	if err != nil {
		t.Fatalf("Failed to open SQLite database: %v", err)
	}
	defer db.Close()

	var valStr string
	err = db.QueryRow("SELECT value FROM settings WHERE key = 'server-settings'").Scan(&valStr)
	if err != nil {
		t.Fatalf("Failed to query settings table from DB: %v", err)
	}

	var dbSettings map[string]interface{}
	if err := json.Unmarshal([]byte(valStr), &dbSettings); err != nil {
		t.Fatalf("Failed to unmarshal DB settings JSON: %v", err)
	}

	for key, expectedVal := range targetSettings {
		actualVal, ok := dbSettings[key]
		if !ok {
			t.Errorf("DB settings JSON missing key %q", key)
			continue
		}
		if actualVal != expectedVal {
			t.Errorf("DB settings JSON mismatch for key %q: expected %v, got %v", key, expectedVal, actualVal)
		}
	}
}
