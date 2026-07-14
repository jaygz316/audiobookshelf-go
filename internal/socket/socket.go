// Package socket provides the WebSocket authority for managing connected clients.
package socket

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	gosocket "github.com/zishang520/socket.io/v2/socket"

	"audiobookshelf/internal/core"
	"audiobookshelf/internal/db"
	log "audiobookshelf/internal/logger"
)

// PublicUser represents the public user structure sent on online/offline events.
type PublicUser struct {
	ID        string      `json:"id"`
	Username  string      `json:"username"`
	Type      string      `json:"type"`
	Session   interface{} `json:"session"` // can be nil
	LastSeen  int64       `json:"lastSeen"`
	CreatedAt int64       `json:"createdAt"`
}

// OnlineUser represents the online user structure for admins.
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

// SocketClient wraps a socket connection with authentication information.
type SocketClient struct {
	ID          string
	Socket      *gosocket.Socket
	ConnectedAt time.Time
	User        *core.UserSession // Loaded from auth middleware
}

// Authority manages WebSocket connections and dynamic permission filtering.
type Authority struct {
	mu               sync.RWMutex
	clients          map[string]*SocketClient
	database         *sql.DB
	io               *gosocket.Server
	activeSearches   map[string]context.CancelFunc
	activeSearchLock sync.Mutex
	logListeners     map[string]int
	logListenersMu   sync.RWMutex
}

// GlobalAuth is the global reference to the socket authority.
var GlobalAuth *Authority

// NewAuthority creates a new instance of Authority.
func NewAuthority(database *sql.DB) *Authority {
	return &Authority{
		clients:        make(map[string]*SocketClient),
		database:       database,
		activeSearches: make(map[string]context.CancelFunc),
		logListeners:   make(map[string]int),
	}
}

// SetDB updates the database connection used by the Authority.
func (sa *Authority) SetDB(database *sql.DB) {
	sa.mu.Lock()
	defer sa.mu.Unlock()
	sa.database = database
}

// AuthenticateSocket validates the JWT token and binds the user session to the socket.
func (sa *Authority) AuthenticateSocket(client *gosocket.Socket, token string) {
	userSession, err := sa.resolveUserSession(token)
	if err != nil {
		log.Printf("[Socket] Auth failed: %v", err)
		client.Emit("auth_failed", map[string]string{"message": "Invalid token"})
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

	createdAtMs, lastSeenMs, err := sa.updateUserActivity(userSession.ID)
	if err != nil {
		log.Printf("[Socket] Failed to update user activity: %v", err)
	}

	sa.broadcastUserOnline(client, userSession, createdAtMs, lastSeenMs)
}

// resolveUserSession decodes the token and retrieves the corresponding session.
func (sa *Authority) resolveUserSession(token string) (*core.UserSession, error) {
	tokenSecret := db.GetTokenSecret(sa.database)
	userID, err := validateToken(token, tokenSecret)
	if err != nil {
		return nil, fmt.Errorf("token validation failed: %w", err)
	}

	userSession, err := db.GetUserByIDOrOldID(sa.database, userID)
	if err != nil {
		return nil, fmt.Errorf("user lookup failed: %w", err)
	}

	if !userSession.IsActive {
		return nil, fmt.Errorf("user %s is inactive", userSession.Username)
	}

	return userSession, nil
}

// updateUserActivity updates user activity timestamps in the database.
func (sa *Authority) updateUserActivity(userID string) (createdAtMs int64, lastSeenMs int64, err error) {
	var createdAtStr, lastSeenStr sql.NullString
	err = sa.database.QueryRow("SELECT createdAt, lastSeen FROM users WHERE id = ?", userID).Scan(&createdAtStr, &lastSeenStr)
	if err != nil {
		return 0, 0, err
	}

	if createdAtStr.Valid && createdAtStr.String != "" {
		if t, err := db.ParseSQLiteTime(createdAtStr.String); err == nil {
			createdAtMs = t.UnixMilli()
		}
	}
	if lastSeenStr.Valid && lastSeenStr.String != "" {
		if t, err := db.ParseSQLiteTime(lastSeenStr.String); err == nil {
			lastSeenMs = t.UnixMilli()
		}
	} else {
		lastSeenMs = time.Now().UnixMilli()
	}

	nowStr := time.Now().UTC().Format("2006-01-02 15:04:05.000 +00:00")
	_, err = sa.database.Exec("UPDATE users SET lastSeen = ?, updatedAt = ? WHERE id = ?", nowStr, nowStr, userID)
	return createdAtMs, lastSeenMs, err
}

// broadcastUserOnline handles post-login user broadcasting and client initialization.
func (sa *Authority) broadcastUserOnline(client *gosocket.Socket, userSession *core.UserSession, createdAtMs, lastSeenMs int64) {
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
		"userId":           userSession.ID,
		"username":         userSession.Username,
		"playbackSessions": sa.getPlaybackSessionsForUser(userSession.ID),
	}
	if userSession.Type == "root" || userSession.Type == "admin" {
		initialPayload["usersOnline"] = sa.GetUsersOnline()
	}
	client.Emit("init", initialPayload)
}

// GetUsersOnline returns a list of online users for admins.
func (sa *Authority) GetUsersOnline() []OnlineUser {
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
				PlaybackSessions: sa.getPlaybackSessionsForUser(client.User.ID),
			}
		}
	}

	var users []OnlineUser
	for _, u := range onlineUsersMap {
		users = append(users, *u)
	}
	return users
}

// ClientCount returns the number of currently connected clients.
func (sa *Authority) ClientCount() int {
	sa.mu.RLock()
	defer sa.mu.RUnlock()
	return len(sa.clients)
}

// RemoveClient deletes a client and cleans up their state on disconnect.
func (sa *Authority) RemoveClient(socketID string) {
	sa.mu.Lock()
	client, exists := sa.clients[socketID]
	if exists {
		delete(sa.clients, socketID)
	}
	sa.mu.Unlock()

	sa.logListenersMu.Lock()
	delete(sa.logListeners, socketID)
	sa.logListenersMu.Unlock()

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

// RequireAdminSocket checks if the socket client is an admin.
func (sa *Authority) RequireAdminSocket(client *gosocket.Socket, eventName string) bool {
	sa.mu.RLock()
	c, ok := sa.clients[string(client.Id())]
	sa.mu.RUnlock()

	if ok && c.User != nil && (c.User.Type == "root" || c.User.Type == "admin") {
		return true
	}

	log.Printf("[SocketAuthority] Unauthorized %s socket event from socket %s", eventName, client.Id())
	return false
}

// Close gracefully closes the Socket.IO server.
func (sa *Authority) Close() {
	if sa.io != nil {
		sa.io.Close(nil)
	}
}

// validateToken parses the JWT token using standard auth claims.
func validateToken(tokenStr string, tokenSecret string) (string, error) {
	claims := &core.AuthClaims{}
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

// ValidateToken is an exported wrapper for JWT token validation.
func ValidateToken(tokenStr string, tokenSecret string) (string, error) {
	return validateToken(tokenStr, tokenSecret)
}

// Emitter emits an event to all authorized clients.
func (sa *Authority) Emitter(evt string, data interface{}, filter func(*core.UserSession) bool) {
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

// ClientEmitter emits an event to all clients of a specific user.
func (sa *Authority) ClientEmitter(userID string, evt string, data interface{}) {
	sa.mu.RLock()
	defer sa.mu.RUnlock()

	for _, client := range sa.clients {
		if client.User != nil && client.User.ID == userID {
			client.Socket.Emit(evt, data)
		}
	}
}

// BroadcastToAll broadcasts an event to all connected authenticated users.
func (sa *Authority) BroadcastToAll(evt string, data interface{}) {
	sa.Emitter(evt, data, nil)
}

// BroadcastToUser broadcasts an event to all connected sockets of a user.
func (sa *Authority) BroadcastToUser(userID string, evt string, data interface{}) {
	sa.ClientEmitter(userID, evt, data)
}

// BroadcastToAdmins broadcasts an event to all connected admin sockets.
func (sa *Authority) BroadcastToAdmins(evt string, data interface{}) {
	sa.AdminEmitter(evt, data)
}

// AdminEmitter emits an event to all connected admin clients.
func (sa *Authority) AdminEmitter(evt string, data interface{}) {
	sa.mu.RLock()
	defer sa.mu.RUnlock()

	for _, client := range sa.clients {
		if client.User != nil && (client.User.Type == "root" || client.User.Type == "admin") {
			client.Socket.Emit(evt, data)
		}
	}
}

// LibraryItemEmitter emits an event with a single library item, filtered by user permissions.
func (sa *Authority) LibraryItemEmitter(evt string, item map[string]interface{}) {
	sa.mu.RLock()
	defer sa.mu.RUnlock()

	for _, client := range sa.clients {
		if client.User != nil && client.User.CheckCanAccessLibraryItem(item) {
			client.Socket.Emit(evt, item)
		}
	}
}

// LibraryItemsEmitter emits an event with multiple library items, filtered by user permissions.
func (sa *Authority) LibraryItemsEmitter(evt string, items []map[string]interface{}) {
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

// BroadcastLog sends a log message to all connected clients registered for this log level or lower.
func (sa *Authority) BroadcastLog(msg core.LogMessage) {
	sa.logListenersMu.RLock()
	defer sa.logListenersMu.RUnlock()

	sa.mu.RLock()
	defer sa.mu.RUnlock()

	for socketID, minLevel := range sa.logListeners {
		if msg.Level >= minLevel {
			if client, ok := sa.clients[socketID]; ok && client.Socket != nil {
				client.Socket.Emit("log", msg)
			}
		}
	}
}

func (sa *Authority) getPlaybackSessionsForUser(userID string) []interface{} {
	sa.mu.RLock()
	dbConn := sa.database
	sa.mu.RUnlock()
	if dbConn == nil {
		return []interface{}{}
	}

	query := `
		SELECT ps.id, ps.userId, u.username, ps.mediaItemId, ps.mediaItemType, ps.startTime, 
		       COALESCE(ps.updatedAt, ps.createdAt, '') as updatedAt, COALESCE(ps.extraData, '') as extraData,
		       CASE 
		           WHEN ps.mediaItemType = 'podcastEpisode' THEN COALESCE(pe.title, '')
		           WHEN ps.mediaItemType = 'podcast' THEN COALESCE(p.title, '')
		           ELSE COALESCE(b.title, '')
		       END as title,
		       CASE 
		           WHEN ps.mediaItemType = 'podcastEpisode' THEN COALESCE(p.author, '')
		           WHEN ps.mediaItemType = 'podcast' THEN COALESCE(p.author, '')
		           ELSE COALESCE(li.authorNamesFirstLast, '')
		       END as author
		FROM playbackSessions ps
		LEFT JOIN users u ON u.id = ps.userId
		LEFT JOIN books b ON b.id = ps.mediaItemId AND ps.mediaItemType = 'book'
		LEFT JOIN libraryItems li ON li.mediaId = ps.mediaItemId AND li.mediaType = 'book' AND ps.mediaItemType = 'book'
		LEFT JOIN podcastEpisodes pe ON pe.id = ps.mediaItemId AND ps.mediaItemType = 'podcastEpisode'
		LEFT JOIN podcasts p ON (p.id = pe.podcastId AND ps.mediaItemType = 'podcastEpisode') OR (p.id = ps.mediaItemId AND ps.mediaItemType = 'podcast')
		WHERE ps.userId = ?
		ORDER BY COALESCE(ps.updatedAt, ps.createdAt) DESC
	`
	rows, err := dbConn.Query(query, userID)
	if err != nil {
		log.Printf("[Socket] Failed to query playback sessions for user %s: %v", userID, err)
		return []interface{}{}
	}
	defer rows.Close()

	var sessions []interface{}
	for rows.Next() {
		var id, uID, username, mediaItemId, mediaItemType, updatedAt, extraDataStr, title, author string
		var startTime float64

		err := rows.Scan(
			&id, &uID, &username, &mediaItemId, &mediaItemType, &startTime, &updatedAt, &extraDataStr, &title, &author,
		)
		if err != nil {
			log.Printf("[Socket] Failed to scan playback session: %v", err)
			continue
		}

		playMethod := "HLS"
		deviceInfo := "Web Client"
		timeListened := 0.0
		lastTime := 0.0

		if extraDataStr != "" {
			var extra map[string]interface{}
			if err := json.Unmarshal([]byte(extraDataStr), &extra); err == nil {
				if val, ok := extra["playMethod"]; ok {
					if s, ok2 := val.(string); ok2 {
						playMethod = s
					}
				}
				if val, ok := extra["deviceInfo"]; ok {
					if s, ok2 := val.(string); ok2 {
						deviceInfo = s
					}
				}
				if val, ok := extra["timeListened"]; ok {
					if f, ok2 := val.(float64); ok2 {
						timeListened = f
					}
				}
				if val, ok := extra["lastTime"]; ok {
					if f, ok2 := val.(float64); ok2 {
						lastTime = f
					}
				}
			}
		}

		sessions = append(sessions, map[string]interface{}{
			"id":            id,
			"userId":        uID,
			"username":      username,
			"mediaItemId":   mediaItemId,
			"mediaItemType": mediaItemType,
			"title":         title,
			"author":        author,
			"startTime":     startTime,
			"timeListened":  timeListened,
			"lastTime":      lastTime,
			"updatedAt":     updatedAt,
			"playMethod":    playMethod,
			"deviceInfo":    deviceInfo,
		})
	}
	return sessions
}

func (sa *Authority) getPlaybackSessionByID(sessionID string) (map[string]interface{}, error) {
	sa.mu.RLock()
	dbConn := sa.database
	sa.mu.RUnlock()
	if dbConn == nil {
		return nil, fmt.Errorf("database connection is nil")
	}

	query := `
		SELECT ps.id, ps.userId, u.username, ps.mediaItemId, ps.mediaItemType, ps.startTime, 
		       COALESCE(ps.updatedAt, ps.createdAt, '') as updatedAt, COALESCE(ps.extraData, '') as extraData,
		       CASE 
		           WHEN ps.mediaItemType = 'podcastEpisode' THEN COALESCE(pe.title, '')
		           WHEN ps.mediaItemType = 'podcast' THEN COALESCE(p.title, '')
		           ELSE COALESCE(b.title, '')
		       END as title,
		       CASE 
		           WHEN ps.mediaItemType = 'podcastEpisode' THEN COALESCE(p.author, '')
		           WHEN ps.mediaItemType = 'podcast' THEN COALESCE(p.author, '')
		           ELSE COALESCE(li.authorNamesFirstLast, '')
		       END as author
		FROM playbackSessions ps
		LEFT JOIN users u ON u.id = ps.userId
		LEFT JOIN books b ON b.id = ps.mediaItemId AND ps.mediaItemType = 'book'
		LEFT JOIN libraryItems li ON li.mediaId = ps.mediaItemId AND li.mediaType = 'book' AND ps.mediaItemType = 'book'
		LEFT JOIN podcastEpisodes pe ON pe.id = ps.mediaItemId AND ps.mediaItemType = 'podcastEpisode'
		LEFT JOIN podcasts p ON (p.id = pe.podcastId AND ps.mediaItemType = 'podcastEpisode') OR (p.id = ps.mediaItemId AND ps.mediaItemType = 'podcast')
		WHERE ps.id = ?
	`
	var id, uID, username, mediaItemId, mediaItemType, updatedAt, extraDataStr, title, author string
	var startTime float64

	err := dbConn.QueryRow(query, sessionID).Scan(
		&id, &uID, &username, &mediaItemId, &mediaItemType, &startTime, &updatedAt, &extraDataStr, &title, &author,
	)
	if err != nil {
		return nil, err
	}

	playMethod := "HLS"
	deviceInfo := "Web Client"
	timeListened := 0.0
	lastTime := 0.0

	if extraDataStr != "" {
		var extra map[string]interface{}
		if err := json.Unmarshal([]byte(extraDataStr), &extra); err == nil {
			if val, ok := extra["playMethod"]; ok {
				if s, ok2 := val.(string); ok2 {
					playMethod = s
				}
			}
			if val, ok := extra["deviceInfo"]; ok {
				if s, ok2 := val.(string); ok2 {
					deviceInfo = s
				}
			}
			if val, ok := extra["timeListened"]; ok {
				if f, ok2 := val.(float64); ok2 {
					timeListened = f
				}
			}
			if val, ok := extra["lastTime"]; ok {
				if f, ok2 := val.(float64); ok2 {
					lastTime = f
				}
			}
		}
	}

	return map[string]interface{}{
		"id":            id,
		"userId":        uID,
		"username":      username,
		"mediaItemId":   mediaItemId,
		"mediaItemType": mediaItemType,
		"title":         title,
		"author":        author,
		"startTime":     startTime,
		"timeListened":  timeListened,
		"lastTime":      lastTime,
		"updatedAt":     updatedAt,
		"playMethod":    playMethod,
		"deviceInfo":    deviceInfo,
	}, nil
}

func (sa *Authority) BroadcastPlaybackSessionAdded(userID string, sessionID string) {
	sess, err := sa.getPlaybackSessionByID(sessionID)
	if err != nil {
		log.Printf("[Socket] Failed to retrieve playback session %s: %v", sessionID, err)
		return
	}
	sa.ClientEmitter(userID, "playback_session_added", sess)
	sa.AdminEmitter("playback_session_added", sess)
}

func (sa *Authority) BroadcastPlaybackSessionUpdated(userID string, sessionID string) {
	sess, err := sa.getPlaybackSessionByID(sessionID)
	if err != nil {
		log.Printf("[Socket] Failed to retrieve playback session %s: %v", sessionID, err)
		return
	}
	sa.ClientEmitter(userID, "playback_session_updated", sess)
	sa.AdminEmitter("playback_session_updated", sess)
}

func (sa *Authority) BroadcastPlaybackSessionRemoved(userID string, sessionID string) {
	payload := map[string]interface{}{
		"id":     sessionID,
		"userId": userID,
	}
	sa.ClientEmitter(userID, "playback_session_removed", payload)
	sa.AdminEmitter("playback_session_removed", payload)
}
