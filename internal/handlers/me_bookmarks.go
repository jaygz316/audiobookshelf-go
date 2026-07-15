package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"audiobookshelf/internal/core"
	idb "audiobookshelf/internal/db"
	isocket "audiobookshelf/internal/socket"
	"audiobookshelf/internal/utils"
)

// Bookmark represents a user's bookmark inside a library item.
type Bookmark struct {
	LibraryItemID string  `json:"libraryItemId"`
	Time          float64 `json:"time"`
	Title         string  `json:"title"`
	Note          string  `json:"note"`
	Color         string  `json:"color"`
	Cfi           string  `json:"cfi,omitempty"`
	CreatedAt     int64   `json:"createdAt"`
}

func handleMeCreateBookmark(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Go] POST %s", r.URL.Path)
		userSess, ok := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if !ok {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}

		sub := utils.TrimAPIPath(r.URL.Path, "/api/me/item/")
		libraryItemID := strings.TrimSuffix(sub, "/bookmark")
		if libraryItemID == "" || strings.Contains(libraryItemID, "/") {
			http.Error(w, `{"error": "Bad Request"}`, http.StatusBadRequest)
			return
		}

		// Read body
		var body struct {
			Time  float64 `json:"time"`
			Title string  `json:"title"`
			Note  string  `json:"note"`
			Color string  `json:"color"`
			Cfi   string  `json:"cfi"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
			return
		}

		if body.Title == "" {
			http.Error(w, `{"error": "Title required"}`, http.StatusBadRequest)
			return
		}

		user, err := idb.GetUserFullByID(r.Context(), db, userSess.ID)
		if err != nil || user == nil {
			http.Error(w, "idb.User not found", http.StatusNotFound)
			return
		}

		var bookmarks []Bookmark
		if len(user.Bookmarks) > 0 {
			json.Unmarshal(user.Bookmarks, &bookmarks)
		}

		// Create new bookmark
		newBookmark := Bookmark{
			LibraryItemID: libraryItemID,
			Time:          body.Time,
			Title:         body.Title,
			Note:          body.Note,
			Color:         body.Color,
			Cfi:           body.Cfi,
			CreatedAt:     time.Now().UnixMilli(),
		}
		bookmarks = append(bookmarks, newBookmark)

		bookmarksBytes, _ := json.Marshal(bookmarks)
		_, err = db.ExecContext(r.Context(), "UPDATE users SET bookmarks = ?, updatedAt = ? WHERE id = ?", string(bookmarksBytes), idb.TimeToDBStr(time.Now()), user.ID)
		if err != nil {
			log.Errorf("[Me Bookmark] DB error: %v", err)
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}

		user, _ = idb.GetUserFullByID(r.Context(), db, userSess.ID)
		userJSON := user.ToOldJSONForBrowser(user.Type != "root")
		if isocket.GlobalAuth != nil {
			isocket.GlobalAuth.BroadcastToUser(userSess.ID, "user_updated", userJSON)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(newBookmark)
	}
}

func handleMeUpdateBookmark(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Go] PATCH %s", r.URL.Path)
		userSess, ok := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if !ok {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}

		sub := utils.TrimAPIPath(r.URL.Path, "/api/me/item/")
		libraryItemID := strings.TrimSuffix(sub, "/bookmark")
		if libraryItemID == "" || strings.Contains(libraryItemID, "/") {
			http.Error(w, `{"error": "Bad Request"}`, http.StatusBadRequest)
			return
		}

		// Read body
		var body struct {
			Time  float64 `json:"time"`
			Title string  `json:"title"`
			Note  string  `json:"note"`
			Color string  `json:"color"`
			Cfi   string  `json:"cfi"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
			return
		}

		user, err := idb.GetUserFullByID(r.Context(), db, userSess.ID)
		if err != nil || user == nil {
			http.Error(w, "idb.User not found", http.StatusNotFound)
			return
		}

		var bookmarks []Bookmark
		if len(user.Bookmarks) > 0 {
			json.Unmarshal(user.Bookmarks, &bookmarks)
		}

		found := false
		var updated Bookmark
		for i, b := range bookmarks {
			diff := b.Time - body.Time
			if diff < 0 {
				diff = -diff
			}
			if b.LibraryItemID == libraryItemID && diff < 0.001 {
				bookmarks[i].Title = body.Title
				bookmarks[i].Note = body.Note
				bookmarks[i].Color = body.Color
				if body.Cfi != "" {
					bookmarks[i].Cfi = body.Cfi
				}
				updated = bookmarks[i]
				found = true
				break
			}
		}

		if !found {
			http.Error(w, "Bookmark not found", http.StatusNotFound)
			return
		}

		bookmarksBytes, _ := json.Marshal(bookmarks)
		_, err = db.ExecContext(r.Context(), "UPDATE users SET bookmarks = ?, updatedAt = ? WHERE id = ?", string(bookmarksBytes), idb.TimeToDBStr(time.Now()), user.ID)
		if err != nil {
			log.Errorf("[Me Bookmark] DB error: %v", err)
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}

		user, _ = idb.GetUserFullByID(r.Context(), db, userSess.ID)
		userJSON := user.ToOldJSONForBrowser(user.Type != "root")
		if isocket.GlobalAuth != nil {
			isocket.GlobalAuth.BroadcastToUser(userSess.ID, "user_updated", userJSON)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(updated)
	}
}

func handleMeRemoveBookmark(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Go] DELETE %s", r.URL.Path)
		userSess, ok := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if !ok {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}

		// Route format: /api/me/item/:id/bookmark/:time
		sub := utils.TrimAPIPath(r.URL.Path, "/api/me/item/")
		parts := strings.Split(sub, "/bookmark/")
		if len(parts) != 2 {
			http.Error(w, `{"error": "Bad Request"}`, http.StatusBadRequest)
			return
		}

		libraryItemID := parts[0]
		timeStr, err := url.PathUnescape(parts[1])
		if err != nil {
			http.Error(w, `{"error": "Bad Request"}`, http.StatusBadRequest)
			return
		}

		var timeVal float64
		if _, err := fmt.Sscanf(timeStr, "%f", &timeVal); err != nil {
			http.Error(w, `{"error": "Invalid time value"}`, http.StatusBadRequest)
			return
		}

		user, err := idb.GetUserFullByID(r.Context(), db, userSess.ID)
		if err != nil || user == nil {
			http.Error(w, "idb.User not found", http.StatusNotFound)
			return
		}

		var bookmarks []Bookmark
		if len(user.Bookmarks) > 0 {
			json.Unmarshal(user.Bookmarks, &bookmarks)
		}

		newBookmarks := []Bookmark{}
		found := false
		for _, b := range bookmarks {
			diff := b.Time - timeVal
			if diff < 0 {
				diff = -diff
			}
			if b.LibraryItemID == libraryItemID && diff < 0.001 {
				found = true
			} else {
				newBookmarks = append(newBookmarks, b)
			}
		}

		if !found {
			http.Error(w, "Bookmark not found", http.StatusNotFound)
			return
		}

		bookmarksBytes, _ := json.Marshal(newBookmarks)
		_, err = db.ExecContext(r.Context(), "UPDATE users SET bookmarks = ?, updatedAt = ? WHERE id = ?", string(bookmarksBytes), idb.TimeToDBStr(time.Now()), user.ID)
		if err != nil {
			log.Errorf("[Me Bookmark] DB error: %v", err)
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}

		user, _ = idb.GetUserFullByID(r.Context(), db, userSess.ID)
		userJSON := user.ToOldJSONForBrowser(user.Type != "root")
		if isocket.GlobalAuth != nil {
			isocket.GlobalAuth.BroadcastToUser(userSess.ID, "user_updated", userJSON)
		}

		w.WriteHeader(http.StatusOK)
	}
}
