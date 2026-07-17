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
