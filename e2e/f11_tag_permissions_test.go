package e2e

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite"
)

func TestF11TagPermissions(t *testing.T) {
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

	// 1. Setup Admin Root & login
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

	// 2. Open DB to insert books, library items, and restricted user
	db, err := sql.Open("sqlite", h.DBPath)
	if err != nil {
		t.Fatalf("Failed to open DB: %v", err)
	}
	defer db.Close()

	// Seed books
	_, err = db.Exec(`INSERT INTO books (id, title, tags, explicit) VALUES (?, ?, ?, 0)`,
		"book_1", "Fiction Book", `["Fiction"]`)
	if err != nil {
		t.Fatalf("Failed to insert book_1: %v", err)
	}
	_, err = db.Exec(`INSERT INTO books (id, title, tags, explicit) VALUES (?, ?, ?, 0)`,
		"book_2", "Horror Book", `["Horror"]`)
	if err != nil {
		t.Fatalf("Failed to insert book_2: %v", err)
	}
	_, err = db.Exec(`INSERT INTO books (id, title, tags, explicit) VALUES (?, ?, ?, 0)`,
		"book_3", "History Book", `["History"]`)
	if err != nil {
		t.Fatalf("Failed to insert book_3: %v", err)
	}

	// Seed libraryItems
	_, err = db.Exec(`INSERT INTO libraryItems (id, libraryId, mediaType, mediaId, title) VALUES (?, ?, 'book', ?, ?)`,
		"li_1", libA_ID, "book_1", "Fiction Book")
	if err != nil {
		t.Fatalf("Failed to insert li_1: %v", err)
	}
	_, err = db.Exec(`INSERT INTO libraryItems (id, libraryId, mediaType, mediaId, title) VALUES (?, ?, 'book', ?, ?)`,
		"li_2", libA_ID, "book_2", "Horror Book")
	if err != nil {
		t.Fatalf("Failed to insert li_2: %v", err)
	}
	_, err = db.Exec(`INSERT INTO libraryItems (id, libraryId, mediaType, mediaId, title) VALUES (?, ?, 'book', ?, ?)`,
		"li_3", libA_ID, "book_3", "History Book")
	if err != nil {
		t.Fatalf("Failed to insert li_3: %v", err)
	}

	// Setup normal user with: ALLOW only Fiction
	hashedPash, err := bcrypt.GenerateFromPassword([]byte("userpassword123"), 8)
	if err != nil {
		t.Fatalf("Failed to hash password: %v", err)
	}
	userID := uuid.New().String()
	permsJSON := `{"download":true,"accessExplicitContent":false,"accessAllLibraries":true,"librariesAccessible":[],"accessAllTags":false,"itemTagsSelected":["Fiction"],"selectedTagsNotAccessible":false}`
	_, err = db.Exec(`INSERT INTO users (id, username, email, type, pash, token, isActive, permissions, extraData, bookmarks, createdAt, updatedAt)
		VALUES (?, ?, ?, ?, ?, ?, 1, ?, '{}', '[]', datetime('now'), datetime('now'))`,
		userID, "restricteduser", "user@test.com", "user", string(hashedPash), "token-restricted", permsJSON)
	if err != nil {
		t.Fatalf("Failed to insert restricted user: %v", err)
	}

	// Close DB so app can write to it
	db.Close()

	// Login restricted user
	rLoginPayload := map[string]string{
		"username": "restricteduser",
		"password": "userpassword123",
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

	// Call GET items (should only return Fiction Book)
	reqGet, _ := http.NewRequest("GET", h.BaseURL+"/api/libraries/"+libA_ID+"/items", nil)
	reqGet.Header.Set("Authorization", "Bearer "+rToken)
	respGet, err := client.Do(reqGet)
	if err != nil {
		t.Fatalf("Failed to GET library items: %v", err)
	}
	var getResultA map[string]interface{}
	json.NewDecoder(respGet.Body).Decode(&getResultA)
	respGet.Body.Close()

	resultsA, _ := getResultA["results"].([]interface{})
	if len(resultsA) != 1 {
		t.Fatalf("Expected exactly 1 library item, got %d: %v", len(resultsA), getResultA)
	}
	mediaObjA, _ := resultsA[0].(map[string]interface{})["media"].(map[string]interface{})
	if mediaObjA["id"].(string) != "book_1" {
		t.Errorf("Expected book_1, got %v", resultsA[0])
	}

	// 3. Update permissions using PATCH /api/users/{id}
	// Let's set BLOCK list: Block Horror (so we should see Fiction and History)
	patchPayload := map[string]interface{}{
		"permissions": map[string]interface{}{
			"accessAllTags":             false,
			"selectedTagsNotAccessible": true,
		},
		"itemTagsSelected": []string{"Horror"},
	}
	patchBody, _ := json.Marshal(patchPayload)
	reqPatch, _ := http.NewRequest("PATCH", h.BaseURL+"/api/users/"+userID, bytes.NewReader(patchBody))
	reqPatch.Header.Set("Authorization", "Bearer "+adminToken)
	reqPatch.Header.Set("Content-Type", "application/json")
	respPatch, err := client.Do(reqPatch)
	if err != nil {
		t.Fatalf("Failed to PATCH user: %v", err)
	}
	if respPatch.StatusCode != http.StatusOK {
		t.Fatalf("Expected PATCH status 200, got %d", respPatch.StatusCode)
	}
	respPatch.Body.Close()

	// Call GET items again (should return book_1 and book_3, but NOT book_2)
	reqGet2, _ := http.NewRequest("GET", h.BaseURL+"/api/libraries/"+libA_ID+"/items", nil)
	reqGet2.Header.Set("Authorization", "Bearer "+rToken)
	respGet2, err := client.Do(reqGet2)
	if err != nil {
		t.Fatalf("Failed to GET library items: %v", err)
	}
	var getResultB map[string]interface{}
	json.NewDecoder(respGet2.Body).Decode(&getResultB)
	respGet2.Body.Close()

	resultsB, _ := getResultB["results"].([]interface{})
	if len(resultsB) != 2 {
		t.Fatalf("Expected exactly 2 library items, got %d: %v", len(resultsB), getResultB)
	}

	foundFiction := false
	foundHistory := false
	for _, item := range resultsB {
		mediaObjB, _ := item.(map[string]interface{})["media"].(map[string]interface{})
		mId := mediaObjB["id"].(string)
		if mId == "book_1" {
			foundFiction = true
		} else if mId == "book_3" {
			foundHistory = true
		}
	}
	if !foundFiction || !foundHistory {
		t.Errorf("Expected Fiction and History books, got results: %v", resultsB)
	}

	// 4. Update permissions: Clear tag restrictions (AccessAllTags = true, itemTagsSelected = [])
	patchPayload2 := map[string]interface{}{
		"permissions": map[string]interface{}{
			"accessAllTags":             true,
			"selectedTagsNotAccessible": false,
		},
		"itemTagsSelected": []string{},
	}
	patchBody2, _ := json.Marshal(patchPayload2)
	reqPatch2, _ := http.NewRequest("PATCH", h.BaseURL+"/api/users/"+userID, bytes.NewReader(patchBody2))
	reqPatch2.Header.Set("Authorization", "Bearer "+adminToken)
	reqPatch2.Header.Set("Content-Type", "application/json")
	respPatch2, err := client.Do(reqPatch2)
	if err != nil {
		t.Fatalf("Failed to PATCH user to clear restrictions: %v", err)
	}
	if respPatch2.StatusCode != http.StatusOK {
		t.Fatalf("Expected PATCH 200, got %d", respPatch2.StatusCode)
	}
	respPatch2.Body.Close()

	// Call GET items again (should return all 3 books)
	reqGet3, _ := http.NewRequest("GET", h.BaseURL+"/api/libraries/"+libA_ID+"/items", nil)
	reqGet3.Header.Set("Authorization", "Bearer "+rToken)
	respGet3, err := client.Do(reqGet3)
	if err != nil {
		t.Fatalf("Failed to GET library items: %v", err)
	}
	var getResultC map[string]interface{}
	json.NewDecoder(respGet3.Body).Decode(&getResultC)
	respGet3.Body.Close()

	resultsC, _ := getResultC["results"].([]interface{})
	if len(resultsC) != 3 {
		t.Errorf("Expected all 3 library items, got %d: %v", len(resultsC), getResultC)
	}
}
