package e2e

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestF12ListeningHistory(t *testing.T) {
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

	// 2. Open DB and seed a book and library item
	db, err := sql.Open("sqlite", h.DBPath)
	if err != nil {
		t.Fatalf("Failed to open DB: %v", err)
	}
	defer db.Close()

	_, err = db.Exec(`INSERT INTO books (id, title) VALUES ('book_lh_1', 'The Listening History Guide')`)
	if err != nil {
		t.Fatalf("Failed to seed book: %v", err)
	}
	_, err = db.Exec(`INSERT INTO libraryItems (id, mediaId, mediaType, authorNamesFirstLast, title, libraryId) 
		VALUES ('item_lh_1', 'book_lh_1', 'book', 'Author LH', 'The Listening History Guide', 'lib-lh-1')`)
	if err != nil {
		t.Fatalf("Failed to seed libraryItems: %v", err)
	}

	// 3. Play item to create initial session
	playPayload := map[string]interface{}{
		"startTime": 0.0,
	}
	playBody, _ := json.Marshal(playPayload)
	reqPlay, _ := http.NewRequest("POST", h.BaseURL+"/api/items/item_lh_1/play", bytes.NewReader(playBody))
	reqPlay.Header.Set("Authorization", "Bearer "+adminToken)
	reqPlay.Header.Set("Content-Type", "application/json")
	respPlay, err := client.Do(reqPlay)
	if err != nil {
		t.Fatalf("Failed to call play API: %v", err)
	}
	var playResp map[string]interface{}
	json.NewDecoder(respPlay.Body).Decode(&playResp)
	respPlay.Body.Close()

	sessionID, ok := playResp["id"].(string)
	if !ok || sessionID == "" {
		t.Fatalf("Failed to get sessionID from play API response: %v", playResp)
	}

	// 4. Update progress incrementally
	// 4.1 First update: currentTime = 5.0 (delta = 5.0)
	progPayload1 := map[string]interface{}{
		"currentTime": 5.0,
		"duration":    1000.0,
		"isFinished":  false,
	}
	progBody1, _ := json.Marshal(progPayload1)
	reqProg1, _ := http.NewRequest("PATCH", h.BaseURL+"/api/me/progress/item_lh_1", bytes.NewReader(progBody1))
	reqProg1.Header.Set("Authorization", "Bearer "+adminToken)
	reqProg1.Header.Set("Content-Type", "application/json")
	respProg1, err := client.Do(reqProg1)
	if err != nil {
		t.Fatalf("Failed to patch progress 1: %v", err)
	}
	respProg1.Body.Close()
	if respProg1.StatusCode != http.StatusOK {
		t.Fatalf("Progress PATCH 1 returned status %d", respProg1.StatusCode)
	}

	// Wait a tiny bit to make sure updates process
	time.Sleep(100 * time.Millisecond)

	// 4.2 Second update: currentTime = 12.0 (delta = 7.0)
	progPayload2 := map[string]interface{}{
		"currentTime": 12.0,
		"duration":    1000.0,
		"isFinished":  false,
	}
	progBody2, _ := json.Marshal(progPayload2)
	reqProg2, _ := http.NewRequest("PATCH", h.BaseURL+"/api/me/progress/item_lh_1", bytes.NewReader(progBody2))
	reqProg2.Header.Set("Authorization", "Bearer "+adminToken)
	reqProg2.Header.Set("Content-Type", "application/json")
	respProg2, err := client.Do(reqProg2)
	if err != nil {
		t.Fatalf("Failed to patch progress 2: %v", err)
	}
	respProg2.Body.Close()

	// 5. Get Listening Stats
	reqStats, _ := http.NewRequest("GET", h.BaseURL+"/api/me/listening-stats", nil)
	reqStats.Header.Set("Authorization", "Bearer "+adminToken)
	respStats, err := client.Do(reqStats)
	if err != nil {
		t.Fatalf("Failed to get listening stats: %v", err)
	}
	var stats map[string]interface{}
	json.NewDecoder(respStats.Body).Decode(&stats)
	respStats.Body.Close()

	totalTime := stats["totalTime"].(float64)
	today := stats["today"].(float64)
	recentSessions := stats["recentSessions"].([]interface{})

	if totalTime < 5.0 || totalTime > 15.0 {
		t.Errorf("Expected totalTime to be approximately 12.0, got %f", totalTime)
	}
	if today < 5.0 || today > 15.0 {
		t.Errorf("Expected today listening time to be approximately 12.0, got %f", today)
	}
	if len(recentSessions) == 0 {
		t.Errorf("Expected recentSessions list to not be empty")
	}

	// 6. Get Listening Sessions (paginated)
	reqSessions, _ := http.NewRequest("GET", h.BaseURL+"/api/me/listening-sessions?page=0&itemsPerPage=5", nil)
	reqSessions.Header.Set("Authorization", "Bearer "+adminToken)
	respSessions, err := client.Do(reqSessions)
	if err != nil {
		t.Fatalf("Failed to get listening sessions: %v", err)
	}
	var sessionsResp map[string]interface{}
	json.NewDecoder(respSessions.Body).Decode(&sessionsResp)
	respSessions.Body.Close()

	totalSessions := int(sessionsResp["total"].(float64))
	sessionsList := sessionsResp["sessions"].([]interface{})

	if totalSessions != 1 {
		t.Errorf("Expected total sessions count to be 1, got %d", totalSessions)
	}
	if len(sessionsList) != 1 {
		t.Errorf("Expected 1 session in paginated response, got %d", len(sessionsList))
	}
}
