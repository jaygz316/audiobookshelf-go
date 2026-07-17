package podcast

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestDownloadEpisode_LatencyAndProgress(t *testing.T) {
	m := NewPodcastManager(nil)
	content := []byte("lat-test-data-for-progress")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond) // Simulating latency
		w.Header().Set("Content-Type", "audio/mpeg")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(content)
	}))
	defer server.Close()
	configureTestClient(t, m, server.URL)

	tempDir := t.TempDir()
	dest := filepath.Join(tempDir, "ep.mp3")

	var progressCalled int32
	var finalBytes int64
	err := m.DownloadEpisodeWithProgress(context.Background(), server.URL, dest, func(dl, tot int64) {
		atomic.StoreInt64(&finalBytes, dl)
		atomic.AddInt32(&progressCalled, 1)
	})
	if err != nil {
		t.Fatalf("Download failed with latency: %v", err)
	}

	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("failed to read dest: %v", err)
	}
	if string(data) != string(content) {
		t.Errorf("content mismatch")
	}
	if atomic.LoadInt32(&progressCalled) == 0 {
		t.Error("expected progress callback to be called")
	}
	if atomic.LoadInt64(&finalBytes) != int64(len(content)) {
		t.Errorf("expected final bytes to be %d, got %d", len(content), finalBytes)
	}
}

func TestDownloadEpisode_Timeout(t *testing.T) {
	m := NewPodcastManager(nil)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond) // Exceeds timeout
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("too-late"))
	}))
	defer server.Close()
	configureTestClient(t, m, server.URL)

	tempDir := t.TempDir()
	dest := filepath.Join(tempDir, "ep.mp3")

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := m.DownloadEpisode(ctx, server.URL, dest)
	if err == nil {
		t.Error("expected error due to timeout, got nil")
	}
}

func TestDownloadEpisode_HTTPError(t *testing.T) {
	m := NewPodcastManager(nil)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	configureTestClient(t, m, server.URL)

	tempDir := t.TempDir()
	dest := filepath.Join(tempDir, "ep.mp3")

	err := m.DownloadEpisode(context.Background(), server.URL, dest)
	if err == nil {
		t.Error("expected error for 500 status code, got nil")
	}
}

func TestDownloadEpisode_ConnectionFailure(t *testing.T) {
	m := NewPodcastManager(nil)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Close connection mid-stream
		hj, ok := w.(http.Hijacker)
		if !ok {
			return
		}
		conn, _, _ := hj.Hijack()
		_ = conn.Close()
	}))
	defer server.Close()
	configureTestClient(t, m, server.URL)

	tempDir := t.TempDir()
	dest := filepath.Join(tempDir, "ep.mp3")

	err := m.DownloadEpisode(context.Background(), server.URL, dest)
	if err == nil {
		t.Error("expected error when connection is closed prematurely, got nil")
	}
}

func TestDownloadEpisode_FileWriteError(t *testing.T) {
	m := NewPodcastManager(nil)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("some data"))
	}))
	defer server.Close()
	configureTestClient(t, m, server.URL)

	// dest is a directory, os.Create should fail
	tempDir := t.TempDir()
	dest := filepath.Join(tempDir, "sub")
	if err := os.Mkdir(dest, 0755); err != nil {
		t.Fatalf("failed to create directory: %v", err)
	}

	err := m.DownloadEpisode(context.Background(), server.URL, dest)
	if err == nil {
		t.Error("expected error when target path is a directory, got nil")
	}
}
