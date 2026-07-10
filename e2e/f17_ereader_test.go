package e2e

import (
	"bytes"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite"
)

// startMockSMTPServer runs a background TCP listener pretending to be an SMTP server
func startMockSMTPServer(t *testing.T) (string, int, func()) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to listen: %v", err)
	}
	_, portStr, _ := net.SplitHostPort(ln.Addr().String())
	port, _ := strconv.Atoi(portStr)

	closed := make(chan struct{})

	go func() {
		defer close(closed)
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				// Send SMTP Greeting
				fmt.Fprintf(c, "220 localhost ESMTP\r\n")

				buf := make([]byte, 4096)
				for {
					n, err := c.Read(buf)
					if err != nil {
						return
					}
					line := string(buf[:n])
					if strings.HasPrefix(line, "EHLO") || strings.HasPrefix(line, "HELO") {
						fmt.Fprintf(c, "250-localhost\r\n250 AUTH PLAIN\r\n")
					} else if strings.HasPrefix(line, "AUTH PLAIN") {
						fmt.Fprintf(c, "235 Authentication successful\r\n")
					} else if strings.HasPrefix(line, "MAIL FROM") {
						fmt.Fprintf(c, "250 2.1.0 Ok\r\n")
					} else if strings.HasPrefix(line, "RCPT TO") {
						fmt.Fprintf(c, "250 2.1.5 Ok\r\n")
					} else if strings.HasPrefix(line, "DATA") {
						fmt.Fprintf(c, "354 Start mail input; end with <CR><LF>.<CR><LF>\r\n")
					} else if strings.Contains(line, "\r\n.\r\n") || line == ".\r\n" || strings.HasSuffix(line, "\r\n.\r\n") {
						fmt.Fprintf(c, "250 2.0.0 Ok: queued\r\n")
					} else if strings.HasPrefix(line, "QUIT") {
						fmt.Fprintf(c, "221 2.0.0 Bye\r\n")
						return
					} else {
						fmt.Fprintf(c, "250 Ok\r\n")
					}
				}
			}(conn)
		}
	}()

	return "127.0.0.1", port, func() {
		ln.Close()
		<-closed
	}
}

func generateUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func TestF17EReader(t *testing.T) {
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

	// 2. Open DB to insert libraries, books, items, and a regular user
	db, err := sql.Open("sqlite", h.DBPath)
	if err != nil {
		t.Fatalf("Failed to open DB: %v", err)
	}
	defer db.Close()

	// Insert ebook library
	_, err = db.Exec(`INSERT INTO libraries (id, name, mediaType) VALUES ('lib-ebook', 'Ebooks Library', 'book')`)
	if err != nil {
		t.Fatalf("Failed to insert library: %v", err)
	}

	// Write a fake EPUB file to disk
	tempDir := t.TempDir()
	fakeEpubPath := filepath.Join(tempDir, "test.epub")
	if err := os.WriteFile(fakeEpubPath, []byte("fake epub content"), 0644); err != nil {
		t.Fatalf("Failed to write fake epub file: %v", err)
	}

	// Insert Book
	epubEbookJSON := `{"ebookFormat":"epub", "metadata":{"filename":"test.epub", "ext":".epub", "path":"` + filepath.ToSlash(fakeEpubPath) + `", "size":17}}`
	_, err = db.Exec(`INSERT INTO books (id, title, duration, coverPath, narrators, audioFiles, ebookFile, chapters, tags, genres) VALUES 
		('book-send', 'Send Ebook Test Book', 0, '', '[]', '[]', ?, '[]', '[]', '[]')`, epubEbookJSON)
	if err != nil {
		t.Fatalf("Failed to insert epub book: %v", err)
	}

	_, err = db.Exec(`INSERT INTO libraryItems (id, libraryId, mediaType, mediaId, title) VALUES ('item-send', 'lib-ebook', 'book', 'book-send', 'Send Ebook Test Book')`)
	if err != nil {
		t.Fatalf("Failed to insert epub libraryItem: %v", err)
	}

	// Insert regular user
	hashedPash, err := bcrypt.GenerateFromPassword([]byte("user_password123"), 8)
	if err != nil {
		t.Fatalf("Failed to hash password: %v", err)
	}
	regularUserID := generateUUID()
	permsJSON := `{"download":true,"accessExplicitContent":true,"accessAllLibraries":true,"librariesAccessible":[],"accessAllTags":true,"itemTagsSelected":[],"selectedTagsNotAccessible":false}`
	_, err = db.Exec(`INSERT INTO users (id, username, email, type, pash, token, isActive, permissions, extraData, bookmarks, createdAt, updatedAt)
		VALUES (?, ?, NULL, 'user', ?, 'token-regular', 1, ?, '{}', '[]', datetime('now'), datetime('now'))`,
		regularUserID, "regular_user", string(hashedPash), permsJSON)
	if err != nil {
		t.Fatalf("Failed to insert regular user: %v", err)
	}

	// Start Mock SMTP Server
	smtpHost, smtpPort, smtpCleanup := startMockSMTPServer(t)
	defer smtpCleanup()

	// 3. Test Patch Email Settings (Admin)
	t.Run("Update Email Settings and Fetch", func(t *testing.T) {
		settingsPayload := map[string]interface{}{
			"host":               smtpHost,
			"port":               smtpPort,
			"secure":             false,
			"rejectUnauthorized": false,
			"user":               "smtpuser",
			"pass":               "smtppass",
			"fromAddress":        "admin@my-domain.com",
			"testAddress":        "test-recipient@domain.com",
			"ereaderDevices": []map[string]interface{}{
				{"name": "Kindle-All", "email": "all@kindle.com", "availabilityOption": "allUsers", "users": []string{}},
				{"name": "Kindle-AdminOnly", "email": "admin@kindle.com", "availabilityOption": "adminOrUp", "users": []string{}},
				{"name": "Kindle-Specific", "email": "specific@kindle.com", "availabilityOption": "specificUsers", "users": []string{regularUserID}},
			},
		}

		settingsBody, _ := json.Marshal(settingsPayload)
		req, _ := http.NewRequest("PATCH", h.BaseURL+"/api/emails/settings", bytes.NewReader(settingsBody))
		req.Header.Set("Authorization", "Bearer "+adminToken)
		req.Header.Set("Content-Type", "application/json")

		respPatch, err := client.Do(req)
		if err != nil {
			t.Fatalf("PATCH email settings failed: %v", err)
		}
		defer respPatch.Body.Close()

		if respPatch.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(respPatch.Body)
			t.Fatalf("Expected 200 PATCH settings, got %d: %s", respPatch.StatusCode, body)
		}

		// GET settings & verify they are returned with sanitized password
		reqGet, _ := http.NewRequest("GET", h.BaseURL+"/api/emails/settings", nil)
		reqGet.Header.Set("Authorization", "Bearer "+adminToken)
		respGet, err := client.Do(reqGet)
		if err != nil {
			t.Fatalf("GET email settings failed: %v", err)
		}
		defer respGet.Body.Close()

		if respGet.StatusCode != http.StatusOK {
			t.Fatalf("Expected 200 GET settings, got %d", respGet.StatusCode)
		}

		var settingsResp map[string]interface{}
		json.NewDecoder(respGet.Body).Decode(&settingsResp)

		if hostVal := settingsResp["host"].(string); hostVal != smtpHost {
			t.Errorf("Expected host %q, got %q", smtpHost, hostVal)
		}
		if passVal := settingsResp["pass"].(string); passVal != "********" {
			t.Errorf("Expected pass to be masked '********', got %q", passVal)
		}
		devicesList := settingsResp["ereaderDevices"].([]interface{})
		if len(devicesList) != 3 {
			t.Errorf("Expected 3 ereader devices, got %d", len(devicesList))
		}
	})

	// 4. Test Update EReader Devices CRUD endpoint directly
	t.Run("Update Devices endpoint", func(t *testing.T) {
		devicesPayload := map[string]interface{}{
			"ereaderDevices": []map[string]interface{}{
				{"name": "Updated-Kindle-All", "email": "all@kindle.com", "availabilityOption": "allUsers", "users": []string{}},
				{"name": "Kindle-AdminOnly", "email": "admin@kindle.com", "availabilityOption": "adminOrUp", "users": []string{}},
				{"name": "Kindle-Specific", "email": "specific@kindle.com", "availabilityOption": "specificUsers", "users": []string{regularUserID}},
			},
		}
		devicesBody, _ := json.Marshal(devicesPayload)
		req, _ := http.NewRequest("POST", h.BaseURL+"/api/emails/ereader-devices", bytes.NewReader(devicesBody))
		req.Header.Set("Authorization", "Bearer "+adminToken)
		req.Header.Set("Content-Type", "application/json")

		respPost, err := client.Do(req)
		if err != nil {
			t.Fatalf("POST devices failed: %v", err)
		}
		defer respPost.Body.Close()

		if respPost.StatusCode != http.StatusOK {
			t.Fatalf("Expected 200 POST devices, got %d", respPost.StatusCode)
		}
	})

	// 5. Test Available Devices filtering
	t.Run("Available Devices Listing and Authorization", func(t *testing.T) {
		// Test Admin sees all 3 devices
		reqAdmin, _ := http.NewRequest("GET", h.BaseURL+"/api/emails/devices", nil)
		reqAdmin.Header.Set("Authorization", "Bearer "+adminToken)
		respAdmin, err := client.Do(reqAdmin)
		if err != nil {
			t.Fatalf("Admin GET devices failed: %v", err)
		}
		defer respAdmin.Body.Close()

		var adminDevices []map[string]interface{}
		json.NewDecoder(respAdmin.Body).Decode(&adminDevices)
		if len(adminDevices) != 3 {
			t.Errorf("Expected Admin to see 3 devices, got %d", len(adminDevices))
		}

		// Log in regular user to obtain token
		jarReg, _ := cookiejar.New(nil)
		clientReg := &http.Client{Jar: jarReg}
		rLoginPayload := map[string]string{
			"username": "regular_user",
			"password": "user_password123",
		}
		rLoginBody, _ := json.Marshal(rLoginPayload)
		respR, err := clientReg.Post(h.BaseURL+"/login", "application/json", bytes.NewReader(rLoginBody))
		if err != nil {
			t.Fatalf("Failed to login regular user: %v", err)
		}
		var userResp map[string]interface{}
		json.NewDecoder(respR.Body).Decode(&userResp)
		respR.Body.Close()
		userToken := userResp["user"].(map[string]interface{})["accessToken"].(string)

		// Test Regular User sees only 2 devices (Updated-Kindle-All and Kindle-Specific, but not Kindle-AdminOnly)
		reqUser, _ := http.NewRequest("GET", h.BaseURL+"/api/emails/devices", nil)
		reqUser.Header.Set("Authorization", "Bearer "+userToken)
		respUser, err := clientReg.Do(reqUser)
		if err != nil {
			t.Fatalf("User GET devices failed: %v", err)
		}
		defer respUser.Body.Close()

		var userDevices []map[string]interface{}
		json.NewDecoder(respUser.Body).Decode(&userDevices)
		if len(userDevices) != 2 {
			t.Errorf("Expected regular user to see 2 devices, got %d", len(userDevices))
		}

		// Ensure correct names are returned
		namesMap := make(map[string]bool)
		for _, dev := range userDevices {
			namesMap[dev["name"].(string)] = true
		}
		if !namesMap["Updated-Kindle-All"] || !namesMap["Kindle-Specific"] {
			t.Errorf("Expected Updated-Kindle-All and Kindle-Specific, got: %v", namesMap)
		}
	})

	// 6. Test Send SMTP Test Email
	t.Run("Send Test Email", func(t *testing.T) {
		testPayload := map[string]interface{}{
			"host":               smtpHost,
			"port":               smtpPort,
			"secure":             false,
			"rejectUnauthorized": false,
			"user":               "smtpuser",
			"pass":               "smtppass",
			"fromAddress":        "admin@my-domain.com",
			"testAddress":        "recipient@test.com",
		}
		testBody, _ := json.Marshal(testPayload)
		req, _ := http.NewRequest("POST", h.BaseURL+"/api/emails/test", bytes.NewReader(testBody))
		req.Header.Set("Authorization", "Bearer "+adminToken)
		req.Header.Set("Content-Type", "application/json")

		respTest, err := client.Do(req)
		if err != nil {
			t.Fatalf("POST email test failed: %v", err)
		}
		defer respTest.Body.Close()

		if respTest.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(respTest.Body)
			t.Fatalf("Expected 200 status for test email, got %d: %s", respTest.StatusCode, body)
		}
	})

	// 7. Test Send EBook to Device
	t.Run("Send EBook to Device", func(t *testing.T) {
		sendPayload := map[string]interface{}{
			"libraryItemId": "item-send",
			"deviceName":    "Updated-Kindle-All",
		}
		sendBody, _ := json.Marshal(sendPayload)
		req, _ := http.NewRequest("POST", h.BaseURL+"/api/emails/send-ebook-to-device", bytes.NewReader(sendBody))
		req.Header.Set("Authorization", "Bearer "+adminToken)
		req.Header.Set("Content-Type", "application/json")

		respSend, err := client.Do(req)
		if err != nil {
			t.Fatalf("POST send-ebook failed: %v", err)
		}
		defer respSend.Body.Close()

		if respSend.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(respSend.Body)
			t.Fatalf("Expected 200 status for send-ebook, got %d: %s", respSend.StatusCode, body)
		}
	})
}
