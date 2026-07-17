package socket

import (
	"audiobookshelf/internal/core"
)

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
