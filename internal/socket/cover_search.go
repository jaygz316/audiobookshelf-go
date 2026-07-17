package socket

import (
	"context"
	"time"

	gosocket "github.com/zishang520/socket.io/v2/socket"

	log "audiobookshelf/internal/logger"
)

// handleSearchCovers initiates an asynchronous cover search process.
func (sa *Authority) handleSearchCovers(client *gosocket.Socket, args ...any) {
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

// handleCancelCoverSearch processes a request to cancel a cover search.
func (sa *Authority) handleCancelCoverSearch(client *gosocket.Socket, args ...any) {
	if len(args) == 0 {
		return
	}
	reqID, _ := args[0].(string)
	sa.HandleCancelCoverSearch(client, reqID)
}

// HandleCoverSearch begins a simulated cover search process asynchronously.
func (sa *Authority) HandleCoverSearch(client *gosocket.Socket, payload map[string]interface{}) {
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

// runAsyncCoverSearch spawns a goroutine to simulate the search.
func (sa *Authority) runAsyncCoverSearch(ctx context.Context, client *gosocket.Socket, reqID string) {
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

// HandleCancelCoverSearch cancels an ongoing cover search.
func (sa *Authority) HandleCancelCoverSearch(client *gosocket.Socket, reqID string) {
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

// cancelSocketCoverSearches logs socket disconnect cleanup.
func (sa *Authority) cancelSocketCoverSearches(socketID string) {
	log.Printf("[SocketAuthority] Socket %s disconnected, any active searches will timeout", socketID)
}
