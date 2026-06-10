package e2e_tests

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func timeToDBStr(t time.Time) string {
	return t.UTC().Format("2006-01-02 15:04:05.000 +00:00")
}

func TestF1Authentication(t *testing.T) {
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

	var accessToken string
	reqURL, err := url.Parse(h.BaseURL)
	if err != nil {
		t.Fatalf("Failed to parse base URL: %v", err)
	}

	// 1. Initializing root user (Tier 1 & Tier 2: reject duplicate)
	t.Run("POST /init - success", func(t *testing.T) {
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
		respBody, _ := io.ReadAll(resp.Body)
		if string(respBody) != "OK" {
			t.Errorf("Expected response 'OK', got %q", string(respBody))
		}
	})

	t.Run("POST /init - reject duplicate root", func(t *testing.T) {
		payload := map[string]interface{}{
			"newRoot": map[string]string{
				"username": "anotherroot",
				"password": "password",
			},
		}
		body, _ := json.Marshal(payload)

		resp, err := client.Post(h.BaseURL+"/init", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("Failed to send POST /init: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			t.Errorf("Expected status non-200 (duplicate root should fail), got %d", resp.StatusCode)
		}
	})

	// 2. Login (Tier 1 & Tier 2: reject invalid credentials)
	t.Run("POST /login - invalid credentials", func(t *testing.T) {
		payload := map[string]string{
			"username": "rootadmin",
			"password": "wrongpassword",
		}
		body, _ := json.Marshal(payload)

		resp, err := client.Post(h.BaseURL+"/login", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("Failed to send POST /login: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("Expected status 401, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /login - success", func(t *testing.T) {
		payload := map[string]string{
			"username": "rootadmin",
			"password": "supersecurepassword123",
		}
		body, _ := json.Marshal(payload)

		resp, err := client.Post(h.BaseURL+"/login", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("Failed to send POST /login: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Expected status 200, got %d", resp.StatusCode)
		}

		var respData map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&respData); err != nil {
			t.Fatalf("Failed to decode login response: %v", err)
		}

		userObj, ok := respData["user"].(map[string]interface{})
		if !ok {
			t.Fatalf("Response missing 'user' object: %v", respData)
		}

		tok, ok := userObj["accessToken"].(string)
		if !ok || tok == "" {
			t.Fatalf("User object missing 'accessToken': %v", userObj)
		}
		accessToken = tok

		// Verify that the refresh token cookie was set
		cookies := jar.Cookies(reqURL)
		hasCookie := false
		for _, cookie := range cookies {
			if cookie.Name == "refresh_token" {
				hasCookie = true
				if cookie.Value == "" {
					t.Errorf("refresh_token cookie value is empty")
				}
			}
		}
		if !hasCookie {
			t.Errorf("refresh_token cookie not found in cookie jar")
		}
	})

	// 3. Authorize API access
	t.Run("POST /api/authorize - success", func(t *testing.T) {
		req, err := http.NewRequest("POST", h.BaseURL+"/api/authorize", nil)
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}
		req.Header.Set("Authorization", "Bearer "+accessToken)

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Failed to send request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}

		var respData map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&respData); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		userObj, ok := respData["user"].(map[string]interface{})
		if !ok || userObj["username"] != "rootadmin" {
			t.Errorf("Unexpected user object in response: %v", respData)
		}
	})

	// 4. Token Rotation and Grace Period (Tier 2)
	t.Run("POST /auth/refresh - rotation & grace period", func(t *testing.T) {
		// Save old refresh token value from cookie jar
		var oldRefreshToken string
		for _, cookie := range jar.Cookies(reqURL) {
			if cookie.Name == "refresh_token" {
				oldRefreshToken = cookie.Value
			}
		}
		if oldRefreshToken == "" {
			t.Fatalf("Could not locate refresh token in jar")
		}

		// Sleep for 1.1s to guarantee the exp timestamp will change
		time.Sleep(1100 * time.Millisecond)

		// Perform first refresh (rotates token)
		resp1, err := client.Post(h.BaseURL+"/auth/refresh", "application/json", nil)
		if err != nil {
			t.Fatalf("Failed to call refresh 1: %v", err)
		}
		defer resp1.Body.Close()
		if resp1.StatusCode != http.StatusOK {
			t.Fatalf("Refresh 1 failed: status %d", resp1.StatusCode)
		}

		// Retrieve rotated access token
		var respData1 map[string]interface{}
		if err := json.NewDecoder(resp1.Body).Decode(&respData1); err != nil {
			t.Fatalf("Failed to decode response 1: %v", err)
		}
		userObj1 := respData1["user"].(map[string]interface{})
		rotatedAccessToken := userObj1["accessToken"].(string)

		// Check that a new refresh token is in the cookie jar
		var newRefreshToken string
		for _, cookie := range jar.Cookies(reqURL) {
			if cookie.Name == "refresh_token" {
				newRefreshToken = cookie.Value
			}
		}
		if newRefreshToken == "" || newRefreshToken == oldRefreshToken {
			t.Errorf("Refresh token did not rotate correctly: %q vs %q", newRefreshToken, oldRefreshToken)
		}

		// Perform second refresh using the OLD refresh token (validating grace period)
		req2, err := http.NewRequest("POST", h.BaseURL+"/auth/refresh", nil)
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}
		req2.Header.Set("Cookie", "refresh_token="+oldRefreshToken)
		resp2, err := client.Do(req2)
		if err != nil {
			t.Fatalf("Failed to call refresh 2: %v", err)
		}
		defer resp2.Body.Close()

		if resp2.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200 (grace period active), got %d", resp2.StatusCode)
		}

		// Update global access token with the rotated one for subsequent steps
		accessToken = rotatedAccessToken
	})

	// 5. Expired Refresh Token (Tier 2)
	t.Run("POST /auth/refresh - reject expired token", func(t *testing.T) {
		// Open DB and update session expiresAt to 1 hour ago
		db, err := sql.Open("sqlite", h.DBPath)
		if err != nil {
			t.Fatalf("Failed to open SQLite db: %v", err)
		}
		defer db.Close()

		pastTime := time.Now().Add(-1 * time.Hour)
		pastTimeStr := timeToDBStr(pastTime)

		_, err = db.Exec("UPDATE sessions SET expiresAt = ?", pastTimeStr)
		if err != nil {
			t.Fatalf("Failed to update expiresAt in DB: %v", err)
		}

		// Perform refresh -> should fail because it is expired
		resp, err := client.Post(h.BaseURL+"/auth/refresh", "application/json", nil)
		if err != nil {
			t.Fatalf("Failed to send POST /auth/refresh: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("Expected status 401 for expired refresh token, got %d", resp.StatusCode)
		}

		// Verify that the session was deleted from DB
		var count int
		err = db.QueryRow("SELECT count(*) FROM sessions").Scan(&count)
		if err != nil {
			t.Fatalf("Failed to query sessions count: %v", err)
		}
		if count != 0 {
			t.Errorf("Expected expired session to be deleted from DB, but %d sessions remain", count)
		}
	})

	// 6. Relogin to perform logout
	t.Run("POST /login - relogin for logout test", func(t *testing.T) {
		payload := map[string]string{
			"username": "rootadmin",
			"password": "supersecurepassword123",
		}
		body, _ := json.Marshal(payload)

		resp, err := client.Post(h.BaseURL+"/login", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("Failed to send POST /login: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Expected status 200, got %d", resp.StatusCode)
		}

		var respData map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&respData)
		userObj := respData["user"].(map[string]interface{})
		accessToken = userObj["accessToken"].(string)
	})

	// 7. Logout
	t.Run("POST /auth/logout - success", func(t *testing.T) {
		resp, err := client.Post(h.BaseURL+"/auth/logout", "application/json", nil)
		if err != nil {
			t.Fatalf("Failed to send POST /auth/logout: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}

		// Verify cookie was cleared
		cookies := jar.Cookies(reqURL)
		for _, cookie := range cookies {
			if cookie.Name == "refresh_token" && cookie.Value != "" {
				t.Errorf("refresh_token cookie was not cleared")
			}
		}
	})

	t.Run("POST /auth/refresh - fail after logout", func(t *testing.T) {
		resp, err := client.Post(h.BaseURL+"/auth/refresh", "application/json", nil)
		if err != nil {
			t.Fatalf("Failed to send POST /auth/refresh: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			t.Errorf("Expected status non-200, got %d", resp.StatusCode)
		}
	})
}
