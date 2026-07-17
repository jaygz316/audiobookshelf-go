package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"audiobookshelf/internal/core"
	idb "audiobookshelf/internal/db"
	isocket "audiobookshelf/internal/socket"
)

func handleUserCreate(db *sql.DB, w http.ResponseWriter, r *http.Request, userSess *core.UserSession) {
	log.Info("[Go] POST /api/users")
	if userSess.Type != "root" && userSess.Type != "admin" {
		http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
		return
	}

	var body struct {
		Username            string                      `json:"username"`
		Password            string                      `json:"password"`
		Email               string                      `json:"email"`
		Type                string                      `json:"type"`
		IsActive            *bool                       `json:"isActive"`
		Permissions         idb.UserPermissionsDetailed `json:"permissions"`
		LibrariesAccessible []string                    `json:"librariesAccessible"`
		ItemTagsSelected    []string                    `json:"itemTagsSelected"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1048576)
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
		return
	}

	if body.Username == "" || body.Password == "" {
		http.Error(w, `{"error": "Username and password are required"}`, http.StatusBadRequest)
		return
	}

	exists, err := idb.CheckUserExistsWithUsername(r.Context(), db, body.Username)
	if err != nil || exists {
		http.Error(w, `{"error": "Username already taken"}`, http.StatusBadRequest)
		return
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(body.Password), 8)
	if err != nil {
		http.Error(w, `{"error": "Failed to create user"}`, http.StatusInternalServerError)
		return
	}

	userID := uuid.New().String()
	secret := getTokenSecret(db)
	apiToken := jwt.NewWithClaims(jwt.SigningMethodHS256, &core.AuthClaims{
		UserID:   userID,
		Username: body.Username,
		Type:     body.Type,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer: "audiobookshelf",
		},
	})
	tokenStr, _ := apiToken.SignedString([]byte(secret))

	userType := body.Type
	if userType == "" {
		userType = "user"
	}
	if userType == "root" && userSess.Type != "root" {
		http.Error(w, `{"error": "Only root users can create root users"}`, http.StatusForbidden)
		return
	}

	perms := map[string]interface{}{
		"download":                  true,
		"upload":                    true,
		"delete":                    false,
		"update":                    false,
		"accessRss":                 true,
		"createShares":              true,
		"accessExplicitContent":     false,
		"accessAllLibraries":        true,
		"librariesAccessible":       []string{},
		"accessAllTags":             true,
		"itemTagsSelected":          []string{},
		"selectedTagsNotAccessible": false,
	}
	if body.Permissions.Download != nil {
		perms["download"] = *body.Permissions.Download
	}
	if body.Permissions.Upload != nil {
		perms["upload"] = *body.Permissions.Upload
	}
	if body.Permissions.Delete != nil {
		perms["delete"] = *body.Permissions.Delete
	}
	if body.Permissions.Update != nil {
		perms["update"] = *body.Permissions.Update
	}
	if body.Permissions.AccessRss != nil {
		perms["accessRss"] = *body.Permissions.AccessRss
	}
	if body.Permissions.CreatePublicShares != nil {
		perms["createShares"] = *body.Permissions.CreatePublicShares
	}
	if body.Permissions.AccessExplicitContent != nil {
		perms["accessExplicitContent"] = *body.Permissions.AccessExplicitContent
	}
	if body.Permissions.AccessAllLibraries != nil {
		perms["accessAllLibraries"] = *body.Permissions.AccessAllLibraries
	}
	if len(body.Permissions.LibrariesAccessible) > 0 {
		perms["librariesAccessible"] = body.Permissions.LibrariesAccessible
	} else if len(body.LibrariesAccessible) > 0 {
		perms["librariesAccessible"] = body.LibrariesAccessible
	}
	if body.Permissions.AccessAllTags != nil {
		perms["accessAllTags"] = *body.Permissions.AccessAllTags
	}
	if len(body.Permissions.ItemTagsSelected) > 0 {
		perms["itemTagsSelected"] = body.Permissions.ItemTagsSelected
	} else if len(body.ItemTagsSelected) > 0 {
		perms["itemTagsSelected"] = body.ItemTagsSelected
	}
	if body.Permissions.SelectedTagsNotAccessible != nil {
		perms["selectedTagsNotAccessible"] = *body.Permissions.SelectedTagsNotAccessible
	}

	permsBytes, _ := json.Marshal(perms)
	nowStr := idb.TimeToDBStr(time.Now())

	var emailVal interface{} = nil
	if body.Email != "" {
		emailVal = body.Email
	}

	_, err = db.ExecContext(r.Context(), `INSERT INTO users (id, username, email, type, pash, token, isActive, permissions, extraData, bookmarks, createdAt, updatedAt) 
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, '{}', '[]', ?, ?)`,
		userID, body.Username, emailVal, userType, string(hashed), tokenStr, func() int {
			if body.IsActive == nil || *body.IsActive {
				return 1
			}
			return 0
		}(), string(permsBytes), nowStr, nowStr)

	if err != nil {
		log.Errorf("[User Create] DB Error: %v", err)
		http.Error(w, `{"error": "Failed to save user"}`, http.StatusInternalServerError)
		return
	}

	savedUser, err := idb.GetUserFullByID(r.Context(), db, userID)
	if err != nil || savedUser == nil {
		http.Error(w, `{"error": "Internal Error"}`, http.StatusInternalServerError)
		return
	}

	userJSON := savedUser.ToOldJSONForBrowser(userSess.Type != "root")

	if isocket.GlobalAuth != nil {
		isocket.GlobalAuth.BroadcastToAdmins("user_added", userJSON)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"user": userJSON,
	})
}
