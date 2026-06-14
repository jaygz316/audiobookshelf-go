package handlers

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// setupUsersTestDB is a helper to set up an in-memory db for user tests
func setupUsersTestDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name()))
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}

	// Create users table for testing handleInit
	_, err = db.Exec(`
		CREATE TABLE users (
			id TEXT PRIMARY KEY,
			username TEXT,
			email TEXT,
			type TEXT,
			pash TEXT,
			token TEXT,
			isActive INTEGER,
			isLocked INTEGER,
			lastSeen TEXT,
			permissions TEXT,
			bookmarks TEXT,
			extraData TEXT,
			createdAt TEXT,
			updatedAt TEXT
		)
	`)
	if err != nil {
		t.Fatalf("failed to create users table: %v", err)
	}

	return db
}

func TestHandleInit_MaxBytesReader(t *testing.T) {
	db := setupUsersTestDB(t)
	defer db.Close()

	// 2 MB payload of random valid JSON-ish structure
	largeBody := []byte(`{"newRoot":{"username":"root","password":"`)
	largeBody = append(largeBody, bytes.Repeat([]byte("A"), 2*1024*1024)...)
	largeBody = append(largeBody, []byte(`"}}`)...)

	req := httptest.NewRequest(http.MethodPost, "/init", bytes.NewReader(largeBody))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()

	handler := handleInit(db)
	handler.ServeHTTP(w, req)

	// MaxBytesReader causes Decoder.Decode to fail with an error containing "http: request body too large"
	// However, the handler explicitly returns "Invalid request" with a 400 StatusBadRequest on any JSON decode error
	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status code %d, got %d. Body: %s", http.StatusBadRequest, w.Code, w.Body.String())
	}

	if !bytes.Contains(w.Body.Bytes(), []byte("Invalid request")) {
		t.Errorf("Expected body to contain 'Invalid request', got %s", w.Body.String())
	}
}
