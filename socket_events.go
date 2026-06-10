package main

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"time"

	"github.com/zishang520/engine.io/v2/types"
	"github.com/zishang520/socket.io/v2/socket"
)

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

		sa.registerSocketEvents(client)
	})

	return io.ServeHandler(nil)
}

// registerSocketEvents binds all client-side event handlers to the socket instance
func (sa *SocketAuthority) registerSocketEvents(client *socket.Socket) {
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

// handleAuth handles the user socket authentication event
func (sa *SocketAuthority) handleAuth(client *socket.Socket, args ...any) {
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

// handleCancelScan processes requests to cancel a library scan
func (sa *SocketAuthority) handleCancelScan(client *socket.Socket, args ...any) {
	if !sa.RequireAdminSocket(client, "cancel_scan") {
		return
	}
	if len(args) == 0 {
		return
	}
	libraryID, _ := args[0].(string)
	log.Printf("[Socket] Cancel scan library: %s", libraryID)
}

// handleSearchCovers initiates an asynchronous cover search process
func (sa *SocketAuthority) handleSearchCovers(client *socket.Socket, args ...any) {
	if len(args) == 0 {
		return
	}
	payload, ok := args[0].(map[string]interface{})
	if !ok {
		log.Printf("[Socket] Cover search payload is not a map")
		return
	}
	sa.HandleCoverSearch(client, payload)
}

// handleCancelCoverSearch processes a request to cancel a cover search
func (sa *SocketAuthority) handleCancelCoverSearch(client *socket.Socket, args ...any) {
	if len(args) == 0 {
		return
	}
	reqID, _ := args[0].(string)
	sa.HandleCancelCoverSearch(client, reqID)
}

// handleSetLogListener updates log level subscriptions for client
func (sa *SocketAuthority) handleSetLogListener(client *socket.Socket, args ...any) {
	if !sa.RequireAdminSocket(client, "set_log_listener") {
		return
	}
	level := 2 // Default: Info
	if len(args) > 0 {
		if val, ok := args[0].(float64); ok {
			level = int(val)
		} else if val, ok := args[0].(int); ok {
			level = val
		}
	}
	sa.logListenersMu.Lock()
	sa.logListeners[string(client.Id())] = level
	sa.logListenersMu.Unlock()
}

// handleRemoveLogListener deletes log level subscriptions for client
func (sa *SocketAuthority) handleRemoveLogListener(client *socket.Socket, args ...any) {
	sa.logListenersMu.Lock()
	delete(sa.logListeners, string(client.Id()))
	sa.logListenersMu.Unlock()
}

// handleMessageAllUsers handles admin-only messages to all connected clients
func (sa *SocketAuthority) handleMessageAllUsers(client *socket.Socket, args ...any) {
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

// handlePing replies with a pong to keep client connected
func (sa *SocketAuthority) handlePing(client *socket.Socket, args ...any) {
	client.Emit("pong")
}

// handleDisconnect removes the client and cleans up their state on disconnect
func (sa *SocketAuthority) handleDisconnect(client *socket.Socket, reason ...any) {
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

	sa.runAsyncCoverSearch(ctx, client, reqID)
}

// runAsyncCoverSearch spawns a goroutine to simulate the search
func (sa *SocketAuthority) runAsyncCoverSearch(ctx context.Context, client *socket.Socket, reqID string) {
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

// cancelSocketCoverSearches logs socket disconnect cleanup
func (sa *SocketAuthority) cancelSocketCoverSearches(socketID string) {
	log.Printf("[SocketAuthority] Socket %s disconnected, any active searches will timeout", socketID)
}
