package socket

import (
	gosocket "github.com/zishang520/socket.io/v2/socket"
)

// handleSetLogListener updates log level subscriptions for client.
func (sa *Authority) handleSetLogListener(client *gosocket.Socket, args ...any) {
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

// handleRemoveLogListener deletes log level subscriptions for client.
func (sa *Authority) handleRemoveLogListener(client *gosocket.Socket, args ...any) {
	sa.logListenersMu.Lock()
	delete(sa.logListeners, string(client.Id()))
	sa.logListenersMu.Unlock()
}
