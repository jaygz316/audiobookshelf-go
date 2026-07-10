package e2e

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"testing"

	_ "modernc.org/sqlite"
)

func TestF18ReverseProxy(t *testing.T) {
	h := NewTestHarness()
	if err := h.Start(); err != nil {
		t.Fatalf("Failed to start harness: %v", err)
	}
	defer h.Stop()

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("Failed to create cookie jar: %v", err)
	}
	client := &http.Client{
		Jar: jar,
	}

	// 1. Initialize root user
	t.Run("POST /init", func(t *testing.T) {
		payload := map[string]interface{}{
			"newRoot": map[string]string{
				"username": "rootadmin",
				"password": "supersecurepassword123",
			},
		}
		body, _ := json.Marshal(payload)

		resp, err := client.Post(h.BaseURL+"/init", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("Failed to send POST /init: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}
	})

	// 2. Login with X-Forwarded-For header
	t.Run("POST /login with X-Forwarded-For", func(t *testing.T) {
		payload := map[string]string{
			"username": "rootadmin",
			"password": "supersecurepassword123",
		}
		body, _ := json.Marshal(payload)

		req, err := http.NewRequest("POST", h.BaseURL+"/login", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Forwarded-For", "203.0.113.195, 70.41.3.18")

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Failed to perform request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}

		// Read database to verify correct IP was recorded in the sessions table
		db, err := sql.Open("sqlite", h.DBPath)
		if err != nil {
			t.Fatalf("Failed to open SQLite db: %v", err)
		}
		defer db.Close()

		var recordedIP string
		err = db.QueryRow("SELECT ipAddress FROM sessions ORDER BY createdAt DESC LIMIT 1").Scan(&recordedIP)
		if err != nil {
			t.Fatalf("Failed to query IP from sessions: %v", err)
		}

		if recordedIP != "203.0.113.195" {
			t.Errorf("Expected recorded IP to be '203.0.113.195', got %q", recordedIP)
		}
	})

	// 3. Login with X-Real-IP header
	t.Run("POST /login with X-Real-IP", func(t *testing.T) {
		payload := map[string]string{
			"username": "rootadmin",
			"password": "supersecurepassword123",
		}
		body, _ := json.Marshal(payload)

		req, err := http.NewRequest("POST", h.BaseURL+"/login", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Real-IP", "198.51.100.1")

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Failed to perform request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}

		db, err := sql.Open("sqlite", h.DBPath)
		if err != nil {
			t.Fatalf("Failed to open SQLite db: %v", err)
		}
		defer db.Close()

		var recordedIP string
		err = db.QueryRow("SELECT ipAddress FROM sessions ORDER BY createdAt DESC LIMIT 1").Scan(&recordedIP)
		if err != nil {
			t.Fatalf("Failed to query IP from sessions: %v", err)
		}

		if recordedIP != "198.51.100.1" {
			t.Errorf("Expected recorded IP to be '198.51.100.1', got %q", recordedIP)
		}
	})
}
