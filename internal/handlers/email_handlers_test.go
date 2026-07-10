package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"audiobookshelf/internal/core"
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

func TestEmailHandlers_GetSettings(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Seed some email settings
	settingsJSON := `{"id":"email-settings","host":"smtp.example.com","port":587,"secure":false,"rejectUnauthorized":true,"user":"user@example.com","pass":"secretpass","testAddress":"test@example.com","fromAddress":"noreply@example.com","ereaderDevices":[]}`
	_, err := db.Exec("INSERT INTO settings (key, value) VALUES ('email-settings', ?)", settingsJSON)
	if err != nil {
		t.Fatalf("failed to seed settings: %v", err)
	}

	adminSession := &core.UserSession{ID: "admin-user", Username: "admin", Type: "admin", IsActive: true}
	userSession := &core.UserSession{ID: "regular-user", Username: "user", Type: "user", IsActive: true}

	t.Run("AdminCanGetSettings", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/emails/settings", nil)
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, adminSession))
		rr := httptest.NewRecorder()

		handleGetEmailSettings(db).ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rr.Code)
		}

		var settings EmailSettings
		if err := json.Unmarshal(rr.Body.Bytes(), &settings); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if settings.Host != "smtp.example.com" {
			t.Errorf("expected smtp.example.com, got %s", settings.Host)
		}
		if settings.Pass != "********" {
			t.Errorf("expected password to be sanitized, got %s", settings.Pass)
		}
	})

	t.Run("UserCannotGetSettings", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/emails/settings", nil)
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, userSession))
		rr := httptest.NewRecorder()

		handleGetEmailSettings(db).ServeHTTP(rr, req)

		if rr.Code != http.StatusForbidden {
			t.Errorf("expected 403, got %d", rr.Code)
		}
	})
}

func TestEmailHandlers_UpdateSettings(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	adminSession := &core.UserSession{ID: "admin-user", Username: "admin", Type: "admin", IsActive: true}

	t.Run("UpdateSettingsAndKeepPassword", func(t *testing.T) {
		// First set settings with password
		initPayload := `{"host":"smtp.test.com","port":25,"pass":"initialSecret"}`
		req := httptest.NewRequest("PATCH", "/api/emails/settings", strings.NewReader(initPayload))
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, adminSession))
		rr := httptest.NewRecorder()
		handleUpdateEmailSettings(db).ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("first patch failed: %s", rr.Body.String())
		}

		// Update host but pass "********" for password
		updatePayload := `{"host":"smtp.updated.com","port":587,"pass":"********"}`
		req2 := httptest.NewRequest("PATCH", "/api/emails/settings", strings.NewReader(updatePayload))
		req2 = req2.WithContext(context.WithValue(req2.Context(), core.UserContextKey, adminSession))
		rr2 := httptest.NewRecorder()
		handleUpdateEmailSettings(db).ServeHTTP(rr2, req2)

		if rr2.Code != http.StatusOK {
			t.Fatalf("second patch failed: %s", rr2.Body.String())
		}

		// Verify settings stored in DB
		settings, err := loadEmailSettings(db)
		if err != nil {
			t.Fatalf("failed to load settings: %v", err)
		}

		if settings.Host != "smtp.updated.com" {
			t.Errorf("expected smtp.updated.com, got %s", settings.Host)
		}
		if settings.Pass != "initialSecret" {
			t.Errorf("expected password to remain initialSecret, got %s", settings.Pass)
		}
	})
}

func TestEmailHandlers_SendTestEmail(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	adminSession := &core.UserSession{ID: "admin-user", Username: "admin", Type: "admin", IsActive: true}

	// Start mock SMTP server
	host, port, cleanup := startMockSMTPServer(t)
	defer cleanup()

	t.Run("SendTestEmailSuccess", func(t *testing.T) {
		payload := fmt.Sprintf(`{"host":"%s","port":%d,"secure":false,"rejectUnauthorized":false,"user":"testuser","pass":"testpass","testAddress":"target@test.com","fromAddress":"source@test.com"}`, host, port)
		req := httptest.NewRequest("POST", "/api/emails/test", strings.NewReader(payload))
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, adminSession))
		rr := httptest.NewRecorder()

		handleSendTestEmail(db).ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected 200, got %d: %s", rr.Code, rr.Body.String())
		}
	})
}

func TestEmailHandlers_SendEBookToDevice(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Temp book file on disk
	tempDir, err := os.MkdirTemp("", "abs-ebook-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	epubPath := filepath.Join(tempDir, "test.epub")
	if err := os.WriteFile(epubPath, []byte("epub content"), 0644); err != nil {
		t.Fatalf("failed to write temp epub: %v", err)
	}

	// Setup SMTP Server
	host, port, cleanup := startMockSMTPServer(t)
	defer cleanup()

	// Seed SMTP + device settings
	devicesJSON := fmt.Sprintf(`{
		"id": "email-settings",
		"host": "%s",
		"port": %d,
		"secure": false,
		"rejectUnauthorized": false,
		"user": "",
		"pass": "",
		"fromAddress": "server@abs.com",
		"ereaderDevices": [
			{
				"name": "Kindle-All",
				"email": "kindleall@kindle.com",
				"availabilityOption": "allUsers",
				"users": []
			},
			{
				"name": "Kindle-Specific",
				"email": "kindlespecific@kindle.com",
				"availabilityOption": "specificUsers",
				"users": ["allowed-user"]
			},
			{
				"name": "Kindle-AdminOnly",
				"email": "kindleadmin@kindle.com",
				"availabilityOption": "adminOrUp",
				"users": []
			}
		]
	}`, host, port)

	_, err = db.Exec("INSERT INTO settings (key, value) VALUES ('email-settings', ?)", devicesJSON)
	if err != nil {
		t.Fatalf("failed to seed settings: %v", err)
	}

	// Seed libraryItem and Book
	ebookFileJSON := fmt.Sprintf(`{"ebookFormat":"epub","metadata":{"path":"%s","filename":"test.epub"}}`, epubPath)
	_, err = db.Exec("INSERT INTO books (id, title, ebookFile) VALUES ('book-1', 'Test EBook', ?)", ebookFileJSON)
	if err != nil {
		t.Fatalf("failed to seed book: %v", err)
	}

	_, err = db.Exec("INSERT INTO libraryItems (id, mediaId, mediaType) VALUES ('item-1', 'book-1', 'book')")
	if err != nil {
		t.Fatalf("failed to seed library item: %v", err)
	}

	// Set up user sessions
	adminSession := &core.UserSession{ID: "admin-user", Username: "admin", Type: "admin", IsActive: true}
	allowedUserSession := &core.UserSession{ID: "allowed-user", Username: "user1", Type: "user", IsActive: true}
	deniedUserSession := &core.UserSession{ID: "denied-user", Username: "user2", Type: "user", IsActive: true}

	t.Run("AllUsersCanSendToAllUsersDevice", func(t *testing.T) {
		payload := `{"libraryItemId":"item-1","deviceName":"Kindle-All"}`
		req := httptest.NewRequest("POST", "/api/emails/send-ebook-to-device", strings.NewReader(payload))
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, deniedUserSession))
		rr := httptest.NewRecorder()

		handleSendEBookToDevice(db).ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected 200, got %d: %s", rr.Code, rr.Body.String())
		}
	})

	t.Run("SpecificUsersAllowedDevice", func(t *testing.T) {
		payload := `{"libraryItemId":"item-1","deviceName":"Kindle-Specific"}`
		
		// Allowed User
		req1 := httptest.NewRequest("POST", "/api/emails/send-ebook-to-device", strings.NewReader(payload))
		req1 = req1.WithContext(context.WithValue(req1.Context(), core.UserContextKey, allowedUserSession))
		rr1 := httptest.NewRecorder()
		handleSendEBookToDevice(db).ServeHTTP(rr1, req1)
		if rr1.Code != http.StatusOK {
			t.Errorf("expected 200 for allowed user, got %d: %s", rr1.Code, rr1.Body.String())
		}

		// Denied User
		req2 := httptest.NewRequest("POST", "/api/emails/send-ebook-to-device", strings.NewReader(payload))
		req2 = req2.WithContext(context.WithValue(req2.Context(), core.UserContextKey, deniedUserSession))
		rr2 := httptest.NewRecorder()
		handleSendEBookToDevice(db).ServeHTTP(rr2, req2)
		if rr2.Code != http.StatusForbidden {
			t.Errorf("expected 403 for denied user, got %d", rr2.Code)
		}
	})

	t.Run("AdminOrUpDevice", func(t *testing.T) {
		payload := `{"libraryItemId":"item-1","deviceName":"Kindle-AdminOnly"}`

		// Non-admin User
		req1 := httptest.NewRequest("POST", "/api/emails/send-ebook-to-device", strings.NewReader(payload))
		req1 = req1.WithContext(context.WithValue(req1.Context(), core.UserContextKey, allowedUserSession))
		rr1 := httptest.NewRecorder()
		handleSendEBookToDevice(db).ServeHTTP(rr1, req1)
		if rr1.Code != http.StatusForbidden {
			t.Errorf("expected 403 for non-admin user, got %d", rr1.Code)
		}

		// Admin User
		req2 := httptest.NewRequest("POST", "/api/emails/send-ebook-to-device", strings.NewReader(payload))
		req2 = req2.WithContext(context.WithValue(req2.Context(), core.UserContextKey, adminSession))
		rr2 := httptest.NewRecorder()
		handleSendEBookToDevice(db).ServeHTTP(rr2, req2)
		if rr2.Code != http.StatusOK {
			t.Errorf("expected 200 for admin user, got %d: %s", rr2.Code, rr2.Body.String())
		}
	})
}

func TestEmailHandlers_GetAvailableDevices(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Seed email settings with devices having different availability policies
	settingsJSON := `{
		"id":"email-settings",
		"host":"smtp.example.com",
		"port":587,
		"ereaderDevices":[
			{"name":"Kindle-All","email":"all@kindle.com","availabilityOption":"allUsers","users":[]},
			{"name":"Kindle-AdminOnly","email":"admin@kindle.com","availabilityOption":"adminOrUp","users":[]},
			{"name":"Kindle-Specific","email":"specific@kindle.com","availabilityOption":"specificUsers","users":["user-allowed"]}
		]
	}`
	_, err := db.Exec("INSERT INTO settings (key, value) VALUES ('email-settings', ?)", settingsJSON)
	if err != nil {
		t.Fatalf("failed to seed settings: %v", err)
	}

	adminSession := &core.UserSession{ID: "admin-user", Username: "admin", Type: "admin", IsActive: true}
	allowedSession := &core.UserSession{ID: "user-allowed", Username: "user1", Type: "user", IsActive: true}
	deniedSession := &core.UserSession{ID: "user-denied", Username: "user2", Type: "user", IsActive: true}

	t.Run("AdminGetAvailableDevices", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/emails/devices", nil)
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, adminSession))
		rr := httptest.NewRecorder()

		handleGetAvailableDevices(db).ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rr.Code)
		}
		var devices []EreaderDevice
		if err := json.Unmarshal(rr.Body.Bytes(), &devices); err != nil {
			t.Fatalf("failed to unmarshal body: %v", err)
		}
		// Admin should see all 3 devices
		if len(devices) != 3 {
			t.Errorf("expected 3 devices for admin, got %d", len(devices))
		}
	})

	t.Run("AllowedUserGetAvailableDevices", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/emails/devices", nil)
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, allowedSession))
		rr := httptest.NewRecorder()

		handleGetAvailableDevices(db).ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rr.Code)
		}
		var devices []EreaderDevice
		if err := json.Unmarshal(rr.Body.Bytes(), &devices); err != nil {
			t.Fatalf("failed to unmarshal body: %v", err)
		}
		// Allowed user should see Kindle-All and Kindle-Specific (2 devices)
		if len(devices) != 2 {
			t.Errorf("expected 2 devices for allowed user, got %d", len(devices))
		}
	})

	t.Run("DeniedUserGetAvailableDevices", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/emails/devices", nil)
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, deniedSession))
		rr := httptest.NewRecorder()

		handleGetAvailableDevices(db).ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rr.Code)
		}
		var devices []EreaderDevice
		if err := json.Unmarshal(rr.Body.Bytes(), &devices); err != nil {
			t.Fatalf("failed to unmarshal body: %v", err)
		}
		// Denied user should see only Kindle-All (1 device)
		if len(devices) != 1 {
			t.Errorf("expected 1 device for denied user, got %d", len(devices))
		}
		if devices[0].Name != "Kindle-All" {
			t.Errorf("expected Kindle-All, got %s", devices[0].Name)
		}
	})
}
