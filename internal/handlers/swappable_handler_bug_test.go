package handlers

import (
	"audiobookshelf/internal/core"
	idb "audiobookshelf/internal/db"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestSwappableHandlerLoopBug(t *testing.T) {
	// Setup temporary directory for DB
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.sqlite")
	db, err := idb.InitDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to init DB: %v", err)
	}
	defer db.Close()

	cfg := &core.Config{
		RouterBasePath: "",
		ConfigPath:     tmpDir,
		MetadataPath:   tmpDir,
	}

	// 1. Setup initial handler
	SetupHandler(db, cfg, true, ".", "2.35.1")

	if ActiveHandler == nil {
		t.Fatalf("ActiveHandler should not be nil")
	}

	// Verify it can route initially
	req := httptest.NewRequest("GET", "/ping", nil)
	rr := httptest.NewRecorder()
	ActiveHandler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("Expected 200 OK, got %d", rr.Code)
	}

	// 2. Invoke reconnectDB (simulating a restore reconnect)
	// We need to set subFS so SetupHandler doesn't panic when looking for frontend
	subFS = os.DirFS("../../frontend")

	err = reconnectDB(dbPath)
	if err != nil {
		t.Fatalf("reconnectDB failed: %v", err)
	}

	// 3. Inspect if ActiveHandler.handler points to ActiveHandler itself
	if ActiveHandler.handler == ActiveHandler {
		t.Errorf("BUG DETECTED: ActiveHandler.handler points to ActiveHandler itself! Any subsequent request will cause a stack overflow.")
	} else {
		t.Log("PASS: ActiveHandler.handler does not point to ActiveHandler.")
	}
}
