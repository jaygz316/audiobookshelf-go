package main

import (
	"io"
	"strings"
	"sync"
	"time"
)

// LogMessage represents a single log entry formatted for the client
type LogMessage struct {
	Timestamp string `json:"timestamp"`
	Level     int    `json:"level"`
	LevelName string `json:"levelName"`
	Message   string `json:"message"`
}

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

// LogWriter is an io.Writer that intercepts logs, parses them, and stores them in the GlobalLogBuffer
type LogWriter struct {
	Stdout io.Writer
}

func (w *LogWriter) Write(p []byte) (n int, err error) {
	// Echo to stdout
	if w.Stdout != nil {
		_, _ = w.Stdout.Write(p)
	}

	msgStr := string(p)
	lines := strings.Split(msgStr, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		// Strip Go default log prefix: "2006/01/02 15:04:05 " (length 20)
		if len(trimmed) >= 20 && trimmed[4] == '/' && trimmed[7] == '/' && trimmed[10] == ' ' && trimmed[13] == ':' && trimmed[16] == ':' {
			trimmed = trimmed[20:]
		}

		level := 2
		levelName := "INFO"

		lowerLine := strings.ToLower(trimmed)
		if strings.Contains(lowerLine, "error") || strings.Contains(lowerLine, "failed") {
			level = 4
			levelName = "ERROR"
		} else if strings.Contains(lowerLine, "warn") || strings.Contains(lowerLine, "warning") {
			level = 3
			levelName = "WARN"
		} else if strings.Contains(lowerLine, "debug") || strings.Contains(lowerLine, "go") || strings.Contains(lowerLine, "backend") {
			level = 1
			levelName = "DEBUG"
		}

		logMsg := LogMessage{
			Timestamp: time.Now().Format("2006-01-02 15:04:05.000"),
			Level:     level,
			LevelName: levelName,
			Message:   trimmed,
		}

		GlobalLogBuffer.Add(logMsg)
		if SocketAuth != nil {
			SocketAuth.BroadcastLog(logMsg)
		}
	}
	return len(p), nil
}
