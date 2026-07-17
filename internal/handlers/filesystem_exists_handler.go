package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"audiobookshelf/internal/core"
	"audiobookshelf/internal/utils"
)

// handleCheckPathExists checks if directory exists inside library folder
func handleCheckPathExists(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Go] POST /api/filesystem/pathexists")
		userSess, ok := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if !ok {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}

		var body struct {
			Directory  string `json:"directory"`
			FolderPath string `json:"folderPath"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			log.Errorf("[FileSystem] Invalid request body: %v", err)
			http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
			return
		}

		if body.Directory == "" || body.FolderPath == "" {
			log.Errorf("[FileSystem] Invalid request body: directory or folderPath is empty")
			http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
			return
		}

		// Check that library folder exists
		var libraryID string
		err := db.QueryRow("SELECT libraryId FROM libraryFolders WHERE path = ?", body.FolderPath).Scan(&libraryID)
		if err == sql.ErrNoRows {
			log.Warnf("[FileSystem] Library folder not found: %s", body.FolderPath)
			http.Error(w, `{"error": "Library folder not found"}`, http.StatusNotFound)
			return
		} else if err != nil {
			log.Errorf("[FileSystem] DB error querying library folder: %v", err)
			http.Error(w, `{"error": "Database error"}`, http.StatusInternalServerError)
			return
		}

		// Check user can access library
		if !userSess.CanAccessLibrary(libraryID) {
			log.Infof("[FileSystem] User %s attempting to check path exists for library %s without access", userSess.Username, libraryID)
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		filePath := filepath.Join(body.FolderPath, body.Directory)
		filePathPOSIX := filepath.ToSlash(filePath)
		folderPathPOSIX := filepath.ToSlash(body.FolderPath)

		// Ensure filepath is inside library folder (prevents directory traversal)
		if !utils.IsSameOrSubPath(folderPathPOSIX, filePathPOSIX) {
			log.Infof("[FileSystem] Filepath is not inside library folder: %s", filePathPOSIX)
			http.Error(w, `{"error": "Invalid path"}`, http.StatusBadRequest)
			return
		}

		exists := false
		if _, err := os.Stat(filePath); err == nil {
			exists = true
		}

		if exists {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"exists":true}`))
			return
		}

		// Check if a library item exists in a subdirectory
		cleanedDirectory := strings.Trim(filepath.ToSlash(body.Directory), "/")
		if strings.Contains(cleanedDirectory, "/") {
			// Can only be 2 levels deep
			var possiblePaths []string
			subdir := filepath.Dir(cleanedDirectory)
			possiblePaths = append(possiblePaths, filepath.ToSlash(filepath.Join(body.FolderPath, subdir)))
			if strings.Contains(subdir, "/") {
				possiblePaths = append(possiblePaths, filepath.ToSlash(filepath.Join(body.FolderPath, filepath.Dir(subdir))))
			}

			if len(possiblePaths) > 0 {
				placeholders := make([]string, len(possiblePaths))
				args := make([]interface{}, len(possiblePaths))
				for i, p := range possiblePaths {
					placeholders[i] = "?"
					args[i] = p
				}
				query := fmt.Sprintf("SELECT title FROM libraryItems WHERE path IN (%s) LIMIT 1", strings.Join(placeholders, ","))
				var title string
				err := db.QueryRow(query, args...).Scan(&title)
				if err == nil {
					w.Header().Set("Content-Type", "application/json")
					json.NewEncoder(w).Encode(map[string]interface{}{
						"exists":           true,
						"libraryItemTitle": title,
					})
					return
				}
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"exists":false}`))
	}
}
