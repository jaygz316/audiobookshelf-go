package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/zishang520/engine.io/v2/types"
	"github.com/zishang520/socket.io/v2/socket"
)

// PublicUser represents the public user structure sent on online/offline events
type PublicUser struct {
	ID        string      `json:"id"`
	Username  string      `json:"username"`
	Type      string      `json:"type"`
	Session   interface{} `json:"session"` // can be nil
	LastSeen  int64       `json:"lastSeen"`
	CreatedAt int64       `json:"createdAt"`
}

// OnlineUser represents the online user structure for admins
type OnlineUser struct {
	ID               string        `json:"id"`
	Username         string        `json:"username"`
	Type             string        `json:"type"`
	IsActive         bool          `json:"isActive"`
	Connections      int           `json:"connections"`
	LastSeen         int64         `json:"lastSeen"`
	Session          interface{}   `json:"session"` // can be nil
	PlaybackSessions []interface{} `json:"playbackSessions"`
}

// SocketClient wraps a socket connection with authentication information
type SocketClient struct {
	ID          string
	Socket      *socket.Socket
	ConnectedAt time.Time
	User        *UserSession // Loaded from auth.go / db.go
}

// SocketAuthority manages WebSocket connections and dynamic permission filtering
type SocketAuthority struct {
	mu             sync.RWMutex
	clients        map[string]*SocketClient
	db             *sql.DB
	io             *socket.Server
	activeSearches map[string]context.CancelFunc
	activeSearchLock sync.Mutex
}

// SocketAuth is the global reference to the socket authority
var SocketAuth *SocketAuthority

// NewSocketAuthority creates a new instance of SocketAuthority
func NewSocketAuthority(db *sql.DB) *SocketAuthority {
	return &SocketAuthority{
		clients:        make(map[string]*SocketClient),
		db:             db,
		activeSearches: make(map[string]context.CancelFunc),
	}
}

// AuthenticateSocket validates the JWT token and binds the user session to the socket
func (sa *SocketAuthority) AuthenticateSocket(client *socket.Socket, token string) {
	tokenSecret := getTokenSecret(sa.db)
	userID, err := validateToken(token, tokenSecret)
	if err != nil {
		log.Printf("[Socket] Authentication failed: %v", err)
		client.Emit("auth_failed", map[string]string{"message": "Invalid token"})
		return
	}

	userSession, err := GetUserByIDOrOldID(sa.db, userID)
	if err != nil {
		log.Printf("[Socket] User lookup failed for ID %s: %v", userID, err)
		client.Emit("auth_failed", map[string]string{"message": "Invalid token"})
		return
	}

	if !userSession.IsActive {
		log.Printf("[Socket] User %s is inactive", userSession.Username)
		client.Emit("auth_failed", map[string]string{"message": "Invalid user"})
		return
	}

	sa.mu.Lock()
	socketClient, exists := sa.clients[string(client.Id())]
	if exists {
		socketClient.User = userSession
	}
	sa.mu.Unlock()

	if !exists {
		log.Printf("[Socket] No client found for socket ID %s", client.Id())
		return
	}

	// Fetch extra fields for user online events
	var createdAtStr, lastSeenStr sql.NullString
	_ = sa.db.QueryRow("SELECT createdAt, lastSeen FROM users WHERE id = ?", userSession.ID).Scan(&createdAtStr, &lastSeenStr)

	var createdAtMs, lastSeenMs int64
	if createdAtStr.Valid && createdAtStr.String != "" {
		if t, err := parseSQLiteTime(createdAtStr.String); err == nil {
			createdAtMs = t.UnixMilli()
		}
	}
	if lastSeenStr.Valid && lastSeenStr.String != "" {
		if t, err := parseSQLiteTime(lastSeenStr.String); err == nil {
			lastSeenMs = t.UnixMilli()
		}
	} else {
		lastSeenMs = time.Now().UnixMilli()
	}

	// Update lastSeen in database
	nowStr := time.Now().UTC().Format("2006-01-02 15:04:05.000 +00:00")
	_, dbErr := sa.db.Exec("UPDATE users SET lastSeen = ?, updatedAt = ? WHERE id = ?", nowStr, nowStr, userSession.ID)
	if dbErr != nil {
		log.Printf("[Socket] Failed to update user lastSeen in database: %v", dbErr)
	}

	log.Printf("[SocketAuthority] User Online: %s", userSession.Username)

	// Broadcast user_online to admins
	onlineUser := PublicUser{
		ID:        userSession.ID,
		Username:  userSession.Username,
		Type:      userSession.Type,
		Session:   nil,
		LastSeen:  lastSeenMs,
		CreatedAt: createdAtMs,
	}
	sa.AdminEmitter("user_online", onlineUser)

	// Emit init event
	initialPayload := map[string]interface{}{
		"userId":   userSession.ID,
		"username": userSession.Username,
	}
	if userSession.Type == "root" || userSession.Type == "admin" {
		initialPayload["usersOnline"] = sa.GetUsersOnline()
	}
	client.Emit("init", initialPayload)
}

// GetUsersOnline returns a list of online users for admins
func (sa *SocketAuthority) GetUsersOnline() []OnlineUser {
	sa.mu.RLock()
	defer sa.mu.RUnlock()

	onlineUsersMap := make(map[string]*OnlineUser)
	for _, client := range sa.clients {
		if client.User == nil {
			continue
		}

		if existing, ok := onlineUsersMap[client.User.ID]; ok {
			existing.Connections++
		} else {
			onlineUsersMap[client.User.ID] = &OnlineUser{
				ID:               client.User.ID,
				Username:         client.User.Username,
				Type:             client.User.Type,
				IsActive:         client.User.IsActive,
				Connections:      1,
				LastSeen:         time.Now().UnixMilli(),
				Session:          nil,
				PlaybackSessions: []interface{}{},
			}
		}
	}

	var users []OnlineUser
	for _, u := range onlineUsersMap {
		users = append(users, *u)
	}
	return users
}

// RemoveClient deletes a client and cleans up their state on disconnect
func (sa *SocketAuthority) RemoveClient(socketID string) {
	sa.mu.Lock()
	client, exists := sa.clients[socketID]
	if exists {
		delete(sa.clients, socketID)
	}
	sa.mu.Unlock()

	if exists && client.User != nil {
		log.Printf("[SocketAuthority] User Offline %s", client.User.Username)

		// Broadcast user_offline to admins
		onlineUser := PublicUser{
			ID:        client.User.ID,
			Username:  client.User.Username,
			Type:      client.User.Type,
			Session:   nil,
			LastSeen:  time.Now().UnixMilli(),
			CreatedAt: 0,
		}
		sa.AdminEmitter("user_offline", onlineUser)

		// Cancel cover searches
		sa.cancelSocketCoverSearches(socketID)
	}
}

// RequireAdminSocket checks if the socket client is an admin
func (sa *SocketAuthority) RequireAdminSocket(client *socket.Socket, eventName string) bool {
	sa.mu.RLock()
	c, ok := sa.clients[string(client.Id())]
	sa.mu.RUnlock()

	if ok && c.User != nil && (c.User.Type == "root" || c.User.Type == "admin") {
		return true
	}

	log.Printf("[SocketAuthority] Unauthorized %s socket event from socket %s", eventName, client.Id())
	return false
}

// HandleCoverSearch begins a simulated cover search process asynchronously
func (sa *SocketAuthority) HandleCoverSearch(client *socket.Socket, payload map[string]interface{}) {
	reqID, _ := payload["requestId"].(string)
	title, _ := payload["title"].(string)
	if reqID == "" || title == "" {
		client.Emit("cover_search_error", map[string]string{
			"requestId": reqID,
			"error":     "Invalid request parameters",
		})
		return
	}

	sa.mu.Lock()
	c, ok := sa.clients[string(client.Id())]
	sa.mu.Unlock()

	if !ok || c.User == nil {
		client.Emit("cover_search_error", map[string]string{
			"requestId": reqID,
			"error":     "Unauthorized",
		})
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	sa.activeSearchLock.Lock()
	if sa.activeSearches == nil {
		sa.activeSearches = make(map[string]context.CancelFunc)
	}
	sa.activeSearches[reqID] = cancel
	sa.activeSearchLock.Unlock()

	go func() {
		defer func() {
			sa.activeSearchLock.Lock()
			delete(sa.activeSearches, reqID)
			sa.activeSearchLock.Unlock()
		}()

		select {
		case <-ctx.Done():
			return
		case <-time.After(100 * time.Millisecond):
			client.Emit("cover_search_result", map[string]interface{}{
				"requestId": reqID,
				"provider":  "openlibrary",
				"covers":    []interface{}{},
				"total":     0,
			})
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(50 * time.Millisecond):
			client.Emit("cover_search_complete", map[string]interface{}{
				"requestId": reqID,
			})
		}
	}()
}

// HandleCancelCoverSearch cancels an ongoing cover search
func (sa *SocketAuthority) HandleCancelCoverSearch(client *socket.Socket, reqID string) {
	sa.activeSearchLock.Lock()
	cancel, ok := sa.activeSearches[reqID]
	if ok {
		delete(sa.activeSearches, reqID)
	}
	sa.activeSearchLock.Unlock()

	if ok {
		cancel()
		client.Emit("cover_search_cancelled", map[string]string{
			"requestId": reqID,
		})
	}
}

func (sa *SocketAuthority) cancelSocketCoverSearches(socketID string) {
	log.Printf("[SocketAuthority] Socket %s disconnected, any active searches will timeout", socketID)
}

// Close gracefully closes the Socket.IO server
func (sa *SocketAuthority) Close() {
	if sa.io != nil {
		sa.io.Close(nil)
	}
}

// validateToken parses the JWT token using standard auth.go claims
func validateToken(tokenStr string, tokenSecret string) (string, error) {
	claims := &AuthClaims{}
	token, err := jwt.ParseWithClaims(tokenStr, claims, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(tokenSecret), nil
	})
	if err != nil || !token.Valid {
		return "", fmt.Errorf("invalid token: %v", err)
	}
	return claims.UserID, nil
}

// CheckCanAccessLibraryItem checks if a user session has permissions to view a library item map representation
func (u *UserSession) CheckCanAccessLibraryItem(item map[string]interface{}) bool {
	libID, _ := item["libraryId"].(string)
	if libID == "" {
		// Fallback
		if media, ok := item["media"].(map[string]interface{}); ok {
			libID, _ = media["libraryId"].(string)
		}
	}
	if !u.CanAccessLibrary(libID) {
		return false
	}

	var isExplicit bool
	if media, ok := item["media"].(map[string]interface{}); ok {
		if exp, ok := media["explicit"].(bool); ok && exp {
			isExplicit = true
		}
		if metadata, ok := media["metadata"].(map[string]interface{}); ok {
			if exp, ok := metadata["explicit"].(bool); ok && exp {
				isExplicit = true
			}
		}
	}
	if isExplicit && !u.CanAccessExplicitContent {
		return false
	}

	var tags []string
	if media, ok := item["media"].(map[string]interface{}); ok {
		if rawTags, ok := media["tags"].([]interface{}); ok {
			for _, t := range rawTags {
				if ts, ok := t.(string); ok {
					tags = append(tags, ts)
				}
			}
		}
	}
	return u.CheckCanAccessLibraryItemWithTags(tags)
}

// CheckCanAccessLibraryItemWithTags validates tag filters
func (u *UserSession) CheckCanAccessLibraryItemWithTags(tags []string) bool {
	if u.AccessAllTags {
		return true
	}

	selectedTags := make(map[string]bool)
	for _, t := range u.ItemTagsSelected {
		selectedTags[t] = true
	}

	if u.SelectedTagsNotAccessible {
		if len(tags) == 0 {
			return true
		}
		for _, t := range tags {
			if selectedTags[t] {
				return false
			}
		}
		return true
	}

	if len(tags) == 0 {
		return false
	}
	for _, t := range tags {
		if selectedTags[t] {
			return true
		}
	}
	return false
}

// Emitter emits an event to all authorized clients
func (sa *SocketAuthority) Emitter(evt string, data interface{}, filter func(*UserSession) bool) {
	sa.mu.RLock()
	defer sa.mu.RUnlock()

	for _, client := range sa.clients {
		if client.User != nil {
			if filter != nil && !filter(client.User) {
				continue
			}
			client.Socket.Emit(evt, data)
		}
	}
}

// ClientEmitter emits an event to all clients of a specific user
func (sa *SocketAuthority) ClientEmitter(userID string, evt string, data interface{}) {
	sa.mu.RLock()
	defer sa.mu.RUnlock()

	for _, client := range sa.clients {
		if client.User != nil && client.User.ID == userID {
			client.Socket.Emit(evt, data)
		}
	}
}

// AdminEmitter emits an event to all connected admin clients
func (sa *SocketAuthority) AdminEmitter(evt string, data interface{}) {
	sa.mu.RLock()
	defer sa.mu.RUnlock()

	for _, client := range sa.clients {
		if client.User != nil && (client.User.Type == "root" || client.User.Type == "admin") {
			client.Socket.Emit(evt, data)
		}
	}
}

// BroadcastToAll broadcasts to all connected clients
func (sa *SocketAuthority) BroadcastToAll(evt string, data interface{}) {
	sa.Emitter(evt, data, nil)
}

// BroadcastToUser broadcasts to all clients of a specific user
func (sa *SocketAuthority) BroadcastToUser(userID string, evt string, data interface{}) {
	sa.ClientEmitter(userID, evt, data)
}

// BroadcastToAdmins broadcasts to all connected admin clients
func (sa *SocketAuthority) BroadcastToAdmins(evt string, data interface{}) {
	sa.AdminEmitter(evt, data)
}

// LibraryItemEmitter emits an event with a single library item, filtered by user library item permissions
func (sa *SocketAuthority) LibraryItemEmitter(evt string, item map[string]interface{}) {
	sa.mu.RLock()
	defer sa.mu.RUnlock()

	for _, client := range sa.clients {
		if client.User != nil && client.User.CheckCanAccessLibraryItem(item) {
			client.Socket.Emit(evt, item)
		}
	}
}

// LibraryItemsEmitter emits an event with multiple library items, filtered by user library item permissions
func (sa *SocketAuthority) LibraryItemsEmitter(evt string, items []map[string]interface{}) {
	sa.mu.RLock()
	defer sa.mu.RUnlock()

	for _, client := range sa.clients {
		if client.User != nil {
			var accessibleItems []map[string]interface{}
			for _, item := range items {
				if client.User.CheckCanAccessLibraryItem(item) {
					accessibleItems = append(accessibleItems, item)
				}
			}
			if len(accessibleItems) > 0 {
				client.Socket.Emit(evt, accessibleItems)
			}
		}
	}
}

// InitSocketAuthority initializes the global Socket.IO server and handlers
func InitSocketAuthority(db *sql.DB) http.Handler {
	sa := NewSocketAuthority(db)
	SocketAuth = sa

	opts := socket.DefaultServerOptions()
	cors := &types.Cors{
		Origin:      "*",
		Methods:     []string{"GET", "POST"},
		Credentials: true,
	}
	opts.SetCors(cors)

	io := socket.NewServer(nil, opts)
	sa.io = io

	io.On("connection", func(clients ...any) {
		client := clients[0].(*socket.Socket)
		socketID := string(client.Id())

		log.Printf("[SocketAuthority] Socket Connected: %s", socketID)

		sa.mu.Lock()
		sa.clients[socketID] = &SocketClient{
			ID:          socketID,
			Socket:      client,
			ConnectedAt: time.Now(),
		}
		sa.mu.Unlock()

		// Auth
		client.On("auth", func(args ...any) {
			if len(args) == 0 {
				return
			}
			token, ok := args[0].(string)
			if !ok {
				log.Printf("[Socket] Auth payload is not a string")
				client.Emit("auth_failed", map[string]string{"message": "Invalid token"})
				return
			}
			sa.AuthenticateSocket(client, token)
		})

		// Cancel scan
		client.On("cancel_scan", func(args ...any) {
			if !sa.RequireAdminSocket(client, "cancel_scan") {
				return
			}
			if len(args) == 0 {
				return
			}
			libraryID, _ := args[0].(string)
			log.Printf("[Socket] Cancel scan library: %s", libraryID)
		})

		// Cover search
		client.On("search_covers", func(args ...any) {
			if len(args) == 0 {
				return
			}
			payload, ok := args[0].(map[string]interface{})
			if !ok {
				log.Printf("[Socket] Cover search payload is not a map")
				return
			}
			sa.HandleCoverSearch(client, payload)
		})

		client.On("cancel_cover_search", func(args ...any) {
			if len(args) == 0 {
				return
			}
			reqID, _ := args[0].(string)
			sa.HandleCancelCoverSearch(client, reqID)
		})

		// Log listener
		client.On("set_log_listener", func(args ...any) {
			if !sa.RequireAdminSocket(client, "set_log_listener") {
				return
			}
		})

		client.On("remove_log_listener", func(args ...any) {
		})

		// message_all_users
		client.On("message_all_users", func(args ...any) {
			if len(args) == 0 {
				return
			}
			payload, ok := args[0].(map[string]interface{})
			if !ok {
				return
			}
			sa.mu.RLock()
			clientSession := sa.clients[socketID]
			sa.mu.RUnlock()

			if clientSession != nil && clientSession.User != nil && (clientSession.User.Type == "root" || clientSession.User.Type == "admin") {
				msg, _ := payload["message"].(string)
				sa.Emitter("admin_message", msg, nil)
			} else {
				log.Printf("[Socket] Non-admin user sent the message_all_users event")
			}
		})

		// Ping
		client.On("ping", func(args ...any) {
			client.Emit("pong")
		})

		// Disconnect
		client.On("disconnect", func(reason ...any) {
			r := "unknown"
			if len(reason) > 0 {
				if rs, ok := reason[0].(string); ok {
					r = rs
				}
			}
			log.Printf("[SocketAuthority] Socket %s disconnected (Reason: %s)", socketID, r)
			sa.RemoveClient(socketID)
		})
	})

	return io.ServeHandler(nil)
}
