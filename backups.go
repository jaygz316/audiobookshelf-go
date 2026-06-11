package main

import (
	"archive/zip"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type BackupInfo struct {
	ID            string `json:"id"`
	Key           string `json:"key"`
	BackupDirPath string `json:"backupDirPath"`
	DatePretty    string `json:"datePretty"`
	FullPath      string `json:"fullPath"`
	Path          string `json:"path"`
	Filename      string `json:"filename"`
	FileSize      int64  `json:"fileSize"`
	CreatedAt     int64  `json:"createdAt"`
	ServerVersion string `json:"serverVersion"`
}

func getBackupDirPath(ctx context.Context, db *sql.DB, metadataPath string) string {
	settings, err := GetServerSettings(db)
	if err == nil && settings != nil && settings.BackupPath != "" {
		return settings.BackupPath
	}
	return filepath.Join(metadataPath, "backups")
}

func loadBackupsList(backupDir string) ([]BackupInfo, error) {
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return nil, err
	}

	files, err := os.ReadDir(backupDir)
	if err != nil {
		return nil, err
	}

	var backups []BackupInfo
	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".audiobookshelf") {
			continue
		}

		fullPath := filepath.Join(backupDir, file.Name())
		info, err := file.Info()
		if err != nil {
			continue
		}

		// Read details entry inside ZIP
		b, err := readBackupDetailsFromZip(fullPath, info.Size())
		if err != nil {
			log.Printf("[Backups] Warning: failed to read zip %s: %v", file.Name(), err)
			continue
		}

		backups = append(backups, b)
	}

	// Sort newest first
	sort.Slice(backups, func(i, j int) bool {
		return backups[i].CreatedAt > backups[j].CreatedAt
	})

	return backups, nil
}

func readBackupDetailsFromZip(zipPath string, fileSize int64) (BackupInfo, error) {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return BackupInfo{}, err
	}
	defer r.Close()

	var detailsContent string
	for _, f := range r.File {
		if f.Name == "details" {
			rc, err := f.Open()
			if err != nil {
				return BackupInfo{}, err
			}
			data, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				return BackupInfo{}, err
			}
			detailsContent = string(data)
			break
		}
	}

	lines := strings.Split(detailsContent, "\n")
	if len(lines) < 3 {
		// Fallback parse from filename
		filename := filepath.Base(zipPath)
		id := strings.TrimSuffix(filename, ".audiobookshelf")
		createdAt := time.Now().UnixNano() / int64(time.Millisecond)
		return BackupInfo{
			ID:            id,
			Key:           "sqlite",
			BackupDirPath: filepath.Dir(zipPath),
			DatePretty:    time.Now().UTC().Format("Mon, Jan _2 2006 15:04"),
			FullPath:      zipPath,
			Path:          filepath.Join("backups", filename),
			Filename:      filename,
			FileSize:      fileSize,
			CreatedAt:     createdAt,
			ServerVersion: "2.8.0",
		}, nil
	}

	id := lines[0]
	key := lines[1]
	var createdAt int64
	fmt.Sscanf(lines[2], "%d", &createdAt)
	serverVersion := "2.8.0"
	if len(lines) > 3 {
		serverVersion = lines[3]
	}

	t := time.Unix(createdAt/1000, 0).UTC()
	datePretty := t.Format("Mon, Jan _2 2006 15:04")

	filename := filepath.Base(zipPath)
	return BackupInfo{
		ID:            id,
		Key:           key,
		BackupDirPath: filepath.Dir(zipPath),
		DatePretty:    datePretty,
		FullPath:      zipPath,
		Path:          filepath.Join("backups", filename),
		Filename:      filename,
		FileSize:      fileSize,
		CreatedAt:     createdAt,
		ServerVersion: serverVersion,
	}, nil
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

		backupDir := getBackupDirPath(r.Context(), db, metadataPath)
		backups, err := loadBackupsList(backupDir)
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

		backupDir := getBackupDirPath(r.Context(), db, metadataPath)
		if err := os.MkdirAll(backupDir, 0755); err != nil {
			http.Error(w, `{"error": "Failed to create backups directory"}`, http.StatusInternalServerError)
			return
		}

		// Generate backup ID
		now := time.Now()
		id := now.Format("2006-01-02T1504")
		filename := id + ".audiobookshelf"
		fullPath := filepath.Join(backupDir, filename)

		// Temporary DB snapshot path
		tempDBPath := filepath.Join(os.TempDir(), fmt.Sprintf("absdatabase-%s.sqlite", id))

		// Try to run sqlite VACUUM INTO to create consistent snapshot
		// Wait, if database is connected, we can run "VACUUM INTO 'tempDBPath'"
		_, err := db.ExecContext(r.Context(), fmt.Sprintf("VACUUM INTO '%s'", tempDBPath))
		if err != nil {
			log.Printf("[Backups] VACUUM INTO failed: %v. Falling back to simple file copy", err)
			// fallback to copying file directly
			srcFile, err := os.Open(filepath.Join(configPath, "absdatabase.sqlite"))
			if err != nil {
				http.Error(w, `{"error": "Failed to open sqlite db"}`, http.StatusInternalServerError)
				return
			}
			destFile, err := os.Create(tempDBPath)
			if err != nil {
				srcFile.Close()
				http.Error(w, `{"error": "Failed to create temp db snapshot"}`, http.StatusInternalServerError)
				return
			}
			_, err = io.Copy(destFile, srcFile)
			srcFile.Close()
			destFile.Close()
			if err != nil {
				os.Remove(tempDBPath)
				http.Error(w, `{"error": "Failed to copy sqlite db snapshot"}`, http.StatusInternalServerError)
				return
			}
		}

		// Create ZIP
		zipFile, err := os.Create(fullPath)
		if err != nil {
			os.Remove(tempDBPath)
			log.Printf("[Backups] ZIP create failed: %v", err)
			http.Error(w, `{"error": "Failed to create backup zip file"}`, http.StatusInternalServerError)
			return
		}
		defer zipFile.Close()

		zw := zip.NewWriter(zipFile)
		defer zw.Close()

		// 1. Add sqlite database
		dbFile, err := os.Open(tempDBPath)
		if err == nil {
			fi, _ := dbFile.Stat()
			header, _ := zip.FileInfoHeader(fi)
			header.Name = "absdatabase.sqlite"
			header.Method = zip.Deflate
			writer, _ := zw.CreateHeader(header)
			io.Copy(writer, dbFile)
			dbFile.Close()
		}
		os.Remove(tempDBPath) // clean up temp file immediately

		// 2. Add details
		createdAt := now.UnixNano() / int64(time.Millisecond)
		detailsString := fmt.Sprintf("%s\nsqlite\n%d\n2.8.0\n", id, createdAt)
		writer, _ := zw.Create("details")
		writer.Write([]byte(detailsString))

		// 3. Add metadata items
		itemsPath := filepath.Join(metadataPath, "items")
		addDirToZip(zw, itemsPath, "metadata-items")

		// 4. Add metadata authors
		authorsPath := filepath.Join(metadataPath, "authors")
		addDirToZip(zw, authorsPath, "metadata-authors")

		zw.Close()
		zipFile.Close()

		// enforce backups to keep limit
		settings, err := GetServerSettings(db)
		backupsToKeep := 2
		if err == nil && settings != nil && settings.BackupsToKeep > 0 {
			backupsToKeep = settings.BackupsToKeep
		}

		backups, err := loadBackupsList(backupDir)
		if err == nil && len(backups) > backupsToKeep {
			// backups are sorted newest first. Delete oldest ones.
			for i := backupsToKeep; i < len(backups); i++ {
				log.Printf("[Backups] Pruning old backup: %s", backups[i].FullPath)
				os.Remove(backups[i].FullPath)
			}
			// reload list
			backups, _ = loadBackupsList(backupDir)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"backups": backups,
		})
	}
}

func addDirToZip(zw *zip.Writer, srcDir string, zipDirName string) {
	filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return nil
		}
		file, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer file.Close()

		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return nil
		}
		header.Name = filepath.Join(zipDirName, rel)
		header.Method = zip.Deflate

		writer, err := zw.CreateHeader(header)
		if err != nil {
			return nil
		}
		io.Copy(writer, file)
		return nil
	})
}

// handleDeleteBackup maps to DELETE /api/backups/:id
func handleDeleteBackup(db *sql.DB, metadataPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := trimAPIPath(r.URL.Path, "/api/backups/")
		log.Printf("[Go] DELETE /api/backups/%s", id)

		userSess := r.Context().Value(UserContextKey).(*UserSession)
		if userSess.Type != "root" && userSess.Type != "admin" {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		backupDir := getBackupDirPath(r.Context(), db, metadataPath)
		filename := id + ".audiobookshelf"
		fullPath := filepath.Join(backupDir, filename)

		if err := os.Remove(fullPath); err != nil {
			log.Printf("[Backups] Delete failed: %v", err)
			http.Error(w, `{"error": "Failed to delete backup"}`, http.StatusNotFound)
			return
		}

		backups, _ := loadBackupsList(backupDir)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"backups": backups,
		})
	}
}

// handleDownloadBackup maps to GET /api/backups/:id/download
func handleDownloadBackup(db *sql.DB, metadataPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := trimAPIPath(r.URL.Path, "/api/backups/")
		id = strings.TrimSuffix(id, "/download")
		log.Printf("[Go] GET /api/backups/%s/download", id)

		userSess := r.Context().Value(UserContextKey).(*UserSession)
		if userSess.Type != "root" && userSess.Type != "admin" {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		backupDir := getBackupDirPath(r.Context(), db, metadataPath)
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

		// Update server-settings
		var valStr string
		err := db.QueryRowContext(r.Context(), "SELECT value FROM settings WHERE key = 'server-settings'").Scan(&valStr)
		currentSettings := make(map[string]interface{})
		if err == nil && valStr != "" {
			json.Unmarshal([]byte(valStr), &currentSettings)
		}

		currentSettings["backupPath"] = newPath
		newValBytes, _ := json.Marshal(currentSettings)
		nowStr := timeToDBStr(time.Now())
		_, err = db.ExecContext(r.Context(), "INSERT INTO settings (key, value, createdAt, updatedAt) VALUES ('server-settings', ?, ?, ?) ON CONFLICT(key) DO UPDATE SET value=excluded.value, updatedAt=excluded.updatedAt",
			string(newValBytes), nowStr, nowStr)
		if err != nil {
			http.Error(w, `{"error": "Failed to update backup settings"}`, http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
	}
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

		err := r.ParseMultipartForm(50 * 1024 * 1024)
		if err != nil {
			http.Error(w, `{"error": "Multipart form parse failed"}`, http.StatusBadRequest)
			return
		}

		file, header, err := r.FormFile("file")
		if err != nil {
			http.Error(w, `{"error": "Failed to get file from request"}`, http.StatusBadRequest)
			return
		}
		defer file.Close()

		if !strings.HasSuffix(header.Filename, ".audiobookshelf") {
			http.Error(w, `{"error": "Invalid backup file type"}`, http.StatusBadRequest)
			return
		}

		backupDir := getBackupDirPath(r.Context(), db, metadataPath)
		if err := os.MkdirAll(backupDir, 0755); err != nil {
			http.Error(w, `{"error": "Failed to create backups directory"}`, http.StatusInternalServerError)
			return
		}

		destPath := filepath.Join(backupDir, header.Filename)
		dest, err := os.Create(destPath)
		if err != nil {
			http.Error(w, `{"error": "Failed to save file"}`, http.StatusInternalServerError)
			return
		}
		defer dest.Close()

		_, err = io.Copy(dest, file)
		if err != nil {
			http.Error(w, `{"error": "Failed to save file contents"}`, http.StatusInternalServerError)
			return
		}

		backups, _ := loadBackupsList(backupDir)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"backups": backups,
		})
	}
}

// handleApplyBackup maps to GET /api/backups/:id/apply
func handleApplyBackup(db *sql.DB, configPath string, metadataPath string, triggerReload func()) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := trimAPIPath(r.URL.Path, "/api/backups/")
		id = strings.TrimSuffix(id, "/apply")
		log.Printf("[Go] GET /api/backups/%s/apply", id)

		userSess := r.Context().Value(UserContextKey).(*UserSession)
		if userSess.Type != "root" && userSess.Type != "admin" {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		backupDir := getBackupDirPath(r.Context(), db, metadataPath)
		filename := id + ".audiobookshelf"
		fullPath := filepath.Join(backupDir, filename)

		// 1. Unzip the SQLite file and extract it
		zr, err := zip.OpenReader(fullPath)
		if err != nil {
			log.Printf("[Apply Backup] Failed to open zip: %v", err)
			http.Error(w, `{"error": "Failed to open backup zip"}`, http.StatusBadRequest)
			return
		}
		defer zr.Close()

		var sqliteEntry *zip.File
		for _, f := range zr.File {
			if f.Name == "absdatabase.sqlite" {
				sqliteEntry = f
				break
			}
		}

		if sqliteEntry == nil {
			http.Error(w, `{"error": "Backup ZIP missing database file"}`, http.StatusBadRequest)
			return
		}

		// Disconnect DB connection in Go
		globalDB.Close()

		tempDBPath := filepath.Join(configPath, "absdatabase-temp.sqlite")
		tempFile, err := os.Create(tempDBPath)
		if err != nil {
			log.Printf("[Apply Backup] Failed to create temp db file: %v", err)
			dbFile, _ := initDB(filepath.Join(configPath, "absdatabase.sqlite"))
			globalDB = dbFile
			http.Error(w, `{"error": "Internal Restore Error"}`, http.StatusInternalServerError)
			return
		}

		rc, err := sqliteEntry.Open()
		if err == nil {
			_, err = io.Copy(tempFile, rc)
			rc.Close()
		}
		tempFile.Close()

		if err != nil {
			log.Printf("[Apply Backup] Extract failed: %v", err)
			os.Remove(tempDBPath)
			dbFile, _ := initDB(filepath.Join(configPath, "absdatabase.sqlite"))
			globalDB = dbFile
			http.Error(w, `{"error": "Failed to extract database from backup"}`, http.StatusInternalServerError)
			return
		}

		// Replace absdatabase.sqlite
		realDBPath := filepath.Join(configPath, "absdatabase.sqlite")
		_ = os.Remove(realDBPath)
		err = os.Rename(tempDBPath, realDBPath)
		if err != nil {
			log.Printf("[Apply Backup] Failed to replace sqlite db: %v", err)
			os.Remove(tempDBPath)
			dbFile, _ := initDB(filepath.Join(configPath, "absdatabase.sqlite"))
			globalDB = dbFile
			http.Error(w, `{"error": "Failed to replace database file"}`, http.StatusInternalServerError)
			return
		}

		// Extract metadata items & authors
		itemsDir := filepath.Join(metadataPath, "items")
		authorsDir := filepath.Join(metadataPath, "authors")

		for _, f := range zr.File {
			if strings.HasPrefix(f.Name, "metadata-items/") {
				rel := strings.TrimPrefix(f.Name, "metadata-items/")
				if rel == "" {
					continue
				}
				dest := filepath.Join(itemsDir, rel)
				os.MkdirAll(filepath.Dir(dest), 0755)
				out, err := os.Create(dest)
				if err == nil {
					frc, err2 := f.Open()
					if err2 == nil {
						io.Copy(out, frc)
						frc.Close()
					}
					out.Close()
				}
			} else if strings.HasPrefix(f.Name, "metadata-authors/") {
				rel := strings.TrimPrefix(f.Name, "metadata-authors/")
				if rel == "" {
					continue
				}
				dest := filepath.Join(authorsDir, rel)
				os.MkdirAll(filepath.Dir(dest), 0755)
				out, err := os.Create(dest)
				if err == nil {
					frc, err2 := f.Open()
					if err2 == nil {
						io.Copy(out, frc)
						frc.Close()
					}
					out.Close()
				}
			}
		}

		// Reconnect DB
		dbFile, err := initDB(realDBPath)
		if err != nil {
			log.Printf("[Apply Backup] Failed to reconnect to restored DB: %v", err)
			http.Error(w, `{"error": "Restored database connection failed"}`, http.StatusInternalServerError)
			return
		}
		globalDB = dbFile
		if SocketAuth != nil {
			SocketAuth.db = dbFile
		}

		if triggerReload != nil {
			triggerReload()
		}

		// Emit socket broadcast if needed
		if SocketAuth != nil {
			SocketAuth.BroadcastToAll("backup_applied", nil)
		}

		w.WriteHeader(http.StatusOK)
	}
}
