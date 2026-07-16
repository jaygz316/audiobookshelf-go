package scanner

import (
	"encoding/json"

	isocket "audiobookshelf/internal/socket"
)

// EmitLibraryItemEvent emits a WebSocket event for a single library item.
func EmitLibraryItemEvent(socketAuth *isocket.Authority, evt string, item *LibraryItemMinifiedJSON) {
	if socketAuth == nil {
		return
	}
	data, err := json.Marshal(item)
	if err != nil {
		return
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err == nil {
		socketAuth.LibraryItemEmitter(evt, m)
	}
}

// EmitLibraryItemsEvent emits a WebSocket event for multiple library items.
func EmitLibraryItemsEvent(socketAuth *isocket.Authority, evt string, item *LibraryItemMinifiedJSON) {
	if socketAuth == nil {
		return
	}
	data, err := json.Marshal(item)
	if err != nil {
		return
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err == nil {
		socketAuth.LibraryItemsEmitter(evt, []map[string]interface{}{m})
	}
}
