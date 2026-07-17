package socket

import (
	"time"

	gosocket "github.com/zishang520/socket.io/v2/socket"

	log "audiobookshelf/internal/logger"
)

// GetUsersOnline returns a list of online users for admins.
func (sa *Authority) GetUsersOnline() []OnlineUser {
	sa.mu.RLock()

	type tempUser struct {
		id       string
		username string
		uType    string
		isActive bool
		conns    int
	}

	tempUsers := make(map[string]*tempUser)
	for _, client := range sa.clients {
		if client.User == nil {
			continue
		}

		if existing, ok := tempUsers[client.User.ID]; ok {
			existing.conns++
		} else {
			tempUsers[client.User.ID] = &tempUser{
				id:       client.User.ID,
				username: client.User.Username,
				uType:    client.User.Type,
				isActive: client.User.IsActive,
				conns:    1,
			}
		}
	}
	sa.mu.RUnlock()

	var users []OnlineUser
	for _, tu := range tempUsers {
		users = append(users, OnlineUser{
			ID:               tu.id,
			Username:         tu.username,
			Type:             tu.uType,
			IsActive:         tu.isActive,
			Connections:      tu.conns,
			LastSeen:         time.Now().UnixMilli(),
			Session:          nil,
			PlaybackSessions: sa.getPlaybackSessionsForUser(tu.id),
		})
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
