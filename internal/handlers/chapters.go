package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"time"

	"audiobookshelf/internal/core"
)

var AudnexusBaseURL = "https://api.audnexus.com"

type ChapterPayload struct {
	ID    int     `json:"id"`
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Title string  `json:"title"`
}

func handleUpdateChapters(db *sql.DB, itemID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Go] POST /api/items/%s/chapters", itemID)

		userVal := r.Context().Value(core.UserContextKey)
		if userVal == nil {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}
		user := userVal.(*core.UserSession)

		if user.Type != "root" && user.Type != "admin" {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		var mediaID, mediaType string
		err := db.QueryRow("SELECT mediaId, mediaType FROM libraryItems WHERE id = ?", itemID).Scan(&mediaID, &mediaType)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		if mediaType != "book" {
			http.Error(w, `{"error": "Only books support chapters"}`, http.StatusBadRequest)
			return
		}

		var payload struct {
			Chapters []ChapterPayload `json:"chapters"`
		}

		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
			return
		}

		// Enforce IDs starting from 1 to avoid mismatches
		for i := range payload.Chapters {
			payload.Chapters[i].ID = i + 1
		}

		chaptersJSON, err := json.Marshal(payload.Chapters)
		if err != nil {
			http.Error(w, "Failed to marshal chapters: "+err.Error(), http.StatusInternalServerError)
			return
		}

		tx, err := db.Begin()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer tx.Rollback()

		_, err = tx.Exec("UPDATE books SET chapters = ? WHERE id = ?", chaptersJSON, mediaID)
		if err != nil {
			http.Error(w, "Failed to update chapters in DB: "+err.Error(), http.StatusInternalServerError)
			return
		}

		nowStr := time.Now().Format("2006-01-02 15:04:05.000")
		_, err = tx.Exec("UPDATE libraryItems SET updatedAt = ? WHERE id = ?", nowStr, itemID)
		if err != nil {
			http.Error(w, "Failed to update library item timestamp: "+err.Error(), http.StatusInternalServerError)
			return
		}

		if err := tx.Commit(); err != nil {
			http.Error(w, "Failed to commit transaction: "+err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"chapters": payload.Chapters,
		})
	}
}

func handleLookupChapters(db *sql.DB, itemID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Go] POST /api/items/%s/chapters/lookup", itemID)

		userVal := r.Context().Value(core.UserContextKey)
		if userVal == nil {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}
		user := userVal.(*core.UserSession)

		if user.Type != "root" && user.Type != "admin" {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		var mediaID, mediaType string
		err := db.QueryRow("SELECT mediaId, mediaType FROM libraryItems WHERE id = ?", itemID).Scan(&mediaID, &mediaType)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		if mediaType != "book" {
			http.Error(w, `{"error": "Only books support chapters"}`, http.StatusBadRequest)
			return
		}

		var asin string
		err = db.QueryRow("SELECT asin FROM books WHERE id = ?", mediaID).Scan(&asin)
		if err != nil || asin == "" {
			http.Error(w, `{"error": "Book must have a valid ASIN for Audnexus chapter lookup."}`, http.StatusBadRequest)
			return
		}

		if !regexp.MustCompile(`^[A-Za-z0-9]{10}$`).MatchString(asin) {
			http.Error(w, `{"error": "Invalid ASIN format"}`, http.StatusBadRequest)
			return
		}

		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Get(AudnexusBaseURL + "/books/" + asin + "/chapters")
		if err != nil {
			http.Error(w, "Failed to connect to Audnexus: "+err.Error(), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusNotFound {
			http.Error(w, "Chapters not found in Audnexus database for ASIN "+asin, http.StatusNotFound)
			return
		}

		if resp.StatusCode != http.StatusOK {
			http.Error(w, fmt.Sprintf("Audnexus returned status %d", resp.StatusCode), http.StatusBadGateway)
			return
		}

		var audnexusResp struct {
			Chapters []struct {
				Title    string  `json:"title"`
				Start    float64 `json:"start"`
				End      float64 `json:"end"`
				Duration float64 `json:"duration"`
			} `json:"chapters"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&audnexusResp); err != nil {
			http.Error(w, "Failed to parse Audnexus response: "+err.Error(), http.StatusInternalServerError)
			return
		}

		var chapters []ChapterPayload
		for i, c := range audnexusResp.Chapters {
			endVal := c.End
			if endVal == 0 && c.Duration > 0 {
				endVal = c.Start + c.Duration
			}
			chapters = append(chapters, ChapterPayload{
				ID:    i + 1,
				Title: c.Title,
				Start: c.Start,
				End:   endVal,
			})
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"chapters": chapters,
		})
	}
}
