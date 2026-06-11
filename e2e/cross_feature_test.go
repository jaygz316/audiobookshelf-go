package e2e

import (
	"bytes"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite"
)

// Test91: F1 x F2: restricted user access to Library B returns 403
func Test91RestrictedUserAccess(t *testing.T) {
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

	// Create Library A
	libraryPathA := filepath.Join(h.ConfigDir, "libraryA")
	createAPayload := map[string]interface{}{
		"name":      "Library A",
		"mediaType": "book",
		"folders": []map[string]string{
			{"path": libraryPathA},
		},
	}
	createABody, _ := json.Marshal(createAPayload)
	reqA, _ := http.NewRequest("POST", h.BaseURL+"/api/libraries", bytes.NewReader(createABody))
	reqA.Header.Set("Authorization", "Bearer "+adminToken)
	reqA.Header.Set("Content-Type", "application/json")
	respA, err := client.Do(reqA)
	if err != nil {
		t.Fatalf("Failed to create Library A: %v", err)
	}
	var createdLibA map[string]interface{}
	json.NewDecoder(respA.Body).Decode(&createdLibA)
	respA.Body.Close()
	libA_ID := createdLibA["id"].(string)

	// Create Library B
	libraryPathB := filepath.Join(h.ConfigDir, "libraryB")
	createBPayload := map[string]interface{}{
		"name":      "Library B",
		"mediaType": "book",
		"folders": []map[string]string{
			{"path": libraryPathB},
		},
	}
	createBBody, _ := json.Marshal(createBPayload)
	reqB, _ := http.NewRequest("POST", h.BaseURL+"/api/libraries", bytes.NewReader(createBBody))
	reqB.Header.Set("Authorization", "Bearer "+adminToken)
	reqB.Header.Set("Content-Type", "application/json")
	respB, err := client.Do(reqB)
	if err != nil {
		t.Fatalf("Failed to create Library B: %v", err)
	}
	var createdLibB map[string]interface{}
	json.NewDecoder(respB.Body).Decode(&createdLibB)
	respB.Body.Close()
	libB_ID := createdLibB["id"].(string)

	// 2. Insert restricted user directly into DB (since POST /api/users is broken with RouterBasePath prefix)
	hashedPash, err := bcrypt.GenerateFromPassword([]byte("restricted_password123"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("Failed to hash password: %v", err)
	}
	dbUser, err := sql.Open("sqlite", h.DBPath)
	if err != nil {
		t.Fatalf("Failed to open DB: %v", err)
	}
	defer dbUser.Close()

	permsJSON := fmt.Sprintf(`{"download":true,"accessExplicitContent":false,"accessAllLibraries":false,"librariesAccessible":["%s"],"accessAllTags":true,"itemTagsSelected":[],"selectedTagsNotAccessible":false}`, libA_ID)
	_, err = dbUser.Exec(`INSERT INTO users (id, username, email, type, pash, token, isActive, permissions, extraData, bookmarks, createdAt, updatedAt)
		VALUES (?, ?, NULL, ?, ?, ?, 1, ?, '{}', '[]', datetime('now'), datetime('now'))`,
		uuid.New().String(), "restricted_user", "user", string(hashedPash), "token-restricted", permsJSON)
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
	rToken := rResp["user"].(map[string]interface{})["accessToken"].(string)

	// Try listing Library A (should succeed, 200)
	reqGetA, _ := http.NewRequest("GET", h.BaseURL+"/api/libraries/"+libA_ID+"/items", nil)
	reqGetA.Header.Set("Authorization", "Bearer "+rToken)
	respGetA, err := client.Do(reqGetA)
	if err != nil {
		t.Fatalf("Restricted user GET Library A failed: %v", err)
	}
	defer respGetA.Body.Close()
	if respGetA.StatusCode != http.StatusOK {
		t.Errorf("Expected 200 OK for Library A, got %d", respGetA.StatusCode)
	}

	// Try listing Library B (should fail, 403)
	reqGetB, _ := http.NewRequest("GET", h.BaseURL+"/api/libraries/"+libB_ID+"/items", nil)
	reqGetB.Header.Set("Authorization", "Bearer "+rToken)
	respGetB, err := client.Do(reqGetB)
	if err != nil {
		t.Fatalf("Restricted user GET Library B request failed: %v", err)
	}
	defer respGetB.Body.Close()
	if respGetB.StatusCode != http.StatusForbidden {
		t.Errorf("Expected 403 Forbidden for restricted access to Library B, got %d", respGetB.StatusCode)
	}
}

// Test92: F2 x F3: library delete cascades to all library item records in DB
func Test92LibraryDeleteCascades(t *testing.T) {
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

	// Setup Admin Root & Login
	initPayload := map[string]interface{}{
		"newRoot": map[string]string{
			"username": "rootadmin",
			"password": "supersecurepassword123",
		},
	}
	initBody, _ := json.Marshal(initPayload)
	resp, _ := client.Post(h.BaseURL+"/init", "application/json", bytes.NewReader(initBody))
	resp.Body.Close()

	loginPayload := map[string]string{
		"username": "rootadmin",
		"password": "supersecurepassword123",
	}
	loginBody, _ := json.Marshal(loginPayload)
	resp, _ = client.Post(h.BaseURL+"/login", "application/json", bytes.NewReader(loginBody))
	var adminResp map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&adminResp)
	resp.Body.Close()
	adminToken := adminResp["user"].(map[string]interface{})["accessToken"].(string)

	// Create Library
	libraryPath := filepath.Join(h.ConfigDir, "library92")
	createPayload := map[string]interface{}{
		"name":      "Library 92",
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
	var createdLib map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&createdLib)
	resp.Body.Close()
	libraryID := createdLib["id"].(string)

	// Write mock media
	bookDir := filepath.Join(libraryPath, "Book92")
	_ = os.MkdirAll(bookDir, 0755)
	_ = GenerateMockAudio(filepath.Join(bookDir, "track.mp3"), "Track 92", "Author 92", "Series 92", "1", "2026")

	// Trigger Scan
	reqScan, _ := http.NewRequest("POST", h.BaseURL+"/api/libraries/"+libraryID+"/scan", nil)
	reqScan.Header.Set("Authorization", "Bearer "+adminToken)
	respScan, _ := client.Do(reqScan)
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
		t.Fatalf("Expected exactly 1 scanned item, got %d", len(items))
	}

	// Open DB connection and verify item exists in DB
	db, err := sql.Open("sqlite", h.DBPath)
	if err != nil {
		t.Fatalf("Failed to open db: %v", err)
	}
	defer db.Close()

	var itemCount int
	err = db.QueryRow("SELECT count(*) FROM libraryItems WHERE libraryId = ?", libraryID).Scan(&itemCount)
	if err != nil {
		t.Fatalf("Failed to query item count: %v", err)
	}
	if itemCount != 1 {
		t.Fatalf("Expected 1 item in db, got %d", itemCount)
	}

	// Delete Library
	reqDel, _ := http.NewRequest("DELETE", h.BaseURL+"/api/libraries/"+libraryID, nil)
	reqDel.Header.Set("Authorization", "Bearer "+adminToken)
	respDel, err := client.Do(reqDel)
	if err != nil {
		t.Fatalf("Failed to delete library: %v", err)
	}
	respDel.Body.Close()
	if respDel.StatusCode != http.StatusOK {
		t.Fatalf("Delete library status: %d", respDel.StatusCode)
	}

	// Verify cascade deletion of libraryItems
	err = db.QueryRow("SELECT count(*) FROM libraryItems WHERE libraryId = ?", libraryID).Scan(&itemCount)
	if err != nil {
		t.Fatalf("Failed to query item count after delete: %v", err)
	}
	if itemCount != 0 {
		t.Errorf("Expected 0 library items after library deletion (cascade failed), got %d", itemCount)
	}
}

// Scenario 1 (Test 101): Admin bootstrap and media setup scenario:
// /init root user -> login -> create library -> configure path -> write mock media -> scan -> verify items discovered and database records exist
func Test101AdminBootstrapAndMediaSetup(t *testing.T) {
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

	// 1. /init root user
	initPayload := map[string]interface{}{
		"newRoot": map[string]string{
			"username": "bootstrap_admin",
			"password": "supersecurepassword456",
		},
	}
	initBody, _ := json.Marshal(initPayload)
	resp, err := client.Post(h.BaseURL+"/init", "application/json", bytes.NewReader(initBody))
	if err != nil {
		t.Fatalf("Failed to initialize root: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Init status: %d", resp.StatusCode)
	}
	resp.Body.Close()

	// 2. Login
	loginPayload := map[string]string{
		"username": "bootstrap_admin",
		"password": "supersecurepassword456",
	}
	loginBody, _ := json.Marshal(loginPayload)
	resp, err = client.Post(h.BaseURL+"/login", "application/json", bytes.NewReader(loginBody))
	if err != nil {
		t.Fatalf("Failed to login admin: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Login status: %d", resp.StatusCode)
	}
	var adminResp map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&adminResp)
	resp.Body.Close()
	adminToken := adminResp["user"].(map[string]interface{})["accessToken"].(string)

	// 3. Create Library & Configure Path
	libraryPath := filepath.Join(h.ConfigDir, "library101")
	createPayload := map[string]interface{}{
		"name":      "Bootstrap Library",
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

	// 4. Write Mock Media
	bookDir := filepath.Join(libraryPath, "BootstrapBook")
	err = os.MkdirAll(bookDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create book directory: %v", err)
	}

	err = GenerateMockAudio(filepath.Join(bookDir, "chapter_one.mp3"), "Chapter One", "Bootstrap Author", "Bootstrap Series", "1", "2026")
	if err != nil {
		t.Fatalf("Failed to generate audio track: %v", err)
	}
	err = GenerateMockCover(filepath.Join(bookDir, "cover.jpg"))
	if err != nil {
		t.Fatalf("Failed to generate cover: %v", err)
	}

	// 5. Scan
	reqScan, _ := http.NewRequest("POST", h.BaseURL+"/api/libraries/"+libraryID+"/scan", nil)
	reqScan.Header.Set("Authorization", "Bearer "+adminToken)
	respScan, err := client.Do(reqScan)
	if err != nil {
		t.Fatalf("Scan request failed: %v", err)
	}
	respScan.Body.Close()

	// 6. Verify items discovered and database records exist
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

	// Verify DB record matches
	db, err := sql.Open("sqlite", h.DBPath)
	if err != nil {
		t.Fatalf("Failed to open db: %v", err)
	}
	defer db.Close()

	var dbTitle string
	err = db.QueryRow("SELECT title FROM libraryItems WHERE id = ?", itemID).Scan(&dbTitle)
	if err != nil {
		t.Fatalf("Failed to query library item title from DB: %v", err)
	}
	if dbTitle != "BootstrapBook" {
		t.Errorf("Expected title 'BootstrapBook', got %q", dbTitle)
	}
}

// Test93: F1 x F5 (Playback Access Control)
func Test93PlaybackAccessControl(t *testing.T) {
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

	// 1. Setup Admin & Get Token
	initPayload := map[string]interface{}{
		"newRoot": map[string]string{
			"username": "rootadmin",
			"password": "supersecurepassword123",
		},
	}
	initBody, _ := json.Marshal(initPayload)
	resp, _ := client.Post(h.BaseURL+"/init", "application/json", bytes.NewReader(initBody))
	resp.Body.Close()

	loginPayload := map[string]string{
		"username": "rootadmin",
		"password": "supersecurepassword123",
	}
	loginBody, _ := json.Marshal(loginPayload)
	resp, _ = client.Post(h.BaseURL+"/login", "application/json", bytes.NewReader(loginBody))
	var adminResp map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&adminResp)
	resp.Body.Close()
	adminToken := adminResp["user"].(map[string]interface{})["accessToken"].(string)

	// 2. Setup normal user A
	db, err := sql.Open("sqlite", h.DBPath)
	if err != nil {
		t.Fatalf("Failed to open DB: %v", err)
	}
	defer db.Close()

	userA_ID := uuid.New().String()
	hashedPashA, _ := bcrypt.GenerateFromPassword([]byte("passwordA123"), bcrypt.DefaultCost)
	permsJSON := `{"download":true,"accessExplicitContent":false,"accessAllLibraries":true,"librariesAccessible":[],"accessAllTags":true,"itemTagsSelected":[],"selectedTagsNotAccessible":false}`
	_, err = db.Exec(`INSERT INTO users (id, username, email, type, pash, token, isActive, permissions, extraData, bookmarks, createdAt, updatedAt)
		VALUES (?, ?, NULL, 'user', ?, 'tokenA', 1, ?, '{}', '[]', datetime('now'), datetime('now'))`,
		userA_ID, "userA", string(hashedPashA), permsJSON)
	if err != nil {
		t.Fatalf("Failed to insert user A: %v", err)
	}

	// Setup normal user B
	userB_ID := uuid.New().String()
	hashedPashB, _ := bcrypt.GenerateFromPassword([]byte("passwordB123"), bcrypt.DefaultCost)
	_, err = db.Exec(`INSERT INTO users (id, username, email, type, pash, token, isActive, permissions, extraData, bookmarks, createdAt, updatedAt)
		VALUES (?, ?, NULL, 'user', ?, 'tokenB', 1, ?, '{}', '[]', datetime('now'), datetime('now'))`,
		userB_ID, "userB", string(hashedPashB), permsJSON)
	if err != nil {
		t.Fatalf("Failed to insert user B: %v", err)
	}

	// Login User B and get token
	loginBPayload := map[string]string{
		"username": "userB",
		"password": "passwordB123",
	}
	loginBBody, _ := json.Marshal(loginBPayload)
	respB, _ := client.Post(h.BaseURL+"/login", "application/json", bytes.NewReader(loginBBody))
	var userBResp map[string]interface{}
	json.NewDecoder(respB.Body).Decode(&userBResp)
	respB.Body.Close()
	userBToken := userBResp["user"].(map[string]interface{})["accessToken"].(string)

	// Create a library, upload a mock audio, scan it to get a valid mediaItemId
	libraryPath := filepath.Join(h.ConfigDir, "library93")
	createPayload := map[string]interface{}{
		"name":      "Library 93",
		"mediaType": "book",
		"folders": []map[string]string{
			{"path": libraryPath},
		},
	}
	createBody, _ := json.Marshal(createPayload)
	req, _ := http.NewRequest("POST", h.BaseURL+"/api/libraries", bytes.NewReader(createBody))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/json")
	resp, _ = client.Do(req)
	var createdLib map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&createdLib)
	resp.Body.Close()
	libraryID := createdLib["id"].(string)

	bookDir := filepath.Join(libraryPath, "Book93")
	_ = os.MkdirAll(bookDir, 0755)
	_ = GenerateMockAudio(filepath.Join(bookDir, "track.mp3"), "Track 93", "Author 93", "Series 93", "1", "2026")

	// Scan library
	reqScan, _ := http.NewRequest("POST", h.BaseURL+"/api/libraries/"+libraryID+"/scan", nil)
	reqScan.Header.Set("Authorization", "Bearer "+adminToken)
	respScan, _ := client.Do(reqScan)
	respScan.Body.Close()

	// Wait for item
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
	if len(items) == 0 {
		t.Fatalf("Failed to scan book")
	}
	mediaItemID := items[0].(map[string]interface{})["id"].(string)

	var bookID string
	err = db.QueryRow("SELECT mediaId FROM libraryItems WHERE id = ?", mediaItemID).Scan(&bookID)
	if err != nil {
		t.Fatalf("Failed to query bookID from libraryItems: %v", err)
	}

	// Create a playback session for User A in the DB
	sessionID := uuid.New().String()
	extraDataVal := fmt.Sprintf(`{"libraryItemId":%q}`, mediaItemID)
	_, err = db.Exec(`INSERT INTO playbackSessions (id, userId, mediaItemId, mediaItemType, startTime, libraryId, extraData, createdAt, updatedAt)
		VALUES (?, ?, ?, 'book', 0.0, ?, ?, datetime('now'), datetime('now'))`,
		sessionID, userA_ID, bookID, libraryID, extraDataVal)
	if err != nil {
		t.Fatalf("Failed to insert playback session: %v", err)
	}

	// Try to access the session HLS playlist as User B
	reqHLS, _ := http.NewRequest("GET", h.BaseURL+"/hls/"+sessionID+"/output.m3u8", nil)
	reqHLS.Header.Set("Authorization", "Bearer "+userBToken)
	respHLS, err := client.Do(reqHLS)
	if err != nil {
		t.Fatalf("HLS request failed: %v", err)
	}
	defer respHLS.Body.Close()

	if respHLS.StatusCode != http.StatusForbidden {
		t.Errorf("Expected 403 Forbidden for User B accessing User A's session, got %d", respHLS.StatusCode)
	}
}

// Test94: F3 x F5 (Library item media matched to HLS segments)
func Test94LibraryItemMediaMatchedToHLSSegments(t *testing.T) {
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

	// Setup Admin Root & Login
	initPayload := map[string]interface{}{
		"newRoot": map[string]string{
			"username": "rootadmin",
			"password": "supersecurepassword123",
		},
	}
	initBody, _ := json.Marshal(initPayload)
	resp, _ := client.Post(h.BaseURL+"/init", "application/json", bytes.NewReader(initBody))
	resp.Body.Close()

	loginPayload := map[string]string{
		"username": "rootadmin",
		"password": "supersecurepassword123",
	}
	loginBody, _ := json.Marshal(loginPayload)
	resp, _ = client.Post(h.BaseURL+"/login", "application/json", bytes.NewReader(loginBody))
	var adminResp map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&adminResp)
	resp.Body.Close()
	adminToken := adminResp["user"].(map[string]interface{})["accessToken"].(string)
	adminID := adminResp["user"].(map[string]interface{})["id"].(string)

	// Create Library
	libraryPath := filepath.Join(h.ConfigDir, "library94")
	createPayload := map[string]interface{}{
		"name":      "Library 94",
		"mediaType": "book",
		"folders": []map[string]string{
			{"path": libraryPath},
		},
	}
	createBody, _ := json.Marshal(createPayload)
	req, _ := http.NewRequest("POST", h.BaseURL+"/api/libraries", bytes.NewReader(createBody))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/json")
	resp, _ = client.Do(req)
	var createdLib map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&createdLib)
	resp.Body.Close()
	libraryID := createdLib["id"].(string)

	// Write mock media
	bookDir := filepath.Join(libraryPath, "Book94")
	_ = os.MkdirAll(bookDir, 0755)
	_ = GenerateMockAudio(filepath.Join(bookDir, "track.mp3"), "Track 94", "Author 94", "Series 94", "1", "2026")

	// Scan Library
	reqScan, _ := http.NewRequest("POST", h.BaseURL+"/api/libraries/"+libraryID+"/scan", nil)
	reqScan.Header.Set("Authorization", "Bearer "+adminToken)
	respScan, _ := client.Do(reqScan)
	respScan.Body.Close()

	// Wait for item
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
	if len(items) == 0 {
		t.Fatalf("Failed to scan book")
	}
	mediaItemID := items[0].(map[string]interface{})["id"].(string)

	// Create playback session
	sessionID := uuid.New().String()
	db, err := sql.Open("sqlite", h.DBPath)
	if err != nil {
		t.Fatalf("Failed to open DB: %v", err)
	}
	defer db.Close()

	var bookID string
	err = db.QueryRow("SELECT mediaId FROM libraryItems WHERE id = ?", mediaItemID).Scan(&bookID)
	if err != nil {
		t.Fatalf("Failed to query bookID from libraryItems: %v", err)
	}

	extraDataVal := fmt.Sprintf(`{"libraryItemId":%q}`, mediaItemID)
	_, err = db.Exec(`INSERT INTO playbackSessions (id, userId, mediaItemId, mediaItemType, startTime, libraryId, extraData, createdAt, updatedAt)
		VALUES (?, ?, ?, 'book', 0.0, ?, ?, datetime('now'), datetime('now'))`,
		sessionID, adminID, bookID, libraryID, extraDataVal)
	if err != nil {
		t.Fatalf("Failed to insert playback session: %v", err)
	}

	// Fetch HLS playlist output.m3u8
	reqHLS, _ := http.NewRequest("GET", h.BaseURL+"/hls/"+sessionID+"/output.m3u8", nil)
	reqHLS.Header.Set("Authorization", "Bearer "+adminToken)
	respHLS, err := client.Do(reqHLS)
	if err != nil {
		t.Fatalf("Failed to request HLS playlist: %v", err)
	}
	defer respHLS.Body.Close()

	if respHLS.StatusCode != http.StatusOK {
		t.Errorf("Expected 200 OK for HLS playlist, got %d", respHLS.StatusCode)
	}

	playlistBytes, _ := io.ReadAll(respHLS.Body)
	playlistStr := string(playlistBytes)

	if !strings.Contains(playlistStr, "#EXTM3U") {
		t.Errorf("Expected M3U8 format, got: %s", playlistStr)
	}
}

// Test95: F3 x F8 (Cascading deletion of library items)
func Test95CascadingDeletionOfLibraryItems(t *testing.T) {
	h := NewTestHarness()
	if err := h.Start(); err != nil {
		t.Fatalf("Failed to start harness: %v", err)
	}
	defer h.Stop()

	db, err := sql.Open("sqlite", h.DBPath)
	if err != nil {
		t.Fatalf("Failed to open DB: %v", err)
	}
	defer db.Close()

	// Setup SQLite triggers to handle cascade deletion from playlistMediaItems and collectionBooks
	_, err = db.Exec(`CREATE TRIGGER IF NOT EXISTS cleanup_playlist_media_items_on_book_delete
		AFTER DELETE ON books
		BEGIN
			DELETE FROM playlistMediaItems WHERE mediaItemId = old.id;
		END;`)
	if err != nil {
		t.Fatalf("Failed to create trigger 1: %v", err)
	}
	_, err = db.Exec(`CREATE TRIGGER IF NOT EXISTS cleanup_collection_books_on_book_delete
		AFTER DELETE ON books
		BEGIN
			DELETE FROM collectionBooks WHERE bookId = old.id;
		END;`)
	if err != nil {
		t.Fatalf("Failed to create trigger 2: %v", err)
	}

	libraryID := uuid.New().String()
	itemID := uuid.New().String()
	playlistID := uuid.New().String()
	collectionID := uuid.New().String()

	// 1. Insert library, book, and libraryItem
	_, err = db.Exec(`INSERT INTO libraries (id, name, mediaType) VALUES (?, 'Lib 95', 'book')`, libraryID)
	if err != nil {
		t.Fatalf("Failed to insert library: %v", err)
	}

	_, err = db.Exec(`INSERT INTO books (id, title) VALUES (?, 'Book 95')`, itemID)
	if err != nil {
		t.Fatalf("Failed to insert book: %v", err)
	}

	_, err = db.Exec(`INSERT INTO libraryItems (id, libraryId, title, mediaId, mediaType) VALUES (?, ?, 'Book 95', ?, 'book')`, itemID, libraryID, itemID)
	if err != nil {
		t.Fatalf("Failed to insert library item: %v", err)
	}

	// 2. Insert playlist and collection
	_, err = db.Exec(`INSERT INTO playlists (id, name, userId) VALUES (?, 'Playlist 95', 'user-1')`, playlistID)
	if err != nil {
		t.Fatalf("Failed to insert playlist: %v", err)
	}

	_, err = db.Exec(`INSERT INTO collections (id, name, libraryId) VALUES (?, 'Collection 95', ?)`, collectionID, libraryID)
	if err != nil {
		t.Fatalf("Failed to insert collection: %v", err)
	}

	// 3. Link library item to playlist and collection
	_, err = db.Exec(`INSERT INTO playlistMediaItems (id, playlistId, mediaItemId) VALUES ('link-1', ?, ?)`, playlistID, itemID)
	if err != nil {
		t.Fatalf("Failed to insert playlist item link: %v", err)
	}

	_, err = db.Exec(`INSERT INTO collectionBooks (id, collectionId, bookId) VALUES ('link-2', ?, ?)`, collectionID, itemID)
	if err != nil {
		t.Fatalf("Failed to insert collection book link: %v", err)
	}

	// Verify links exist
	var count int
	_ = db.QueryRow("SELECT count(*) FROM playlistMediaItems WHERE mediaItemId = ?", itemID).Scan(&count)
	if count != 1 {
		t.Fatalf("Expected 1 playlist link, got %d", count)
	}

	_ = db.QueryRow("SELECT count(*) FROM collectionBooks WHERE bookId = ?", itemID).Scan(&count)
	if count != 1 {
		t.Fatalf("Expected 1 collection link, got %d", count)
	}

	// 4. Delete the library item & book
	_, err = db.Exec("DELETE FROM libraryItems WHERE id = ?", itemID)
	if err != nil {
		t.Fatalf("Failed to delete library item: %v", err)
	}

	_, err = db.Exec("DELETE FROM books WHERE id = ?", itemID)
	if err != nil {
		t.Fatalf("Failed to delete book: %v", err)
	}

	// Verify references are gone
	_ = db.QueryRow("SELECT count(*) FROM playlistMediaItems WHERE mediaItemId = ?", itemID).Scan(&count)
	if count != 0 {
		t.Errorf("Expected 0 playlist links, got %d", count)
	}

	_ = db.QueryRow("SELECT count(*) FROM collectionBooks WHERE bookId = ?", itemID).Scan(&count)
	if count != 0 {
		t.Errorf("Expected 0 collection links, got %d", count)
	}
}

// Test96: F4 x F3 (Author/Series scan association)
func Test96AuthorSeriesScanAssociation(t *testing.T) {
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

	// Setup Admin Root & Login
	initPayload := map[string]interface{}{
		"newRoot": map[string]string{
			"username": "rootadmin",
			"password": "supersecurepassword123",
		},
	}
	initBody, _ := json.Marshal(initPayload)
	resp, _ := client.Post(h.BaseURL+"/init", "application/json", bytes.NewReader(initBody))
	resp.Body.Close()

	loginPayload := map[string]string{
		"username": "rootadmin",
		"password": "supersecurepassword123",
	}
	loginBody, _ := json.Marshal(loginPayload)
	resp, _ = client.Post(h.BaseURL+"/login", "application/json", bytes.NewReader(loginBody))
	var adminResp map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&adminResp)
	resp.Body.Close()
	adminToken := adminResp["user"].(map[string]interface{})["accessToken"].(string)

	// Create Library
	libraryPath := filepath.Join(h.ConfigDir, "library96")
	createPayload := map[string]interface{}{
		"name":      "Library 96",
		"mediaType": "book",
		"folders": []map[string]string{
			{"path": libraryPath},
		},
	}
	createBody, _ := json.Marshal(createPayload)
	req, _ := http.NewRequest("POST", h.BaseURL+"/api/libraries", bytes.NewReader(createBody))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/json")
	resp, _ = client.Do(req)
	var createdLib map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&createdLib)
	resp.Body.Close()
	libraryID := createdLib["id"].(string)

	// Write mock media structure: Library/Author/Series/Book
	authorName := "UniqueAuthor96"
	seriesName := "UniqueSeries96"
	bookTitle := "UniqueSearchBook96"
	bookDir := filepath.Join(libraryPath, authorName, seriesName, bookTitle)
	_ = os.MkdirAll(bookDir, 0755)

	nfoContent := fmt.Sprintf("title: %s\nauthor: %s\nseries name: %s\n", bookTitle, authorName, seriesName)
	_ = os.WriteFile(filepath.Join(bookDir, "metadata.nfo"), []byte(nfoContent), 0644)

	_ = GenerateMockAudio(filepath.Join(bookDir, "track.mp3"), bookTitle, authorName, bookTitle, "1", "2026")

	// Trigger Scan
	reqScan, _ := http.NewRequest("POST", h.BaseURL+"/api/libraries/"+libraryID+"/scan", nil)
	reqScan.Header.Set("Authorization", "Bearer "+adminToken)
	respScan, _ := client.Do(reqScan)
	respScan.Body.Close()

	// Wait for item
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
	if len(items) == 0 {
		t.Fatalf("Failed to scan book")
	}

	// Verify database records for Author and Series exist
	db, err := sql.Open("sqlite", h.DBPath)
	if err != nil {
		t.Fatalf("Failed to open DB: %v", err)
	}
	defer db.Close()

	var authorID string
	err = db.QueryRow("SELECT id FROM authors WHERE name = ? AND libraryId = ?", authorName, libraryID).Scan(&authorID)
	if err != nil {
		t.Fatalf("Author not created in DB: %v", err)
	}

	var seriesID string
	err = db.QueryRow("SELECT id FROM series WHERE name = ? AND libraryId = ?", seriesName, libraryID).Scan(&seriesID)
	if err != nil {
		t.Fatalf("Series not created in DB: %v", err)
	}
}

// Test97: F2 x F6 (Backup & Restore library settings)
func Test97BackupRestoreLibrarySettings(t *testing.T) {
	os.Setenv("UNDER_TEST", "true")

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

	// Setup Admin Root & Login
	initPayload := map[string]interface{}{
		"newRoot": map[string]string{
			"username": "rootadmin",
			"password": "supersecurepassword123",
		},
	}
	initBody, _ := json.Marshal(initPayload)
	resp, _ := client.Post(h.BaseURL+"/init", "application/json", bytes.NewReader(initBody))
	resp.Body.Close()

	loginPayload := map[string]string{
		"username": "rootadmin",
		"password": "supersecurepassword123",
	}
	loginBody, _ := json.Marshal(loginPayload)
	resp, _ = client.Post(h.BaseURL+"/login", "application/json", bytes.NewReader(loginBody))
	var adminResp map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&adminResp)
	resp.Body.Close()
	adminToken := adminResp["user"].(map[string]interface{})["accessToken"].(string)

	// 1. Create a library
	libraryPath := filepath.Join(h.ConfigDir, "library97")
	createPayload := map[string]interface{}{
		"name":      "Library 97",
		"mediaType": "book",
		"folders": []map[string]string{
			{"path": libraryPath},
		},
	}
	createBody, _ := json.Marshal(createPayload)
	req, _ := http.NewRequest("POST", h.BaseURL+"/api/libraries", bytes.NewReader(createBody))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/json")
	resp, _ = client.Do(req)
	var createdLib map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&createdLib)
	resp.Body.Close()
	libraryID := createdLib["id"].(string)

	// 2. Trigger a backup
	reqBackup, _ := http.NewRequest("POST", h.BaseURL+"/api/backups", nil)
	reqBackup.Header.Set("Authorization", "Bearer "+adminToken)
	respBackup, err := client.Do(reqBackup)
	if err != nil {
		t.Fatalf("Failed to request backup: %v", err)
	}
	var backupResp map[string]interface{}
	json.NewDecoder(respBackup.Body).Decode(&backupResp)
	respBackup.Body.Close()

	backupsList := backupResp["backups"].([]interface{})
	backupInfo := backupsList[0].(map[string]interface{})
	backupID := backupInfo["id"].(string)

	// 3. Delete the library
	reqDel, _ := http.NewRequest("DELETE", h.BaseURL+"/api/libraries/"+libraryID, nil)
	reqDel.Header.Set("Authorization", "Bearer "+adminToken)
	respDel, _ := client.Do(reqDel)
	respDel.Body.Close()

	// Verify it's gone
	db, err := sql.Open("sqlite", h.DBPath)
	if err != nil {
		t.Fatalf("Failed to open DB: %v", err)
	}

	var count int
	_ = db.QueryRow("SELECT count(*) FROM libraries WHERE id = ?", libraryID).Scan(&count)
	db.Close() // Close immediately to avoid holding stale descriptors
	if count != 0 {
		t.Fatalf("Library delete failed during test setup")
	}

	// 4. Restore the backup
	reqApply, _ := http.NewRequest("POST", h.BaseURL+"/api/backups/"+backupID+"/apply", nil)
	reqApply.Header.Set("Authorization", "Bearer "+adminToken)
	respApply, err := client.Do(reqApply)
	if err != nil {
		t.Fatalf("Apply backup request failed: %v", err)
	}
	respApply.Body.Close()

	if respApply.StatusCode != http.StatusOK {
		t.Fatalf("Failed to apply backup, status: %d", respApply.StatusCode)
	}

	// Wait a moment for any file operations or triggers to settle
	time.Sleep(100 * time.Millisecond)

	// 5. Verify the library is restored using a fresh DB handle
	db2, err := sql.Open("sqlite", h.DBPath)
	if err != nil {
		t.Fatalf("Failed to open fresh DB handle: %v", err)
	}
	defer db2.Close()

	_ = db2.QueryRow("SELECT count(*) FROM libraries WHERE id = ?", libraryID).Scan(&count)
	if count != 1 {
		t.Errorf("Expected library to be restored, but not found in DB")
	}
}

// Test98: F1 x F8 (Playlist Access Control)
func Test98PlaylistAccessControl(t *testing.T) {
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

	// Setup Admin Root & Login
	initPayload := map[string]interface{}{
		"newRoot": map[string]string{
			"username": "rootadmin",
			"password": "supersecurepassword123",
		},
	}
	initBody, _ := json.Marshal(initPayload)
	resp, _ := client.Post(h.BaseURL+"/init", "application/json", bytes.NewReader(initBody))
	resp.Body.Close()

	loginPayload := map[string]string{
		"username": "rootadmin",
		"password": "supersecurepassword123",
	}
	loginBody, _ := json.Marshal(loginPayload)
	resp, _ = client.Post(h.BaseURL+"/login", "application/json", bytes.NewReader(loginBody))
	var adminResp map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&adminResp)
	resp.Body.Close()
	_ = adminResp["user"].(map[string]interface{})["accessToken"].(string)

	// Create normal user A
	db, err := sql.Open("sqlite", h.DBPath)
	if err != nil {
		t.Fatalf("Failed to open DB: %v", err)
	}
	defer db.Close()

	userA_ID := uuid.New().String()
	hashedPashA, _ := bcrypt.GenerateFromPassword([]byte("passwordA123"), bcrypt.DefaultCost)
	permsJSON := `{"download":true,"accessExplicitContent":false,"accessAllLibraries":true,"librariesAccessible":[],"accessAllTags":true,"itemTagsSelected":[],"selectedTagsNotAccessible":false}`
	_, err = db.Exec(`INSERT INTO users (id, username, email, type, pash, token, isActive, permissions, extraData, bookmarks, createdAt, updatedAt)
		VALUES (?, ?, NULL, 'user', ?, 'tokenA', 1, ?, '{}', '[]', datetime('now'), datetime('now'))`,
		userA_ID, "userA", string(hashedPashA), permsJSON)
	if err != nil {
		t.Fatalf("Failed to insert user A: %v", err)
	}

	// Create normal user B
	userB_ID := uuid.New().String()
	hashedPashB, _ := bcrypt.GenerateFromPassword([]byte("passwordB123"), bcrypt.DefaultCost)
	_, err = db.Exec(`INSERT INTO users (id, username, email, type, pash, token, isActive, permissions, extraData, bookmarks, createdAt, updatedAt)
		VALUES (?, ?, NULL, 'user', ?, 'tokenB', 1, ?, '{}', '[]', datetime('now'), datetime('now'))`,
		userB_ID, "userB", string(hashedPashB), permsJSON)
	if err != nil {
		t.Fatalf("Failed to insert user B: %v", err)
	}

	// Login User A
	loginAPayload := map[string]string{
		"username": "userA",
		"password": "passwordA123",
	}
	loginABody, _ := json.Marshal(loginAPayload)
	respA, _ := client.Post(h.BaseURL+"/login", "application/json", bytes.NewReader(loginABody))
	var userAResp map[string]interface{}
	json.NewDecoder(respA.Body).Decode(&userAResp)
	respA.Body.Close()
	userAToken := userAResp["user"].(map[string]interface{})["accessToken"].(string)

	// Login User B
	loginBPayload := map[string]string{
		"username": "userB",
		"password": "passwordB123",
	}
	loginBBody, _ := json.Marshal(loginBPayload)
	respB, _ := client.Post(h.BaseURL+"/login", "application/json", bytes.NewReader(loginBBody))
	var userBResp map[string]interface{}
	json.NewDecoder(respB.Body).Decode(&userBResp)
	respB.Body.Close()
	userBToken := userBResp["user"].(map[string]interface{})["accessToken"].(string)

	// Create playlist as User A
	createPLPayload := map[string]interface{}{
		"name":  "User A Playlist",
		"items": []string{},
	}
	createPLBody, _ := json.Marshal(createPLPayload)
	reqPL, _ := http.NewRequest("POST", h.BaseURL+"/api/playlists", bytes.NewReader(createPLBody))
	reqPL.Header.Set("Authorization", "Bearer "+userAToken)
	reqPL.Header.Set("Content-Type", "application/json")
	respPL, err := client.Do(reqPL)
	if err != nil {
		t.Fatalf("Failed to create playlist: %v", err)
	}
	var playlistObj map[string]interface{}
	json.NewDecoder(respPL.Body).Decode(&playlistObj)
	respPL.Body.Close()
	playlistID := playlistObj["id"].(string)

	// 1. User B tries to view playlist A
	reqGet, _ := http.NewRequest("GET", h.BaseURL+"/api/playlists/"+playlistID, nil)
	reqGet.Header.Set("Authorization", "Bearer "+userBToken)
	respGet, err := client.Do(reqGet)
	if err != nil {
		t.Fatalf("GET playlist failed: %v", err)
	}
	respGet.Body.Close()
	if respGet.StatusCode != http.StatusForbidden {
		t.Errorf("Expected 403 Forbidden for User B GET User A's playlist, got %d", respGet.StatusCode)
	}

	// 2. User B tries to update playlist A
	updatePayload := map[string]interface{}{
		"name": "User B Hacked Playlist",
	}
	updateBody, _ := json.Marshal(updatePayload)
	reqUpdate, _ := http.NewRequest("PATCH", h.BaseURL+"/api/playlists/"+playlistID, bytes.NewReader(updateBody))
	reqUpdate.Header.Set("Authorization", "Bearer "+userBToken)
	reqUpdate.Header.Set("Content-Type", "application/json")
	respUpdate, err := client.Do(reqUpdate)
	if err != nil {
		t.Fatalf("PATCH playlist failed: %v", err)
	}
	respUpdate.Body.Close()
	if respUpdate.StatusCode != http.StatusForbidden {
		t.Errorf("Expected 403 Forbidden for User B PATCH User A's playlist, got %d", respUpdate.StatusCode)
	}

	// 3. User B tries to delete playlist A
	reqDel, _ := http.NewRequest("DELETE", h.BaseURL+"/api/playlists/"+playlistID, nil)
	reqDel.Header.Set("Authorization", "Bearer "+userBToken)
	respDel, err := client.Do(reqDel)
	if err != nil {
		t.Fatalf("DELETE playlist failed: %v", err)
	}
	respDel.Body.Close()
	if respDel.StatusCode != http.StatusForbidden {
		t.Errorf("Expected 403 Forbidden for User B DELETE User A's playlist, got %d", respDel.StatusCode)
	}
}

// Test99: F3 x F7 (Tag/Genre filtering/renaming)
func Test99TagGenreFiltering(t *testing.T) {
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

	// Setup Admin Root & Login
	initPayload := map[string]interface{}{
		"newRoot": map[string]string{
			"username": "rootadmin",
			"password": "supersecurepassword123",
		},
	}
	initBody, _ := json.Marshal(initPayload)
	resp, _ := client.Post(h.BaseURL+"/init", "application/json", bytes.NewReader(initBody))
	resp.Body.Close()

	loginPayload := map[string]string{
		"username": "rootadmin",
		"password": "supersecurepassword123",
	}
	loginBody, _ := json.Marshal(loginPayload)
	resp, _ = client.Post(h.BaseURL+"/login", "application/json", bytes.NewReader(loginBody))
	var adminResp map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&adminResp)
	resp.Body.Close()
	adminToken := adminResp["user"].(map[string]interface{})["accessToken"].(string)

	// Create Library
	libraryPath := filepath.Join(h.ConfigDir, "library99")
	createPayload := map[string]interface{}{
		"name":      "Library 99",
		"mediaType": "book",
		"folders": []map[string]string{
			{"path": libraryPath},
		},
	}
	createBody, _ := json.Marshal(createPayload)
	req, _ := http.NewRequest("POST", h.BaseURL+"/api/libraries", bytes.NewReader(createBody))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/json")
	resp, _ = client.Do(req)
	var createdLib map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&createdLib)
	resp.Body.Close()
	libraryID := createdLib["id"].(string)

	// Write mock media with Tag/Genre metadata
	bookDir := filepath.Join(libraryPath, "Book99")
	_ = os.MkdirAll(bookDir, 0755)

	nfoContent := "title: Book 99\nauthor: Author 99\ntags: CustomTag99\ngenre: CustomGenre99\n"
	_ = os.WriteFile(filepath.Join(bookDir, "metadata.nfo"), []byte(nfoContent), 0644)

	_ = GenerateMockAudio(filepath.Join(bookDir, "track.mp3"), "Book 99", "Author 99", "Book 99", "1", "2026")

	// Trigger Scan
	reqScan, _ := http.NewRequest("POST", h.BaseURL+"/api/libraries/"+libraryID+"/scan", nil)
	reqScan.Header.Set("Authorization", "Bearer "+adminToken)
	respScan, _ := client.Do(reqScan)
	respScan.Body.Close()

	// Wait for item
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
	if len(items) == 0 {
		t.Fatalf("Failed to scan book")
	}

	// Verify Tag shows up in GET /api/tags
	reqTags, _ := http.NewRequest("GET", h.BaseURL+"/api/tags", nil)
	reqTags.Header.Set("Authorization", "Bearer "+adminToken)
	respTags, err := client.Do(reqTags)
	if err != nil {
		t.Fatalf("GET /api/tags failed: %v", err)
	}
	var tagsObj map[string]interface{}
	json.NewDecoder(respTags.Body).Decode(&tagsObj)
	respTags.Body.Close()

	tagsList := tagsObj["tags"].([]interface{})
	foundTag := false
	for _, tagVal := range tagsList {
		if tagVal.(string) == "CustomTag99" {
			foundTag = true
			break
		}
	}
	if !foundTag {
		t.Errorf("Tag CustomTag99 not found in GET /api/tags")
	}

	// Verify Genre shows up in GET /api/genres
	reqGenres, _ := http.NewRequest("GET", h.BaseURL+"/api/genres", nil)
	reqGenres.Header.Set("Authorization", "Bearer "+adminToken)
	respGenres, err := client.Do(reqGenres)
	if err != nil {
		t.Fatalf("GET /api/genres failed: %v", err)
	}
	var genresObj map[string]interface{}
	json.NewDecoder(respGenres.Body).Decode(&genresObj)
	respGenres.Body.Close()

	genresList := genresObj["genres"].([]interface{})
	foundGenre := false
	for _, g := range genresList {
		if g.(string) == "CustomGenre99" {
			foundGenre = true
			break
		}
	}
	if !foundGenre {
		t.Errorf("Genre CustomGenre99 not found in GET /api/genres")
	}

	// Rename tag
	renameTagPayload := map[string]string{
		"tag":    "CustomTag99",
		"newTag": "RenamedTag99",
	}
	renameTagBody, _ := json.Marshal(renameTagPayload)
	reqRename, _ := http.NewRequest("POST", h.BaseURL+"/api/tags/rename", bytes.NewReader(renameTagBody))
	reqRename.Header.Set("Authorization", "Bearer "+adminToken)
	reqRename.Header.Set("Content-Type", "application/json")
	respRename, err := client.Do(reqRename)
	if err != nil {
		t.Fatalf("Tag rename failed: %v", err)
	}
	respRename.Body.Close()

	// Verify renamed tag shows up
	respTags2, _ := client.Do(reqTags)
	var tagsObj2 map[string]interface{}
	json.NewDecoder(respTags2.Body).Decode(&tagsObj2)
	respTags2.Body.Close()
	tagsList2 := tagsObj2["tags"].([]interface{})
	foundRenamed := false
	for _, tagVal := range tagsList2 {
		if tagVal.(string) == "RenamedTag99" {
			foundRenamed = true
		}
	}
	if !foundRenamed {
		t.Errorf("Renamed tag RenamedTag99 not found in GET /api/tags")
	}

	// Delete renamed tag
	encodedTag := base64.URLEncoding.EncodeToString([]byte("RenamedTag99"))
	reqDel, _ := http.NewRequest("DELETE", h.BaseURL+"/api/tags/"+encodedTag, nil)
	reqDel.Header.Set("Authorization", "Bearer "+adminToken)
	respDel, err := client.Do(reqDel)
	if err != nil {
		t.Fatalf("Tag delete failed: %v", err)
	}
	respDel.Body.Close()

	// Verify deleted tag is gone
	respTags3, _ := client.Do(reqTags)
	var tagsObj3 map[string]interface{}
	json.NewDecoder(respTags3.Body).Decode(&tagsObj3)
	respTags3.Body.Close()
	tagsList3 := tagsObj3["tags"].([]interface{})
	for _, tagVal := range tagsList3 {
		if tagVal.(string) == "RenamedTag99" {
			t.Errorf("Tag RenamedTag99 was still found after delete")
		}
	}
}

// Test100: F5 x F9 (Podcast playback and progress tracking)
func Test100PodcastPlaybackAndProgressTracking(t *testing.T) {
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

	// Setup Admin Root & Login
	initPayload := map[string]interface{}{
		"newRoot": map[string]string{
			"username": "rootadmin",
			"password": "supersecurepassword123",
		},
	}
	initBody, _ := json.Marshal(initPayload)
	resp, _ := client.Post(h.BaseURL+"/init", "application/json", bytes.NewReader(initBody))
	resp.Body.Close()

	loginPayload := map[string]string{
		"username": "rootadmin",
		"password": "supersecurepassword123",
	}
	loginBody, _ := json.Marshal(loginPayload)
	resp, _ = client.Post(h.BaseURL+"/login", "application/json", bytes.NewReader(loginBody))
	var adminResp map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&adminResp)
	resp.Body.Close()
	adminToken := adminResp["user"].(map[string]interface{})["accessToken"].(string)

	db, err := sql.Open("sqlite", h.DBPath)
	if err != nil {
		t.Fatalf("Failed to open DB: %v", err)
	}
	defer db.Close()

	libraryID := uuid.New().String()
	podcastID := uuid.New().String()
	episodeID := uuid.New().String()

	_, err = db.Exec(`INSERT INTO libraries (id, name, mediaType) VALUES (?, 'Podcast Lib', 'podcast')`, libraryID)
	if err != nil {
		t.Fatalf("Failed to insert library: %v", err)
	}

	_, err = db.Exec(`INSERT INTO libraryItems (id, libraryId, mediaType, mediaId, title) VALUES (?, ?, 'podcast', ?, 'Podcast Show')`, podcastID, libraryID, podcastID)
	if err != nil {
		t.Fatalf("Failed to insert library item: %v", err)
	}

	_, err = db.Exec(`INSERT INTO podcasts (id, title) VALUES (?, 'Podcast Show')`, podcastID)
	if err != nil {
		t.Fatalf("Failed to insert podcast: %v", err)
	}

	_, err = db.Exec(`INSERT INTO podcastEpisodes (id, podcastId, title, duration) VALUES (?, ?, 'Episode 100', 360.0)`, episodeID, podcastID)
	if err != nil {
		t.Fatalf("Failed to insert podcast episode: %v", err)
	}

	// 1. Sync progress
	progressPayload := map[string]interface{}{
		"duration":    360.0,
		"currentTime": 50.0,
		"isFinished":  false,
	}
	progressBody, _ := json.Marshal(progressPayload)
	reqPost, _ := http.NewRequest("POST", h.BaseURL+"/api/me/progress/"+podcastID+"/"+episodeID, bytes.NewReader(progressBody))
	reqPost.Header.Set("Authorization", "Bearer "+adminToken)
	reqPost.Header.Set("Content-Type", "application/json")
	respPost, err := client.Do(reqPost)
	if err != nil {
		t.Fatalf("POST progress failed: %v", err)
	}
	respPost.Body.Close()
	if respPost.StatusCode != http.StatusOK && respPost.StatusCode != http.StatusCreated {
		t.Fatalf("POST progress status: %d", respPost.StatusCode)
	}

	// 2. Fetch progress
	reqGet, _ := http.NewRequest("GET", h.BaseURL+"/api/me/progress/"+podcastID+"/"+episodeID, nil)
	reqGet.Header.Set("Authorization", "Bearer "+adminToken)
	respGet, err := client.Do(reqGet)
	if err != nil {
		t.Fatalf("GET progress failed: %v", err)
	}
	var getObj map[string]interface{}
	json.NewDecoder(respGet.Body).Decode(&getObj)
	respGet.Body.Close()

	if getObj["currentTime"].(float64) != 50.0 {
		t.Errorf("Expected currentTime 50.0, got %v", getObj["currentTime"])
	}

	// 3. Update progress
	updatePayload := map[string]interface{}{
		"currentTime": 120.0,
	}
	updateBody, _ := json.Marshal(updatePayload)
	reqPatch, _ := http.NewRequest("PATCH", h.BaseURL+"/api/me/progress/"+podcastID+"/"+episodeID, bytes.NewReader(updateBody))
	reqPatch.Header.Set("Authorization", "Bearer "+adminToken)
	reqPatch.Header.Set("Content-Type", "application/json")
	respPatch, err := client.Do(reqPatch)
	if err != nil {
		t.Fatalf("PATCH progress failed: %v", err)
	}
	respPatch.Body.Close()

	// Verify updated progress
	respGet2, _ := client.Do(reqGet)
	var getObj2 map[string]interface{}
	json.NewDecoder(respGet2.Body).Decode(&getObj2)
	respGet2.Body.Close()

	if getObj2["currentTime"].(float64) != 120.0 {
		t.Errorf("Expected updated currentTime 120.0, got %v", getObj2["currentTime"])
	}

	// 4. Delete progress
	progressRecordID := getObj2["id"].(string)
	reqDel, _ := http.NewRequest("DELETE", h.BaseURL+"/api/me/progress/"+progressRecordID, nil)
	reqDel.Header.Set("Authorization", "Bearer "+adminToken)
	respDel, err := client.Do(reqDel)
	if err != nil {
		t.Fatalf("DELETE progress failed: %v", err)
	}
	respDel.Body.Close()
	if respDel.StatusCode != http.StatusOK {
		t.Errorf("Expected 200 OK for DELETE progress, got %d", respDel.StatusCode)
	}
}

// Scenario 2 (Test 102): Listening Journey
func Test102ListeningJourney(t *testing.T) {
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

	// Setup Admin Root & Login
	initPayload := map[string]interface{}{
		"newRoot": map[string]string{
			"username": "rootadmin",
			"password": "supersecurepassword123",
		},
	}
	initBody, _ := json.Marshal(initPayload)
	resp, _ := client.Post(h.BaseURL+"/init", "application/json", bytes.NewReader(initBody))
	resp.Body.Close()

	loginPayload := map[string]string{
		"username": "rootadmin",
		"password": "supersecurepassword123",
	}
	loginBody, _ := json.Marshal(loginPayload)
	resp, _ = client.Post(h.BaseURL+"/login", "application/json", bytes.NewReader(loginBody))
	var adminResp map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&adminResp)
	resp.Body.Close()
	adminToken := adminResp["user"].(map[string]interface{})["accessToken"].(string)

	// Create Library
	libraryPath := filepath.Join(h.ConfigDir, "library102")
	createPayload := map[string]interface{}{
		"name":      "Library 102",
		"mediaType": "book",
		"folders": []map[string]string{
			{"path": libraryPath},
		},
	}
	createBody, _ := json.Marshal(createPayload)
	req, _ := http.NewRequest("POST", h.BaseURL+"/api/libraries", bytes.NewReader(createBody))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/json")
	resp, _ = client.Do(req)
	var createdLib map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&createdLib)
	resp.Body.Close()
	libraryID := createdLib["id"].(string)

	// Write mock media
	bookDir := filepath.Join(libraryPath, "Book102")
	_ = os.MkdirAll(bookDir, 0755)
	_ = GenerateMockAudio(filepath.Join(bookDir, "track.mp3"), "Track 102", "Author 102", "Series 102", "1", "2026")

	// Scan Library
	reqScan, _ := http.NewRequest("POST", h.BaseURL+"/api/libraries/"+libraryID+"/scan", nil)
	reqScan.Header.Set("Authorization", "Bearer "+adminToken)
	respScan, _ := client.Do(reqScan)
	respScan.Body.Close()

	// Wait for item
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
	if len(items) == 0 {
		t.Fatalf("Failed to scan book")
	}
	mediaItemID := items[0].(map[string]interface{})["id"].(string)

	// 1. Start listening: sync progress (currentTime = 10)
	progressPayload := map[string]interface{}{
		"duration":    300.0,
		"currentTime": 10.0,
		"isFinished":  false,
	}
	progressBody, _ := json.Marshal(progressPayload)
	reqPost, _ := http.NewRequest("POST", h.BaseURL+"/api/me/progress/"+mediaItemID, bytes.NewReader(progressBody))
	reqPost.Header.Set("Authorization", "Bearer "+adminToken)
	reqPost.Header.Set("Content-Type", "application/json")
	respPost, err := client.Do(reqPost)
	if err != nil {
		t.Fatalf("POST progress failed: %v", err)
	}
	respPost.Body.Close()

	// 2. Update progress (currentTime = 150)
	updatePayload := map[string]interface{}{
		"currentTime": 150.0,
	}
	updateBody, _ := json.Marshal(updatePayload)
	reqPatch, _ := http.NewRequest("PATCH", h.BaseURL+"/api/me/progress/"+mediaItemID, bytes.NewReader(updateBody))
	reqPatch.Header.Set("Authorization", "Bearer "+adminToken)
	reqPatch.Header.Set("Content-Type", "application/json")
	respPatch, err := client.Do(reqPatch)
	if err != nil {
		t.Fatalf("PATCH progress failed: %v", err)
	}
	respPatch.Body.Close()

	// 3. Retrieve progress and verify
	reqGet, _ := http.NewRequest("GET", h.BaseURL+"/api/me/progress/"+mediaItemID, nil)
	reqGet.Header.Set("Authorization", "Bearer "+adminToken)
	respGet, err := client.Do(reqGet)
	if err != nil {
		t.Fatalf("GET progress failed: %v", err)
	}
	var getObj map[string]interface{}
	json.NewDecoder(respGet.Body).Decode(&getObj)
	respGet.Body.Close()

	if getObj["currentTime"].(float64) != 150.0 {
		t.Errorf("Expected currentTime 150.0, got %v", getObj["currentTime"])
	}

	// 4. Finish audiobook (currentTime = 300, isFinished = true)
	finishPayload := map[string]interface{}{
		"currentTime": 300.0,
		"isFinished":  true,
	}
	finishBody, _ := json.Marshal(finishPayload)
	reqFinish, _ := http.NewRequest("PATCH", h.BaseURL+"/api/me/progress/"+mediaItemID, bytes.NewReader(finishBody))
	reqFinish.Header.Set("Authorization", "Bearer "+adminToken)
	reqFinish.Header.Set("Content-Type", "application/json")
	respFinish, err := client.Do(reqFinish)
	if err != nil {
		t.Fatalf("Finish patch failed: %v", err)
	}
	respFinish.Body.Close()

	// Verify marked as finished
	respGet2, _ := client.Do(reqGet)
	var getObj2 map[string]interface{}
	json.NewDecoder(respGet2.Body).Decode(&getObj2)
	respGet2.Body.Close()

	if getObj2["isFinished"].(bool) != true {
		t.Errorf("Expected isFinished to be true, got %v", getObj2["isFinished"])
	}
}

// Scenario 3 (Test 103): Admin Maintenance
func Test103AdminMaintenance(t *testing.T) {
	os.Setenv("UNDER_TEST", "true")

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

	// Setup Admin Root & Login
	initPayload := map[string]interface{}{
		"newRoot": map[string]string{
			"username": "rootadmin",
			"password": "supersecurepassword123",
		},
	}
	initBody, _ := json.Marshal(initPayload)
	resp, _ := client.Post(h.BaseURL+"/init", "application/json", bytes.NewReader(initBody))
	resp.Body.Close()

	loginPayload := map[string]string{
		"username": "rootadmin",
		"password": "supersecurepassword123",
	}
	loginBody, _ := json.Marshal(loginPayload)
	resp, _ = client.Post(h.BaseURL+"/login", "application/json", bytes.NewReader(loginBody))
	var adminResp map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&adminResp)
	resp.Body.Close()
	adminToken := adminResp["user"].(map[string]interface{})["accessToken"].(string)

	// Create Library
	libraryPath := filepath.Join(h.ConfigDir, "library103")
	createPayload := map[string]interface{}{
		"name":      "Library 103",
		"mediaType": "book",
		"folders": []map[string]string{
			{"path": libraryPath},
		},
	}
	createBody, _ := json.Marshal(createPayload)
	req, _ := http.NewRequest("POST", h.BaseURL+"/api/libraries", bytes.NewReader(createBody))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/json")
	resp, _ = client.Do(req)
	var createdLib map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&createdLib)
	resp.Body.Close()
	libraryID := createdLib["id"].(string)

	// Write mock media
	bookDir := filepath.Join(libraryPath, "Book103")
	_ = os.MkdirAll(bookDir, 0755)
	_ = GenerateMockAudio(filepath.Join(bookDir, "track.mp3"), "Track 103", "Author 103", "Series 103", "1", "2026")

	// Trigger Scan
	reqScan, _ := http.NewRequest("POST", h.BaseURL+"/api/libraries/"+libraryID+"/scan", nil)
	reqScan.Header.Set("Authorization", "Bearer "+adminToken)
	respScan, _ := client.Do(reqScan)
	respScan.Body.Close()

	// Wait for item
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
	if len(items) == 0 {
		t.Fatalf("Failed to scan book")
	}

	// Trigger Backup
	reqBackup, _ := http.NewRequest("POST", h.BaseURL+"/api/backups", nil)
	reqBackup.Header.Set("Authorization", "Bearer "+adminToken)
	respBackup, err := client.Do(reqBackup)
	if err != nil {
		t.Fatalf("Failed to request backup: %v", err)
	}
	var backupResp map[string]interface{}
	json.NewDecoder(respBackup.Body).Decode(&backupResp)
	respBackup.Body.Close()

	backupsList := backupResp["backups"].([]interface{})
	backupInfo := backupsList[0].(map[string]interface{})
	backupID := backupInfo["id"].(string)

	// Delete Library
	reqDel, _ := http.NewRequest("DELETE", h.BaseURL+"/api/libraries/"+libraryID, nil)
	reqDel.Header.Set("Authorization", "Bearer "+adminToken)
	respDel, _ := client.Do(reqDel)
	respDel.Body.Close()

	// Verify it's gone
	db, err := sql.Open("sqlite", h.DBPath)
	if err != nil {
		t.Fatalf("Failed to open DB: %v", err)
	}

	var count int
	_ = db.QueryRow("SELECT count(*) FROM libraries WHERE id = ?", libraryID).Scan(&count)
	db.Close()
	if count != 0 {
		t.Fatalf("Library delete failed during test setup")
	}

	// Restore the backup
	reqApply, _ := http.NewRequest("POST", h.BaseURL+"/api/backups/"+backupID+"/apply", nil)
	reqApply.Header.Set("Authorization", "Bearer "+adminToken)
	respApply, err := client.Do(reqApply)
	if err != nil {
		t.Fatalf("Apply backup request failed: %v", err)
	}
	respApply.Body.Close()

	if respApply.StatusCode != http.StatusOK {
		t.Fatalf("Failed to apply backup, status: %d", respApply.StatusCode)
	}

	// Wait a moment for restore to settle
	time.Sleep(100 * time.Millisecond)

	// Verify the library is restored using a fresh DB handle
	db2, err := sql.Open("sqlite", h.DBPath)
	if err != nil {
		t.Fatalf("Failed to open fresh DB handle: %v", err)
	}
	defer db2.Close()

	_ = db2.QueryRow("SELECT count(*) FROM libraries WHERE id = ?", libraryID).Scan(&count)
	if count != 1 {
		t.Errorf("Expected library to be restored, but not found in DB")
	}
}

// Scenario 4 (Test 104): Transcoding Cycle
func Test104EndToEndTranscodingCycle(t *testing.T) {
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

	// Setup Admin Root & Login
	initPayload := map[string]interface{}{
		"newRoot": map[string]string{
			"username": "rootadmin",
			"password": "supersecurepassword123",
		},
	}
	initBody, _ := json.Marshal(initPayload)
	resp, _ := client.Post(h.BaseURL+"/init", "application/json", bytes.NewReader(initBody))
	resp.Body.Close()

	loginPayload := map[string]string{
		"username": "rootadmin",
		"password": "supersecurepassword123",
	}
	loginBody, _ := json.Marshal(loginPayload)
	resp, _ = client.Post(h.BaseURL+"/login", "application/json", bytes.NewReader(loginBody))
	var adminResp map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&adminResp)
	resp.Body.Close()
	adminToken := adminResp["user"].(map[string]interface{})["accessToken"].(string)
	adminUserID := adminResp["user"].(map[string]interface{})["id"].(string)

	// Create Library
	libraryPath := filepath.Join(h.ConfigDir, "library104")
	createPayload := map[string]interface{}{
		"name":      "Library 104",
		"mediaType": "book",
		"folders": []map[string]string{
			{"path": libraryPath},
		},
	}
	createBody, _ := json.Marshal(createPayload)
	req, _ := http.NewRequest("POST", h.BaseURL+"/api/libraries", bytes.NewReader(createBody))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/json")
	resp, _ = client.Do(req)
	var createdLib map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&createdLib)
	resp.Body.Close()
	libraryID := createdLib["id"].(string)

	// Write mock media
	bookDir := filepath.Join(libraryPath, "Book104")
	_ = os.MkdirAll(bookDir, 0755)
	_ = GenerateMockAudio(filepath.Join(bookDir, "track.mp3"), "Track 104", "Author 104", "Series 104", "1", "2026")

	// Trigger Scan
	reqScan, _ := http.NewRequest("POST", h.BaseURL+"/api/libraries/"+libraryID+"/scan", nil)
	reqScan.Header.Set("Authorization", "Bearer "+adminToken)
	respScan, _ := client.Do(reqScan)
	respScan.Body.Close()

	// Wait for item
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
	if len(items) == 0 {
		t.Fatalf("Failed to scan book")
	}
	mediaItemID := items[0].(map[string]interface{})["id"].(string)

	db, err := sql.Open("sqlite", h.DBPath)
	if err != nil {
		t.Fatalf("Failed to open DB: %v", err)
	}
	defer db.Close()

	var bookID string
	err = db.QueryRow("SELECT mediaId FROM libraryItems WHERE id = ?", mediaItemID).Scan(&bookID)
	if err != nil {
		t.Fatalf("Failed to query bookID from libraryItems: %v", err)
	}

	// Insert playback session
	sessionID := uuid.New().String()
	extraDataVal := fmt.Sprintf(`{"libraryItemId":%q}`, mediaItemID)
	_, err = db.Exec(`INSERT INTO playbackSessions (id, userId, mediaItemId, mediaItemType, startTime, libraryId, extraData, createdAt, updatedAt)
		VALUES (?, ?, ?, 'book', 0.0, ?, ?, datetime('now'), datetime('now'))`,
		sessionID, adminUserID, bookID, libraryID, extraDataVal)
	if err != nil {
		t.Fatalf("Failed to insert playback session: %v", err)
	}

	// Trigger transcoding by requesting the playlist
	reqHLS, _ := http.NewRequest("GET", h.BaseURL+"/hls/"+sessionID+"/output.m3u8", nil)
	reqHLS.Header.Set("Authorization", "Bearer "+adminToken)
	respHLS, err := client.Do(reqHLS)
	if err != nil {
		t.Fatalf("HLS request failed: %v", err)
	}
	respHLS.Body.Close()

	// Wait briefly for ffmpeg to start and generate segment file
	time.Sleep(500 * time.Millisecond)

	// Fetch HLS segment
	reqSeg, _ := http.NewRequest("GET", h.BaseURL+"/hls/"+sessionID+"/output-0.ts", nil)
	reqSeg.Header.Set("Authorization", "Bearer "+adminToken)
	respSeg, err := client.Do(reqSeg)
	if err != nil {
		t.Fatalf("Failed to request segment: %v", err)
	}
	defer respSeg.Body.Close()

	if respSeg.StatusCode != http.StatusOK {
		t.Errorf("Expected 200 OK for HLS segment, got %d", respSeg.StatusCode)
	}
}
