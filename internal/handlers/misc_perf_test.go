package handlers

import (
	"context"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"strconv"

	"audiobookshelf/internal/core"
)

// This function will setup a DB with 1000 books having a specific tag.
func BenchmarkRenameTag(b *testing.B) {
	db := setupTestDB(b)
	defer db.Close()

	// Insert 1000 books with tags
	for i := 0; i < 1000; i++ {
		id := "book_" + strconv.Itoa(i)
		tags := `["fantasy", "sci-fi"]`
		_, err := db.Exec("INSERT INTO books (id, tags) VALUES (?, ?)", id, tags)
		if err != nil {
			b.Fatalf("Failed to insert book: %v", err)
		}
	}

	handler := handleRenameTag(db)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()

		// Ensure books are reset
		db.Exec("UPDATE books SET tags = '[\"fantasy\", \"sci-fi\"]'")

		reqBody := map[string]string{
			"tag": "fantasy",
			"newTag": "epic-fantasy",
		}
		bodyBytes, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("POST", "/api/tags/rename", bytes.NewBuffer(bodyBytes))

		// Add user context
		userSess := &core.UserSession{
			Type: "root",
		}
		ctx := context.WithValue(req.Context(), core.UserContextKey, userSess)
		req = req.WithContext(ctx)

		rr := httptest.NewRecorder()

		b.StartTimer()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			b.Fatalf("Handler failed with status %d: %s", rr.Code, rr.Body.String())
		}
	}
}
