package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"audiobookshelf/internal/core"
	"audiobookshelf/internal/utils"
)

type DirectoryInfo struct {
	Path    string `json:"path"`
	Dirname string `json:"dirname"`
	Level   int    `json:"level"`
}

// handleGetFilesystem retrieves POSIX directories in a path
func handleGetFilesystem(appRoot string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Go] GET /api/filesystem")
		userSess, ok := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if !ok || !userSess.IsAdminOrUp() {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		relpath := r.URL.Query().Get("path")
		levelStr := r.URL.Query().Get("level")
		level := 0
		if levelStr != "" {
			if l, err := strconv.Atoi(levelStr); err == nil {
				level = l
			}
		}

		if relpath == "" {
			relpath = "/"
		}
		relpath = filepath.Clean(relpath)

		// Validate path. Must be absolute
		if !filepath.IsAbs(relpath) {
			log.Warnf("[FileSystem] Path is not absolute: %s", relpath)
			http.Error(w, `Invalid "path" query string`, http.StatusBadRequest)
			return
		}

		// Excluded directories from appRoot
		excludedDirNames := []string{
			"node_modules", "client", "server", ".git", "static", "build", "dist",
			"metadata", "config", "sys", "proc", ".devcontainer", ".nyc_output",
			"sys", "proc", ".github", ".vscode",
		}
		excludedPaths := make(map[string]bool)
		for _, name := range excludedDirNames {
			fullPath := filepath.Join(appRoot, name)
			excludedPaths[filepath.ToSlash(fullPath)] = true
		}

		// Always exclude /sys and /proc on Linux as well
		excludedPaths["/sys"] = true
		excludedPaths["/proc"] = true

		posixPath := filepath.ToSlash(relpath)
		for excl := range excludedPaths {
			if utils.IsSameOrSubPath(excl, posixPath) {
				log.Warnf("[FileSystem] Direct or nested access to excluded path blocked: %s", posixPath)
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}
		}

		fi, err := os.Stat(relpath)
		if err != nil || !fi.IsDir() {
			log.Warnf("[FileSystem] Path does not exist or is not a directory: %s", relpath)
			http.Error(w, `Invalid "path" query string`, http.StatusBadRequest)
			return
		}

		entries, err := os.ReadDir(relpath)
		if err != nil {
			log.Errorf("[FileSystem] Failed to read directory %s: %v", relpath, err)
			http.Error(w, `Failed to read directory`, http.StatusInternalServerError)
			return
		}

		var directories []DirectoryInfo
		for _, entry := range entries {
			if entry.IsDir() {
				// Ignore dot files / dot directories
				if strings.HasPrefix(entry.Name(), ".") {
					continue
				}
				fullPath := filepath.Join(relpath, entry.Name())
				posixPath := filepath.ToSlash(fullPath)
				if excludedPaths[posixPath] {
					continue
				}
				directories = append(directories, DirectoryInfo{
					Path:    posixPath,
					Dirname: entry.Name(),
					Level:   level,
				})
			}
		}

		if directories == nil {
			directories = []DirectoryInfo{}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"posix":       runtime.GOOS != "windows",
			"directories": directories,
		})
	}
}

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
