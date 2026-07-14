package handlers

import (
	log "audiobookshelf/internal/logger"
	"context"
	"database/sql"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"audiobookshelf/internal/core"
	_ "modernc.org/sqlite"
)

func BenchmarkDeleteTag(b *testing.B) {
	log.SetOutput(io.Discard) // disable logs

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		b.Fatalf("Failed to open db: %v", err)
	}
	defer db.Close()

	// Setup schema
	_, err = db.Exec(`
		CREATE TABLE books (id TEXT PRIMARY KEY, tags TEXT);
		CREATE TABLE podcasts (id TEXT PRIMARY KEY, tags TEXT);
		CREATE TABLE users (id TEXT PRIMARY KEY, permissions TEXT);
	`)
	if err != nil {
		b.Fatalf("Failed to create schema: %v", err)
	}

	// Insert test data
	tagToDel := "TagToDelete"
	otherTag := "OtherTag"
	for i := 0; i < 1000; i++ {
		// Half with target tag, half without
		tags := fmt.Sprintf(`["%s"]`, otherTag)
		if i%2 == 0 {
			tags = fmt.Sprintf(`["%s", "%s"]`, tagToDel, otherTag)
		}

		_, err := db.Exec(`INSERT INTO books (id, tags) VALUES (?, ?)`, fmt.Sprintf("book%d", i), tags)
		if err != nil {
			b.Fatalf("Failed to insert book: %v", err)
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		_, err := db.Exec(`UPDATE books SET tags = '["TagToDelete", "OtherTag"]' WHERE id LIKE '%0'`)
		if err != nil {
			b.Fatalf("Failed to update books: %v", err)
		}
		b.StartTimer()

		handler := handleDeleteTag(db)
		tagParam := base64.StdEncoding.EncodeToString([]byte(tagToDel))
		req := httptest.NewRequest("DELETE", "/api/tags/"+tagParam, nil)

		userSess := &core.UserSession{
			Type: "admin",
		}
		ctx := context.WithValue(req.Context(), core.UserContextKey, userSess)
		req = req.WithContext(ctx)

		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if status := rr.Code; status != http.StatusOK {
			b.Fatalf("Handler returned wrong status code: got %v want %v", status, http.StatusOK)
		}
	}
}
