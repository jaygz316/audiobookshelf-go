//go:build ignore
// +build ignore

package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

func main() {
	db, err := sql.Open("sqlite", "config/absdatabase.sqlite")
	if err != nil {
		log.Fatalf("Failed to open DB: %v", err)
	}
	defer db.Close()

	// 1. Get tokenSecret
	var tokenSecret string
	var valStr string
	err = db.QueryRow("SELECT value FROM settings WHERE key = 'server-settings'").Scan(&valStr)
	if err != nil {
		log.Fatalf("Failed to get server-settings: %v", err)
	}
	fmt.Printf("Settings value: %s\n", valStr)

	// In modernc.org/sqlite database, globalDB is used in main packages
	globalDB = db

	// Set up StreamManager
	streamManager = NewStreamManager()

	// Get metadata path
	metadataPath, _ := filepath.Abs("metadata")

	// Token from screenshot
	token := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ1c2VySWQiOiI3NDMzMzZiZi1hNGY2LTRkYTItOGE1Zi05YTFlYjNmYTc0ZmQiLCJ1c2VybmFtZSI6InJvb3QiLCJ0eXBlIjoicm9vdCIsImlhdCI6MTc4MTExNDU4MSwiZXhwIjoxNzgxOTc4NTgxfQ.FBL99emR_P6bUtECfsC5ZppB4_kg2HnRRpiN3KCZnSY"

	// Create mock request
	url := fmt.Sprintf("/audiobookshelf/hls/7c0a8a89-3d4f-4dc3-9d4a-98b11535e538/output-0.ts?token=%s", token)
	req := httptest.NewRequest("GET", url, nil)
	rr := httptest.NewRecorder()

	// Set up route
	handler := AuthMiddlewareWrapper(db, serveHLS(metadataPath, streamManager))

	handler.ServeHTTP(rr, req)

	fmt.Printf("Response Code: %d\n", rr.Code)
	fmt.Printf("Response Headers: %v\n", rr.Header())
	fmt.Printf("Response Body Length: %d\n", rr.Body.Len())
	if rr.Code != http.StatusOK {
		fmt.Printf("Response Body: %s\n", rr.Body.String())
	}
}
