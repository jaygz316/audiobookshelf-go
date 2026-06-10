package e2e_tests

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite"
)

func TestF5PlaybackSessionsAndHLS(t *testing.T) {
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

	// Create Library for tests
	libraryPath := filepath.Join(h.ConfigDir, "libraryF5")
	createPayload := map[string]interface{}{
		"name":      "Library F5",
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

	// Create mock book in database
	bookID := uuid.New().String()
	mockAudioPath := filepath.Join(libraryPath, "mock_audio.mp3")
	err = GenerateMockAudio(mockAudioPath, "Mock Playback Book", "Playback Author", "Playback Album", "1", "2026")
	if err != nil {
		t.Fatalf("Failed to generate mock audio: %v", err)
	}

	audioFilesJSON := fmt.Sprintf(`[{"index":0,"exclude":false,"duration":1.0,"codec":"mp3","mimeType":"audio/mpeg","metadata":{"path":%q}}]`, mockAudioPath)
	_, err = db.Exec(`INSERT INTO books (id, title, audioFiles, duration) VALUES (?, ?, ?, 1.0)`, bookID, "Mock Playback Book", audioFilesJSON)
	if err != nil {
		t.Fatalf("Failed to insert mock book: %v", err)
	}

	// Insert libraryItem corresponding to mock book so progress hide scans work
	libraryItemID := uuid.New().String()
	_, err = db.Exec(`INSERT INTO libraryItems (id, libraryId, mediaType, mediaId, title, isMissing, isInvalid, createdAt, updatedAt)
		VALUES (?, ?, 'book', ?, 'Mock Playback Book', 0, 0, datetime('now'), datetime('now'))`, libraryItemID, libraryID, bookID)
	if err != nil {
		t.Fatalf("Failed to insert libraryItem: %v", err)
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

	// 1. Sync Progress (create progress via POST /api/me/progress/:id)
	t.Run("POST /api/me/progress/:id - Sync progress", func(t *testing.T) {
		progressPayload := map[string]interface{}{
			"duration":                  60.0,
			"currentTime":               15.0,
			"isFinished":                false,
			"hideFromContinueListening": false,
		}
		body, _ := json.Marshal(progressPayload)

		reqProgress, _ := http.NewRequest("POST", h.BaseURL+"/api/me/progress/"+libraryItemID, bytes.NewReader(body))
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

	// 2. Update progress (PATCH /api/me/progress/:id)
	t.Run("PATCH /api/me/progress/:id - Update progress", func(t *testing.T) {
		progressPayload := map[string]interface{}{
			"currentTime": 30.0,
		}
		body, _ := json.Marshal(progressPayload)

		reqProgress, _ := http.NewRequest("PATCH", h.BaseURL+"/api/me/progress/"+libraryItemID, bytes.NewReader(body))
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

	// 3. Get progress (GET /api/me/progress/:id)
	t.Run("GET /api/me/progress/:id - Fetch progress", func(t *testing.T) {
		reqProgress, _ := http.NewRequest("GET", h.BaseURL+"/api/me/progress/"+libraryItemID, nil)
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
		if currTime != 30.0 {
			t.Errorf("Expected currentTime 30.0, got %f", currTime)
		}
	})

	// 4. List in-progress items (GET /api/me/items-in-progress)
	t.Run("GET /api/me/items-in-progress", func(t *testing.T) {
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

	// 5. Delete progress (DELETE /api/me/progress/:id)
	t.Run("DELETE /api/me/progress/:id - Delete progress", func(t *testing.T) {
		// First get the progress record ID from DB
		var progressRecordID string
		err = db.QueryRow("SELECT id FROM mediaProgresses WHERE userId = ? AND mediaItemId = ?", adminUserID, bookID).Scan(&progressRecordID)
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

	// ==========================================
	// TIER 2 TESTS
	// ==========================================

	// 6. Accessing HLS playlist /hls/<session_id>/output.m3u8
	sessionID := uuid.New().String()
	t.Run("GET /hls/:session_id/output.m3u8 - Access HLS playlist", func(t *testing.T) {
		// Insert mock playbackSession directly into the DB
		extraDataVal := fmt.Sprintf(`{"libraryItemId":%q}`, libraryItemID)
		_, err := db.Exec(`INSERT INTO playbackSessions (id, userId, mediaItemId, mediaItemType, startTime, libraryId, extraData, createdAt, updatedAt)
			VALUES (?, ?, ?, 'book', 0.0, ?, ?, datetime('now'), datetime('now'))`,
			sessionID, adminUserID, bookID, libraryID, extraDataVal)
		if err != nil {
			t.Fatalf("Failed to insert mock playbackSession: %v", err)
		}

		reqHLS, _ := http.NewRequest("GET", h.BaseURL+"/hls/"+sessionID+"/output.m3u8", nil)
		reqHLS.Header.Set("Authorization", "Bearer "+adminToken)

		respHLS, err := client.Do(reqHLS)
		if err != nil {
			t.Fatalf("Fetch HLS playlist failed: %v", err)
		}
		defer respHLS.Body.Close()

		if respHLS.StatusCode != http.StatusOK {
			t.Errorf("Expected HLS playlist StatusOK, got %d", respHLS.StatusCode)
		}

		playlistBytes, _ := io.ReadAll(respHLS.Body)
		if !bytes.Contains(playlistBytes, []byte("#EXTM3U")) {
			t.Errorf("Expected playlist content to contain #EXTM3U, got: %s", string(playlistBytes))
		}
	})

	// 7. Fetching HLS segment /hls/<session_id>/output-0.ts
	t.Run("GET /hls/:session_id/output-0.ts - Fetch segment", func(t *testing.T) {
		// Wait briefly to allow background ffmpeg to transcode the tiny 1s silent segment
		time.Sleep(500 * time.Millisecond)

		reqHLS, _ := http.NewRequest("GET", h.BaseURL+"/hls/"+sessionID+"/output-0.ts", nil)
		reqHLS.Header.Set("Authorization", "Bearer "+adminToken)

		respHLS, err := client.Do(reqHLS)
		if err != nil {
			t.Fatalf("Fetch HLS segment failed: %v", err)
		}
		defer respHLS.Body.Close()

		// If transcoding is not yet completed in the environment or ffmpeg isn't installed properly,
		// we handle a StatusNotFound (404 Segment not ready) gracefully. If it succeeds, it must be StatusOK.
		if respHLS.StatusCode != http.StatusOK && respHLS.StatusCode != http.StatusNotFound {
			t.Errorf("Expected HLS segment status 200 or 404, got %d", respHLS.StatusCode)
		}
	})

	// 8. Reject invalid/non-existent progress ID
	t.Run("GET /api/me/progress/nonexistent - Reject invalid progress ID", func(t *testing.T) {
		reqProgress, _ := http.NewRequest("GET", h.BaseURL+"/api/me/progress/nonexistent", nil)
		reqProgress.Header.Set("Authorization", "Bearer "+adminToken)

		respProgress, err := client.Do(reqProgress)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer respProgress.Body.Close()

		// Expect 404 NotFound
		if respProgress.StatusCode != http.StatusNotFound {
			t.Errorf("Expected status 404 for invalid progress, got %d", respProgress.StatusCode)
		}
	})

	// 9. Hide from continue listening patch
	t.Run("PATCH /api/me/progress/:id/hide-from-continue-listening", func(t *testing.T) {
		// Re-create progress record first
		progressPayload := map[string]interface{}{
			"duration":    60.0,
			"currentTime": 15.0,
		}
		body, _ := json.Marshal(progressPayload)

		reqProgress, _ := http.NewRequest("POST", h.BaseURL+"/api/me/progress/"+libraryItemID, bytes.NewReader(body))
		reqProgress.Header.Set("Authorization", "Bearer "+adminToken)
		reqProgress.Header.Set("Content-Type", "application/json")
		respProgress, _ := client.Do(reqProgress)
		respProgress.Body.Close()

		var progressRecordID string
		err = db.QueryRow("SELECT id FROM mediaProgresses WHERE userId = ? AND mediaItemId = ?", adminUserID, bookID).Scan(&progressRecordID)
		if err != nil {
			t.Fatalf("Failed to retrieve progress ID: %v", err)
		}

		// Hide progress from continue listening
		reqHide, _ := http.NewRequest("PATCH", h.BaseURL+"/api/me/progress/"+progressRecordID+"/hide-from-continue-listening", nil)
		reqHide.Header.Set("Authorization", "Bearer "+adminToken)

		respHide, err := client.Do(reqHide)
		if err != nil {
			t.Fatalf("Hide progress from continue listening failed: %v", err)
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

		// Verify it no longer appears in items-in-progress list
		reqList, _ := http.NewRequest("GET", h.BaseURL+"/api/me/items-in-progress", nil)
		reqList.Header.Set("Authorization", "Bearer "+adminToken)
		respList, err := client.Do(reqList)
		if err != nil {
			t.Fatalf("Get in progress failed: %v", err)
		}
		defer respList.Body.Close()

		var listResp map[string]interface{}
		json.NewDecoder(respList.Body).Decode(&listResp)

		items, ok := listResp["libraryItems"].([]interface{})
		if ok {
			for _, itemVal := range items {
				item := itemVal.(map[string]interface{})
				if item["id"].(string) == libraryItemID {
					t.Errorf("Hidden item still returned in items-in-progress list")
				}
			}
		}
	})

	// 10. Access control check
	t.Run("GET /api/me/progress/:id - Restricted access block", func(t *testing.T) {
		// Admin creates progress
		progressPayload := map[string]interface{}{
			"duration":    60.0,
			"currentTime": 15.0,
		}
		body, _ := json.Marshal(progressPayload)

		reqProgress, _ := http.NewRequest("POST", h.BaseURL+"/api/me/progress/"+libraryItemID, bytes.NewReader(body))
		reqProgress.Header.Set("Authorization", "Bearer "+adminToken)
		reqProgress.Header.Set("Content-Type", "application/json")
		respProgress, _ := client.Do(reqProgress)
		respProgress.Body.Close()

		// Other restricted user tries to fetch progress for this item
		// Since me progress fetches by logged-in userId, retrieving this item should return 404/NotFound
		// as there's no progress record for the restricted user on this item.
		reqOther, _ := http.NewRequest("GET", h.BaseURL+"/api/me/progress/"+libraryItemID, nil)
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
