package logger

import (
	"io"
	"strings"
	"time"
)

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
