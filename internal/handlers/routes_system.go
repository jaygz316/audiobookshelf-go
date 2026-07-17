package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"

	"audiobookshelf/internal/core"
	idb "audiobookshelf/internal/db"
	isocket "audiobookshelf/internal/socket"
)

func registerBaseRoutes(mux *http.ServeMux, cfg *core.Config, db *sql.DB, dbConnected bool, version string) {
	dbPath := filepath.Join(cfg.ConfigPath, "absdatabase.sqlite")

	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/ping"), func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Go] GET /ping")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true}`))
	})

	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/healthcheck"), func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Go] GET /healthcheck")
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/status"), func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Go] GET /status")
		if !dbConnected {
			var reconnectErr error
			db, reconnectErr = idb.InitDB(dbPath)
			if reconnectErr == nil {
				dbConnected = true
				SetGlobalDB(db)
				if isocket.GlobalAuth != nil {
					isocket.GlobalAuth.SetDB(db)
				}
				log.Infof("Connected to SQLite database on-demand: %s", dbPath)
				reinitManagers(db)
			}
		}

		if !dbConnected || db == nil {
			log.Infof("[Status] DB not connected.")
			http.Error(w, `{"error": "Database not connected"}`, http.StatusInternalServerError)
			return
		}

		isInit, err := idb.HasRootUser(db)
		if err != nil {
			log.Errorf("[Status] Failed to check root user: %v", err)
			http.Error(w, `{"error": "Failed to check status"}`, http.StatusInternalServerError)
			return
		}

		settings, err := idb.GetServerSettings(db)
		if err != nil {
			log.Errorf("[Status] Failed to get server settings: %v", err)
			http.Error(w, `{"error": "Failed to check status"}`, http.StatusInternalServerError)
			return
		}

		payload := map[string]interface{}{
			"app":           "audiobookshelf",
			"serverVersion": version,
			"isInit":        isInit,
			"language":      settings.Language,
			"authMethods":   settings.AuthActiveAuthMethods,
			"authFormData": map[string]interface{}{
				"authLoginCustomMessage": settings.AuthLoginCustomMessage,
			},
		}

		if !isInit {
			payload["ConfigPath"] = cfg.ConfigPath
			payload["MetadataPath"] = cfg.MetadataPath
		}

		w.Header().Set("Content-Type", "application/json")
		if !settings.AllowIframe {
			w.Header().Set("X-Frame-Options", "SAMEORIGIN")
			w.Header().Set("Content-Security-Policy", "frame-ancestors 'self'")
		}
		w.Header().Set("Referrer-Policy", "no-referrer")
		_ = json.NewEncoder(w).Encode(payload)
	})
}

func registerDocsRoutes(mux *http.ServeMux, cfg *core.Config) {
	docsPath := joinPath(cfg.RouterBasePath, "/docs")

	// Redirect /docs to /docs/ for clean trailing slash
	mux.HandleFunc(docsPath, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, docsPath+"/", http.StatusMovedPermanently)
	})

	if docsFS != nil {
		mux.Handle(docsPath+"/", http.StripPrefix(docsPath+"/", http.FileServer(http.FS(docsFS))))
	} else {
		mux.HandleFunc(docsPath+"/", func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "Documentation not embedded", http.StatusNotFound)
		})
	}
}

func registerFallbackRoutes(mux *http.ServeMux, cfg *core.Config, db *sql.DB, appRoot string) {
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if strings.HasPrefix(path, cfg.RouterBasePath) {
			path = trimBasePath(path, cfg.RouterBasePath)
		}

		prefixes := []string{"/api/", "/auth/", "/hls/", "/public/", "/feed/", "/status", "/login", "/logout", "/init", "/docs/", "/opds/"}
		isBackend := false
		for _, prefix := range prefixes {
			if strings.HasPrefix(path, prefix) || path == prefix[:len(prefix)-1] {
				isBackend = true
				break
			}
		}

		if isBackend {
			log.Warnf("[Backend] 404 Not Found: %s %s", r.Method, r.URL.Path)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error": "API route not found"}`))
			return
		}

		serveStaticOrSPA(subFS, cfg.RouterBasePath)(w, r)
	})
}
