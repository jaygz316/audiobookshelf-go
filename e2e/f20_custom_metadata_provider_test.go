package e2e

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"testing"
)

func TestCustomMetadataProvidersE2E(t *testing.T) {
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

	// 1. Get initial custom providers
	req, _ := http.NewRequest("GET", h.BaseURL+"/api/custom-metadata-providers", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Failed to send GET /api/custom-metadata-providers: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", resp.StatusCode)
	}
	var listResp map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}
	resp.Body.Close()

	provs, ok := listResp["providers"].([]interface{})
	if !ok {
		t.Fatalf("Expected 'providers' array in response, got %v", listResp)
	}
	if len(provs) != 0 {
		t.Errorf("Expected 0 custom providers, got %d", len(provs))
	}

	// 2. Create a custom provider
	payload := map[string]interface{}{
		"name":            "E2E Test Provider",
		"url":             "http://127.0.0.1:9999/search",
		"mediaType":       "book",
		"authHeaderValue": "Bearer e2e-token",
	}
	bodyBytes, _ := json.Marshal(payload)
	req, _ = http.NewRequest("POST", h.BaseURL+"/api/custom-metadata-providers", bytes.NewReader(bodyBytes))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("Failed to send POST /api/custom-metadata-providers: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", resp.StatusCode)
	}
	var createResp map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&createResp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}
	resp.Body.Close()

	provMap, ok := createResp["provider"].(map[string]interface{})
	if !ok {
		t.Fatalf("Expected 'provider' object in response, got %v", createResp)
	}
	createdID, ok := provMap["id"].(string)
	if !ok || createdID == "" {
		t.Fatalf("Expected provider 'id' to be set in response, got %v", provMap)
	}

	// 3. Get custom providers again and verify
	req, _ = http.NewRequest("GET", h.BaseURL+"/api/custom-metadata-providers", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("Failed to send GET /api/custom-metadata-providers: %v", err)
	}
	var listResp2 map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&listResp2)
	resp.Body.Close()

	provs2, _ := listResp2["providers"].([]interface{})
	if len(provs2) != 1 {
		t.Fatalf("Expected 1 custom provider, got %d", len(provs2))
	}
	p0 := provs2[0].(map[string]interface{})
	if p0["name"] != "E2E Test Provider" || p0["id"] != createdID {
		t.Errorf("Mismatch in retrieved custom provider details: %v", p0)
	}

	// 4. Verify in active search providers
	req, _ = http.NewRequest("GET", h.BaseURL+"/api/search/providers", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("Failed to send GET /api/search/providers: %v", err)
	}
	var activeResp map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&activeResp)
	resp.Body.Close()

	providersMap, _ := activeResp["providers"].(map[string]interface{})
	booksArr, _ := providersMap["books"].([]interface{})
	found := false
	expectedVal := "custom-" + createdID
	for _, b := range booksArr {
		bMap := b.(map[string]interface{})
		if bMap["value"] == expectedVal {
			found = true
			if bMap["text"] != "E2E Test Provider" {
				t.Errorf("Expected active provider text 'E2E Test Provider', got %q", bMap["text"])
			}
		}
	}
	if !found {
		t.Errorf("Expected custom provider %s to be listed in active search providers", expectedVal)
	}

	// 5. Delete custom provider
	req, _ = http.NewRequest("DELETE", h.BaseURL+"/api/custom-metadata-providers/"+createdID, nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("Failed to send DELETE /api/custom-metadata-providers: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// 6. Get custom providers again and verify it is empty
	req, _ = http.NewRequest("GET", h.BaseURL+"/api/custom-metadata-providers", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("Failed to send GET: %v", err)
	}
	var listResp3 map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&listResp3)
	resp.Body.Close()

	provs3, _ := listResp3["providers"].([]interface{})
	if len(provs3) != 0 {
		t.Errorf("Expected 0 custom providers after deletion, got %d", len(provs3))
	}
}
