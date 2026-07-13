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
)

func TestF23Waveform(t *testing.T) {
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
	libraryPath := filepath.Join(h.ConfigDir, "libraryF23")
	createPayload := map[string]interface{}{
		"name":      "Library F23",
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

	// Set up a mock audio file inside library
	bookDir := filepath.Join(libraryPath, "WaveformBook")
	err = os.MkdirAll(bookDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create book directory: %v", err)
	}

	err = GenerateMockAudio(filepath.Join(bookDir, "01_track.mp3"), "Waveform Track", "Author F23", "Waveform Book", "1", "2026")
	if err != nil {
		t.Fatalf("Failed to generate audio track: %v", err)
	}

	// Trigger Scan
	reqScan, _ := http.NewRequest("POST", h.BaseURL+"/api/libraries/"+libraryID+"/scan", nil)
	reqScan.Header.Set("Authorization", "Bearer "+adminToken)
	respScan, err := client.Do(reqScan)
	if err != nil {
		t.Fatalf("Scan request failed: %v", err)
	}
	respScan.Body.Close()

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
	itemID := itemObj["id"].(string)

	// 1. GET /api/items/:id/waveform - Verify initial generation
	t.Run("GET /api/items/:id/waveform - Initial generation", func(t *testing.T) {
		reqWf, _ := http.NewRequest("GET", h.BaseURL+"/api/items/"+itemID+"/waveform", nil)
		reqWf.Header.Set("Authorization", "Bearer "+adminToken)
		respWf, err := client.Do(reqWf)
		if err != nil {
			t.Fatalf("Waveform request failed: %v", err)
		}
		defer respWf.Body.Close()

		if respWf.StatusCode != http.StatusOK {
			t.Fatalf("Expected status 200, got %d", respWf.StatusCode)
		}

		var wfResp map[string]interface{}
		if err := json.NewDecoder(respWf.Body).Decode(&wfResp); err != nil {
			t.Fatalf("Failed to decode waveform response: %v", err)
		}

		if wfResp["itemId"].(string) != itemID {
			t.Errorf("Expected itemId %q, got %q", itemID, wfResp["itemId"])
		}

		peaks, ok := wfResp["peaks"].([]interface{})
		if !ok {
			t.Fatalf("peaks is not a slice")
		}

		if len(peaks) != 200 {
			t.Errorf("Expected 200 peaks elements, got %d", len(peaks))
		}
	})

	// 2. GET /api/items/:id/waveform - Verify cached retrieval
	t.Run("GET /api/items/:id/waveform - Cached retrieval", func(t *testing.T) {
		reqWf, _ := http.NewRequest("GET", h.BaseURL+"/api/items/"+itemID+"/waveform", nil)
		reqWf.Header.Set("Authorization", "Bearer "+adminToken)
		respWf, err := client.Do(reqWf)
		if err != nil {
			t.Fatalf("Waveform cached request failed: %v", err)
		}
		defer respWf.Body.Close()

		if respWf.StatusCode != http.StatusOK {
			t.Fatalf("Expected status 200 for cache retrieval, got %d", respWf.StatusCode)
		}

		var wfResp map[string]interface{}
		if err := json.NewDecoder(respWf.Body).Decode(&wfResp); err != nil {
			t.Fatalf("Failed to decode cached waveform response: %v", err)
		}

		if wfResp["itemId"].(string) != itemID {
			t.Errorf("Expected itemId %q, got %q", itemID, wfResp["itemId"])
		}

		peaks, ok := wfResp["peaks"].([]interface{})
		if !ok {
			t.Fatalf("peaks is not a slice")
		}

		if len(peaks) != 200 {
			t.Errorf("Expected 200 peaks elements, got %d", len(peaks))
		}
	})
}
