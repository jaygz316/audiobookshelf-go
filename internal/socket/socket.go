// Package socket provides the WebSocket authority for managing connected clients.
package socket

import (
	"context"
	"database/sql"
	"sync"
	"time"

	gosocket "github.com/zishang520/socket.io/v2/socket"

	"audiobookshelf/internal/core"
)

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

// Close gracefully closes the Socket.IO server.
func (sa *Authority) Close() {
	if sa.io != nil {
		sa.io.Close(nil)
	}
}
