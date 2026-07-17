package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"audiobookshelf/internal/core"
	idb "audiobookshelf/internal/db"
	isocket "audiobookshelf/internal/socket"
)

func updateUserPermissions(targetUser *idb.User, permissions *map[string]interface{}, librariesAccessible []string, itemTagsSelected *[]string) {
	var currentPerms map[string]interface{}
	json.Unmarshal(targetUser.Permissions, &currentPerms)
	if currentPerms == nil {
		currentPerms = make(map[string]interface{})
	}

	if permissions != nil {
		for k, v := range *permissions {
			if k == "librariesAccessible" {
				continue
			}
			currentPerms[k] = v
		}
	}

	if len(librariesAccessible) > 0 {
		currentPerms["librariesAccessible"] = librariesAccessible
	}
	if itemTagsSelected != nil {
		currentPerms["itemTagsSelected"] = *itemTagsSelected
	}

	newPermsBytes, _ := json.Marshal(currentPerms)
	targetUser.Permissions = newPermsBytes
}

func handleUserUpdate(db *sql.DB, w http.ResponseWriter, r *http.Request, userSess *core.UserSession, targetUserID string) {
	log.Infof("[Go] PATCH /api/users/%s", targetUserID)
	if userSess.Type != "root" && userSess.Type != "admin" {
		http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
		return
	}

	targetUser, err := idb.GetUserFullByID(r.Context(), db, targetUserID)
	if err != nil || targetUser == nil {
		http.Error(w, `{"error": "User not found"}`, http.StatusNotFound)
		return
	}

	if targetUser.Type == "root" && userSess.Type != "root" {
		http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
		return
	}

	var body struct {
		Username            *string                 `json:"username"`
		Password            *string                 `json:"password"`
		Email               *string                 `json:"email"`
		Type                *string                 `json:"type"`
		IsActive            *bool                   `json:"isActive"`
		Permissions         *map[string]interface{} `json:"permissions"`
		LibrariesAccessible []string                `json:"librariesAccessible"`
		ItemTagsSelected    *[]string               `json:"itemTagsSelected"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1048576)
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
		return
	}

	hasUpdates := false
	shouldUpdateToken := false

	if body.Username != nil && *body.Username != targetUser.Username {
		exists, err := idb.CheckUserExistsWithUsername(r.Context(), db, *body.Username)
		if err != nil || exists {
			http.Error(w, `{"error": "Username already taken"}`, http.StatusBadRequest)
			return
		}
		targetUser.Username = *body.Username
		shouldUpdateToken = true
		hasUpdates = true
	}

	if body.Password != nil && *body.Password != "" {
		hashed, err := bcrypt.GenerateFromPassword([]byte(*body.Password), 8)
		if err != nil {
			http.Error(w, `{"error": "Failed to update password"}`, http.StatusInternalServerError)
			return
		}
		targetUser.Pash = string(hashed)
		hasUpdates = true
	}

	if body.Email != nil {
		if *body.Email == "" {
			targetUser.Email = nil
		} else {
			targetUser.Email = body.Email
		}
		hasUpdates = true
	}

	if body.Type != nil {
		if *body.Type == "root" && userSess.Type != "root" {
			http.Error(w, `{"error": "Only root users can escalate to root"}`, http.StatusForbidden)
			return
		}
		if targetUser.Type == "root" && *body.Type != "root" {
			http.Error(w, `{"error": "Cannot change type of root user"}`, http.StatusBadRequest)
			return
		}
		if targetUser.Type != "root" {
			targetUser.Type = *body.Type
			hasUpdates = true
		}
	}

	if body.IsActive != nil {
		if targetUser.Type == "root" && !*body.IsActive {
			http.Error(w, `{"error": "Cannot deactivate root user"}`, http.StatusBadRequest)
			return
		}
		targetUser.IsActive = *body.IsActive
		hasUpdates = true
	}

	if body.Permissions != nil || len(body.LibrariesAccessible) > 0 || body.ItemTagsSelected != nil {
		updateUserPermissions(targetUser, body.Permissions, body.LibrariesAccessible, body.ItemTagsSelected)
		hasUpdates = true
	}

	if hasUpdates {
		if shouldUpdateToken {
			secret := getTokenSecret(db)
			apiToken := jwt.NewWithClaims(jwt.SigningMethodHS256, &core.AuthClaims{
				UserID:   targetUser.ID,
				Username: targetUser.Username,
				Type:     targetUser.Type,
				RegisteredClaims: jwt.RegisteredClaims{
					Issuer: "audiobookshelf",
				},
			})
			targetUser.Token, _ = apiToken.SignedString([]byte(secret))
		}

		if shouldUpdateToken {
			db.ExecContext(r.Context(), "DELETE FROM sessions WHERE userId = ?", targetUser.ID)
		}

		nowStr := idb.TimeToDBStr(time.Now())
		var emailVal interface{} = nil
		if targetUser.Email != nil {
			emailVal = *targetUser.Email
		}
		_, err = db.ExecContext(r.Context(), `UPDATE users SET username = ?, email = ?, type = ?, pash = ?, token = ?, isActive = ?, permissions = ?, updatedAt = ? WHERE id = ?`,
			targetUser.Username, emailVal, targetUser.Type, targetUser.Pash, targetUser.Token, func() int {
				if targetUser.IsActive {
					return 1
				}
				return 0
			}(), string(targetUser.Permissions), nowStr, targetUser.ID)

		if err != nil {
			log.Errorf("[User Update] DB Error: %v", err)
			http.Error(w, `{"error": "Failed to update user"}`, http.StatusInternalServerError)
			return
		}
	}

	updatedUser, _ := idb.GetUserFullByID(r.Context(), db, targetUserID)
	userJSON := updatedUser.ToOldJSONForBrowser(userSess.Type != "root")

	if isocket.GlobalAuth != nil {
		isocket.GlobalAuth.BroadcastToUser(targetUserID, "user_updated", userJSON)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"user":    userJSON,
	})
}
