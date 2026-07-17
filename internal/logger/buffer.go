package logger

import (
	"sync"

	"audiobookshelf/internal/core"
)

// LogMessage is an alias for the type in internal/core.
type LogMessage = core.LogMessage

// LogBuffer is a thread-safe ring buffer for LogMessages
type LogBuffer struct {
	mu       sync.Mutex
	messages []LogMessage
	maxSize  int
	start    int
}

// NewLogBuffer creates a LogBuffer with specified size cap
func NewLogBuffer(maxSize int) *LogBuffer {
	return &LogBuffer{
		messages: make([]LogMessage, 0, maxSize),
		maxSize:  maxSize,
	}
}

// Add appends a log entry, evicting the oldest if capacity is exceeded
func (lb *LogBuffer) Add(msg LogMessage) {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	if len(lb.messages) < lb.maxSize {
		lb.messages = append(lb.messages, msg)
	} else {
		lb.messages[lb.start] = msg
		lb.start = (lb.start + 1) % lb.maxSize
	}
}

// Get returns a shallow copy of the cached log entries
func (lb *LogBuffer) Get() []LogMessage {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	copied := make([]LogMessage, len(lb.messages))
	if len(lb.messages) < lb.maxSize {
		copy(copied, lb.messages)
	} else {
		n := copy(copied, lb.messages[lb.start:])
		copy(copied[n:], lb.messages[:lb.start])
	}
	return copied
}

// GlobalLogBuffer stores up to 2000 log lines
var GlobalLogBuffer = NewLogBuffer(2000)

// LogCallback is a callback to broadcast logs (typically via WebSocket).
var LogCallback func(LogMessage)
