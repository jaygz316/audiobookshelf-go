package hls

import (
	"sync"

	"golang.org/x/sync/singleflight"
)

// StreamManager coordinates and tracks active stream sessions.
type StreamManager struct {
	streams   map[string]*Stream
	streamsMu sync.RWMutex
	sf        singleflight.Group
}

// NewStreamManager creates a new StreamManager instance.
func NewStreamManager() *StreamManager {
	return &StreamManager{streams: make(map[string]*Stream)}
}

// GetStream retrieves an active stream session by its ID.
func (sm *StreamManager) GetStream(id string) *Stream {
	sm.streamsMu.RLock()
	defer sm.streamsMu.RUnlock()
	return sm.streams[id]
}

// AddStream adds a new stream session to the manager.
func (sm *StreamManager) AddStream(s *Stream) {
	sm.streamsMu.Lock()
	defer sm.streamsMu.Unlock()
	sm.streams[s.ID] = s
}

// RemoveStream removes a stream session by its ID and closes it.
func (sm *StreamManager) RemoveStream(id string) {
	sm.streamsMu.Lock()
	s, ok := sm.streams[id]
	if ok {
		delete(sm.streams, id)
	}
	sm.streamsMu.Unlock()
	if ok {
		s.Close()
	}
}

// Close closes all active stream sessions managed by the StreamManager.
func (sm *StreamManager) Close() {
	sm.streamsMu.Lock()
	streams := make([]*Stream, 0, len(sm.streams))
	for id, s := range sm.streams {
		streams = append(streams, s)
		delete(sm.streams, id)
	}
	sm.streamsMu.Unlock()
	for _, s := range streams {
		s.Close()
	}
}
