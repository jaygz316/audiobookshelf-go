package logger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
)

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
