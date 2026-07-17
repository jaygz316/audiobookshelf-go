package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"encoding/json"
	"net/http"

	"audiobookshelf/internal/core"
	idb "audiobookshelf/internal/db"
	isocket "audiobookshelf/internal/socket"
)

func handleGetUsers(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Info("[Go] GET /api/users")
		userSess := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if userSess.Type != "root" && userSess.Type != "admin" {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		hideRootToken := userSess.Type != "root"

		rows, err := db.QueryContext(r.Context(), "SELECT id, username, email, pash, type, token, isActive, isLocked, lastSeen, permissions, bookmarks, extraData, createdAt, updatedAt FROM users ORDER BY username ASC")
		if err != nil {
			log.Errorf("[Users] DB Query failed: %v", err)
			http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var usersJSON []map[string]interface{}
		for rows.Next() {
			var u idb.User
			var email sql.NullString
			var lastSeenStr sql.NullString
			var isActiveInt, isLockedInt sql.NullInt64
			var permsStr, bookmarksStr, extraDataStr sql.NullString
			var createdAtStr, updatedAtStr sql.NullString
			var pashStr, tokenStr, typeStr sql.NullString

			err := rows.Scan(&u.ID, &u.Username, &email, &pashStr, &typeStr, &tokenStr, &isActiveInt, &isLockedInt, &lastSeenStr, &permsStr, &bookmarksStr, &extraDataStr, &createdAtStr, &updatedAtStr)
			if err != nil {
				log.Errorf("[Users] Failed to scan user: %v", err)
				continue
			}

			if typeStr.Valid {
				u.Type = typeStr.String
			} else {
				u.Type = "user"
			}

			if pashStr.Valid {
				u.Pash = pashStr.String
			}
			if tokenStr.Valid {
				u.Token = tokenStr.String
			}
			if email.Valid {
				u.Email = &email.String
			}
			u.IsActive = isActiveInt.Valid && isActiveInt.Int64 != 0
			u.IsLocked = isLockedInt.Valid && isLockedInt.Int64 != 0
			if lastSeenStr.Valid && lastSeenStr.String != "" {
				val := idb.ParseTimeStr(lastSeenStr.String)
				u.LastSeen = &val
			}
			if permsStr.Valid {
				u.Permissions = []byte(permsStr.String)
			}
			if bookmarksStr.Valid {
				u.Bookmarks = []byte(bookmarksStr.String)
			}
			if extraDataStr.Valid {
				u.ExtraData = []byte(extraDataStr.String)
			}
			u.CreatedAt = idb.ParseTimeStr(createdAtStr.String)
			u.UpdatedAt = idb.ParseTimeStr(updatedAtStr.String)

			usersJSON = append(usersJSON, u.ToOldJSONForBrowser(hideRootToken))
		}
		if err := rows.Err(); err != nil {
			log.Errorf("[Users] Users query iteration error: %v", err)
			http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"users": usersJSON,
		})
	}
}

func handleGetOnlineUsers(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Info("[Go] GET /api/users/online")
		userSess := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if userSess.Type != "root" && userSess.Type != "admin" {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		var online []isocket.OnlineUser
		if isocket.GlobalAuth != nil {
			online = isocket.GlobalAuth.GetUsersOnline()
		}
		if online == nil {
			online = []isocket.OnlineUser{}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(online)
	}
}
