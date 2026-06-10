package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func isValidBackupID(id string) bool {
	if id == "" {
		return false
	}
	// Reject ID parameters containing "..", "/", or "\" to prevent path traversal
	if strings.Contains(id, "..") || strings.Contains(id, "/") || strings.Contains(id, "\\") {
		return false
	}
	return true
}

func extractBackupID(path string) string {
	idx := strings.Index(path, "/api/backups/")
	if idx != -1 {
		return path[idx+len("/api/backups/"):]
	}
	return strings.TrimPrefix(path, "/")
}

// handleGetBackups maps to GET /api/backups
func handleGetBackups(db *sql.DB, metadataPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[Go] GET /api/backups")
		userSess := r.Context().Value(UserContextKey).(*UserSession)
		if userSess.Type != "root" && userSess.Type != "admin" {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		backupDir := GetBackupDirPath(r.Context(), db, metadataPath)
		backups, err := LoadBackupsList(backupDir)
		if err != nil {
			log.Printf("[Backups] Failed to load list: %v", err)
			http.Error(w, `{"error": "Failed to load backups"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"backups":          backups,
			"backupLocation":   backupDir,
			"backupPathEnvSet": os.Getenv("BACKUP_PATH") != "",
		})
	}
}

// handleCreateBackup maps to POST /api/backups
func handleCreateBackup(db *sql.DB, configPath string, metadataPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[Go] POST /api/backups")
		userSess := r.Context().Value(UserContextKey).(*UserSession)
		if userSess.Type != "root" && userSess.Type != "admin" {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		backupDir := GetBackupDirPath(r.Context(), db, metadataPath)
		backups, err := CreateBackup(r.Context(), db, configPath, metadataPath, backupDir)
		if err != nil {
			log.Printf("[Backups] Create failed: %v", err)
			http.Error(w, `{"error": "Failed to create backup"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"backups": backups,
		})
	}
}

// handleDeleteBackup maps to DELETE /api/backups/:id
func handleDeleteBackup(db *sql.DB, metadataPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := extractBackupID(r.URL.Path)
		log.Printf("[Go] DELETE /api/backups/%s", id)

		userSess := r.Context().Value(UserContextKey).(*UserSession)
		if userSess.Type != "root" && userSess.Type != "admin" {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		if !isValidBackupID(id) {
			http.Error(w, `{"error": "Invalid backup ID"}`, http.StatusBadRequest)
			return
		}

		backupDir := GetBackupDirPath(r.Context(), db, metadataPath)
		filename := id + ".audiobookshelf"
		fullPath := filepath.Join(backupDir, filename)

		if err := os.Remove(fullPath); err != nil {
			log.Printf("[Backups] Delete failed: %v", err)
			http.Error(w, `{"error": "Failed to delete backup"}`, http.StatusNotFound)
			return
		}

		backups, _ := LoadBackupsList(backupDir)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"backups": backups,
		})
	}
}

// handleDownloadBackup maps to GET /api/backups/:id/download
func handleDownloadBackup(db *sql.DB, metadataPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := extractBackupID(r.URL.Path)
		id = strings.TrimSuffix(id, "/download")
		log.Printf("[Go] GET /api/backups/%s/download", id)

		userSess := r.Context().Value(UserContextKey).(*UserSession)
		if userSess.Type != "root" && userSess.Type != "admin" {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		if !isValidBackupID(id) {
			http.Error(w, `{"error": "Invalid backup ID"}`, http.StatusBadRequest)
			return
		}

		backupDir := GetBackupDirPath(r.Context(), db, metadataPath)
		filename := id + ".audiobookshelf"
		fullPath := filepath.Join(backupDir, filename)

		w.Header().Set("Content-Disposition", "attachment; filename="+filename)
		http.ServeFile(w, r, fullPath)
	}
}

// handleUpdateBackupPath maps to PATCH /api/backups/path
func handleUpdateBackupPath(db *sql.DB, metadataPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[Go] PATCH /api/backups/path")
		userSess := r.Context().Value(UserContextKey).(*UserSession)
		if userSess.Type != "root" && userSess.Type != "admin" {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		var body struct {
			Path string `json:"path"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, `{"error": "Invalid request"}`, http.StatusBadRequest)
			return
		}

		newPath := filepath.Clean(body.Path)
		if err := os.MkdirAll(newPath, 0755); err != nil {
			log.Printf("[Backups] Failed to create path: %v", err)
			http.Error(w, `{"error": "Invalid path"}`, http.StatusBadRequest)
			return
		}

		if err := updateBackupPathInSettings(r.Context(), db, newPath); err != nil {
			http.Error(w, `{"error": "Failed to update backup settings"}`, http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
	}
}

func updateBackupPathInSettings(ctx context.Context, db *sql.DB, newPath string) error {
	var valStr string
	err := db.QueryRowContext(ctx, "SELECT value FROM settings WHERE key = 'server-settings'").Scan(&valStr)
	currentSettings := make(map[string]interface{})
	if err == nil && valStr != "" {
		json.Unmarshal([]byte(valStr), &currentSettings)
	}

	currentSettings["backupPath"] = newPath
	newValBytes, _ := json.Marshal(currentSettings)
	nowStr := timeToDBStr(time.Now())

	_, err = db.ExecContext(ctx, "INSERT INTO settings (key, value, createdAt, updatedAt) VALUES ('server-settings', ?, ?, ?) ON CONFLICT(key) DO UPDATE SET value=excluded.value, updatedAt=excluded.updatedAt",
		string(newValBytes), nowStr, nowStr)
	return err
}

// handleUploadBackup maps to POST /api/backups/upload
func handleUploadBackup(db *sql.DB, metadataPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[Go] POST /api/backups/upload")
		userSess := r.Context().Value(UserContextKey).(*UserSession)
		if userSess.Type != "root" && userSess.Type != "admin" {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		if err := r.ParseMultipartForm(50 * 1024 * 1024); err != nil {
			http.Error(w, `{"error": "Multipart form parse failed"}`, http.StatusBadRequest)
			return
		}
		// Clean up multipart temp files on disk
		defer r.MultipartForm.RemoveAll()

		file, header, err := r.FormFile("file")
		if err != nil {
			http.Error(w, `{"error": "Failed to get file from request"}`, http.StatusBadRequest)
			return
		}
		defer file.Close()

		// Path Traversal in Upload File: sanitize using filepath.Base
		safeFilename := filepath.Base(strings.ReplaceAll(header.Filename, "\\", "/"))
		if !strings.HasSuffix(safeFilename, ".audiobookshelf") {
			http.Error(w, `{"error": "Invalid backup file type"}`, http.StatusBadRequest)
			return
		}

		backupDir := GetBackupDirPath(r.Context(), db, metadataPath)
		if err := os.MkdirAll(backupDir, 0755); err != nil {
			http.Error(w, `{"error": "Failed to create backups directory"}`, http.StatusInternalServerError)
			return
		}

		destPath := filepath.Join(backupDir, safeFilename)
		dest, err := os.Create(destPath)
		if err != nil {
			http.Error(w, `{"error": "Failed to save file"}`, http.StatusInternalServerError)
			return
		}
		defer dest.Close()

		if _, err = io.Copy(dest, file); err != nil {
			http.Error(w, `{"error": "Failed to save file contents"}`, http.StatusInternalServerError)
			return
		}

		backups, _ := LoadBackupsList(backupDir)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"backups": backups,
		})
	}
}

// handleApplyBackup maps to POST /api/backups/:id/apply
func handleApplyBackup(db *sql.DB, configPath string, metadataPath string, triggerReload func()) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := extractBackupID(r.URL.Path)
		id = strings.TrimSuffix(id, "/apply")
		log.Printf("[Go] POST /api/backups/%s/apply", id)

		userSess := r.Context().Value(UserContextKey).(*UserSession)
		if userSess.Type != "root" && userSess.Type != "admin" {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		if !isValidBackupID(id) {
			http.Error(w, `{"error": "Invalid backup ID"}`, http.StatusBadRequest)
			return
		}

		backupDir := GetBackupDirPath(r.Context(), db, metadataPath)
		if err := ApplyBackup(r.Context(), configPath, metadataPath, backupDir, id); err != nil {
			log.Printf("[Apply Backup] Restore failed: %v", err)
			http.Error(w, fmt.Sprintf(`{"error": "%v"}`, err), http.StatusInternalServerError)
			return
		}

		if triggerReload != nil {
			triggerReload()
		}

		if SocketAuth != nil {
			SocketAuth.BroadcastToAll("backup_applied", nil)
		}

		w.WriteHeader(http.StatusOK)
	}
}
