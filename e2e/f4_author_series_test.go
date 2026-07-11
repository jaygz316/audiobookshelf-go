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

func TestF4AuthorsAndSeries(t *testing.T) {
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
	libraryPath := filepath.Join(h.ConfigDir, "libraryF4")
	createPayload := map[string]interface{}{
		"name":      "Library F4",
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

	// 2. Populate books with tags to scan
	bookDir := filepath.Join(libraryPath, "F4Book")
	err = os.MkdirAll(bookDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create book directory: %v", err)
	}
	err = GenerateMockAudio(filepath.Join(bookDir, "01_intro.mp3"), "Intro Track", "Author F4", "Series F4", "1", "2026")
	if err != nil {
		t.Fatalf("Failed to generate audio track: %v", err)
	}

	bookDir2 := filepath.Join(libraryPath, "F4Book2")
	err = os.MkdirAll(bookDir2, 0755)
	if err != nil {
		t.Fatalf("Failed to create book directory 2: %v", err)
	}
	err = GenerateMockAudio(filepath.Join(bookDir2, "track.mp3"), "Fellowship (Narrated by Rob Inglis)", "Author F4", "Series F4", "99", "1954")
	if err != nil {
		t.Fatalf("Failed to generate audio track 2: %v", err)
	}

	bookDir3 := filepath.Join(libraryPath, "F4Book3")
	err = os.MkdirAll(bookDir3, 0755)
	if err != nil {
		t.Fatalf("Failed to create book directory 3: %v", err)
	}
	err = GenerateMockAudio(filepath.Join(bookDir3, "track.mp3"), "Fellowship (Narrated by Andy Serkis)", "Author F4", "Series F4", "98", "1954")
	if err != nil {
		t.Fatalf("Failed to generate audio track 3: %v", err)
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

	// 3. Test: List and retrieve Authors (Tier 1 & Tier 2)
	var authorID string
	t.Run("GET /api/libraries/:id/authors - Verify scanned author", func(t *testing.T) {
		reqList, _ := http.NewRequest("GET", h.BaseURL+"/api/libraries/"+libraryID+"/authors", nil)
		reqList.Header.Set("Authorization", "Bearer "+adminToken)
		respList, err := client.Do(reqList)
		if err != nil {
			t.Fatalf("Failed to list library authors: %v", err)
		}
		defer respList.Body.Close()

		if respList.StatusCode != http.StatusOK {
			t.Fatalf("List authors status: %d", respList.StatusCode)
		}

		var authorsResp map[string]interface{}
		json.NewDecoder(respList.Body).Decode(&authorsResp)

		authorsList, _ := authorsResp["results"].([]interface{})

		found := false
		for _, item := range authorsList {
			author := item.(map[string]interface{})
			if author["name"] == "Author F4" {
				found = true
				authorID = author["id"].(string)
				break
			}
		}
		if !found {
			t.Errorf("Scanned Author F4 not found in authors list")
		}
	})

	t.Run("GET /api/authors/:id - Verify author details", func(t *testing.T) {
		if authorID == "" {
			t.Skip("No author ID found from scan")
		}

		reqGet, _ := http.NewRequest("GET", h.BaseURL+"/api/authors/"+authorID, nil)
		reqGet.Header.Set("Authorization", "Bearer "+adminToken)
		respGet, err := client.Do(reqGet)
		if err != nil {
			t.Fatalf("GET author by ID failed: %v", err)
		}
		defer respGet.Body.Close()

		if respGet.StatusCode != http.StatusOK {
			t.Fatalf("GET author by ID status: %d", respGet.StatusCode)
		}

		var author map[string]interface{}
		json.NewDecoder(respGet.Body).Decode(&author)

		if author["name"] != "Author F4" {
			t.Errorf("Expected author name 'Author F4', got %v", author["name"])
		}
	})

	t.Run("GET /api/authors/:id/image - Image fallback 404", func(t *testing.T) {
		if authorID == "" {
			t.Skip("No author ID found from scan")
		}

		reqImg, _ := http.NewRequest("GET", h.BaseURL+"/api/authors/"+authorID+"/image", nil)
		reqImg.Header.Set("Authorization", "Bearer "+adminToken)
		respImg, err := client.Do(reqImg)
		if err != nil {
			t.Fatalf("GET author image failed: %v", err)
		}
		defer respImg.Body.Close()

		if respImg.StatusCode != http.StatusNotFound {
			t.Errorf("Expected 404 for missing author image, got %d", respImg.StatusCode)
		}
	})

	t.Run("POST /api/authors/:id/match - Verify stub", func(t *testing.T) {
		if authorID == "" {
			t.Skip("No author ID found from scan")
		}

		reqMatch, _ := http.NewRequest("POST", h.BaseURL+"/api/authors/"+authorID+"/match", nil)
		reqMatch.Header.Set("Authorization", "Bearer "+adminToken)
		respMatch, err := client.Do(reqMatch)
		if err != nil {
			t.Fatalf("POST author match failed: %v", err)
		}
		defer respMatch.Body.Close()

		if respMatch.StatusCode != http.StatusOK {
			t.Fatalf("POST author match status: %d", respMatch.StatusCode)
		}

		var matchResp map[string]interface{}
		json.NewDecoder(respMatch.Body).Decode(&matchResp)
		if matchResp["updated"] != false {
			t.Errorf("Expected updated=false from stub, got %v", matchResp["updated"])
		}
	})

	t.Run("GET /api/authors/nonexistent - NotFound", func(t *testing.T) {
		reqGet, _ := http.NewRequest("GET", h.BaseURL+"/api/authors/nonexistent-author-id", nil)
		reqGet.Header.Set("Authorization", "Bearer "+adminToken)
		respGet, err := client.Do(reqGet)
		if err != nil {
			t.Fatalf("GET author failed: %v", err)
		}
		defer respGet.Body.Close()

		if respGet.StatusCode != http.StatusNotFound {
			t.Errorf("Expected status 404 for nonexistent author, got %d", respGet.StatusCode)
		}
	})

	// 4. Test: List and retrieve Series (Tier 1 & Tier 2)
	var seriesID string
	t.Run("GET /api/libraries/:id/series - Verify scanned series", func(t *testing.T) {
		reqList, _ := http.NewRequest("GET", h.BaseURL+"/api/libraries/"+libraryID+"/series", nil)
		reqList.Header.Set("Authorization", "Bearer "+adminToken)
		respList, err := client.Do(reqList)
		if err != nil {
			t.Fatalf("Failed to list library series: %v", err)
		}
		defer respList.Body.Close()

		if respList.StatusCode != http.StatusOK {
			t.Fatalf("List series status: %d", respList.StatusCode)
		}

		var seriesResp map[string]interface{}
		json.NewDecoder(respList.Body).Decode(&seriesResp)

		results := seriesResp["results"].([]interface{})
		found := false
		for _, item := range results {
			series := item.(map[string]interface{})
			if series["name"] == "Series F4" {
				found = true
				seriesID = series["id"].(string)
				break
			}
		}
		if !found {
			t.Errorf("Scanned Series F4 not found in series list")
		}
	})

	t.Run("GET /api/libraries/:id/series/:seriesId - Verify series details", func(t *testing.T) {
		if seriesID == "" {
			t.Skip("No series ID found from scan")
		}

		reqGet, _ := http.NewRequest("GET", h.BaseURL+"/api/libraries/"+libraryID+"/series/"+seriesID, nil)
		reqGet.Header.Set("Authorization", "Bearer "+adminToken)
		respGet, err := client.Do(reqGet)
		if err != nil {
			t.Fatalf("GET series failed: %v", err)
		}
		defer respGet.Body.Close()

		if respGet.StatusCode != http.StatusOK {
			t.Fatalf("GET series status: %d", respGet.StatusCode)
		}

		var series map[string]interface{}
		json.NewDecoder(respGet.Body).Decode(&series)

		if series["name"] != "Series F4" {
			t.Errorf("Expected series name 'Series F4', got %v", series["name"])
		}
	})

	t.Run("GET /api/libraries/:id/series/nonexistent - NotFound", func(t *testing.T) {
		reqGet, _ := http.NewRequest("GET", h.BaseURL+"/api/libraries/"+libraryID+"/series/nonexistent-series-id", nil)
		reqGet.Header.Set("Authorization", "Bearer "+adminToken)
		respGet, err := client.Do(reqGet)
		if err != nil {
			t.Fatalf("GET series failed: %v", err)
		}
		defer respGet.Body.Close()

		if respGet.StatusCode != http.StatusNotFound {
			t.Errorf("Expected status 404 for nonexistent series, got %d", respGet.StatusCode)
		}
	})

	t.Run("POST /api/series/:seriesId/auto-number - Verify grouping and ordering", func(t *testing.T) {
		if seriesID == "" {
			t.Skip("No series ID found from scan")
		}

		reqAutoNum, _ := http.NewRequest("POST", h.BaseURL+"/api/series/"+seriesID+"/auto-number", nil)
		reqAutoNum.Header.Set("Authorization", "Bearer "+adminToken)
		respAutoNum, err := client.Do(reqAutoNum)
		if err != nil {
			t.Fatalf("POST auto-number failed: %v", err)
		}
		defer respAutoNum.Body.Close()

		if respAutoNum.StatusCode != http.StatusOK {
			t.Fatalf("Expected auto-number response status 200, got %d", respAutoNum.StatusCode)
		}

		// Now query the books in the series to verify their sequence values
		reqItems, _ := http.NewRequest("GET", h.BaseURL+"/api/libraries/"+libraryID+"/items?filter=series."+seriesID, nil)
		reqItems.Header.Set("Authorization", "Bearer "+adminToken)
		respItems, err := client.Do(reqItems)
		if err != nil {
			t.Fatalf("Failed to fetch library items for series: %v", err)
		}
		defer respItems.Body.Close()

		var payload map[string]interface{}
		json.NewDecoder(respItems.Body).Decode(&payload)
		results := payload["results"].([]interface{})

		// Expected sequences:
		// - "Fellowship (Narrated by Rob Inglis)" and "Fellowship (Narrated by Andy Serkis)" should both be sequence "1"
		// - "Intro Track" should be sequence "2"
		for _, rawItem := range results {
			item := rawItem.(map[string]interface{})
			media := item["media"].(map[string]interface{})
			metadata := media["metadata"].(map[string]interface{})
			title := metadata["title"].(string)
			
			// Find series sequence in the series list of this item
			var sequence string
			seriesList := metadata["series"].([]interface{})
			for _, rawSeries := range seriesList {
				seriesMap := rawSeries.(map[string]interface{})
				if seriesMap["id"] == seriesID {
					if seq, ok := seriesMap["sequence"].(string); ok {
						sequence = seq
					}
				}
			}

			if title == "Fellowship (Narrated by Rob Inglis)" || title == "Fellowship (Narrated by Andy Serkis)" {
				if sequence != "1" {
					t.Errorf("Expected sequence '1' for title %q, got %q", title, sequence)
				}
			} else if title == "Intro Track" {
				if sequence != "2" {
					t.Errorf("Expected sequence '2' for title %q, got %q", title, sequence)
				}
			}
		}
	})
}

