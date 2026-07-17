package handlers

import (
	"database/sql"
	"net/http"

	"audiobookshelf/internal/core"
)

func registerBackupRoutes(mux *http.ServeMux, cfg *core.Config, db *sql.DB) {
	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/api/backups"), func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			AuthMiddlewareWrapper(db, handleGetBackups(db, cfg.MetadataPath)).ServeHTTP(w, r)
		} else if r.Method == http.MethodPost {
			AuthMiddlewareWrapper(db, handleCreateBackup(db, cfg.ConfigPath, cfg.MetadataPath)).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/api/backups/path"), func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			AuthMiddlewareWrapper(db, handleUpdateBackupPath(db, cfg.MetadataPath)).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/api/backups/upload"), func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			AuthMiddlewareWrapper(db, handleUploadBackup(db, cfg.MetadataPath)).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/api/backups/"), handleBackupsDispatch(db, cfg))
}

func registerEmailRoutes(mux *http.ServeMux, cfg *core.Config, db *sql.DB) {
	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/api/emails/settings"), func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			AuthMiddlewareWrapper(db, handleGetEmailSettings(db)).ServeHTTP(w, r)
		} else if r.Method == http.MethodPatch {
			AuthMiddlewareWrapper(db, handleUpdateEmailSettings(db)).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/api/emails/test"), func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			AuthMiddlewareWrapper(db, handleSendTestEmail(db)).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/api/emails/ereader-devices"), func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			AuthMiddlewareWrapper(db, handleUpdateEReaderDevices(db)).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/api/emails/devices"), func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			AuthMiddlewareWrapper(db, handleGetAvailableDevices(db)).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/api/emails/send-ebook-to-device"), func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			AuthMiddlewareWrapper(db, handleSendEBookToDevice(db)).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})
}
