package logger

import (
	"context"
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

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

// SafeWriter wraps an io.Writer with a mutex to allow dynamic redirection.
type SafeWriter struct {
	mu sync.RWMutex
	w  io.Writer
}

func (sw *SafeWriter) Write(p []byte) (n int, err error) {
	sw.mu.RLock()
	defer sw.mu.RUnlock()
	return sw.w.Write(p)
}

func (sw *SafeWriter) Set(w io.Writer) {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	sw.w = w
}

func (sw *SafeWriter) Get() io.Writer {
	sw.mu.RLock()
	defer sw.mu.RUnlock()
	return sw.w
}

var globalSafeWriter = &SafeWriter{w: os.Stdout}

// ABSLogHandler wraps slog.Handler and intercepts logs to populate the UI GlobalLogBuffer.
type ABSLogHandler struct {
	next slog.Handler
}

func NewABSLogHandler(w io.Writer, format string, level slog.Level) *ABSLogHandler {
	var next slog.Handler
	opts := &slog.HandlerOptions{
		Level:     level,
		AddSource: false,
	}
	if format == "text" {
		next = slog.NewTextHandler(w, opts)
	} else {
		next = slog.NewJSONHandler(w, opts)
	}
	return &ABSLogHandler{next: next}
}

func (h *ABSLogHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *ABSLogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &ABSLogHandler{next: h.next.WithAttrs(attrs)}
}

func (h *ABSLogHandler) WithGroup(name string) slog.Handler {
	return &ABSLogHandler{next: h.next.WithGroup(name)}
}

func (h *ABSLogHandler) Handle(ctx context.Context, r slog.Record) error {
	err := h.next.Handle(ctx, r)

	// Format structured attributes for the UI log console
	msgStr := r.Message
	r.Attrs(func(attr slog.Attr) bool {
		msgStr += fmt.Sprintf(" %s=%v", attr.Key, attr.Value.Any())
		return true
	})

	level := 2
	levelName := "INFO"
	switch {
	case r.Level < slog.LevelInfo:
		level = 1
		levelName = "DEBUG"
	case r.Level < slog.LevelWarn:
		level = 2
		levelName = "INFO"
	case r.Level < slog.LevelError:
		level = 3
		levelName = "WARN"
	default:
		level = 4
		levelName = "ERROR"
	}

	logMsg := LogMessage{
		Timestamp: r.Time.Format("2006-01-02 15:04:05.000"),
		Level:     level,
		LevelName: levelName,
		Message:   msgStr,
	}

	GlobalLogBuffer.Add(logMsg)
	if LogCallback != nil {
		LogCallback(logMsg)
	}

	return err
}

var (
	defaultLogger *slog.Logger
	loggerMu      sync.RWMutex
)

// InitLogger initializes structured logging.
func InitLogger(format string, levelStr string) {
	loggerMu.Lock()
	defer loggerMu.Unlock()

	var level slog.Level
	switch strings.ToLower(levelStr) {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn", "warning":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	handler := NewABSLogHandler(globalSafeWriter, format, level)
	defaultLogger = slog.New(handler)
	slog.SetDefault(defaultLogger)

	// Redirect standard library's log to our slog handler
	log.SetOutput(slog.NewLogLogger(handler, slog.LevelInfo).Writer())
}

func GetLogger() *slog.Logger {
	loggerMu.RLock()
	l := defaultLogger
	loggerMu.RUnlock()

	if l == nil {
		// Initialize lazy default logger
		loggerMu.Lock()
		if defaultLogger == nil {
			var level slog.Level
			// Check env
			levelStr := os.Getenv("LOG_LEVEL")
			switch strings.ToLower(levelStr) {
			case "debug":
				level = slog.LevelDebug
			case "warn", "warning":
				level = slog.LevelWarn
			case "error":
				level = slog.LevelError
			default:
				level = slog.LevelInfo
			}
			format := os.Getenv("LOG_FORMAT")
			if format == "" {
				format = "json"
			}
			handler := NewABSLogHandler(globalSafeWriter, format, level)
			defaultLogger = slog.New(handler)
			slog.SetDefault(defaultLogger)
			log.SetOutput(slog.NewLogLogger(handler, slog.LevelInfo).Writer())
		}
		l = defaultLogger
		loggerMu.Unlock()
	}
	return l
}

// Log functions wrapping slog.Logger

func Debug(msg string, args ...any) {
	GetLogger().Debug(msg, args...)
}

func Info(msg string, args ...any) {
	GetLogger().Info(msg, args...)
}

func Warn(msg string, args ...any) {
	GetLogger().Warn(msg, args...)
}

func Error(msg string, args ...any) {
	GetLogger().Error(msg, args...)
}

func Fatal(msg string, args ...any) {
	GetLogger().Error(msg, args...)
	os.Exit(1)
}

func Print(args ...any) {
	GetLogger().Info(fmt.Sprint(args...))
}

func Printf(format string, args ...any) {
	GetLogger().Info(fmt.Sprintf(format, args...))
}

func Println(args ...any) {
	GetLogger().Info(fmt.Sprint(args...))
}

func Fatalf(format string, args ...any) {
	GetLogger().Error(fmt.Sprintf(format, args...))
	os.Exit(1)
}

func Debugf(format string, args ...any) {
	GetLogger().Debug(fmt.Sprintf(format, args...))
}

func Infof(format string, args ...any) {
	GetLogger().Info(fmt.Sprintf(format, args...))
}

func Warnf(format string, args ...any) {
	GetLogger().Warn(fmt.Sprintf(format, args...))
}

func Errorf(format string, args ...any) {
	GetLogger().Error(fmt.Sprintf(format, args...))
}

func SetOutput(w io.Writer) {
	globalSafeWriter.Set(w)
}

func Writer() io.Writer {
	return globalSafeWriter.Get()
}

// LogWriter is preserved for legacy standard log compatibility.
type LogWriter struct {
	Stdout io.Writer
}

func (w *LogWriter) Write(p []byte) (n int, err error) {
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
		// Strip Go default log prefix
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
		if LogCallback != nil {
			LogCallback(logMsg)
		}
	}
	return len(p), nil
}
