package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"

	"audiobookshelf/internal/core")

func generateTestToken(userID, username, userType, secret string) (string, error) {
	claims := &core.AuthClaims{
		UserID:   userID,
		Username: username,
		Type:     userType,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer: "audiobookshelf",
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

func setupSocketTestDB(t *testing.T) *sql.DB {
	db := setupTestDB(t)
	// Add fields needed for WebSocket tracking and user registration
	_, err := db.Exec("ALTER TABLE users ADD COLUMN lastSeen TEXT")
	if err != nil {
		t.Fatalf("Failed to alter table users for lastSeen: %v", err)
	}
	_, err = db.Exec("ALTER TABLE users ADD COLUMN createdAt TEXT")
	if err != nil {
		t.Fatalf("Failed to alter table users for createdAt: %v", err)
	}
	_, err = db.Exec("ALTER TABLE users ADD COLUMN updatedAt TEXT")
	if err != nil {
		t.Fatalf("Failed to alter table users for updatedAt: %v", err)
	}

	// Update server-settings to include the tokenSecret used by tests.
	// internal/socket reads it from the DB (not from the root cachedSecret var).
	const testSecret = "test-secret-12345"
	_, err = db.Exec(`UPDATE settings SET value = ? WHERE key = 'server-settings'`,
		`{"sortingIgnorePrefix":true,"tokenSecret":"test-secret-12345"}`)
	if err != nil {
		t.Fatalf("Failed to update server settings with tokenSecret: %v", err)
	}

	// Also set cachedSecret for any root-level code that still uses it.
	cachedSecret = testSecret

	return db
}

// socketTestPerms mirrors the permissions structure for building test JSON.
type socketTestPerms struct {
	Download                  *bool    `json:"download"`
	AccessExplicitContent     *bool    `json:"accessExplicitContent"`
	AccessAllLibraries        *bool    `json:"accessAllLibraries"`
	LibrariesAccessible       []string `json:"librariesAccessible"`
	Libraries                 []string `json:"libraries"`
	AccessAllTags             *bool    `json:"accessAllTags"`
	ItemTagsSelected          []string `json:"itemTagsSelected"`
	SelectedTagsNotAccessible *bool    `json:"selectedTagsNotAccessible"`
}

func insertTestUser(t *testing.T, db *sql.DB, id, username, uType string, isActive bool, download, explicit, allLibs, allTags, tagsNotAccessible bool, libsAccessible, tagsSelected []string) {
	perm := socketTestPerms{
		Download:                  &download,
		AccessExplicitContent:     &explicit,
		AccessAllLibraries:        &allLibs,
		LibrariesAccessible:       libsAccessible,
		AccessAllTags:             &allTags,
		ItemTagsSelected:          tagsSelected,
		SelectedTagsNotAccessible: &tagsNotAccessible,
	}
	permBytes, err := json.Marshal(perm)
	if err != nil {
		t.Fatalf("Failed to marshal permissions: %v", err)
	}

	nowStr := time.Now().UTC().Format("2006-01-02 15:04:05.000 +00:00")
	_, err = db.Exec(`INSERT INTO users (id, username, type, isActive, permissions, extraData, createdAt, updatedAt) VALUES (?, ?, ?, ?, ?, '{}', ?, ?)`,
		id, username, uType, func() int {
			if isActive {
				return 1
			}
			return 0
		}(), string(permBytes), nowStr, nowStr)
	if err != nil {
		t.Fatalf("Failed to insert user: %v", err)
	}
}


type SocketEvent struct {
	Name string
	Args []interface{}
}

type TestClient struct {
	Conn *websocket.Conn
	Chan chan SocketEvent
}

func newTestClient(conn *websocket.Conn) *TestClient {
	tc := &TestClient{
		Conn: conn,
		Chan: make(chan SocketEvent, 100),
	}
	go func() {
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			msgStr := string(msg)
			if strings.HasPrefix(msgStr, "42") {
				payload := msgStr[2:]
				var data []interface{}
				if err := json.Unmarshal([]byte(payload), &data); err == nil && len(data) > 0 {
					if evtName, ok := data[0].(string); ok {
						tc.Chan <- SocketEvent{
							Name: evtName,
							Args: data[1:],
						}
					}
				}
			}
		}
	}()
	return tc
}

func (tc *TestClient) expectEvent(t *testing.T, expectedName string) SocketEvent {
	select {
	case ev := <-tc.Chan:
		if ev.Name != expectedName {
			t.Fatalf("Expected event %q, got %q (args: %v)", expectedName, ev.Name, ev.Args)
		}
		return ev
	case <-time.After(2 * time.Second):
		t.Fatalf("Timeout waiting for event %q", expectedName)
	}
	return SocketEvent{}
}

func (tc *TestClient) assertNoEvent(t *testing.T, timeout time.Duration) {
	select {
	case ev := <-tc.Chan:
		t.Fatalf("Expected no event, but received: %q (args: %v)", ev.Name, ev.Args)
	case <-time.After(timeout):
		// Success
	}
}

func TestSocketAuthAndHandshake(t *testing.T) {
	db := setupSocketTestDB(t)
	defer db.Close()

	// Initialize test settings & key
	cachedSecret = "test-secret-12345"
	insertTestUser(t, db, "user-root", "rootuser", "root", true, true, true, true, true, false, nil, nil)

	handler := InitSocketAuthority(db)
	defer SocketAuth.Close()

	server := httptest.NewServer(handler)
	defer server.Close()

	// Connect to socket.io
	u := url.URL{Scheme: "ws", Host: strings.TrimPrefix(server.URL, "http://"), Path: "/socket.io/"}
	q := u.Query()
	q.Set("EIO", "4")
	q.Set("transport", "websocket")
	u.RawQuery = q.Encode()

	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		t.Fatalf("WebSocket connection failed: %v", err)
	}
	defer c.Close()

	// 1. Read Engine.io Open packet (starts with 0)
	_, msg, err := c.ReadMessage()
	if err != nil {
		t.Fatalf("Failed to read open packet: %v", err)
	}
	if !strings.HasPrefix(string(msg), "0") {
		t.Errorf("Expected Engine.io Open packet, got: %s", string(msg))
	}

	// 2. Send Engine.io Connect packet (40)
	err = c.WriteMessage(websocket.TextMessage, []byte("40"))
	if err != nil {
		t.Fatalf("Failed to send connect packet: %v", err)
	}

	// 3. Read Engine.io Connect response
	_, msg, err = c.ReadMessage()
	if err != nil {
		t.Fatalf("Failed to read connect response: %v", err)
	}
	if !strings.HasPrefix(string(msg), "40") {
		t.Errorf("Expected Socket.io Connect packet, got: %s", string(msg))
	}

	// 4. Send Auth token
	token, err := generateTestToken("user-root", "rootuser", "root", cachedSecret)
	if err != nil {
		t.Fatalf("Failed to generate JWT: %v", err)
	}

	authPayload := fmt.Sprintf(`42["auth",%q]`, token)
	err = c.WriteMessage(websocket.TextMessage, []byte(authPayload))
	if err != nil {
		t.Fatalf("Failed to send auth payload: %v", err)
	}

	tc := newTestClient(c)

	// 5. Read events (expecting "init" and "user_online")
	var initData map[string]interface{}
	var foundInit, foundUserOnline bool
	for i := 0; i < 2; i++ {
		select {
		case ev := <-tc.Chan:
			if ev.Name == "init" {
				foundInit = true
				initData = ev.Args[0].(map[string]interface{})
			} else if ev.Name == "user_online" {
				foundUserOnline = true
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("Timeout waiting for initial handshakes")
		}
	}

	if !foundInit {
		t.Fatalf("Expected event 'init' to be received")
	}
	if !foundUserOnline {
		t.Errorf("Expected event 'user_online' to be received by admin client")
	}

	if initData["userId"] != "user-root" || initData["username"] != "rootuser" {
		t.Errorf("Unexpected init values: %v", initData)
	}

	// Verify online users list contains rootuser
	usersOnline, ok := initData["usersOnline"].([]interface{})
	if !ok || len(usersOnline) == 0 {
		t.Errorf("Expected non-empty usersOnline list, got: %v", initData["usersOnline"])
	} else {
		found := false
		for _, uo := range usersOnline {
			uoMap := uo.(map[string]interface{})
			if uoMap["id"] == "user-root" && uoMap["username"] == "rootuser" {
				found = true
				if val, ok := uoMap["connections"].(float64); ok {
					if val != 1.0 {
						t.Errorf("Expected connections to be 1, got %v", val)
					}
				} else {
					t.Errorf("connections is not a float64: %T", uoMap["connections"])
				}
			}
		}
		if !found {
			t.Errorf("rootuser not found in usersOnline list")
		}
	}

	// 6. Test ping-pong custom events
	err = c.WriteMessage(websocket.TextMessage, []byte(`42["ping"]`))
	if err != nil {
		t.Fatalf("Failed to send ping: %v", err)
	}
	tc.expectEvent(t, "pong")

	// Verify database lastSeen was updated
	var lastSeen sql.NullString
	err = db.QueryRow("SELECT lastSeen FROM users WHERE id = 'user-root'").Scan(&lastSeen)
	if err != nil {
		t.Fatalf("Failed to query user lastSeen: %v", err)
	}
	if !lastSeen.Valid || lastSeen.String == "" {
		t.Errorf("Expected lastSeen to be updated, got null or empty")
	}

	// 7. Verify Client removal on Disconnect
	c.Close()

	// Wait briefly for disconnect handler to process
	time.Sleep(100 * time.Millisecond)

	clientsLen := SocketAuth.ClientCount()
	if clientsLen != 0 {
		t.Errorf("Expected clients registry to be empty after disconnect, got size %d", clientsLen)
	}
}

func TestSocketAuthFailed(t *testing.T) {
	db := setupSocketTestDB(t)
	defer db.Close()

	cachedSecret = "test-secret-12345"

	handler := InitSocketAuthority(db)
	defer SocketAuth.Close()

	server := httptest.NewServer(handler)
	defer server.Close()

	u := url.URL{Scheme: "ws", Host: strings.TrimPrefix(server.URL, "http://"), Path: "/socket.io/"}
	q := u.Query()
	q.Set("EIO", "4")
	q.Set("transport", "websocket")
	u.RawQuery = q.Encode()

	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		t.Fatalf("WebSocket connection failed: %v", err)
	}
	defer c.Close()

	// Read open
	_, _, _ = c.ReadMessage()
	// Write Connect
	_ = c.WriteMessage(websocket.TextMessage, []byte("40"))
	// Read Connect
	_, _, _ = c.ReadMessage()

	tc := newTestClient(c)

	// Write invalid auth
	authPayload := `42["auth","invalid-token-value"]`
	err = c.WriteMessage(websocket.TextMessage, []byte(authPayload))
	if err != nil {
		t.Fatalf("Failed to send invalid auth payload: %v", err)
	}

	ev := tc.expectEvent(t, "auth_failed")
	failData, ok := ev.Args[0].(map[string]interface{})
	if !ok || failData["message"] != "Invalid token" {
		t.Errorf("Unexpected auth_failed payload: %v", ev.Args)
	}
}

func TestPermissionBroadcasting(t *testing.T) {
	db := setupSocketTestDB(t)
	defer db.Close()

	cachedSecret = "test-secret-12345"

	// 1. Insert 3 users with different permissions
	// user1: Admin, full access
	insertTestUser(t, db, "user1", "adminuser", "admin", true, true, true, true, true, false, nil, nil)
	// user2: Regular user, no explicit allowed, access to all tags
	insertTestUser(t, db, "user2", "regular-no-explicit", "user", true, true, false, true, true, false, nil, nil)
	// user3: Regular user, explicit allowed, tag filter TagA only
	insertTestUser(t, db, "user3", "regular-tag-filtered", "user", true, true, true, true, false, false, nil, []string{"TagA"})

	handler := InitSocketAuthority(db)
	defer SocketAuth.Close()

	server := httptest.NewServer(handler)
	defer server.Close()

	connectAndAuth := func(userID, username, uType string) *TestClient {
		u := url.URL{Scheme: "ws", Host: strings.TrimPrefix(server.URL, "http://"), Path: "/socket.io/"}
		q := u.Query()
		q.Set("EIO", "4")
		q.Set("transport", "websocket")
		u.RawQuery = q.Encode()

		c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
		if err != nil {
			t.Fatalf("WebSocket connection failed for %s: %v", username, err)
		}

		// read open, send connect, read connect
		_, _, _ = c.ReadMessage()
		_ = c.WriteMessage(websocket.TextMessage, []byte("40"))
		_, _, _ = c.ReadMessage()

		token, err := generateTestToken(userID, username, uType, cachedSecret)
		if err != nil {
			t.Fatalf("Failed to generate JWT: %v", err)
		}
		_ = c.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf(`42["auth",%q]`, token)))

		tc := newTestClient(c)

		// read init (and user_online if admin)
		var expectedEvents = 1
		if uType == "admin" || uType == "root" {
			expectedEvents = 2
		}
		var hasInit, hasUserOnline bool
		for i := 0; i < expectedEvents; i++ {
			select {
			case ev := <-tc.Chan:
				if ev.Name == "init" {
					hasInit = true
				} else if ev.Name == "user_online" {
					hasUserOnline = true
				}
			case <-time.After(2 * time.Second):
				t.Fatalf("Timeout waiting for handshake events on %s", username)
			}
		}

		if !hasInit {
			t.Fatalf("Expected init event for %s", username)
		}
		if expectedEvents == 2 && !hasUserOnline {
			t.Fatalf("Expected user_online event for admin %s", username)
		}

		return tc
	}

	c1 := connectAndAuth("user1", "adminuser", "admin")
	defer c1.Conn.Close()

	c2 := connectAndAuth("user2", "regular-no-explicit", "user")
	defer c2.Conn.Close()
	// Admin (c1) gets "user_online" event for c2. Let's consume it.
	c1.expectEvent(t, "user_online")

	c3 := connectAndAuth("user3", "regular-tag-filtered", "user")
	defer c3.Conn.Close()
	// Admin (c1) gets "user_online" event for c3. Let's consume it.
	c1.expectEvent(t, "user_online")

	// 2. Broadcast itemSafe: not explicit, has tag "TagA". Everyone should receive it.
	itemSafe := map[string]interface{}{
		"libraryId": "lib1",
		"media": map[string]interface{}{
			"explicit": false,
			"tags":     []interface{}{"TagA"},
		},
	}

	SocketAuth.LibraryItemEmitter("item_broadcast", itemSafe)

	// Read from c1, c2, c3
	ev1 := c1.expectEvent(t, "item_broadcast")
	payload := ev1.Args[0].(map[string]interface{})
	if payload["libraryId"] != "lib1" {
		t.Errorf("Unexpected library ID in user1 payload: %v", payload)
	}

	c2.expectEvent(t, "item_broadcast")
	c3.expectEvent(t, "item_broadcast")

	// 3. Broadcast itemExplicit: explicit=true, tags=["TagA"]. Only user1 (admin) and user3 (can access explicit) should receive it.
	itemExplicit := map[string]interface{}{
		"libraryId": "lib1",
		"media": map[string]interface{}{
			"explicit": true,
			"tags":     []interface{}{"TagA"},
		},
	}

	SocketAuth.LibraryItemEmitter("item_broadcast_exp", itemExplicit)

	c1.expectEvent(t, "item_broadcast_exp")
	c3.expectEvent(t, "item_broadcast_exp")

	// User2 should NOT receive itemExplicit because User2 lacks explicit permission.
	c2.assertNoEvent(t, 100*time.Millisecond)

	// 4. Broadcast itemTagB: explicit=false, tags=["TagB"]. Only user1 (admin) and user2 (access all tags) should receive it.
	// user3 only has access to TagA, so user3 should NOT receive it.
	itemTagB := map[string]interface{}{
		"libraryId": "lib1",
		"media": map[string]interface{}{
			"explicit": false,
			"tags":     []interface{}{"TagB"},
		},
	}

	SocketAuth.LibraryItemEmitter("item_broadcast_tag_b", itemTagB)

	c1.expectEvent(t, "item_broadcast_tag_b")
	c2.expectEvent(t, "item_broadcast_tag_b")

	// User3 should NOT receive because of tag restriction
	c3.assertNoEvent(t, 100*time.Millisecond)
}
