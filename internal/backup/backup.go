// Package backup provides backup creation, restoration, and management functionality.
package backup

import (
	"archive/zip"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"audiobookshelf/internal/core"
	log "audiobookshelf/internal/logger"
	inotification "audiobookshelf/internal/notification"
)

// BackupInfo holds details about a single backup file.
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

// GetServerSettings is a dependency interface for reading server settings.
type ServerSettings struct {
	BackupPath    string
	BackupsToKeep int
}

// GetBackupDirPath resolves the backup directory from server settings or environment.
func GetBackupDirPath(ctx context.Context, db *sql.DB, metadataPath string) string {
	settings, err := getServerSettings(db)
	if err == nil && settings != nil && settings.BackupPath != "" {
		return settings.BackupPath
	}
	return filepath.Join(metadataPath, "backups")
}

// getServerSettings reads the server settings from the database.
func getServerSettings(db *sql.DB) (*ServerSettings, error) {
	var valStr string
	err := db.QueryRow("SELECT value FROM settings WHERE key = 'server-settings'").Scan(&valStr)
	if err != nil {
		return nil, err
	}

	var s struct {
		BackupPath    string `json:"backupPath"`
		BackupsToKeep int    `json:"backupsToKeep"`
	}
	if err := json.Unmarshal([]byte(valStr), &s); err != nil {
		return nil, err
	}

	return &ServerSettings{
		BackupPath:    s.BackupPath,
		BackupsToKeep: s.BackupsToKeep,
	}, nil
}

// LoadBackupsList returns the list of backups sorted by newest first.
func LoadBackupsList(backupDir string) ([]BackupInfo, error) {
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

		b, err := ReadBackupDetailsFromZip(fullPath, info.Size())
		if err != nil {
			log.Printf("[Backups] Warning: failed to read zip %s: %v", file.Name(), err)
			continue
		}

		backups = append(backups, b)
	}

	sort.Slice(backups, func(i, j int) bool {
		return backups[i].CreatedAt > backups[j].CreatedAt
	})

	return backups, nil
}

// ReadBackupDetailsFromZip extracts the backup details from the details file in the ZIP.
func ReadBackupDetailsFromZip(zipPath string, fileSize int64) (BackupInfo, error) {
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

	return parseBackupDetails(detailsContent, zipPath, fileSize), nil
}

func parseBackupDetails(detailsContent string, zipPath string, fileSize int64) BackupInfo {
	lines := strings.Split(detailsContent, "\n")
	filename := filepath.Base(zipPath)

	if len(lines) < 3 {
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
		}
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
	return BackupInfo{
		ID:            id,
		Key:           key,
		BackupDirPath: filepath.Dir(zipPath),
		DatePretty:    t.Format("Mon, Jan _2 2006 15:04"),
		FullPath:      zipPath,
		Path:          filepath.Join("backups", filename),
		Filename:      filename,
		FileSize:      fileSize,
		CreatedAt:     createdAt,
		ServerVersion: serverVersion,
	}
}

// CreateBackup orchestrates generating database snapshot and metadata archives.
func CreateBackup(ctx context.Context, db *sql.DB, configPath, metadataPath, backupDir string) ([]BackupInfo, error) {
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		inotification.TriggerEvent(ctx, db, "onBackupFailed", nil, "Backup Failed", fmt.Sprintf("Failed to create backup directory: %v", err), nil)
		return nil, err
	}

	now := time.Now()
	id := now.Format("2006-01-02T1504")
	fullPath := filepath.Join(backupDir, id+".audiobookshelf")

	tempDBPath, err := createDBSnapshot(ctx, db, configPath, id)
	if err != nil {
		inotification.TriggerEvent(ctx, db, "onBackupFailed", nil, "Backup Failed", fmt.Sprintf("Failed to create DB snapshot: %v", err), nil)
		return nil, err
	}
	defer os.Remove(tempDBPath)

	if err := zipBackupContents(fullPath, tempDBPath, metadataPath, id, now); err != nil {
		inotification.TriggerEvent(ctx, db, "onBackupFailed", nil, "Backup Failed", fmt.Sprintf("Failed to zip backup contents: %v", err), nil)
		return nil, err
	}

	if err := pruneOldBackups(db, backupDir); err != nil {
		log.Printf("[Backups] Pruning old backups failed: %v", err)
	}

	// Trigger onBackupCompleted!
	inotification.TriggerEvent(ctx, db, "onBackupCompleted", nil, "Backup Completed", "Metadata backup completed successfully.", map[string]string{
		"backupId": id,
		"filename": id + ".audiobookshelf",
	})

	return LoadBackupsList(backupDir)
}

func createDBSnapshot(ctx context.Context, db *sql.DB, configPath, id string) (string, error) {
	tempDBPath := filepath.Join(os.TempDir(), fmt.Sprintf("absdatabase-%s.sqlite", id))

	_, err := db.ExecContext(ctx, fmt.Sprintf("VACUUM INTO '%s'", tempDBPath))
	if err == nil {
		return tempDBPath, nil
	}

	log.Printf("[Backups] VACUUM INTO failed: %v. Falling back to simple file copy", err)
	srcFile, err := os.Open(filepath.Join(configPath, "absdatabase.sqlite"))
	if err != nil {
		return "", err
	}
	defer srcFile.Close()

	destFile, err := os.Create(tempDBPath)
	if err != nil {
		return "", err
	}
	defer destFile.Close()

	if _, err = io.Copy(destFile, srcFile); err != nil {
		os.Remove(tempDBPath)
		return "", err
	}

	return tempDBPath, nil
}

func zipBackupContents(destZipPath, tempDBPath, metadataPath, id string, now time.Time) error {
	zipFile, err := os.Create(destZipPath)
	if err != nil {
		return err
	}
	defer zipFile.Close()

	zw := zip.NewWriter(zipFile)
	defer zw.Close()

	if err := addFileToZip(zw, tempDBPath, "absdatabase.sqlite"); err != nil {
		return err
	}

	createdAt := now.UnixNano() / int64(time.Millisecond)
	detailsString := fmt.Sprintf("%s\nsqlite\n%d\n2.8.0\n", id, createdAt)
	writer, err := zw.Create("details")
	if err != nil {
		return err
	}
	if _, err := writer.Write([]byte(detailsString)); err != nil {
		return err
	}

	if err := addDirToZip(zw, filepath.Join(metadataPath, "items"), "metadata-items"); err != nil {
		return err
	}

	if err := addDirToZip(zw, filepath.Join(metadataPath, "authors"), "metadata-authors"); err != nil {
		return err
	}

	// Verify Zip Writer Close error
	if err := zw.Close(); err != nil {
		return err
	}

	return zipFile.Close()
}

func addFileToZip(zw *zip.Writer, srcPath, destName string) error {
	file, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer file.Close()

	fi, err := file.Stat()
	if err != nil {
		return err
	}

	header, err := zip.FileInfoHeader(fi)
	if err != nil {
		return err
	}
	header.Name = destName
	header.Method = zip.Deflate

	writer, err := zw.CreateHeader(header)
	if err != nil {
		return err
	}

	_, err = io.Copy(writer, file)
	return err
}

func addDirToZip(zw *zip.Writer, srcDir string, zipDirName string) error {
	return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
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
		// Windows separator normalization: filepath.ToSlash
		header.Name = filepath.ToSlash(filepath.Join(zipDirName, rel))
		header.Method = zip.Deflate

		writer, err := zw.CreateHeader(header)
		if err != nil {
			return nil
		}
		io.Copy(writer, file)
		return nil
	})
}

func pruneOldBackups(db *sql.DB, backupDir string) error {
	settings, err := getServerSettings(db)
	backupsToKeep := 2
	if err == nil && settings != nil && settings.BackupsToKeep > 0 {
		backupsToKeep = settings.BackupsToKeep
	}

	backups, err := LoadBackupsList(backupDir)
	if err != nil {
		return err
	}

	if len(backups) > backupsToKeep {
		for i := backupsToKeep; i < len(backups); i++ {
			log.Printf("[Backups] Pruning old backup: %s", backups[i].FullPath)
			os.Remove(backups[i].FullPath)
		}
	}
	return nil
}

// DBReconnectFunc is the function signature for reconnecting to the database after restore.
type DBReconnectFunc func(dbPath string) error

// ApplyBackup performs low-level backup restore operations on db and metadata directories.
func ApplyBackup(ctx context.Context, configPath, metadataPath, backupDir, id string, reconnect DBReconnectFunc) error {
	fullPath := filepath.Join(backupDir, id+".audiobookshelf")

	if err := restoreDBFile(configPath, fullPath); err != nil {
		if reconnect != nil {
			reconnect(filepath.Join(configPath, "absdatabase.sqlite"))
		}
		return err
	}

	if err := restoreMetadataFiles(metadataPath, fullPath); err != nil {
		if reconnect != nil {
			reconnect(filepath.Join(configPath, "absdatabase.sqlite"))
		}
		return err
	}

	if reconnect != nil {
		return reconnect(filepath.Join(configPath, "absdatabase.sqlite"))
	}
	return nil
}

func restoreDBFile(configPath, zipPath string) error {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
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
		return fmt.Errorf("backup ZIP missing database file")
	}

	tempDBPath := filepath.Join(configPath, "absdatabase-temp.sqlite")
	tempFile, err := os.Create(tempDBPath)
	if err != nil {
		return err
	}
	defer tempFile.Close()

	rc, err := sqliteEntry.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	if _, err = io.Copy(tempFile, rc); err != nil {
		os.Remove(tempDBPath)
		return err
	}

	tempFile.Close()
	rc.Close()

	realDBPath := filepath.Join(configPath, "absdatabase.sqlite")
	_ = os.Remove(realDBPath)
	_ = os.Remove(realDBPath + "-wal")
	_ = os.Remove(realDBPath + "-shm")
	if err := os.Rename(tempDBPath, realDBPath); err != nil {
		os.Remove(tempDBPath)
		return err
	}

	return nil
}

func restoreMetadataFiles(metadataPath, zipPath string) error {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer zr.Close()

	itemsDir := filepath.Join(metadataPath, "items")
	authorsDir := filepath.Join(metadataPath, "authors")

	for _, f := range zr.File {
		var baseDir string
		var rel string

		if strings.HasPrefix(f.Name, "metadata-items/") {
			baseDir = itemsDir
			rel = strings.TrimPrefix(f.Name, "metadata-items/")
		} else if strings.HasPrefix(f.Name, "metadata-authors/") {
			baseDir = authorsDir
			rel = strings.TrimPrefix(f.Name, "metadata-authors/")
		} else {
			continue
		}

		if rel == "" {
			continue
		}

		dest := filepath.Join(baseDir, rel)

		// Zip Slip vulnerability check
		if !isSafePath(baseDir, dest) {
			return fmt.Errorf("zip slip detected: path %s goes outside %s", dest, baseDir)
		}

		if err := extractZipEntry(f, dest); err != nil {
			return err
		}
	}
	return nil
}

func extractZipEntry(f *zip.File, destPath string) error {
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return err
	}

	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()

	frc, err := f.Open()
	if err != nil {
		return err
	}
	defer frc.Close()

	_, err = io.Copy(out, frc)
	return err
}

func isSafePath(baseDir, targetPath string) bool {
	normalizedBase := filepath.ToSlash(filepath.Clean(baseDir))
	normalizedTarget := filepath.ToSlash(filepath.Clean(targetPath))

	if normalizedTarget == normalizedBase {
		return true
	}

	if !strings.HasSuffix(normalizedBase, "/") {
		normalizedBase += "/"
	}

	return strings.HasPrefix(normalizedTarget, normalizedBase)
}

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

func timeToDBStr(t time.Time) string {
	return t.UTC().Format("2006-01-02 15:04:05.000 +00:00")
}

// HandleGetBackups returns an HTTP handler for GET /api/backups.
func HandleGetBackups(db *sql.DB, metadataPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[Go] GET /api/backups")
		userSess := r.Context().Value(core.UserContextKey).(*core.UserSession)
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

// HandleCreateBackup returns an HTTP handler for POST /api/backups.
func HandleCreateBackup(db *sql.DB, configPath string, metadataPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[Go] POST /api/backups")
		userSess := r.Context().Value(core.UserContextKey).(*core.UserSession)
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

// HandleDeleteBackup returns an HTTP handler for DELETE /api/backups/:id.
func HandleDeleteBackup(db *sql.DB, metadataPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := extractBackupID(r.URL.Path)
		log.Printf("[Go] DELETE /api/backups/%s", id)

		userSess := r.Context().Value(core.UserContextKey).(*core.UserSession)
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

// HandleDownloadBackup returns an HTTP handler for GET /api/backups/:id/download.
func HandleDownloadBackup(db *sql.DB, metadataPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := extractBackupID(r.URL.Path)
		id = strings.TrimSuffix(id, "/download")
		log.Printf("[Go] GET /api/backups/%s/download", id)

		userSess := r.Context().Value(core.UserContextKey).(*core.UserSession)
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

// HandleUpdateBackupPath returns an HTTP handler for PATCH /api/backups/path.
func HandleUpdateBackupPath(db *sql.DB, metadataPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[Go] PATCH /api/backups/path")
		userSess := r.Context().Value(core.UserContextKey).(*core.UserSession)
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

// HandleUploadBackup returns an HTTP handler for POST /api/backups/upload.
func HandleUploadBackup(db *sql.DB, metadataPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[Go] POST /api/backups/upload")
		userSess := r.Context().Value(core.UserContextKey).(*core.UserSession)
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

// HandleApplyBackup returns an HTTP handler for POST /api/backups/:id/apply.
func HandleApplyBackup(db *sql.DB, configPath string, metadataPath string, reconnect DBReconnectFunc, triggerReload func(), broadcastApplied func()) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := extractBackupID(r.URL.Path)
		id = strings.TrimSuffix(id, "/apply")
		log.Printf("[Go] POST /api/backups/%s/apply", id)

		userSess := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if userSess.Type != "root" && userSess.Type != "admin" {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		if !isValidBackupID(id) {
			http.Error(w, `{"error": "Invalid backup ID"}`, http.StatusBadRequest)
			return
		}

		backupDir := GetBackupDirPath(r.Context(), db, metadataPath)
		if err := ApplyBackup(r.Context(), configPath, metadataPath, backupDir, id, reconnect); err != nil {
			log.Printf("[Apply Backup] Restore failed: %v", err)
			http.Error(w, fmt.Sprintf(`{"error": "%v"}`, err), http.StatusInternalServerError)
			return
		}

		if triggerReload != nil {
			triggerReload()
		}

		if broadcastApplied != nil {
			broadcastApplied()
		}

		w.WriteHeader(http.StatusOK)
	}
}

// IsValidBackupIDExported is exported for testing.
func IsValidBackupIDExported(id string) bool {
	return isValidBackupID(id)
}

// IsSafePathExported is exported for testing.
func IsSafePathExported(baseDir, targetPath string) bool {
	return isSafePath(baseDir, targetPath)
}

// RestoreMetadataFilesExported is exported for testing.
func RestoreMetadataFilesExported(metadataPath, zipPath string) error {
	return restoreMetadataFiles(metadataPath, zipPath)
}

// PruneOldBackupsExported is exported for testing.
func PruneOldBackupsExported(db *sql.DB, backupDir string) error {
	return pruneOldBackups(db, backupDir)
}
