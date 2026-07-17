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
)

// handleInit handles initializing the server by creating the root user.
func handleInit(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Info("[Go] POST /init")
		db := getDB(db)
		if db == nil {
			http.Error(w, `{"error": "Database not connected"}`, http.StatusInternalServerError)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
			return
		}

		hasRoot, err := idb.HasRootUser(db)
		if err != nil {
			log.Errorf("[Init] Error checking root user: %v", err)
			http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
			return
		}
		if hasRoot {
			log.Warnf("[Init] Attempt to init server when root user already exists")
			http.Error(w, `{"error": "Root user already exists"}`, http.StatusForbidden)
			return
		}

		var reqBody struct {
			NewRoot struct {
				Username string `json:"username"`
				Password string `json:"password"`
			} `json:"newRoot"`
		}
		r.Body = http.MaxBytesReader(w, r.Body, 1048576)
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			http.Error(w, `{"error": "Invalid request"}`, http.StatusBadRequest)
			return
		}

		username := reqBody.NewRoot.Username
		if username == "" {
			username = "root"
		}
		password := reqBody.NewRoot.Password

		hashed, err := bcrypt.GenerateFromPassword([]byte(password), 8)
		if err != nil {
			log.Errorf("[Init] Hashing failed: %v", err)
			http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
			return
		}

		userID := uuid.New().String()
		apiToken := jwt.NewWithClaims(jwt.SigningMethodHS256, &core.AuthClaims{
			UserID:   userID,
			Username: username,
			Type:     "root",
			RegisteredClaims: jwt.RegisteredClaims{
				Issuer: "audiobookshelf",
			},
		})
		tokenStr, err := apiToken.SignedString([]byte(getTokenSecret(db)))
		if err != nil {
			log.Errorf("[Init] Token signing failed: %v", err)
			http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
			return
		}

		nowStr := idb.TimeToDBStr(time.Now())
		defaultPerms := idb.GetDefaultPermissionsForUserType("root")

		_, err = db.ExecContext(r.Context(), `INSERT INTO users (id, username, type, pash, token, isActive, permissions, extraData, bookmarks, createdAt, updatedAt) 
			VALUES (?, ?, 'root', ?, ?, 1, ?, '{}', '[]', ?, ?)`,
			userID, username, string(hashed), tokenStr, defaultPerms, nowStr, nowStr)
		if err != nil {
			log.Errorf("[Init] Failed to create root user: %v", err)
			http.Error(w, `{"error": "Failed to create root user"}`, http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}
}
