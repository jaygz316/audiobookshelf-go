package handlers

import (
	log "audiobookshelf/internal/logger"
	"encoding/json"
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
