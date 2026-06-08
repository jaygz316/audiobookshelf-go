package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	_ "modernc.org/sqlite"
)

type Config struct {
	ConfigPath     string
	MetadataPath   string
	Port           string
	Host           string
	Source         string
	Dev            bool
	ProdWithDevEnv bool
	LegacyURL      string
	RouterBasePath string
}

var cachedSecret string
var globalDB *sql.DB
var streamManager = NewStreamManager()

func getTokenSecret(db *sql.DB) string {
	if envSecret := os.Getenv("JWT_SECRET_KEY"); envSecret != "" {
		return envSecret
	}
	if cachedSecret != "" {
		return cachedSecret
	}
	settings, err := GetServerSettings(db)
	if err == nil && settings != nil && settings.TokenSecret != "" {
		cachedSecret = settings.TokenSecret
		return cachedSecret
	}
	return ""
}

func getVersion(appRoot string) string {
	pkgPath := filepath.Join(appRoot, "package.json")
	file, err := os.Open(pkgPath)
	if err != nil {
		return "2.35.1" // Fallback
	}
	defer file.Close()

	var pkg struct {
		Version string `json:"version"`
	}
	if err := json.NewDecoder(file).Decode(&pkg); err != nil {
		return "2.35.1"
	}
	return pkg.Version
}

func main() {
	cfg := parseConfig()
	legacyURL = cfg.LegacyURL

	appRoot, err := os.Getwd()
	if err != nil {
		log.Fatalf("Failed to get working directory: %v", err)
	}

	version := getVersion(appRoot)

	log.Printf("=== Starting Go Gateway ===")
	log.Printf("Options: CONFIG_PATH=%s, METADATA_PATH=%s, PORT=%s, HOST=%s, SOURCE=%s, LEGACY_URL=%s, ROUTER_BASE_PATH=%s",
		cfg.ConfigPath, cfg.MetadataPath, cfg.Port, cfg.Host, cfg.Source, cfg.LegacyURL, cfg.RouterBasePath)

	// Ensure config and metadata directories exist
	if err := os.MkdirAll(cfg.ConfigPath, 0755); err != nil {
		log.Fatalf("Failed to create config directory: %v", err)
	}
	if err := os.MkdirAll(cfg.MetadataPath, 0755); err != nil {
		log.Fatalf("Failed to create metadata directory: %v", err)
	}

	// Connect to database
	dbPath := filepath.Join(cfg.ConfigPath, "absdatabase.sqlite")
	db, err := initDB(dbPath)
	var dbConnected bool
	if err != nil {
		log.Printf("Warning: Failed to connect to SQLite database: %v. Node.js server might initialize it.", err)
	} else {
		defer db.Close()
		log.Printf("Successfully connected to SQLite database: %s", dbPath)
		dbConnected = true
		globalDB = db
	}

	handler := setupHandler(db, cfg, dbConnected, appRoot, version)

	serverAddr := fmt.Sprintf("%s:%s", cfg.Host, cfg.Port)
	srv := &http.Server{
		Addr:    serverAddr,
		Handler: handler,
	}

	// Handle graceful shutdown
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		sig := <-sigChan
		log.Printf("Received signal %v. Shutting down Go Gateway...", sig)
		srv.Close()
	}()

	log.Printf("Go Gateway listening on http://%s", serverAddr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server ListenAndServe failed: %v", err)
	}
	log.Printf("Go Gateway stopped.")
}

func setupHandler(db *sql.DB, cfg *Config, dbConnected bool, appRoot string, version string) http.Handler {
	// Set up reverse proxy or graceful fallback if legacy-url is empty
	var proxy http.Handler
	if cfg.LegacyURL == "" {
		proxy = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			log.Printf("[Proxy] Fallback requested but legacy URL is empty: %s %s", r.Method, r.URL.Path)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadGateway)
			w.Write([]byte(`{"error":"Legacy server not configured and endpoint is not implemented natively"}`))
		})
	} else {
		target, err := url.Parse(cfg.LegacyURL)
		if err != nil {
			log.Fatalf("Invalid legacy URL %s: %v", cfg.LegacyURL, err)
		}
		reverseProxy := httputil.NewSingleHostReverseProxy(target)
		originalDirector := reverseProxy.Director
		reverseProxy.Director = func(req *http.Request) {
			originalDirector(req)
			req.Header.Set("X-Forwarded-Host", req.Host)
			req.Header.Set("X-Forwarded-Proto", req.URL.Scheme)
			req.Host = target.Host
		}
		reverseProxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
			log.Printf("[Proxy] Proxying failed: %v", err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadGateway)
			w.Write([]byte(`{"error":"Legacy server connection failed"}`))
		}
		proxy = reverseProxy
	}

	// Setup serve multiplexer
	mux := http.NewServeMux()

	// Base path prefix redirect/rewrite middleware
	handler := http.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Ensure path starts with RouterBasePath
		if !strings.HasPrefix(r.URL.Path, cfg.RouterBasePath) {
			r.URL.Path = cfg.RouterBasePath + r.URL.Path
		}
		mux.ServeHTTP(w, r)
	}))

	// Define Go Native endpoints
	mux.HandleFunc(cfg.RouterBasePath+"/ping", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[Go] GET /ping")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"success":true}`))
	})

	mux.HandleFunc(cfg.RouterBasePath+"/healthcheck", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[Go] GET /healthcheck")
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc(cfg.RouterBasePath+"/status", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[Go] GET /status")
		dbPath := filepath.Join(cfg.ConfigPath, "absdatabase.sqlite")
		if !dbConnected {
			// Try to reconnect if not connected yet (might have been initialized by Node now)
			var reconnectErr error
			db, reconnectErr = initDB(dbPath)
			if reconnectErr == nil {
				dbConnected = true
				globalDB = db
				if SocketAuth != nil {
					SocketAuth.db = db
				}
				log.Printf("Connected to SQLite database on-demand: %s", dbPath)
			}
		}

		if !dbConnected {
			log.Printf("[Status] DB not connected. Proxying to Node.")
			proxy.ServeHTTP(w, r)
			return
		}

		isInit, err := HasRootUser(db)
		if err != nil {
			log.Printf("[Status] Failed to check root user: %v. Proxying to Node.", err)
			proxy.ServeHTTP(w, r)
			return
		}

		settings, err := GetServerSettings(db)
		if err != nil {
			log.Printf("[Status] Failed to get server settings: %v. Proxying to Node.", err)
			proxy.ServeHTTP(w, r)
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
		w.Header().Set("Content-Security-Policy", "frame-ancestors 'self'")
		w.Header().Set("Referrer-Policy", "no-referrer")
		json.NewEncoder(w).Encode(payload)
	})

	// Auth Endpoints (native Go implementation)
	mux.HandleFunc(cfg.RouterBasePath+"/init", handleInit(db))
	mux.HandleFunc(cfg.RouterBasePath+"/login", handleLogin(db))
	mux.HandleFunc(cfg.RouterBasePath+"/logout", handleLogout(db))
	mux.HandleFunc(cfg.RouterBasePath+"/auth/refresh", handleRefresh(db))

	// Libraries API (native Go implementation)
	mux.HandleFunc(cfg.RouterBasePath+"/api/libraries", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[Go] %s /api/libraries", r.Method)
		if r.Method == http.MethodPost {
			AuthMiddleware(db, getTokenSecret(db), http.HandlerFunc(handleCreateLibrary(db))).ServeHTTP(w, r)
			return
		}
		if r.Method != http.MethodGet {
			proxy.ServeHTTP(w, r)
			return
		}
		AuthMiddleware(db, getTokenSecret(db), http.HandlerFunc(handleGetLibraries(db))).ServeHTTP(w, r)
	})

	mux.HandleFunc(cfg.RouterBasePath+"/api/libraries/", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[Go] %s %s", r.Method, r.URL.Path)
		subPath := strings.TrimPrefix(r.URL.Path, cfg.RouterBasePath+"/api/libraries/")
		parts := strings.Split(subPath, "/")

		if r.Method == http.MethodPost {
			if len(parts) == 2 && parts[1] == "scan" {
				libraryID := parts[0]
				AuthMiddleware(db, getTokenSecret(db), http.HandlerFunc(handleScanLibrary(db, libraryID))).ServeHTTP(w, r)
				return
			}
		}
		if r.Method == http.MethodPatch {
			if len(parts) == 1 && parts[0] != "" {
				libraryID := parts[0]
				AuthMiddleware(db, getTokenSecret(db), http.HandlerFunc(handleUpdateLibrary(db, libraryID))).ServeHTTP(w, r)
				return
			}
		}
		if r.Method == http.MethodDelete {
			if len(parts) == 1 && parts[0] != "" {
				libraryID := parts[0]
				AuthMiddleware(db, getTokenSecret(db), http.HandlerFunc(handleDeleteLibrary(db, libraryID))).ServeHTTP(w, r)
				return
			}
		}

		if r.Method != http.MethodGet {
			proxy.ServeHTTP(w, r)
			return
		}

		if subPath == "" {
			AuthMiddleware(db, getTokenSecret(db), http.HandlerFunc(handleGetLibraries(db))).ServeHTTP(w, r)
			return
		}

		if len(parts) == 1 && parts[0] != "" {
			libraryID := parts[0]
			AuthMiddleware(db, getTokenSecret(db), http.HandlerFunc(handleGetLibraryByID(db, libraryID, proxy))).ServeHTTP(w, r)
			return
		} else if len(parts) == 2 && parts[1] == "items" {
			libraryID := parts[0]
			AuthMiddleware(db, getTokenSecret(db), http.HandlerFunc(handleGetLibraryItems(db, libraryID))).ServeHTTP(w, r)
			return
		}

		proxy.ServeHTTP(w, r)
	})

	// Me API (native Go implementation)
	mux.HandleFunc(cfg.RouterBasePath+"/api/me", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[Go] %s /api/me", r.Method)
		AuthMiddleware(db, getTokenSecret(db), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet {
				handleGetMe(db)(w, r)
				return
			}
			proxy.ServeHTTP(w, r)
		})).ServeHTTP(w, r)
	})

	mux.HandleFunc(cfg.RouterBasePath+"/api/me/", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[Go] %s %s", r.Method, r.URL.Path)
		AuthMiddleware(db, getTokenSecret(db), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			subPath := strings.TrimPrefix(r.URL.Path, cfg.RouterBasePath+"/api/me/")

			if subPath == "password" && r.Method == http.MethodPatch {
				handleUpdateMePassword(db)(w, r)
				return
			}
			if subPath == "items-in-progress" && r.Method == http.MethodGet {
				handleGetAllLibraryItemsInProgress(db)(w, r)
				return
			}
			if strings.HasPrefix(subPath, "progress") {
				if strings.HasSuffix(subPath, "/remove-from-continue-listening") {
					handleHideMeProgressFromContinueListening(db)(w, r)
					return
				}
				if r.Method == http.MethodGet {
					handleGetMeProgress(db)(w, r)
					return
				}
				if r.Method == http.MethodPatch {
					handleCreateUpdateMeProgress(db)(w, r)
					return
				}
				if r.Method == http.MethodDelete {
					handleRemoveMeProgress(db)(w, r)
					return
				}
			}
			if strings.HasPrefix(subPath, "series/") {
				if strings.HasSuffix(subPath, "/remove-from-continue-listening") {
					handleRemoveSeriesFromContinueListening(db)(w, r)
					return
				}
				if strings.HasSuffix(subPath, "/readd-to-continue-listening") {
					handleReaddSeriesFromContinueListening(db)(w, r)
					return
				}
			}
			if strings.HasPrefix(subPath, "item/") && strings.HasSuffix(subPath, "/bookmark") {
				if r.Method == http.MethodPost {
					handleMeCreateBookmark(db)(w, r)
					return
				}
				if r.Method == http.MethodPatch {
					handleMeUpdateBookmark(db)(w, r)
					return
				}
			}
			if strings.HasPrefix(subPath, "item/") && strings.Contains(subPath, "/bookmark/") && r.Method == http.MethodDelete {
				handleMeRemoveBookmark(db)(w, r)
				return
			}

			proxy.ServeHTTP(w, r)
		})).ServeHTTP(w, r)
	})

	// Users API (native Go implementation)
	mux.HandleFunc(cfg.RouterBasePath+"/api/users", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[Go] %s /api/users", r.Method)
		AuthMiddleware(db, getTokenSecret(db), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet {
				handleGetUsers(db)(w, r)
				return
			}
			if r.Method == http.MethodPost {
				handleUserCRUD(db)(w, r)
				return
			}
			proxy.ServeHTTP(w, r)
		})).ServeHTTP(w, r)
	})

	mux.HandleFunc(cfg.RouterBasePath+"/api/users/", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[Go] %s %s", r.Method, r.URL.Path)
		AuthMiddleware(db, getTokenSecret(db), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			subPath := strings.TrimPrefix(r.URL.Path, cfg.RouterBasePath+"/api/users/")

			if subPath == "online" && r.Method == http.MethodGet {
				handleGetOnlineUsers(db)(w, r)
				return
			}
			if subPath != "" {
				handleUserCRUD(db)(w, r)
				return
			}

			proxy.ServeHTTP(w, r)
		})).ServeHTTP(w, r)
	})

	// Backups API (native Go implementation)
	mux.HandleFunc(cfg.RouterBasePath+"/api/backups", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[Go] %s /api/backups", r.Method)
		AuthMiddleware(db, getTokenSecret(db), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet {
				handleGetBackups(db, cfg.MetadataPath)(w, r)
				return
			}
			if r.Method == http.MethodPost {
				handleCreateBackup(db, cfg.ConfigPath, cfg.MetadataPath)(w, r)
				return
			}
			proxy.ServeHTTP(w, r)
		})).ServeHTTP(w, r)
	})

	mux.HandleFunc(cfg.RouterBasePath+"/api/backups/", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[Go] %s %s", r.Method, r.URL.Path)
		AuthMiddleware(db, getTokenSecret(db), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			subPath := strings.TrimPrefix(r.URL.Path, cfg.RouterBasePath+"/api/backups/")

			if subPath == "upload" && r.Method == http.MethodPost {
				handleUploadBackup(db, cfg.MetadataPath)(w, r)
				return
			}
			if strings.HasPrefix(subPath, "apply/") && r.Method == http.MethodPost {
				reloadFunc := func() {
					log.Printf("[Backup Restore] Triggering reload")
					cachedSecret = ""
					if GlobalWatcher != nil {
						GlobalWatcher.Reload()
					}
				}
				handleApplyBackup(db, cfg.ConfigPath, cfg.MetadataPath, reloadFunc)(w, r)
				return
			}
			if strings.HasSuffix(subPath, "/download") && r.Method == http.MethodGet {
				handleDownloadBackup(db, cfg.MetadataPath)(w, r)
				return
			}
			if r.Method == http.MethodDelete {
				handleDeleteBackup(db, cfg.MetadataPath)(w, r)
				return
			}

			proxy.ServeHTTP(w, r)
		})).ServeHTTP(w, r)
	})

	// Tags API (native Go implementation)
	mux.HandleFunc(cfg.RouterBasePath+"/api/tags", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[Go] %s /api/tags", r.Method)
		AuthMiddleware(db, getTokenSecret(db), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet {
				handleGetAllTags(db)(w, r)
				return
			}
			proxy.ServeHTTP(w, r)
		})).ServeHTTP(w, r)
	})

	mux.HandleFunc(cfg.RouterBasePath+"/api/tags/", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[Go] %s %s", r.Method, r.URL.Path)
		AuthMiddleware(db, getTokenSecret(db), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			subPath := strings.TrimPrefix(r.URL.Path, cfg.RouterBasePath+"/api/tags/")

			if subPath == "rename" && r.Method == http.MethodPost {
				handleRenameTag(db)(w, r)
				return
			}
			if r.Method == http.MethodDelete {
				handleDeleteTag(db)(w, r)
				return
			}

			proxy.ServeHTTP(w, r)
		})).ServeHTTP(w, r)
	})

	// Genres API (native Go implementation)
	mux.HandleFunc(cfg.RouterBasePath+"/api/genres", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[Go] %s /api/genres", r.Method)
		AuthMiddleware(db, getTokenSecret(db), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet {
				handleGetAllGenres(db)(w, r)
				return
			}
			proxy.ServeHTTP(w, r)
		})).ServeHTTP(w, r)
	})

	mux.HandleFunc(cfg.RouterBasePath+"/api/genres/", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[Go] %s %s", r.Method, r.URL.Path)
		AuthMiddleware(db, getTokenSecret(db), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			subPath := strings.TrimPrefix(r.URL.Path, cfg.RouterBasePath+"/api/genres/")

			if subPath == "rename" && r.Method == http.MethodPost {
				handleRenameGenre(db)(w, r)
				return
			}
			if r.Method == http.MethodDelete {
				handleDeleteGenre(db)(w, r)
				return
			}

			proxy.ServeHTTP(w, r)
		})).ServeHTTP(w, r)
	})

	// Settings API (native Go implementation)
	mux.HandleFunc(cfg.RouterBasePath+"/api/settings", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[Go] %s /api/settings", r.Method)
		AuthMiddleware(db, getTokenSecret(db), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet {
				handleGetServerSettings(db)(w, r)
				return
			}
			if r.Method == http.MethodPost {
				handleUpdateServerSettings(db)(w, r)
				return
			}
			proxy.ServeHTTP(w, r)
		})).ServeHTTP(w, r)
	})

	mux.HandleFunc(cfg.RouterBasePath+"/api/settings/", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[Go] %s %s", r.Method, r.URL.Path)
		AuthMiddleware(db, getTokenSecret(db), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			subPath := strings.TrimPrefix(r.URL.Path, cfg.RouterBasePath+"/api/settings/")

			if subPath == "auth" {
				if r.Method == http.MethodGet {
					handleGetAuthSettings(db)(w, r)
					return
				}
				if r.Method == http.MethodPost {
					handleUpdateAuthSettings(db)(w, r)
					return
				}
			}
			if subPath == "sorting-prefixes" && r.Method == http.MethodPost {
				handleUpdateSortingPrefixes(db)(w, r)
				return
			}
			if subPath == "metadata-providers" && r.Method == http.MethodGet {
				handleGetMetadataProviders(db)(w, r)
				return
			}
			if subPath == "custom-metadata-providers" {
				if r.Method == http.MethodGet {
					handleGetCustomMetadataProviders(db)(w, r)
					return
				}
				if r.Method == http.MethodPost {
					handleCreateCustomMetadataProvider(db)(w, r)
					return
				}
			}
			if strings.HasPrefix(subPath, "custom-metadata-providers/") && r.Method == http.MethodDelete {
				handleDeleteCustomMetadataProvider(db)(w, r)
				return
			}

			proxy.ServeHTTP(w, r)
		})).ServeHTTP(w, r)
	})

	// Cover and download endpoints (native Go implementation)
	mux.HandleFunc(cfg.RouterBasePath+"/api/items/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if strings.HasSuffix(path, "/cover") {
			serveCover(db, cfg.MetadataPath, proxy)(w, r)
			return
		}
		if strings.HasSuffix(path, "/download") {
			AuthMiddleware(db, getTokenSecret(db), http.HandlerFunc(serveDownload(db, proxy))).ServeHTTP(w, r)
			return
		}

		// Fallback proxy for all other /api/items/ requests
		dest := cfg.LegacyURL
		if dest == "" {
			dest = "(disabled)"
		}
		log.Printf("[Proxy] %s %s -> %s", r.Method, r.URL.Path, dest)
		proxy.ServeHTTP(w, r)
	})

	// Socket.io endpoints (native Go implementation)
	socketHandler := InitSocketAuthority(db)
	mux.Handle(cfg.RouterBasePath+"/socket.io/", socketHandler)
	if cfg.RouterBasePath != "" {
		mux.Handle("/socket.io/", socketHandler)
	}

	// FS Watcher (native Go implementation)
	InitFSWatcher(db)

	// HLS streaming (native Go implementation)
	mux.HandleFunc(cfg.RouterBasePath+"/hls/", serveHLS(cfg.MetadataPath, streamManager))

	// Default fallback: backend routing or static file serving
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if strings.HasPrefix(path, cfg.RouterBasePath) {
			path = strings.TrimPrefix(path, cfg.RouterBasePath)
		}

		// List of backend routing prefixes
		prefixes := []string{"/api/", "/auth/", "/hls/", "/public/", "/feed/", "/status", "/login", "/logout", "/init"}
		isBackend := false
		for _, prefix := range prefixes {
			if strings.HasPrefix(path, prefix) || path == prefix[:len(prefix)-1] {
				isBackend = true
				break
			}
		}

		if isBackend {
			// Proxy backend calls to Node.js
			dest := cfg.LegacyURL
			if dest == "" {
				dest = "(disabled)"
			}
			log.Printf("[Proxy] %s %s -> %s", r.Method, r.URL.Path, dest)
			proxy.ServeHTTP(w, r)
			return
		}

		// Serve static frontend assets directly in Go
		distPath := filepath.Join(appRoot, "client", "dist")
		serveStaticOrSPA(distPath, cfg.RouterBasePath)(w, r)
	})

	return handler
}

func serveStaticOrSPA(distPath, routerBasePath string) http.HandlerFunc {
	fileServer := http.FileServer(http.Dir(distPath))
	return func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if strings.HasPrefix(path, routerBasePath) {
			path = strings.TrimPrefix(path, routerBasePath)
		}
		if path == "" {
			path = "/"
		}

		// Check if file exists in distPath
		fullPath := filepath.Join(distPath, filepath.Clean(path))
		stat, err := os.Stat(fullPath)
		if err == nil && !stat.IsDir() {
			// Strip prefix and serve file
			http.StripPrefix(routerBasePath, fileServer).ServeHTTP(w, r)
			return
		}

		// Serve index.html as fallback for Client-side SPA routing
		log.Printf("[SPA] Fallback for GET %s -> index.html", r.URL.Path)
		http.ServeFile(w, r, filepath.Join(distPath, "index.html"))
	}
}

func serveCover(db *sql.DB, metadataPath string, proxy http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(r.URL.Path, "/")
		var itemID string
		for i, part := range parts {
			if part == "items" && i+1 < len(parts) {
				itemID = parts[i+1]
				break
			}
		}

		if itemID == "" {
			http.Error(w, `{"error": "Invalid Item ID"}`, http.StatusBadRequest)
			return
		}

		raw := r.URL.Query().Get("raw") == "1"

		if raw {
			coverPath, err := GetCoverPath(db, itemID)
			if err != nil || coverPath == "" {
				proxy.ServeHTTP(w, r)
				return
			}
			if r.URL.Query().Get("ts") != "" {
				w.Header().Set("Cache-Control", "private, max-age=86400")
			}
			http.ServeFile(w, r, coverPath)
			return
		}

		// Non-raw: check cover cache first
		format := r.URL.Query().Get("format")
		if format == "" {
			if strings.Contains(r.Header.Get("Accept"), "image/webp") {
				format = "webp"
			} else {
				format = "jpeg"
			}
		}
		width := r.URL.Query().Get("width")
		if width == "" {
			width = "400"
		}
		height := r.URL.Query().Get("height")

		cacheFilename := itemID + "_" + width
		if height != "" {
			cacheFilename += "x" + height
		}
		cacheFilename += "." + format

		cachePath := filepath.Join(metadataPath, "cache", "covers", cacheFilename)

		if _, err := os.Stat(cachePath); err == nil {
			if r.URL.Query().Get("ts") != "" {
				w.Header().Set("Cache-Control", "private, max-age=86400")
			}
			w.Header().Set("Content-Type", "image/"+format)
			http.ServeFile(w, r, cachePath)
			return
		}

		// Cache miss: delegate to Node.js to resize/format and cache it
		log.Printf("[Cover] Cache miss for %s. Proxying to Node.js.", cacheFilename)
		proxy.ServeHTTP(w, r)
	}
}

func serveDownload(db *sql.DB, proxy http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userVal := r.Context().Value(UserContextKey)
		if userVal == nil {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}
		user := userVal.(*UserSession)

		if !user.CanDownload {
			log.Printf("[Download] Forbidden: User %s does not have download permissions", user.Username)
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		parts := strings.Split(r.URL.Path, "/")
		var itemID string
		for i, part := range parts {
			if part == "items" && i+1 < len(parts) {
				itemID = parts[i+1]
				break
			}
		}

		if itemID == "" {
			http.Error(w, `{"error": "Invalid Item ID"}`, http.StatusBadRequest)
			return
		}

		info, err := GetLibraryItemDownloadInfo(db, itemID)
		if err != nil {
			log.Printf("[Download] Failed to get library item info: %v. Proxying to Node.js.", err)
			proxy.ServeHTTP(w, r)
			return
		}

		log.Printf("[Download] User %s requested download for item %s (isFile: %t)", user.Username, itemID, info.IsFile)

		if info.IsFile {
			w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filepath.Base(info.RelPath)))
			http.ServeFile(w, r, info.Path)
			return
		}

		// Directory zip downloads require Node.js on-the-fly zip helpers
		log.Printf("[Download] Item is a directory. Proxying to Node.js for zipping.")
		proxy.ServeHTTP(w, r)
	}
}

func parseConfig() *Config {
	configFlag := flag.String("c", "", "Config path")
	metadataFlag := flag.String("m", "", "Metadata path")
	portFlag := flag.String("p", "", "Port")
	hostFlag := flag.String("h", "", "Host")
	sourceFlag := flag.String("s", "", "Source")
	devFlag := flag.Bool("d", false, "Dev mode")
	prodDevFlag := flag.Bool("r", false, "Prod with dev env")
	legacyURLFlag := flag.String("legacy-url", "http://localhost:3334", "Legacy Node.js server URL")

	flag.Parse()

	configPath := *configFlag
	if configPath == "" {
		configPath = os.Getenv("CONFIG_PATH")
	}
	if configPath == "" {
		configPath = "config"
	}
	configPath, _ = filepath.Abs(configPath)

	metadataPath := *metadataFlag
	if metadataPath == "" {
		metadataPath = os.Getenv("METADATA_PATH")
	}
	if metadataPath == "" {
		metadataPath = "metadata"
	}
	metadataPath, _ = filepath.Abs(metadataPath)

	port := *portFlag
	if port == "" {
		port = os.Getenv("PORT")
	}
	if port == "" {
		port = "3333"
	}

	host := *hostFlag
	if host == "" {
		host = os.Getenv("HOST")
	}

	source := *sourceFlag
	if source == "" {
		source = os.Getenv("SOURCE")
	}
	if source == "" {
		source = "debian"
	}

	routerBasePath := os.Getenv("ROUTER_BASE_PATH")
	if routerBasePath == "" {
		routerBasePath = "/audiobookshelf"
	}

	return &Config{
		ConfigPath:     configPath,
		MetadataPath:   metadataPath,
		Port:           port,
		Host:           host,
		Source:         source,
		Dev:            *devFlag,
		ProdWithDevEnv: *prodDevFlag,
		LegacyURL:      *legacyURLFlag,
		RouterBasePath: routerBasePath,
	}
}

func initDB(dbPath string) (*sql.DB, error) {
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("database file %s does not exist yet", dbPath)
	}

	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode=WAL&_pragma=busy_timeout=5000", dbPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}

	return db, nil
}

func handleGetLibraries(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userVal := r.Context().Value(UserContextKey)
		if userVal == nil {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}
		user := userVal.(*UserSession)

		libs, err := GetLibraries(db)
		if err != nil {
			log.Printf("[Go] Failed to get libraries: %v", err)
			http.Error(w, fmt.Sprintf(`{"error": "%s"}`, err.Error()), http.StatusInternalServerError)
			return
		}

		var filteredLibs []*LibraryJSON = []*LibraryJSON{}
		includeStats := strings.Contains(r.URL.Query().Get("include"), "stats")

		for _, lib := range libs {
			if user.CanAccessLibrary(lib.ID) {
				if includeStats {
					var stats *LibraryStats
					var err error
					if lib.MediaType == "book" {
						stats, err = GetBookLibraryStats(db, lib.ID)
					} else if lib.MediaType == "podcast" {
						stats, err = GetPodcastLibraryStats(db, lib.ID)
					}
					if err == nil {
						lib.Stats = stats
					}
				}
				filteredLibs = append(filteredLibs, lib)
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"libraries": filteredLibs,
		})
	}
}

func handleGetLibraryByID(db *sql.DB, libraryID string, proxy http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userVal := r.Context().Value(UserContextKey)
		if userVal == nil {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}
		user := userVal.(*UserSession)

		if !user.CanAccessLibrary(libraryID) {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		if strings.Contains(r.URL.RawQuery, "include=filterdata") {
			proxy.ServeHTTP(w, r)
			return
		}

		lib, err := GetLibraryByID(db, libraryID)
		if err != nil {
			log.Printf("[Go] Failed to get library %s: %v", libraryID, err)
			http.Error(w, fmt.Sprintf(`{"error": "%s"}`, err.Error()), http.StatusInternalServerError)
			return
		}
		if lib == nil {
			http.Error(w, "Library not found", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(lib)
	}
}

func handleGetLibraryItems(db *sql.DB, libraryID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userVal := r.Context().Value(UserContextKey)
		if userVal == nil {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}
		user := userVal.(*UserSession)

		if !user.CanAccessLibrary(libraryID) {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		lib, err := GetLibraryByID(db, libraryID)
		if err != nil {
			log.Printf("[Go] Failed to get library %s: %v", libraryID, err)
			http.Error(w, fmt.Sprintf(`{"error": "%s"}`, err.Error()), http.StatusInternalServerError)
			return
		}
		if lib == nil {
			http.Error(w, "Library not found", http.StatusNotFound)
			return
		}

		q := r.URL.Query()
		limitVal := 0
		if q.Get("limit") != "" {
			fmt.Sscanf(q.Get("limit"), "%d", &limitVal)
		}
		pageVal := 0
		if q.Get("page") != "" {
			fmt.Sscanf(q.Get("page"), "%d", &pageVal)
		}

		sortBy := q.Get("sort")
		sortDesc := q.Get("desc") == "1"
		filterBy := q.Get("filter")
		minified := q.Get("minified") == "1"
		collapseseries := q.Get("collapseseries") == "1"
		include := q.Get("include")

		var includeArray []string
		if include != "" {
			for _, part := range strings.Split(include, ",") {
				includeArray = append(includeArray, strings.TrimSpace(part))
			}
		}

		opts := GetFilteredLibraryItemsOptions{
			LibraryID:      libraryID,
			User:           user,
			FilterBy:       filterBy,
			SortBy:         sortBy,
			SortDesc:       sortDesc,
			Limit:          limitVal,
			Page:           pageVal,
			CollapseSeries: collapseseries,
			Include:        includeArray,
			MediaType:      lib.MediaType,
			Minified:       minified,
		}

		results, total, err := GetFilteredLibraryItems(db, opts)
		if err != nil {
			log.Printf("[Go] Failed to get filtered items for library %s: %v", libraryID, err)
			http.Error(w, fmt.Sprintf(`{"error": "%s"}`, err.Error()), http.StatusInternalServerError)
			return
		}

		payload := map[string]interface{}{
			"results":        results,
			"total":          total,
			"limit":          limitVal,
			"page":           pageVal,
			"sortBy":         sortBy,
			"sortDesc":       sortDesc,
			"filterBy":       filterBy,
			"mediaType":      lib.MediaType,
			"minified":       minified,
			"collapseseries": collapseseries,
			"include":        include,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(payload)
	}
}

func handleCreateLibrary(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userVal := r.Context().Value(UserContextKey)
		if userVal == nil {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}
		user := userVal.(*UserSession)

		if user.Type != "admin" && user.Type != "root" {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		var payload CreateLibraryPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
			return
		}

		if payload.Name == "" {
			http.Error(w, `{"error": "Name is required"}`, http.StatusBadRequest)
			return
		}

		for i, f := range payload.Folders {
			fpath := f.FullPath
			if fpath == "" {
				fpath = f.Path
			}
			if fpath == "" {
				http.Error(w, `{"error": "Folder path is required"}`, http.StatusBadRequest)
				return
			}
			absPath, err := filepath.Abs(fpath)
			if err != nil {
				absPath = fpath
			}
			absPath = filepath.ToSlash(absPath)
			if err := os.MkdirAll(absPath, 0755); err != nil {
				log.Printf("Failed to create folder directory %s: %v", absPath, err)
				http.Error(w, fmt.Sprintf(`{"error": "Invalid folder directory %s"}`, absPath), http.StatusBadRequest)
				return
			}
			payload.Folders[i].Path = absPath
		}

		lib, err := CreateLibrary(db, &payload)
		if err != nil {
			log.Printf("[Go] Failed to create library: %v", err)
			http.Error(w, fmt.Sprintf(`{"error": "%s"}`, err.Error()), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(lib)
	}
}

func handleUpdateLibrary(db *sql.DB, libraryID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userVal := r.Context().Value(UserContextKey)
		if userVal == nil {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}
		user := userVal.(*UserSession)

		if user.Type != "admin" && user.Type != "root" {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		var payload UpdateLibraryPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
			return
		}

		if payload.Folders != nil {
			for i, f := range payload.Folders {
				if f.ID == "" {
					fpath := f.FullPath
					if fpath == "" {
						fpath = f.Path
					}
					if fpath == "" {
						http.Error(w, `{"error": "Folder path is required"}`, http.StatusBadRequest)
						return
					}
					absPath, err := filepath.Abs(fpath)
					if err != nil {
						absPath = fpath
					}
					absPath = filepath.ToSlash(absPath)
					if err := os.MkdirAll(absPath, 0755); err != nil {
						log.Printf("Failed to create folder directory %s: %v", absPath, err)
						http.Error(w, fmt.Sprintf(`{"error": "Invalid folder directory %s"}`, absPath), http.StatusBadRequest)
						return
					}
					payload.Folders[i].Path = absPath
				}
			}
		}

		lib, err := UpdateLibrary(db, libraryID, &payload)
		if err != nil {
			log.Printf("[Go] Failed to update library %s: %v", libraryID, err)
			if err.Error() == "library not found" {
				http.Error(w, `{"error": "Library not found"}`, http.StatusNotFound)
			} else {
				http.Error(w, fmt.Sprintf(`{"error": "%s"}`, err.Error()), http.StatusInternalServerError)
			}
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(lib)
	}
}

func handleDeleteLibrary(db *sql.DB, libraryID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userVal := r.Context().Value(UserContextKey)
		if userVal == nil {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}
		user := userVal.(*UserSession)

		if user.Type != "admin" && user.Type != "root" {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		lib, err := DeleteLibrary(db, libraryID)
		if err != nil {
			log.Printf("[Go] Failed to delete library %s: %v", libraryID, err)
			if err.Error() == "library not found" {
				http.Error(w, `{"error": "Library not found"}`, http.StatusNotFound)
			} else {
				http.Error(w, fmt.Sprintf(`{"error": "%s"}`, err.Error()), http.StatusInternalServerError)
			}
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(lib)
	}
}
