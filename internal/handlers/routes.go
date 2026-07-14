package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"encoding/json"
	"flag"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"audiobookshelf/internal/auth"
	"audiobookshelf/internal/core"
	idb "audiobookshelf/internal/db"
	ihls "audiobookshelf/internal/hls"
	ilogger "audiobookshelf/internal/logger"
	iscanner "audiobookshelf/internal/scanner"
	isocket "audiobookshelf/internal/socket"
	iwatcher "audiobookshelf/internal/watcher"
)

var subFS fs.FS

func SetSubFS(f fs.FS) {
	subFS = f
}

var docsFS fs.FS

func SetDocsFS(f fs.FS) {
	docsFS = f
}

var MetadataPath string

func joinPath(basePath, routePath string) string {
	if basePath == "" {
		basePath = "/"
	}
	if !strings.HasPrefix(routePath, "/") {
		routePath = "/" + routePath
	}
	if basePath == "/" {
		return routePath
	}
	basePath = strings.TrimSuffix(basePath, "/")
	return basePath + routePath
}

func trimBasePath(p, base string) string {
	if base == "" || base == "/" {
		return p
	}
	trimmed := strings.TrimPrefix(p, base)
	if !strings.HasPrefix(trimmed, "/") {
		trimmed = "/" + trimmed
	}
	return trimmed
}

func SetupHandler(db *sql.DB, cfg *core.Config, dbConnected bool, appRoot string, version string) http.Handler {
	globalDB = db
	MetadataPath = cfg.MetadataPath
	iscanner.MetadataPath = cfg.MetadataPath
	mux := http.NewServeMux()

	registerBaseRoutes(mux, cfg, db, dbConnected, version)
	registerAuthAndUserRoutes(mux, cfg, db, appRoot)
	registerLibraryRoutes(mux, cfg, db)
	registerPodcastRoutes(mux, cfg, db)
	registerPlaylistCollectionRoutes(mux, cfg, db)
	registerShareRoutes(mux, cfg, db)
	registerSearchRoutes(mux, cfg, db)
	registerBackupRoutes(mux, cfg, db)
	registerMiscRoutes(mux, cfg, db, appRoot)
	registerDocsRoutes(mux, cfg)
	registerFallbackRoutes(mux, cfg, db, appRoot)

	mainHandler := BasePathRewriteMiddleware(cfg.RouterBasePath, mux)
	handlerWithCORS := CORSMiddleware(db, mainHandler)
	handlerWithLogging := LoggingMiddleware(handlerWithCORS)
	return MetricsMiddleware(handlerWithLogging)
}

func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ilogger.Info("[HTTP] Request",
			"method", r.Method,
			"path", r.URL.Path,
			"remote", r.RemoteAddr,
			"userAgent", r.UserAgent(),
		)

		sanitizedHeaders := make(http.Header)
		for k, v := range r.Header {
			lowerK := strings.ToLower(k)
			if strings.Contains(lowerK, "token") ||
				strings.Contains(lowerK, "key") ||
				strings.Contains(lowerK, "secret") ||
				strings.Contains(lowerK, "auth") ||
				strings.Contains(lowerK, "cookie") ||
				strings.Contains(lowerK, "password") {
				sanitizedHeaders[k] = []string{"[REDACTED]"}
			} else {
				sanitizedHeaders[k] = v
			}
		}
		ilogger.Info("[HTTP] Request Headers", "headers", sanitizedHeaders)
		next.ServeHTTP(w, r)
	})
}

type corsResponseWriter struct {
	http.ResponseWriter
	allowedOrigin string
	headersSet    bool
}

func (w *corsResponseWriter) setCORSHeaders() {
	if w.headersSet {
		return
	}
	h := w.ResponseWriter.Header()
	if w.allowedOrigin != "" {
		h.Set("Access-Control-Allow-Origin", w.allowedOrigin)
		h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		h.Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With, Origin, Accept")
		h.Set("Access-Control-Allow-Credentials", "true")
	} else {
		h.Del("Access-Control-Allow-Origin")
		h.Del("Access-Control-Allow-Methods")
		h.Del("Access-Control-Allow-Headers")
		h.Del("Access-Control-Allow-Credentials")
	}
	w.headersSet = true
}

func (w *corsResponseWriter) WriteHeader(statusCode int) {
	w.setCORSHeaders()
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *corsResponseWriter) Write(b []byte) (int, error) {
	w.setCORSHeaders()
	return w.ResponseWriter.Write(b)
}

func CORSMiddleware(db *sql.DB, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		var allowedOrigin string

		if origin != "" {
			settings, err := idb.GetServerSettings(db)
			if err == nil && settings != nil && settings.AllowedCorsOrigins != "" {
				origins := strings.Split(settings.AllowedCorsOrigins, ",")
				for _, o := range origins {
					if strings.TrimSpace(o) == origin {
						allowedOrigin = origin
						break
					}
				}
			}
		}

		if r.Method == "OPTIONS" {
			if allowedOrigin != "" {
				w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With, Origin, Accept")
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}

		writerWrapper := &corsResponseWriter{
			ResponseWriter: w,
			allowedOrigin:  allowedOrigin,
		}

		next.ServeHTTP(writerWrapper, r)
	})
}

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
				globalDB = db
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

func registerAuthAndUserRoutes(mux *http.ServeMux, cfg *core.Config, db *sql.DB, appRoot string) {
	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/login"), func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			RateLimitMiddleware(LoginRateLimiter)(http.HandlerFunc(handleLogin(db))).ServeHTTP(w, r)
		} else if r.Method == http.MethodGet {
			r.URL.Path = joinPath(cfg.RouterBasePath, "/index.html")
			serveStaticOrSPA(subFS, cfg.RouterBasePath)(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/logout"), func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			handleLogout(db)(w, r)
		} else if r.Method == http.MethodGet {
			r.URL.Path = joinPath(cfg.RouterBasePath, "/index.html")
			serveStaticOrSPA(subFS, cfg.RouterBasePath)(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/init"), func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			RateLimitMiddleware(LoginRateLimiter)(http.HandlerFunc(handleInit(db))).ServeHTTP(w, r)
		} else if r.Method == http.MethodGet {
			r.URL.Path = joinPath(cfg.RouterBasePath, "/index.html")
			serveStaticOrSPA(subFS, cfg.RouterBasePath)(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/auth/logout"), func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			handleLogout(db)(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/auth/refresh"), func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			handleRefresh(db)(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/api/authorize"), func(w http.ResponseWriter, r *http.Request) {
		AuthMiddlewareWrapper(db, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost || r.Method == http.MethodGet {
				handleAuthorize(db)(w, r)
			} else {
				http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
			}
		})).ServeHTTP(w, r)
	})

	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/api/users/online"), func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			AuthMiddlewareWrapper(db, handleGetOnlineUsers(db)).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})

	handleUsersDispatch := func(db *sql.DB) http.HandlerFunc {
		usersHandler := handleGetUsers(db)
		crudHandler := handleUserCRUD(db)
		return func(w http.ResponseWriter, r *http.Request) {
			pathWithoutPrefix := trimBasePath(r.URL.Path, cfg.RouterBasePath)
			if pathWithoutPrefix == "/api/users" || pathWithoutPrefix == "/api/users/" {
				if r.Method == http.MethodGet {
					usersHandler(w, r)
					return
				}
			}
			crudHandler(w, r)
		}
	}

	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/api/users"), func(w http.ResponseWriter, r *http.Request) {
		AuthMiddlewareWrapper(db, handleUsersDispatch(db)).ServeHTTP(w, r)
	})
	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/api/users/"), func(w http.ResponseWriter, r *http.Request) {
		AuthMiddlewareWrapper(db, handleUsersDispatch(db)).ServeHTTP(w, r)
	})
}

func registerLibraryRoutes(mux *http.ServeMux, cfg *core.Config, db *sql.DB) {
	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/api/libraries"), func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Go] %s /api/libraries", r.Method)
		if r.Method == http.MethodGet {
			AuthMiddlewareWrapper(db, http.HandlerFunc(HandleGetLibraries(db))).ServeHTTP(w, r)
		} else if r.Method == http.MethodPost {
			AuthMiddlewareWrapper(db, http.HandlerFunc(HandleCreateLibrary(db))).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/api/libraries/"), handleLibrariesDispatch(db, cfg))

	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/api/upload"), func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			AuthMiddlewareWrapper(db, handleUpload(db)).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})
}

func registerPodcastRoutes(mux *http.ServeMux, cfg *core.Config, db *sql.DB) {
	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/api/podcasts"), func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			AuthMiddlewareWrapper(db, handleCreatePodcast(db)).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/api/podcasts/"), handlePodcastsDispatch(db, cfg))
}

func registerPlaylistCollectionRoutes(mux *http.ServeMux, cfg *core.Config, db *sql.DB) {
	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/api/playlists"), func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			AuthMiddlewareWrapper(db, http.HandlerFunc(handleGetPlaylists(db))).ServeHTTP(w, r)
		} else if r.Method == http.MethodPost {
			AuthMiddlewareWrapper(db, http.HandlerFunc(handleCreatePlaylist(db))).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/api/playlists/"), handlePlaylistsDispatch(db, cfg))

	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/api/collections"), func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			AuthMiddlewareWrapper(db, http.HandlerFunc(handleGetCollections(db))).ServeHTTP(w, r)
		} else if r.Method == http.MethodPost {
			AuthMiddlewareWrapper(db, http.HandlerFunc(handleCreateCollection(db))).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/api/collections/"), handleCollectionsDispatch(db, cfg))
}

func registerShareRoutes(mux *http.ServeMux, cfg *core.Config, db *sql.DB) {
	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/api/share/mediaitem"), func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			AuthMiddlewareWrapper(db, http.HandlerFunc(handleCreateShare(db))).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/api/share/mediaitem/"), handleSharesDispatch(db, cfg))

	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/api/shares"), func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			AuthMiddlewareWrapper(db, http.HandlerFunc(handleGetShares(db))).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/api/s/"), func(w http.ResponseWriter, r *http.Request) {
		RateLimitMiddleware(ShareRateLimiter)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			pathWithoutPrefix := trimBasePath(r.URL.Path, cfg.RouterBasePath)
			subPath := strings.TrimPrefix(pathWithoutPrefix, "/api/s/")
			parts := strings.Split(subPath, "/")
			if len(parts) == 0 || parts[0] == "" {
				http.NotFound(w, r)
				return
			}
			// parts[0] is the slug
			if len(parts) == 2 {
				if parts[1] == "download" {
					if r.Method == http.MethodGet {
						handleGetPublicShareDownload(db).ServeHTTP(w, r)
						return
					}
				} else if parts[1] == "stream" {
					if r.Method == http.MethodGet {
						handleGetPublicShareStream(db).ServeHTTP(w, r)
						return
					}
				} else if parts[1] == "cover" {
					if r.Method == http.MethodGet {
						handleGetPublicShareCover(db, cfg.MetadataPath).ServeHTTP(w, r)
						return
					}
				}
			} else if len(parts) == 1 {
				if r.Method == http.MethodGet {
					handleGetPublicShare(db).ServeHTTP(w, r)
					return
				}
			}
			http.NotFound(w, r)
		})).ServeHTTP(w, r)
	})
}

func registerSearchRoutes(mux *http.ServeMux, cfg *core.Config, db *sql.DB) {
	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/api/search/books"), func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			AuthMiddlewareWrapper(db, http.HandlerFunc(handleSearchBooks(db))).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/api/search/podcast"), func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			AuthMiddlewareWrapper(db, http.HandlerFunc(handleSearchPodcasts(db))).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/api/search/authors"), func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			AuthMiddlewareWrapper(db, http.HandlerFunc(handleSearchAuthors(db))).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})
}

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

func registerSettingsRoutes(mux *http.ServeMux, cfg *core.Config, db *sql.DB) {
	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/api/settings"), func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			AuthMiddlewareWrapper(db, handleGetServerSettings(db)).ServeHTTP(w, r)
		} else if r.Method == http.MethodPatch {
			AuthMiddlewareWrapper(db, handleUpdateServerSettings(db)).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/api/sorting-prefixes"), func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch {
			AuthMiddlewareWrapper(db, handleUpdateSortingPrefixes(db)).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/api/auth-settings"), func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			AuthMiddlewareWrapper(db, handleGetAuthSettings(db)).ServeHTTP(w, r)
		} else if r.Method == http.MethodPatch {
			AuthMiddlewareWrapper(db, handleUpdateAuthSettings(db)).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/api/playback-sessions"), func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			AuthMiddlewareWrapper(db, handleGetPlaybackSessions(db)).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/api/playback-sessions/"), func(w http.ResponseWriter, r *http.Request) {
		subPath := strings.TrimPrefix(r.URL.Path, joinPath(cfg.RouterBasePath, "/api/playback-sessions/"))
		parts := strings.Split(subPath, "/")
		if len(parts) == 1 && parts[0] != "" {
			sessionID := parts[0]
			if r.Method == http.MethodDelete {
				AuthMiddlewareWrapper(db, handleClosePlaybackSession(db, sessionID)).ServeHTTP(w, r)
				return
			}
		}
		http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
	})
	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/api/session/"), func(w http.ResponseWriter, r *http.Request) {
		subPath := strings.TrimPrefix(r.URL.Path, joinPath(cfg.RouterBasePath, "/api/session/"))
		parts := strings.Split(subPath, "/")
		if len(parts) == 1 {
			if parts[0] == "local-all" {
				if r.Method == http.MethodPost {
					AuthMiddlewareWrapper(db, handleSyncLocalSessions(db)).ServeHTTP(w, r)
					return
				}
			} else if parts[0] == "local" {
				if r.Method == http.MethodPost {
					AuthMiddlewareWrapper(db, handleSyncLocalSession(db)).ServeHTTP(w, r)
					return
				}
			}
		}
		http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
	})
}

func registerMetadataRoutes(mux *http.ServeMux, cfg *core.Config, db *sql.DB) {
	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/api/search/providers"), func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			AuthMiddlewareWrapper(db, handleGetMetadataProviders(db)).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/api/custom-metadata-providers"), func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			AuthMiddlewareWrapper(db, handleGetCustomMetadataProviders(db)).ServeHTTP(w, r)
		} else if r.Method == http.MethodPost {
			AuthMiddlewareWrapper(db, handleCreateCustomMetadataProvider(db)).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/api/custom-metadata-providers/"), func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			AuthMiddlewareWrapper(db, handleDeleteCustomMetadataProvider(db)).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})
}

func registerMockAndFeedRoutes(mux *http.ServeMux, cfg *core.Config, db *sql.DB) {
	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/api/api-keys"), func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			AuthMiddlewareWrapper(db, handleGetApiKeys(db)).ServeHTTP(w, r)
		} else if r.Method == http.MethodPost {
			RateLimitMiddleware(LoginRateLimiter)(AuthMiddlewareWrapper(db, handlePostApiKey(db))).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/api/api-keys/"), func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			AuthMiddlewareWrapper(db, handleDeleteApiKey(db)).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/api/notifications"), func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			AuthMiddlewareWrapper(db, handleGetNotifications(db)).ServeHTTP(w, r)
		} else if r.Method == http.MethodPost || r.Method == http.MethodPatch {
			AuthMiddlewareWrapper(db, handleUpdateNotifications(db)).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/api/notifications/"), handleNotificationsDispatch(db, cfg))

	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/api/feeds"), func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			AuthMiddlewareWrapper(db, handleGetFeeds(db)).ServeHTTP(w, r)
		} else if r.Method == http.MethodPost {
			AuthMiddlewareWrapper(db, handleCreateFeed(db)).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/api/feeds/"), func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			AuthMiddlewareWrapper(db, handleDeleteFeed(db)).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})
}

func registerMeRoutes(mux *http.ServeMux, cfg *core.Config, db *sql.DB) {
	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/api/me"), func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			AuthMiddlewareWrapper(db, handleGetMe(db)).ServeHTTP(w, r)
			return
		}
		http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
	})
	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/api/me/"), handleMeDispatch(db, cfg))
}

func registerTagsAndGenresRoutes(mux *http.ServeMux, cfg *core.Config, db *sql.DB) {
	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/api/tags"), func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			AuthMiddlewareWrapper(db, handleGetAllTags(db)).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/api/tags/rename"), func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			AuthMiddlewareWrapper(db, handleRenameTag(db)).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/api/tags/"), func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			AuthMiddlewareWrapper(db, handleDeleteTag(db)).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/api/genres"), func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			AuthMiddlewareWrapper(db, handleGetAllGenres(db)).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/api/genres/rename"), func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			AuthMiddlewareWrapper(db, handleRenameGenre(db)).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/api/genres/"), func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			AuthMiddlewareWrapper(db, handleDeleteGenre(db)).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})
}

func registerStatsAndFilesystemRoutes(mux *http.ServeMux, cfg *core.Config, db *sql.DB, appRoot string) {
	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/api/server-listening-stats"), func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			AuthMiddlewareWrapper(db, handleGetServerListeningStats(db)).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/api/server-listening-sessions"), func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			AuthMiddlewareWrapper(db, handleGetServerListeningSessions(db)).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/api/stats/year/"), func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			AuthMiddlewareWrapper(db, handleGetAdminStatsForYear(db)).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/api/logger-data"), func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			AuthMiddlewareWrapper(db, handleGetLoggerData(db)).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/api/validate-cron"), func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			AuthMiddlewareWrapper(db, http.HandlerFunc(handleValidateCron)).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/api/watcher/update"), func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			AuthMiddlewareWrapper(db, http.HandlerFunc(handleWatcherUpdate)).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/api/filesystem"), func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			AuthMiddlewareWrapper(db, handleGetFilesystem(appRoot)).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/api/filesystem/"), func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			AuthMiddlewareWrapper(db, handleGetFilesystem(appRoot)).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/api/filesystem/pathexists"), func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			AuthMiddlewareWrapper(db, handleCheckPathExists(db)).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})
}

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
		if pathWithoutPrefix == "/api/tasks/cancel-all" {
			if r.Method == http.MethodPost {
				AuthMiddlewareWrapper(db, handleCancelAllTasks(db)).ServeHTTP(w, r)
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
		if err != nil || s.IssuerURL == "" {
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
	ilogger.LogCallback = isocket.GlobalAuth.BroadcastLog
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

func registerMiscRoutes(mux *http.ServeMux, cfg *core.Config, db *sql.DB, appRoot string) {
	registerSettingsRoutes(mux, cfg, db)
	registerMetadataRoutes(mux, cfg, db)
	registerMockAndFeedRoutes(mux, cfg, db)
	registerMeRoutes(mux, cfg, db)
	registerTagsAndGenresRoutes(mux, cfg, db)
	registerStatsAndFilesystemRoutes(mux, cfg, db, appRoot)
	registerTasksAndOtherRoutes(mux, cfg, db, cfg.MetadataPath)
	registerEmailRoutes(mux, cfg, db)

	// OPDS Catalog routes
	mux.Handle(joinPath(cfg.RouterBasePath, "/opds"), AuthMiddlewareWrapper(db, ServeOPDS(db)))
	mux.Handle(joinPath(cfg.RouterBasePath, "/opds/"), AuthMiddlewareWrapper(db, ServeOPDS(db)))

	// Metrics route (Prometheus scraper endpoint)
	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/metrics"), handleMetrics(db))
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

func handleLibrarySubRouteDispatch(db *sql.DB, w http.ResponseWriter, r *http.Request, parts []string) bool {
	libraryID := parts[0]
	action := parts[1]

	switch action {
	case "personalized":
		if r.Method == http.MethodGet {
			AuthMiddlewareWrapper(db, http.HandlerFunc(HandleGetLibraryPersonalized(db, libraryID))).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
		return true
	case "items":
		if r.Method == http.MethodGet {
			AuthMiddlewareWrapper(db, http.HandlerFunc(HandleGetLibraryItems(db, libraryID))).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
		return true
	case "authors":
		if r.Method == http.MethodGet {
			AuthMiddlewareWrapper(db, http.HandlerFunc(handleGetLibraryAuthors(db, libraryID))).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
		return true
	case "narrators":
		if r.Method == http.MethodGet {
			AuthMiddlewareWrapper(db, http.HandlerFunc(handleGetLibraryNarrators(db, libraryID))).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
		return true
	case "series":
		if r.Method == http.MethodGet {
			AuthMiddlewareWrapper(db, http.HandlerFunc(handleGetLibrarySeries(db, libraryID))).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
		return true
	case "filterdata":
		if r.Method == http.MethodGet {
			AuthMiddlewareWrapper(db, http.HandlerFunc(handleGetLibraryFilterData(db, libraryID))).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
		return true
	case "playlists":
		if r.Method == http.MethodGet {
			AuthMiddlewareWrapper(db, http.HandlerFunc(handleGetLibraryPlaylists(db, libraryID))).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
		return true
	case "collections":
		if r.Method == http.MethodGet {
			AuthMiddlewareWrapper(db, http.HandlerFunc(handleGetLibraryCollections(db, libraryID))).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
		return true
	case "opml":
		if r.Method == http.MethodGet {
			AuthMiddlewareWrapper(db, http.HandlerFunc(handleGetLibraryOPML(db, libraryID))).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
		return true
	case "scan":
		if r.Method == http.MethodPost {
			AuthMiddlewareWrapper(db, iscanner.HandleScanLibrary(db, libraryID, isocket.GlobalAuth)).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
		return true
	case "stats":
		if r.Method == http.MethodGet {
			AuthMiddlewareWrapper(db, http.HandlerFunc(handleGetLibraryStats(db, libraryID))).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
		return true
	}
	return false
}

func handleLibrariesDispatch(db *sql.DB, cfg *core.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Go] %s %s", r.Method, r.URL.Path)

		subPath := strings.TrimPrefix(r.URL.Path, joinPath(cfg.RouterBasePath, "/api/libraries/"))
		if subPath == "" {
			if r.Method == http.MethodGet {
				AuthMiddlewareWrapper(db, http.HandlerFunc(HandleGetLibraries(db))).ServeHTTP(w, r)
				return
			}
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
			return
		}

		parts := strings.Split(subPath, "/")
		if len(parts) == 1 && parts[0] != "" {
			libraryID := parts[0]
			if r.Method == http.MethodGet {
				AuthMiddlewareWrapper(db, http.HandlerFunc(HandleGetLibraryByID(db, libraryID))).ServeHTTP(w, r)
				return
			} else if r.Method == http.MethodPatch {
				AuthMiddlewareWrapper(db, http.HandlerFunc(HandleUpdateLibrary(db, libraryID))).ServeHTTP(w, r)
				return
			} else if r.Method == http.MethodDelete {
				AuthMiddlewareWrapper(db, http.HandlerFunc(HandleDeleteLibrary(db, libraryID))).ServeHTTP(w, r)
				return
			}
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
			return
		} else if len(parts) == 2 {
			if handleLibrarySubRouteDispatch(db, w, r, parts) {
				return
			}
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
			return
		} else if len(parts) == 3 && parts[1] == "series" {
			libraryID := parts[0]
			seriesID := parts[2]
			if r.Method == http.MethodGet {
				AuthMiddlewareWrapper(db, http.HandlerFunc(handleGetLibrarySeriesByID(db, libraryID, seriesID))).ServeHTTP(w, r)
				return
			}
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
			return
		}

		http.NotFound(w, r)
	}
}

func handleMeDispatch(db *sql.DB, cfg *core.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		subPath := strings.TrimPrefix(r.URL.Path, joinPath(cfg.RouterBasePath, "/api/me/"))
		parts := strings.Split(subPath, "/")

		if len(parts) == 1 && parts[0] == "password" {
			if r.Method == http.MethodPost {
				RateLimitMiddleware(LoginRateLimiter)(AuthMiddlewareWrapper(db, handleUpdateMePassword(db))).ServeHTTP(w, r)
				return
			}
		} else if len(parts) == 1 && parts[0] == "listening-stats" {
			if r.Method == http.MethodGet {
				AuthMiddlewareWrapper(db, handleGetMeListeningStats(db)).ServeHTTP(w, r)
				return
			}
		} else if len(parts) == 1 && parts[0] == "listening-sessions" {
			if r.Method == http.MethodGet {
				AuthMiddlewareWrapper(db, handleGetMeListeningSessions(db)).ServeHTTP(w, r)
				return
			}
		} else if len(parts) == 1 && parts[0] == "sessions" {
			if r.Method == http.MethodGet {
				AuthMiddlewareWrapper(db, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					userSess := r.Context().Value(core.UserContextKey).(*core.UserSession)
					handleGetUserLoginSessions(db, userSess.ID)(w, r)
				})).ServeHTTP(w, r)
				return
			}
		} else if len(parts) == 2 && parts[0] == "sessions" {
			if r.Method == http.MethodDelete {
				AuthMiddlewareWrapper(db, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					userSess := r.Context().Value(core.UserContextKey).(*core.UserSession)
					handleDeleteUserLoginSession(db, userSess.ID, parts[1])(w, r)
				})).ServeHTTP(w, r)
				return
			}
		} else if len(parts) == 1 && parts[0] == "items-in-progress" {
			if r.Method == http.MethodGet {
				AuthMiddlewareWrapper(db, handleGetAllLibraryItemsInProgress(db)).ServeHTTP(w, r)
				return
			}
		} else if (len(parts) == 2 || (len(parts) == 3 && parts[2] != "hide-from-continue-listening" && parts[2] != "remove-from-continue-listening")) && parts[0] == "progress" {
			if r.Method == http.MethodGet {
				AuthMiddlewareWrapper(db, handleGetMeProgress(db)).ServeHTTP(w, r)
				return
			} else if r.Method == http.MethodPatch || r.Method == http.MethodPost {
				AuthMiddlewareWrapper(db, handleCreateUpdateMeProgress(db)).ServeHTTP(w, r)
				return
			} else if r.Method == http.MethodDelete {
				AuthMiddlewareWrapper(db, handleRemoveMeProgress(db)).ServeHTTP(w, r)
				return
			}
		} else if len(parts) == 3 && parts[0] == "progress" && (parts[2] == "hide-from-continue-listening" || parts[2] == "remove-from-continue-listening") {
			if r.Method == http.MethodGet || r.Method == http.MethodPatch {
				AuthMiddlewareWrapper(db, handleHideMeProgressFromContinueListening(db)).ServeHTTP(w, r)
				return
			}
		} else if len(parts) == 3 && parts[0] == "series" && parts[2] == "remove" {
			if r.Method == http.MethodPost {
				AuthMiddlewareWrapper(db, handleRemoveSeriesFromContinueListening(db)).ServeHTTP(w, r)
				return
			}
		} else if len(parts) == 3 && parts[0] == "series" && parts[2] == "readd" {
			if r.Method == http.MethodPost {
				AuthMiddlewareWrapper(db, handleReaddSeriesFromContinueListening(db)).ServeHTTP(w, r)
				return
			}
		} else if parts[0] == "item" && len(parts) >= 3 && parts[2] == "bookmark" {
			if len(parts) == 3 {
				if r.Method == http.MethodPost {
					AuthMiddlewareWrapper(db, handleMeCreateBookmark(db)).ServeHTTP(w, r)
					return
				} else if r.Method == http.MethodPatch {
					AuthMiddlewareWrapper(db, handleMeUpdateBookmark(db)).ServeHTTP(w, r)
					return
				}
			} else if len(parts) == 4 {
				if r.Method == http.MethodDelete {
					AuthMiddlewareWrapper(db, handleMeRemoveBookmark(db)).ServeHTTP(w, r)
					return
				}
			}
		} else if len(parts) == 1 && parts[0] == "sync-local-progress" {
			if r.Method == http.MethodPost {
				AuthMiddlewareWrapper(db, handleSyncLocalProgress(db)).ServeHTTP(w, r)
				return
			}
		}

		http.NotFound(w, r)
	}
}

func handleBackupsDispatch(db *sql.DB, cfg *core.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		subPath := strings.TrimPrefix(r.URL.Path, joinPath(cfg.RouterBasePath, "/api/backups/"))
		parts := strings.Split(subPath, "/")
		if len(parts) == 1 && parts[0] != "" {
			if r.Method == http.MethodDelete {
				AuthMiddlewareWrapper(db, handleDeleteBackup(db, cfg.MetadataPath)).ServeHTTP(w, r)
				return
			}
		} else if len(parts) == 2 && parts[1] == "download" {
			if r.Method == http.MethodGet {
				AuthMiddlewareWrapper(db, handleDownloadBackup(db, cfg.MetadataPath)).ServeHTTP(w, r)
				return
			}
		} else if len(parts) == 2 && parts[1] == "apply" {
			if r.Method == http.MethodPost {
				AuthMiddlewareWrapper(db, handleApplyBackup(db, cfg.ConfigPath, cfg.MetadataPath, func() {
					log.Infof("[Backup Apply] Restarting Go Gateway process...")
					go func() {
						time.Sleep(500 * time.Millisecond)

						if flag.Lookup("test.v") != nil || os.Getenv("UNDER_TEST") == "true" {
							log.Infof("[Backup Apply] Test environment detected, skipping syscall.Exec.")
							return
						}

						if globalDB != nil {
							globalDB.Close()
						}

						binary, err := exec.LookPath(os.Args[0])
						if err != nil {
							binary = os.Args[0]
						}

						log.Infof("[Backup Apply] Executing %s %v", binary, os.Args)
						err = syscall.Exec(binary, os.Args, os.Environ())
						if err != nil {
							log.Errorf("[Backup Apply] syscall.Exec failed: %v", err)
							os.Exit(1)
						}
					}()
				})).ServeHTTP(w, r)
				return
			}
		}
		http.NotFound(w, r)
	}
}

func HandleItemsDispatch(db *sql.DB, cfg *core.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if strings.HasSuffix(path, "/cover") {
			serveCover(db, cfg.MetadataPath)(w, r)
			return
		}
		if strings.HasSuffix(path, "/download") {
			AuthMiddlewareWrapper(db, http.HandlerFunc(serveDownload(db))).ServeHTTP(w, r)
			return
		}

		subPath := strings.TrimPrefix(path, joinPath(cfg.RouterBasePath, "/api/items/"))
		parts := strings.Split(subPath, "/")
		if len(parts) == 2 && parts[0] == "batch" && parts[1] == "update" {
			if r.Method == http.MethodPost {
				AuthMiddlewareWrapper(db, http.HandlerFunc(handleBatchUpdateLibraryItems(db, cfg))).ServeHTTP(w, r)
				return
			}
		}
		if len(parts) == 1 && parts[0] != "" {
			itemID := parts[0]
			if r.Method == http.MethodGet {
				AuthMiddlewareWrapper(db, http.HandlerFunc(handleGetLibraryItemByID(db, itemID))).ServeHTTP(w, r)
				return
			} else if r.Method == http.MethodPatch {
				AuthMiddlewareWrapper(db, http.HandlerFunc(handleUpdateLibraryItemByID(db, itemID))).ServeHTTP(w, r)
				return
			}
		} else if len(parts) == 2 && parts[1] == "waveform" {
			itemID := parts[0]
			if r.Method == http.MethodGet {
				AuthMiddlewareWrapper(db, http.HandlerFunc(handleGetWaveform(db, cfg, itemID))).ServeHTTP(w, r)
				return
			}
		} else if len(parts) == 2 && parts[1] == "ebook" {
			itemID := parts[0]
			if r.Method == http.MethodGet {
				AuthMiddlewareWrapper(db, http.HandlerFunc(handleServeEbook(db, itemID, ""))).ServeHTTP(w, r)
				return
			}
		} else if len(parts) == 3 && parts[1] == "ebook" {
			itemID := parts[0]
			fileID := parts[2]
			if r.Method == http.MethodGet {
				AuthMiddlewareWrapper(db, http.HandlerFunc(handleServeEbook(db, itemID, fileID))).ServeHTTP(w, r)
				return
			}
		} else if (len(parts) == 2 || len(parts) == 3) && parts[1] == "play" {
			if r.Method == http.MethodPost {
				AuthMiddlewareWrapper(db, ihls.HandlePlayItem(db, streamManager)).ServeHTTP(w, r)
				return
			}
		} else if len(parts) == 2 && parts[1] == "cover-from-url" {
			itemID := parts[0]
			if r.Method == http.MethodPost {
				AuthMiddlewareWrapper(db, http.HandlerFunc(handleUpdateCoverFromURL(db, cfg, itemID))).ServeHTTP(w, r)
				return
			}
		} else if len(parts) == 2 && parts[1] == "chapters" {
			itemID := parts[0]
			if r.Method == http.MethodPost {
				AuthMiddlewareWrapper(db, http.HandlerFunc(handleUpdateChapters(db, itemID))).ServeHTTP(w, r)
				return
			}
		} else if len(parts) == 3 && parts[1] == "chapters" && parts[2] == "lookup" {
			itemID := parts[0]
			if r.Method == http.MethodPost {
				AuthMiddlewareWrapper(db, http.HandlerFunc(handleLookupChapters(db, itemID))).ServeHTTP(w, r)
				return
			}
		} else if len(parts) == 2 && parts[1] == "embed-metadata" {
			itemID := parts[0]
			if r.Method == http.MethodPost {
				AuthMiddlewareWrapper(db, http.HandlerFunc(handleEmbedMetadata(db, cfg, itemID))).ServeHTTP(w, r)
				return
			}
		} else if len(parts) == 2 && parts[1] == "merge" {
			if r.Method == http.MethodPost {
				AuthMiddlewareWrapper(db, http.HandlerFunc(handleMergeAudioFiles(db))).ServeHTTP(w, r)
				return
			}
		}

		log.Warnf("[Backend] 404 Not Found: %s %s", r.Method, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error": "API route not found"}`))
	}
}

func handleAuthorsDispatch(db *sql.DB, cfg *core.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Go] %s %s", r.Method, r.URL.Path)

		subPath := strings.TrimPrefix(r.URL.Path, joinPath(cfg.RouterBasePath, "/api/authors/"))
		parts := strings.Split(subPath, "/")

		if len(parts) == 1 && parts[0] != "" {
			authorID := parts[0]
			if r.Method == http.MethodGet {
				AuthMiddlewareWrapper(db, http.HandlerFunc(handleGetAuthorByID(db, authorID))).ServeHTTP(w, r)
				return
			} else if r.Method == http.MethodPatch {
				AuthMiddlewareWrapper(db, http.HandlerFunc(handleUpdateAuthor(db, authorID))).ServeHTTP(w, r)
				return
			}
		} else if len(parts) == 2 && parts[1] == "image" {
			authorID := parts[0]
			if r.Method == http.MethodGet {
				handleGetAuthorImage(db, cfg.MetadataPath, authorID)(w, r)
				return
			} else if r.Method == http.MethodDelete {
				AuthMiddlewareWrapper(db, http.HandlerFunc(handleDeleteAuthorImage(db, cfg, authorID))).ServeHTTP(w, r)
				return
			}
		} else if len(parts) == 2 && parts[1] == "match" {
			authorID := parts[0]
			if r.Method == http.MethodPost {
				AuthMiddlewareWrapper(db, http.HandlerFunc(handleMatchAuthor(db, cfg, authorID))).ServeHTTP(w, r)
				return
			}
		}

		http.NotFound(w, r)
	}
}

func handleSeriesDispatch(db *sql.DB, cfg *core.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Go] %s %s", r.Method, r.URL.Path)

		subPath := strings.TrimPrefix(r.URL.Path, joinPath(cfg.RouterBasePath, "/api/series/"))
		parts := strings.Split(subPath, "/")

		if len(parts) == 1 && parts[0] != "" {
			seriesID := parts[0]
			if r.Method == http.MethodPatch {
				AuthMiddlewareWrapper(db, http.HandlerFunc(handleUpdateSeries(db, seriesID))).ServeHTTP(w, r)
				return
			}
		} else if len(parts) == 2 && parts[1] == "auto-number" {
			seriesID := parts[0]
			if r.Method == http.MethodPost {
				AuthMiddlewareWrapper(db, http.HandlerFunc(handleAutoNumberSeries(db, seriesID))).ServeHTTP(w, r)
				return
			}
		}

		http.NotFound(w, r)
	}
}

func handlePlaylistsDispatch(db *sql.DB, cfg *core.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		pathWithoutPrefix := trimBasePath(r.URL.Path, cfg.RouterBasePath)
		id := strings.TrimPrefix(pathWithoutPrefix, "/api/playlists/")
		if id == "" {
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodGet {
			AuthMiddlewareWrapper(db, http.HandlerFunc(handleGetPlaylist(db, id))).ServeHTTP(w, r)
		} else if r.Method == http.MethodPatch {
			AuthMiddlewareWrapper(db, http.HandlerFunc(handleUpdatePlaylist(db, id))).ServeHTTP(w, r)
		} else if r.Method == http.MethodDelete {
			AuthMiddlewareWrapper(db, http.HandlerFunc(handleDeletePlaylist(db, id))).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	}
}

func handleCollectionsDispatch(db *sql.DB, cfg *core.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		pathWithoutPrefix := trimBasePath(r.URL.Path, cfg.RouterBasePath)
		id := strings.TrimPrefix(pathWithoutPrefix, "/api/collections/")
		if id == "" {
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodGet {
			AuthMiddlewareWrapper(db, http.HandlerFunc(handleGetCollection(db, id))).ServeHTTP(w, r)
		} else if r.Method == http.MethodPatch {
			AuthMiddlewareWrapper(db, http.HandlerFunc(handleUpdateCollection(db, id))).ServeHTTP(w, r)
		} else if r.Method == http.MethodDelete {
			AuthMiddlewareWrapper(db, http.HandlerFunc(handleDeleteCollection(db, id))).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	}
}

func handleSharesDispatch(db *sql.DB, cfg *core.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		pathWithoutPrefix := trimBasePath(r.URL.Path, cfg.RouterBasePath)
		id := strings.TrimPrefix(pathWithoutPrefix, "/api/share/mediaitem/")
		if id == "" {
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodDelete {
			AuthMiddlewareWrapper(db, http.HandlerFunc(handleDeleteShare(db, id))).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	}
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

func handleNotificationsDispatch(db *sql.DB, cfg *core.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		subPath := strings.TrimPrefix(r.URL.Path, joinPath(cfg.RouterBasePath, "/api/notifications/"))
		subPath = strings.TrimSuffix(subPath, "/")

		// Case 1: "/api/notifications/test"
		if subPath == "test" {
			if r.Method == http.MethodGet {
				AuthMiddlewareWrapper(db, handleSendDefaultTestNotification(db)).ServeHTTP(w, r)
			} else {
				http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
			}
			return
		}

		// Case 2: "/api/notifications/{id}/test"
		if strings.HasSuffix(subPath, "/test") {
			if r.Method == http.MethodGet {
				AuthMiddlewareWrapper(db, handleSendTestNotification(db)).ServeHTTP(w, r)
			} else {
				http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
			}
			return
		}

		// Case 3: "/api/notifications/{id}"
		if subPath != "" {
			if r.Method == http.MethodPatch {
				AuthMiddlewareWrapper(db, handleUpdateNotification(db)).ServeHTTP(w, r)
			} else if r.Method == http.MethodDelete {
				AuthMiddlewareWrapper(db, handleDeleteNotification(db)).ServeHTTP(w, r)
			} else {
				http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
			}
			return
		}

		http.Error(w, `{"error": "Not Found"}`, http.StatusNotFound)
	}
}
