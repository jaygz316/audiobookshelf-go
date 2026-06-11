package e2e

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite"
)

func TestF9PodcastFeedAndEpisodes(t *testing.T) {
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

	// login admin
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

	// Get admin user ID from DB
	db, err := sql.Open("sqlite", h.DBPath)
	if err != nil {
		t.Fatalf("Failed to open DB: %v", err)
	}
	defer db.Close()

	var adminUserID string
	err = db.QueryRow("SELECT id FROM users WHERE username = 'rootadmin'").Scan(&adminUserID)
	if err != nil {
		t.Fatalf("Failed to scan admin user ID: %v", err)
	}

	// Create Library of type "podcast"
	libraryPath := filepath.Join(h.ConfigDir, "libraryF9")
	createPayload := map[string]interface{}{
		"name":      "Podcast Library",
		"mediaType": "podcast",
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

	// Write mock episode file on disk inside the podcast directory
	podcastDir := filepath.Join(libraryPath, "MyPodcast")
	err = os.MkdirAll(podcastDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create podcast directory: %v", err)
	}

	mockEpisodePath := filepath.Join(podcastDir, "episode_one.mp3")
	err = GenerateMockAudio(mockEpisodePath, "Episode One Title", "Podcast Artist", "Podcast Album", "1", "2026")
	if err != nil {
		t.Fatalf("Failed to generate mock episode audio: %v", err)
	}

	// Trigger library scan (POST /api/libraries/:id/scan)
	reqScan, _ := http.NewRequest("POST", h.BaseURL+"/api/libraries/"+libraryID+"/scan", nil)
	reqScan.Header.Set("Authorization", "Bearer "+adminToken)
	respScan, err := client.Do(reqScan)
	if err != nil {
		t.Fatalf("Scan request failed: %v", err)
	}
	respScan.Body.Close()

	// Wait for item to be scanned
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
		t.Fatalf("Expected exactly 1 scanned podcast item, got %d", len(items))
	}

	podcastItem := items[0].(map[string]interface{})
	libraryItemID := podcastItem["id"].(string)

	// Query podcastEpisodes table to find the generated episode ID
	var episodeID string
	err = db.QueryRow("SELECT id FROM podcastEpisodes").Scan(&episodeID)
	if err != nil {
		t.Fatalf("Failed to scan episode ID from DB: %v", err)
	}

	// Create a restricted user for access control check
	hashedPash, err := bcrypt.GenerateFromPassword([]byte("restricted_password123"), 8)
	if err != nil {
		t.Fatalf("Failed to hash password: %v", err)
	}
	restrictedUserID := uuid.New().String()
	permsJSON := fmt.Sprintf(`{"download":true,"accessExplicitContent":false,"accessAllLibraries":false,"librariesAccessible":["%s"],"accessAllTags":true,"itemTagsSelected":[],"selectedTagsNotAccessible":false}`, libraryID)
	_, err = db.Exec(`INSERT INTO users (id, username, email, type, pash, token, isActive, permissions, extraData, bookmarks, createdAt, updatedAt)
		VALUES (?, 'restricted_user', NULL, 'user', ?, 'token-restricted', 1, ?, '{}', '[]', datetime('now'), datetime('now'))`,
		restrictedUserID, string(hashedPash), permsJSON)
	if err != nil {
		t.Fatalf("Failed to insert restricted user: %v", err)
	}

	// Login restricted user
	rLoginPayload := map[string]string{
		"username": "restricted_user",
		"password": "restricted_password123",
	}
	rLoginBody, _ := json.Marshal(rLoginPayload)
	respR, err := client.Post(h.BaseURL+"/login", "application/json", bytes.NewReader(rLoginBody))
	if err != nil {
		t.Fatalf("Failed to login restricted user: %v", err)
	}
	var rResp map[string]interface{}
	json.NewDecoder(respR.Body).Decode(&rResp)
	respR.Body.Close()
	restrictedToken := rResp["user"].(map[string]interface{})["accessToken"].(string)

	// ==========================================
	// TIER 1 TESTS
	// ==========================================

	// 1. GET /api/libraries/:id/items - Verify discoverability
	t.Run("GET /api/libraries/:id/items - Podcast list", func(t *testing.T) {
		reqList, _ := http.NewRequest("GET", h.BaseURL+"/api/libraries/"+libraryID+"/items", nil)
		reqList.Header.Set("Authorization", "Bearer "+adminToken)
		respList, err := client.Do(reqList)
		if err != nil {
			t.Fatalf("List library items failed: %v", err)
		}
		defer respList.Body.Close()

		var listResp map[string]interface{}
		json.NewDecoder(respList.Body).Decode(&listResp)
		results := listResp["results"].([]interface{})
		if len(results) != 1 {
			t.Errorf("Expected 1 item, got %d", len(results))
		}
		item := results[0].(map[string]interface{})
		if item["mediaType"].(string) != "podcast" {
			t.Errorf("Expected mediaType 'podcast', got %q", item["mediaType"])
		}
	})

	// 2. GET /api/items/:id - Verify detail retrieval
	t.Run("GET /api/items/:id - Podcast detail", func(t *testing.T) {
		reqDetail, _ := http.NewRequest("GET", h.BaseURL+"/api/items/"+libraryItemID, nil)
		reqDetail.Header.Set("Authorization", "Bearer "+adminToken)
		respDetail, err := client.Do(reqDetail)
		if err != nil {
			t.Fatalf("Get item detail failed: %v", err)
		}
		defer respDetail.Body.Close()

		if respDetail.StatusCode != http.StatusOK {
			t.Fatalf("Expected 200, got %d", respDetail.StatusCode)
		}

		var detail map[string]interface{}
		json.NewDecoder(respDetail.Body).Decode(&detail)

		if detail["mediaType"].(string) != "podcast" {
			t.Errorf("Expected mediaType 'podcast', got %q", detail["mediaType"])
		}
	})

	// 3. Sync Progress (create progress via POST /api/me/progress/:id/:episodeId)
	t.Run("POST /api/me/progress/:id/:episodeId - Sync progress", func(t *testing.T) {
		progressPayload := map[string]interface{}{
			"duration":                  120.0,
			"currentTime":               20.0,
			"isFinished":                false,
			"hideFromContinueListening": false,
		}
		body, _ := json.Marshal(progressPayload)

		reqProgress, _ := http.NewRequest("POST", h.BaseURL+"/api/me/progress/"+libraryItemID+"/"+episodeID, bytes.NewReader(body))
		reqProgress.Header.Set("Authorization", "Bearer "+adminToken)
		reqProgress.Header.Set("Content-Type", "application/json")

		respProgress, err := client.Do(reqProgress)
		if err != nil {
			t.Fatalf("Sync progress failed: %v", err)
		}
		defer respProgress.Body.Close()

		if respProgress.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", respProgress.StatusCode)
		}
	})

	// 4. Update progress (PATCH /api/me/progress/:id/:episodeId)
	t.Run("PATCH /api/me/progress/:id/:episodeId - Update progress", func(t *testing.T) {
		progressPayload := map[string]interface{}{
			"currentTime": 45.0,
		}
		body, _ := json.Marshal(progressPayload)

		reqProgress, _ := http.NewRequest("PATCH", h.BaseURL+"/api/me/progress/"+libraryItemID+"/"+episodeID, bytes.NewReader(body))
		reqProgress.Header.Set("Authorization", "Bearer "+adminToken)
		reqProgress.Header.Set("Content-Type", "application/json")

		respProgress, err := client.Do(reqProgress)
		if err != nil {
			t.Fatalf("Update progress failed: %v", err)
		}
		defer respProgress.Body.Close()

		if respProgress.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", respProgress.StatusCode)
		}
	})

	// 5. Get progress (GET /api/me/progress/:id/:episodeId)
	t.Run("GET /api/me/progress/:id/:episodeId - Fetch progress", func(t *testing.T) {
		reqProgress, _ := http.NewRequest("GET", h.BaseURL+"/api/me/progress/"+libraryItemID+"/"+episodeID, nil)
		reqProgress.Header.Set("Authorization", "Bearer "+adminToken)

		respProgress, err := client.Do(reqProgress)
		if err != nil {
			t.Fatalf("Get progress failed: %v", err)
		}
		defer respProgress.Body.Close()

		if respProgress.StatusCode != http.StatusOK {
			t.Fatalf("Expected status 200, got %d", respProgress.StatusCode)
		}

		var progress map[string]interface{}
		json.NewDecoder(respProgress.Body).Decode(&progress)

		currTime := progress["currentTime"].(float64)
		if currTime != 45.0 {
			t.Errorf("Expected currentTime 45.0, got %f", currTime)
		}
	})

	// ==========================================
	// TIER 2 TESTS
	// ==========================================

	// 6. List in-progress items (GET /api/me/items-in-progress)
	t.Run("GET /api/me/items-in-progress - Podcast in list", func(t *testing.T) {
		reqProgress, _ := http.NewRequest("GET", h.BaseURL+"/api/me/items-in-progress", nil)
		reqProgress.Header.Set("Authorization", "Bearer "+adminToken)

		respProgress, err := client.Do(reqProgress)
		if err != nil {
			t.Fatalf("List progress items failed: %v", err)
		}
		defer respProgress.Body.Close()

		if respProgress.StatusCode != http.StatusOK {
			t.Fatalf("Expected status 200, got %d", respProgress.StatusCode)
		}

		var listResp map[string]interface{}
		json.NewDecoder(respProgress.Body).Decode(&listResp)

		items, ok := listResp["libraryItems"].([]interface{})
		if !ok || len(items) == 0 {
			t.Errorf("Expected at least one in-progress library item, got: %v", listResp)
		} else {
			item := items[0].(map[string]interface{})
			id := item["id"].(string)
			if id != libraryItemID {
				t.Errorf("Expected in-progress item ID %s, got %s", libraryItemID, id)
			}
		}
	})

	// 7. Hide from continue listening patch (PATCH /api/me/progress/:id/hide-from-continue-listening)
	t.Run("PATCH /api/me/progress/:id/hide-from-continue-listening - Podcast hide", func(t *testing.T) {
		var progressRecordID string
		err = db.QueryRow("SELECT id FROM mediaProgresses WHERE userId = ? AND mediaItemId = ?", adminUserID, episodeID).Scan(&progressRecordID)
		if err != nil {
			t.Fatalf("Failed to retrieve progress ID: %v", err)
		}

		reqHide, _ := http.NewRequest("PATCH", h.BaseURL+"/api/me/progress/"+progressRecordID+"/hide-from-continue-listening", nil)
		reqHide.Header.Set("Authorization", "Bearer "+adminToken)

		respHide, err := client.Do(reqHide)
		if err != nil {
			t.Fatalf("Hide progress failed: %v", err)
		}
		defer respHide.Body.Close()

		if respHide.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", respHide.StatusCode)
		}

		// Verify it is hidden (hideFromContinueListening = 1 in DB)
		var isHidden int
		err = db.QueryRow("SELECT hideFromContinueListening FROM mediaProgresses WHERE id = ?", progressRecordID).Scan(&isHidden)
		if err != nil {
			t.Fatalf("Failed to select hideFromContinueListening: %v", err)
		}
		if isHidden != 1 {
			t.Errorf("Expected progress to be marked hidden in DB (1), got %d", isHidden)
		}
	})

	// 8. Delete progress (DELETE /api/me/progress/:id)
	t.Run("DELETE /api/me/progress/:id - Delete podcast progress", func(t *testing.T) {
		var progressRecordID string
		err = db.QueryRow("SELECT id FROM mediaProgresses WHERE userId = ? AND mediaItemId = ?", adminUserID, episodeID).Scan(&progressRecordID)
		if err != nil {
			t.Fatalf("Failed to retrieve progress ID: %v", err)
		}

		reqDel, _ := http.NewRequest("DELETE", h.BaseURL+"/api/me/progress/"+progressRecordID, nil)
		reqDel.Header.Set("Authorization", "Bearer "+adminToken)

		respDel, err := client.Do(reqDel)
		if err != nil {
			t.Fatalf("Delete progress request failed: %v", err)
		}
		defer respDel.Body.Close()

		if respDel.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", respDel.StatusCode)
		}

		// Verify deletion
		var count int
		db.QueryRow("SELECT count(*) FROM mediaProgresses WHERE id = ?", progressRecordID).Scan(&count)
		if count != 0 {
			t.Errorf("Expected progress record to be deleted, but it still exists")
		}
	})

	// 9. Reject invalid/non-existent episode ID
	t.Run("GET /api/me/progress/:id/nonexistent - Reject invalid episode ID", func(t *testing.T) {
		reqProgress, _ := http.NewRequest("GET", h.BaseURL+"/api/me/progress/"+libraryItemID+"/nonexistent", nil)
		reqProgress.Header.Set("Authorization", "Bearer "+adminToken)

		respProgress, err := client.Do(reqProgress)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer respProgress.Body.Close()

		if respProgress.StatusCode != http.StatusNotFound {
			t.Errorf("Expected status 404 for invalid episode, got %d", respProgress.StatusCode)
		}
	})

	// 10. Access control check (Restricted user cannot fetch admin's podcast progress)
	t.Run("GET /api/me/progress/:id/:episodeId - Restricted access block", func(t *testing.T) {
		// Admin creates progress first
		progressPayload := map[string]interface{}{
			"duration":    120.0,
			"currentTime": 20.0,
		}
		body, _ := json.Marshal(progressPayload)

		reqProgress, _ := http.NewRequest("POST", h.BaseURL+"/api/me/progress/"+libraryItemID+"/"+episodeID, bytes.NewReader(body))
		reqProgress.Header.Set("Authorization", "Bearer "+adminToken)
		reqProgress.Header.Set("Content-Type", "application/json")
		respProgress, _ := client.Do(reqProgress)
		respProgress.Body.Close()

		// Other restricted user tries to fetch progress for this item + episode
		reqOther, _ := http.NewRequest("GET", h.BaseURL+"/api/me/progress/"+libraryItemID+"/"+episodeID, nil)
		reqOther.Header.Set("Authorization", "Bearer "+restrictedToken)

		respOther, err := client.Do(reqOther)
		if err != nil {
			t.Fatalf("Restricted request failed: %v", err)
		}
		defer respOther.Body.Close()

		if respOther.StatusCode != http.StatusNotFound {
			t.Errorf("Expected status 404 NotFound for another user's progress, got %d", respOther.StatusCode)
		}
	})
}
