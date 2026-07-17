package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"audiobookshelf/internal/auth"
	"audiobookshelf/internal/core"
	ihls "audiobookshelf/internal/hls"
	iscanner "audiobookshelf/internal/scanner"
	isocket "audiobookshelf/internal/socket"
	iwatcher "audiobookshelf/internal/watcher"
)

func registerTasksAndOtherRoutes(mux *http.ServeMux, cfg *core.Config, db *sql.DB, metadataPath string) {
	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/api/tasks"), func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			AuthMiddlewareWrapper(db, handleGetTasks(db)).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/api/tasks/"), func(w http.ResponseWriter, r *http.Request) {
		pathWithoutPrefix := trimBasePath(r.URL.Path, cfg.RouterBasePath)
		actionPath := strings.TrimPrefix(pathWithoutPrefix, "/api/tasks/")

		if actionPath == "cancel-all" {
			if r.Method == http.MethodPost {
				AuthMiddlewareWrapper(db, handleCancelAllTasks(db)).ServeHTTP(w, r)
				return
			}
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
			return
		}

		parts := strings.Split(actionPath, "/")
		if len(parts) == 2 {
			taskID := parts[0]
			action := parts[1]
			if r.Method == http.MethodPost {
				AuthMiddlewareWrapper(db, handleSingleTaskAction(db, taskID, action)).ServeHTTP(w, r)
				return
			}
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
			return
		}

		if r.Method == http.MethodGet {
			AuthMiddlewareWrapper(db, handleGetTasks(db)).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/api/authors/"), handleAuthorsDispatch(db, cfg))
	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/api/series/"), handleSeriesDispatch(db, cfg))

	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/api/cache/purge-all"), func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		AuthMiddlewareWrapper(db, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userSess, ok := r.Context().Value(core.UserContextKey).(*core.UserSession)
			if !ok || (userSess.Type != "root" && userSess.Type != "admin") {
				http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
				return
			}
			cacheDir := filepath.Join(metadataPath, "cache")
			if err := removeContents(cacheDir); err != nil {
				log.Errorf("[Cache Purge] Failed to purge cache: %v", err)
				http.Error(w, fmt.Sprintf(`{"error": "Failed to purge cache: %s"}`, err.Error()), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"message": "Cache purged successfully"}`))
		})).ServeHTTP(w, r)
	})

	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/api/cache/purge-items"), func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		AuthMiddlewareWrapper(db, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userSess, ok := r.Context().Value(core.UserContextKey).(*core.UserSession)
			if !ok || (userSess.Type != "root" && userSess.Type != "admin") {
				http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
				return
			}
			coversDir := filepath.Join(metadataPath, "cache", "covers")
			if err := removeContents(coversDir); err != nil {
				log.Errorf("[Cache Purge] Failed to purge items cover cache: %v", err)
				http.Error(w, fmt.Sprintf(`{"error": "Failed to purge items cover cache: %s"}`, err.Error()), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"message": "Items cover cache purged successfully"}`))
		})).ServeHTTP(w, r)
	})

	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/feed/"), func(w http.ResponseWriter, r *http.Request) {
		initManagers(db)
		pathWithoutPrefix := trimBasePath(r.URL.Path, cfg.RouterBasePath)
		subPath := strings.TrimPrefix(pathWithoutPrefix, "/feed/")
		parts := strings.Split(subPath, "/")
		if len(parts) == 0 || parts[0] == "" {
			http.NotFound(w, r)
			return
		}
		slug := parts[0]
		globalFeedManager.ServeRSSFeed(slug).ServeHTTP(w, r)
	})

	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/auth/openid"), func(w http.ResponseWriter, r *http.Request) {
		s, err := getOIDCSettings(db)
		if err != nil || (s.IssuerURL == "" && (s.AuthorizationURL == "" || s.TokenURL == "")) {
			log.Infof("[OIDC Login] Error getting OIDC settings or not configured: %v", err)
			http.Error(w, "OIDC is not configured or settings error", http.StatusBadRequest)
			return
		}
		globalOIDCHandlerMu.Lock()
		if globalOIDCHandler == nil {
			globalOIDCHandler = auth.NewOIDCHandler(s, nil)
		} else {
			globalOIDCHandler.UpdateSettings(s)
		}
		globalOIDCHandlerMu.Unlock()
		globalOIDCHandler.HandleLogin(w, r)
	})
	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/auth/openid/callback"), func(w http.ResponseWriter, r *http.Request) {
		handleOIDCCallback(db)(w, r)
	})
	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/auth/openid/mobile-redirect"), func(w http.ResponseWriter, r *http.Request) {
		handleOIDCCallback(db)(w, r)
	})

	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/api/items/"), HandleItemsDispatch(db, cfg))

	isocket.GlobalAuth = isocket.NewAuthority(db)
	log.LogCallback = isocket.GlobalAuth.BroadcastLog
	socketHandler := isocket.InitSocketAuthority(isocket.GlobalAuth)
	mux.Handle(joinPath(cfg.RouterBasePath, "/socket.io/"), socketHandler)
	if cfg.RouterBasePath != "" && cfg.RouterBasePath != "/" {
		mux.Handle("/socket.io/", socketHandler)
	}

	iwatcher.InitFSWatcher(db, func(db *sql.DB, libraryID string) error {
		return iscanner.ScanLibrary(db, libraryID, isocket.GlobalAuth)
	})

	mux.Handle(joinPath(cfg.RouterBasePath, "/hls/"), AuthMiddlewareWrapper(db, ihls.ServeHLS(db, metadataPath, streamManager, isocket.GlobalAuth)))
}

func removeContents(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // Already deleted or doesn't exist yet
		}
		return err
	}
	defer d.Close()
	names, err := d.Readdirnames(-1)
	if err != nil {
		return err
	}
	for _, name := range names {
		err = os.RemoveAll(filepath.Join(dir, name))
		if err != nil {
			return err
		}
	}
	return nil
}
