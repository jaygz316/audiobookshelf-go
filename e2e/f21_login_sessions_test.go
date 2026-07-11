package e2e

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"testing"
)

func TestLoginSessionsE2E(t *testing.T) {
	h := NewTestHarness()
	if err := h.Start(); err != nil {
		t.Fatalf("Failed to start harness: %v", err)
	}
	defer h.Stop()

	// Client 1 setup (will log in, verify sessions, delete Client 2 session)
	jar1, _ := cookiejar.New(nil)
	client1 := &http.Client{Jar: jar1}
	adminToken1 := setupAdmin(t, h, client1)

	// Client 2 setup (will also log in, creating a second active session)
	jar2, _ := cookiejar.New(nil)
	client2 := &http.Client{Jar: jar2}
	loginPayload := map[string]string{
		"username": "rootadmin",
		"password": "supersecurepassword123",
	}
	loginBody, _ := json.Marshal(loginPayload)
	resp, err := client2.Post(h.BaseURL+"/login", "application/json", bytes.NewReader(loginBody))
	if err != nil {
		t.Fatalf("Client 2 failed to login: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Client 2 login returned status %d", resp.StatusCode)
	}

	var loginResp2 map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&loginResp2)
	_ = loginResp2["user"].(map[string]interface{})["accessToken"].(string)


	// 1. Get all active sessions using Client 1
	req, _ := http.NewRequest("GET", h.BaseURL+"/api/me/sessions", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken1)
	resp, err = client1.Do(req)
	if err != nil {
		t.Fatalf("Failed to fetch sessions for Client 1: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected GET /api/me/sessions to return 200, got %d", resp.StatusCode)
	}

	var sessions []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&sessions); err != nil {
		t.Fatalf("Failed to decode sessions: %v", err)
	}

	// There should be exactly 2 active sessions (Client 1 and Client 2)
	if len(sessions) != 2 {
		t.Errorf("Expected 2 active sessions, got %d", len(sessions))
	}

	// Verify that exactly one is current for Client 1
	var client2SessionID string
	var client1SessionID string
	currentCount := 0
	for _, s := range sessions {
		isCurrent := s["isCurrent"].(bool)
		id := s["id"].(string)
		if isCurrent {
			currentCount++
			client1SessionID = id
		} else {
			client2SessionID = id
		}
	}
	if currentCount != 1 {
		t.Errorf("Expected exactly 1 current session for Client 1, got %d", currentCount)
	}
	if client2SessionID == "" {
		t.Fatalf("Could not locate Client 2 session ID")
	}

	// 2. Revoke Client 2's session using Client 1
	req, _ = http.NewRequest("DELETE", h.BaseURL+"/api/me/sessions/"+client2SessionID, nil)
	req.Header.Set("Authorization", "Bearer "+adminToken1)
	resp, err = client1.Do(req)
	if err != nil {
		t.Fatalf("Failed to delete Client 2 session: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected DELETE /api/me/sessions/%s to return 200, got %d", client2SessionID, resp.StatusCode)
	}

	// 3. Verify on Client 1 that only 1 session remains (which is Client 1's session)
	req, _ = http.NewRequest("GET", h.BaseURL+"/api/me/sessions", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken1)
	resp, err = client1.Do(req)
	if err != nil {
		t.Fatalf("Failed to fetch sessions: %v", err)
	}
	defer resp.Body.Close()

	var sessionsAfter []map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&sessionsAfter)
	if len(sessionsAfter) != 1 {
		t.Errorf("Expected 1 active session after deletion, got %d", len(sessionsAfter))
	}
	if sessionsAfter[0]["id"].(string) != client1SessionID {
		t.Errorf("Expected remaining session to be Client 1 (%s), got %s", client1SessionID, sessionsAfter[0]["id"].(string))
	}

	// 4. Verify that Client 2 is now unauthorized on refresh
	req, _ = http.NewRequest("POST", h.BaseURL+"/auth/refresh", nil)
	// Refresh uses the refresh_token cookie stored in jar2, so it will present Client 2's revoked refresh token
	resp, err = client2.Do(req)
	if err != nil {
		t.Fatalf("Failed to request token refresh for Client 2: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		t.Errorf("Expected token refresh to fail (unauthorized) for revoked Client 2 session, but got 200 OK")
	} else {
		t.Logf("Token refresh for revoked session correctly failed with status %d", resp.StatusCode)
	}
}
