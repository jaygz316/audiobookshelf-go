package e2e

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestF3LibraryItems(t *testing.T) {
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

	// 1. Setup Admin Root
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

	// Create Library
	libraryPath := filepath.Join(h.ConfigDir, "libraryF3")
	createPayload := map[string]interface{}{
		"name":      "Library F3",
		"mediaType": "book",
		"folders": []map[string]string{
			{"path": libraryPath},
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

	// 2. Test: Empty library scan (Tier 2)
	t.Run("POST /api/libraries/:id/scan - Empty library", func(t *testing.T) {
		req, _ := http.NewRequest("POST", h.BaseURL+"/api/libraries/"+libraryID+"/scan", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Scan request failed: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}
		resp.Body.Close()

		// Wait briefly and verify items count is 0
		time.Sleep(100 * time.Millisecond)

		reqList, _ := http.NewRequest("GET", h.BaseURL+"/api/libraries/"+libraryID+"/items", nil)
		reqList.Header.Set("Authorization", "Bearer "+adminToken)
		respList, err := client.Do(reqList)
		if err != nil {
			t.Fatalf("List items failed: %v", err)
		}
		defer respList.Body.Close()
		var listResp map[string]interface{}
		json.NewDecoder(respList.Body).Decode(&listResp)
		total := listResp["total"].(float64)
		if total != 0 {
			t.Errorf("Expected 0 items, got %f", total)
		}
	})

	// 3. Test: Serve default fallback cover (Tier 2)
	t.Run("GET /api/items/nonexistent/cover - Fallback NotFound", func(t *testing.T) {
		req, _ := http.NewRequest("GET", h.BaseURL+"/api/items/nonexistent/cover?raw=1", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Cover request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("Expected status 404 for invalid cover, got %d", resp.StatusCode)
		}
	})

	// 4. Test: Get invalid item by ID (Tier 2)
	t.Run("GET /api/items/nonexistent - NotFound", func(t *testing.T) {
		req, _ := http.NewRequest("GET", h.BaseURL+"/api/items/nonexistent", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Get item failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("Expected status 404, got %d", resp.StatusCode)
		}
	})

	// 5. Test: Multi-file tracks library scan (Tier 1 & Tier 2)
	var itemID string
	t.Run("POST /api/libraries/:id/scan - Multi-file tracks & GetItem & ServeCover", func(t *testing.T) {
		bookDir := filepath.Join(libraryPath, "MultiTrackBook")
		err := os.MkdirAll(bookDir, 0755)
		if err != nil {
			t.Fatalf("Failed to create book directory: %v", err)
		}

		// Generate 2 mock audio files
		err = GenerateMockAudio(filepath.Join(bookDir, "01_intro.mp3"), "Intro Track", "Author F3", "Multi-Track Book", "1", "2026")
		if err != nil {
			t.Fatalf("Failed to generate audio track 1: %v", err)
		}
		err = GenerateMockAudio(filepath.Join(bookDir, "02_story.mp3"), "Story Track", "Author F3", "Multi-Track Book", "2", "2026")
		if err != nil {
			t.Fatalf("Failed to generate audio track 2: %v", err)
		}

		// Generate cover
		err = GenerateMockCover(filepath.Join(bookDir, "cover.jpg"))
		if err != nil {
			t.Fatalf("Failed to generate cover: %v", err)
		}

		// Trigger Scan
		req, _ := http.NewRequest("POST", h.BaseURL+"/api/libraries/"+libraryID+"/scan", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Scan request failed: %v", err)
		}
		resp.Body.Close()

		// Poll list items until we see 1 item
		var items []interface{}
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
					items = listResp["results"].([]interface{})
					if len(items) > 0 {
						break
					}
				}
			}
		}

		if len(items) != 1 {
			t.Fatalf("Expected exactly 1 scanned item, got %d", len(items))
		}

		itemObj := items[0].(map[string]interface{})
		itemID = itemObj["id"].(string)

		// Get item details by ID
		reqGet, _ := http.NewRequest("GET", h.BaseURL+"/api/items/"+itemID, nil)
		reqGet.Header.Set("Authorization", "Bearer "+adminToken)
		respGet, err := client.Do(reqGet)
		if err != nil {
			t.Fatalf("GET item by ID failed: %v", err)
		}
		defer respGet.Body.Close()
		if respGet.StatusCode != http.StatusOK {
			t.Fatalf("GET item by ID status: %d", respGet.StatusCode)
		}

		var detailedItem map[string]interface{}
		json.NewDecoder(respGet.Body).Decode(&detailedItem)

		media := detailedItem["media"].(map[string]interface{})
		numAudioFiles := media["numAudioFiles"].(float64)
		if numAudioFiles != 2 {
			t.Errorf("Expected 2 audio files, got %f", numAudioFiles)
		}

		// Test cover serving
		reqCover, _ := http.NewRequest("GET", h.BaseURL+"/api/items/"+itemID+"/cover?raw=1", nil)
		reqCover.Header.Set("Authorization", "Bearer "+adminToken)
		respCover, err := client.Do(reqCover)
		if err != nil {
			t.Fatalf("Serve cover failed: %v", err)
		}
		defer respCover.Body.Close()
		if respCover.StatusCode != http.StatusOK {
			t.Errorf("Expected cover serving 200 OK, got %d", respCover.StatusCode)
		}

		// Verify we got the mock cover bytes (should be small JPEG)
		coverBytes, _ := io.ReadAll(respCover.Body)
		if len(coverBytes) == 0 {
			t.Errorf("Returned empty cover bytes")
		}
	})

	// 6. Test: Missing disk check (Tier 2)
	t.Run("POST /api/libraries/:id/scan - Missing Disk Check", func(t *testing.T) {
		bookDir := filepath.Join(libraryPath, "MultiTrackBook")

		// Remove the directory from disk
		err := os.RemoveAll(bookDir)
		if err != nil {
			t.Fatalf("Failed to remove directory: %v", err)
		}

		// Trigger Scan again
		req, _ := http.NewRequest("POST", h.BaseURL+"/api/libraries/"+libraryID+"/scan", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Scan request failed: %v", err)
		}
		resp.Body.Close()

		// Poll database or list items until the item has isMissing=true
		isMissing := false
		for attempt := 0; attempt < 20; attempt++ {
			time.Sleep(150 * time.Millisecond)
			reqGet, _ := http.NewRequest("GET", h.BaseURL+"/api/items/"+itemID, nil)
			reqGet.Header.Set("Authorization", "Bearer "+adminToken)
			respGet, err := client.Do(reqGet)
			if err == nil && respGet.StatusCode == http.StatusOK {
				var detailedItem map[string]interface{}
				json.NewDecoder(respGet.Body).Decode(&detailedItem)
				respGet.Body.Close()
				if detailedItem["isMissing"] != nil {
					isMissing = detailedItem["isMissing"].(bool)
					if isMissing {
						break
					}
				}
			} else if err == nil {
				respGet.Body.Close()
			}
		}

		if !isMissing {
			t.Errorf("Expected library item to be marked as missing, but it is not")
		}
	})
}
