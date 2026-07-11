package e2e

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestF22Narrators(t *testing.T) {
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
	libraryPath := filepath.Join(h.ConfigDir, "libraryF22")
	createPayload := map[string]interface{}{
		"name":      "Library F22",
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

	// 2. Create mock audiobooks to scan
	bookDirs := []string{"F22Book1", "F22Book2", "F22Book3"}
	for _, bDir := range bookDirs {
		bookPath := filepath.Join(libraryPath, bDir)
		err = os.MkdirAll(bookPath, 0755)
		if err != nil {
			t.Fatalf("Failed to create book directory: %v", err)
		}
		err = GenerateMockAudio(filepath.Join(bookPath, "track.mp3"), bDir+" Title", "Author F22", "Series F22", "1", "2026")
		if err != nil {
			t.Fatalf("Failed to generate audio track: %v", err)
		}
	}

	// Trigger Scan
	reqScan, _ := http.NewRequest("POST", h.BaseURL+"/api/libraries/"+libraryID+"/scan", nil)
	reqScan.Header.Set("Authorization", "Bearer "+adminToken)
	respScan, err := client.Do(reqScan)
	if err != nil {
		t.Fatalf("Scan request failed: %v", err)
	}
	respScan.Body.Close()

	// Poll list items until scanned
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
				if len(items) >= 3 {
					break
				}
			}
		}
	}

	if len(items) != 3 {
		t.Fatalf("Expected exactly 3 scanned items, got %d", len(items))
	}

	// 3. Patch items with Narrators
	// Find item IDs
	var itemIDs []string
	for _, item := range items {
		itemIDs = append(itemIDs, item.(map[string]interface{})["id"].(string))
	}

	narratorsMap := []struct {
		itemID    string
		title     string
		narrators []string
	}{
		{itemIDs[0], "Book 1", []string{"Frank Muller", "Stephen King"}},
		{itemIDs[1], "Book 2", []string{"Frank Muller"}},
		{itemIDs[2], "Book 3", []string{"George Guidall"}},
	}

	for _, n := range narratorsMap {
		payload := map[string]interface{}{
			"title":     n.title,
			"narrators": n.narrators,
		}
		pBody, _ := json.Marshal(payload)
		reqPatch, _ := http.NewRequest("PATCH", h.BaseURL+"/api/items/"+n.itemID, bytes.NewReader(pBody))
		reqPatch.Header.Set("Authorization", "Bearer "+adminToken)
		reqPatch.Header.Set("Content-Type", "application/json")
		respPatch, err := client.Do(reqPatch)
		if err != nil {
			t.Fatalf("Failed to patch item %s: %v", n.itemID, err)
		}
		respPatch.Body.Close()
		if respPatch.StatusCode != http.StatusOK {
			t.Fatalf("Patch item status: %d", respPatch.StatusCode)
		}
	}

	// 4. Test GET /api/libraries/:id/narrators
	t.Run("GET Narrators - Sorting & Search", func(t *testing.T) {
		reqNarrators, _ := http.NewRequest("GET", h.BaseURL+"/api/libraries/"+libraryID+"/narrators?sort=name", nil)
		reqNarrators.Header.Set("Authorization", "Bearer "+adminToken)
		respNarrators, err := client.Do(reqNarrators)
		if err != nil {
			t.Fatalf("Failed to retrieve narrators: %v", err)
		}
		defer respNarrators.Body.Close()

		if respNarrators.StatusCode != http.StatusOK {
			t.Fatalf("GET narrators status: %d", respNarrators.StatusCode)
		}

		var payload map[string]interface{}
		if err := json.NewDecoder(respNarrators.Body).Decode(&payload); err != nil {
			t.Fatalf("Failed to decode JSON: %v", err)
		}

		results := payload["results"].([]interface{})
		if len(results) != 3 {
			t.Fatalf("Expected 3 narrators, got %d", len(results))
		}

		// Check name sorting (alphabetical): Frank Muller, George Guidall, Stephen King
		if results[0].(map[string]interface{})["name"] != "Frank Muller" {
			t.Errorf("Expected Frank Muller first, got %v", results[0])
		}
		if results[1].(map[string]interface{})["name"] != "George Guidall" {
			t.Errorf("Expected George Guidall second, got %v", results[1])
		}

		// Count sorting test
		reqCount, _ := http.NewRequest("GET", h.BaseURL+"/api/libraries/"+libraryID+"/narrators?sort=numBooks&desc=true", nil)
		reqCount.Header.Set("Authorization", "Bearer "+adminToken)
		respCount, err := client.Do(reqCount)
		if err != nil {
			t.Fatalf("Failed to retrieve counts: %v", err)
		}
		defer respCount.Body.Close()

		var countPayload map[string]interface{}
		json.NewDecoder(respCount.Body).Decode(&countPayload)
		countResults := countPayload["results"].([]interface{})
		if countResults[0].(map[string]interface{})["name"] != "Frank Muller" || countResults[0].(map[string]interface{})["numBooks"].(float64) != 2 {
			t.Errorf("Expected Frank Muller to be first with 2 books, got %v", countResults[0])
		}

		// Search test
		reqSearch, _ := http.NewRequest("GET", h.BaseURL+"/api/libraries/"+libraryID+"/narrators?search=george", nil)
		reqSearch.Header.Set("Authorization", "Bearer "+adminToken)
		respSearch, err := client.Do(reqSearch)
		if err != nil {
			t.Fatalf("Failed search: %v", err)
		}
		defer respSearch.Body.Close()

		var searchPayload map[string]interface{}
		json.NewDecoder(respSearch.Body).Decode(&searchPayload)
		searchResults := searchPayload["results"].([]interface{})
		if len(searchResults) != 1 || searchResults[0].(map[string]interface{})["name"] != "George Guidall" {
			t.Errorf("Expected only George Guidall, got %v", searchResults)
		}
	})

	// 5. Test Filtered Items GET /api/libraries/:id/items?filter=narrators.Frank Muller
	t.Run("GET Items - Filter by Narrated By", func(t *testing.T) {
		reqFilter, _ := http.NewRequest("GET", h.BaseURL+"/api/libraries/"+libraryID+"/items?filter=narrators.Frank%20Muller", nil)
		reqFilter.Header.Set("Authorization", "Bearer "+adminToken)
		respFilter, err := client.Do(reqFilter)
		if err != nil {
			t.Fatalf("Failed to get filtered items: %v", err)
		}
		defer respFilter.Body.Close()

		if respFilter.StatusCode != http.StatusOK {
			t.Fatalf("GET filtered items status: %d", respFilter.StatusCode)
		}

		var payload map[string]interface{}
		json.NewDecoder(respFilter.Body).Decode(&payload)
		results := payload["results"].([]interface{})
		if len(results) != 2 {
			t.Fatalf("Expected 2 books for Frank Muller, got %d", len(results))
		}
	})
}
