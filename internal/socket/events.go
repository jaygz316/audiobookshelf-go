package socket

import (
	"net/http"
	"time"

	"github.com/zishang520/engine.io/v2/types"
	gosocket "github.com/zishang520/socket.io/v2/socket"

	log "audiobookshelf/internal/logger"
)

// InitAuthority initializes the global Socket.IO server and handlers.
func InitAuthority(database interface {
	QueryRow(query string, args ...interface{}) interface {
		Scan(dest ...interface{}) error
	}
}) http.Handler {
	return nil // placeholder
}

// InitSocketAuthority initializes the global Socket.IO server and handlers using a proper db.
func InitSocketAuthority(sa *Authority) http.Handler {
	opts := gosocket.DefaultServerOptions()
	cors := &types.Cors{
		Origin:      "*",
		Methods:     []string{"GET", "POST"},
		Credentials: true,
	}
	opts.SetCors(cors)

	server := gosocket.NewServer(nil, opts)
	sa.io = server

	server.On("connection", func(clients ...any) {
		client := clients[0].(*gosocket.Socket)
		socketID := string(client.Id())

		log.Printf("[SocketAuthority] Socket Connected: %s", socketID)

		sa.mu.Lock()
		sa.clients[socketID] = &SocketClient{
			ID:          socketID,
			Socket:      client,
			ConnectedAt: time.Now(),
		}
		sa.mu.Unlock()

		sa.registerSocketEvents(client)
	})

	return server.ServeHandler(nil)
}

// registerSocketEvents binds all client-side event handlers to the socket instance.
func (sa *Authority) registerSocketEvents(client *gosocket.Socket) {
	client.On("auth", func(args ...any) { sa.handleAuth(client, args...) })
	client.On("cancel_scan", func(args ...any) { sa.handleCancelScan(client, args...) })
	client.On("search_covers", func(args ...any) { sa.handleSearchCovers(client, args...) })
	client.On("cancel_cover_search", func(args ...any) { sa.handleCancelCoverSearch(client, args...) })
	client.On("set_log_listener", func(args ...any) { sa.handleSetLogListener(client, args...) })
	client.On("remove_log_listener", func(args ...any) { sa.handleRemoveLogListener(client, args...) })
	client.On("message_all_users", func(args ...any) { sa.handleMessageAllUsers(client, args...) })
	client.On("ping", func(args ...any) { sa.handlePing(client, args...) })
	client.On("disconnect", func(args ...any) { sa.handleDisconnect(client, args...) })
}

// handleAuth handles the user socket authentication event.
func (sa *Authority) handleAuth(client *gosocket.Socket, args ...any) {
	if len(args) == 0 || args[0] == nil {
		return
	}
	token, ok := args[0].(string)
	if !ok {
		log.Printf("[Socket] Auth payload is not a string")
		client.Emit("auth_failed", map[string]string{"message": "Invalid token"})
		return
	}
	sa.AuthenticateSocket(client, token)
}

// handleCancelScan processes requests to cancel a library scan.
func (sa *Authority) handleCancelScan(client *gosocket.Socket, args ...any) {
	if !sa.RequireAdminSocket(client, "cancel_scan") {
		return
	}
	if len(args) == 0 {
		return
	}
	libraryID, _ := args[0].(string)
	log.Printf("[Socket] Cancel scan library: %s", libraryID)
}

// handleMessageAllUsers handles admin-only messages to all connected clients.
func (sa *Authority) handleMessageAllUsers(client *gosocket.Socket, args ...any) {
	if len(args) == 0 {
		return
	}
	payload, ok := args[0].(map[string]interface{})
	if !ok {
		return
	}
	socketID := string(client.Id())
	sa.mu.RLock()
	clientSession := sa.clients[socketID]
	sa.mu.RUnlock()

	if clientSession != nil && clientSession.User != nil && (clientSession.User.Type == "root" || clientSession.User.Type == "admin") {
		msg, _ := payload["message"].(string)
		sa.Emitter("admin_message", msg, nil)
	} else {
		log.Printf("[Socket] Non-admin user sent the message_all_users event")
	}
}

// handlePing replies with a pong to keep client connected.
func (sa *Authority) handlePing(client *gosocket.Socket, args ...any) {
	client.Emit("pong")
}

// handleDisconnect removes the client and cleans up their state on disconnect.
func (sa *Authority) handleDisconnect(client *gosocket.Socket, reason ...any) {
	r := "unknown"
	if len(reason) > 0 {
		if rs, ok := reason[0].(string); ok {
			r = rs
		}
	}
	socketID := string(client.Id())
	log.Printf("[SocketAuthority] Socket %s disconnected (Reason: %s)", socketID, r)
	sa.RemoveClient(socketID)
}
