package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"net/http"

	idb "audiobookshelf/internal/db"
)

// handleLogout handles user logout by clearing the refresh token from database and cookies.
func handleLogout(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Info("[Go] POST /logout")
		db := getDB(db)
		if db == nil {
			http.Error(w, `{"error": "Database not connected"}`, http.StatusInternalServerError)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
			return
		}

		cookie, err := r.Cookie("refresh_token")
		if err == nil && cookie.Value != "" {
			_, _ = idb.DeleteSessionByRefreshToken(r.Context(), db, cookie.Value)
		}

		// Clear Cookie
		http.SetCookie(w, &http.Cookie{
			Name:     "refresh_token",
			Value:    "",
			Path:     "/",
			MaxAge:   -1,
			HttpOnly: true,
		})

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"success":true}`))
	}
}
