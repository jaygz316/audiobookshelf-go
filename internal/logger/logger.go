package logger

import (
	"log"
	"log/slog"
	"os"
	"strings"
	"sync"
)

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
