package logger

import (
	"io"
	"os"
	"sync"
)

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
