package main

import (
	"archive/zip"
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"audiobookshelf/internal/auth"
	"audiobookshelf/internal/feed"
	"audiobookshelf/internal/finders"
	"audiobookshelf/internal/playlist"
	"audiobookshelf/internal/providers"
	"audiobookshelf/internal/share"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
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

var (
	globalOIDCHandler   *auth.OIDCHandler
	globalOIDCHandlerMu sync.RWMutex

	globalShareManager    *share.ShareManager
	globalPlaylistManager *playlist.PlaylistManager
	globalFeedManager     *feed.FeedManager
	globalFinder          *finders.Finder
)

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

	// Standalone Go Gateway: legacy URL reverse proxy fallback is removed.

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
		if GlobalWatcher != nil {
			if err := GlobalWatcher.Close(); err != nil {
				log.Printf("[Watcher] Error closing watcher: %v", err)
			}
		}
		srv.Close()
	}()

	log.Printf("Go Gateway listening on http://%s", serverAddr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server ListenAndServe failed: %v", err)
	}
	log.Printf("Go Gateway stopped.")
}

func setupHandler(db *sql.DB, cfg *Config, dbConnected bool, appRoot string, version string) http.Handler {
	dbPath := filepath.Join(cfg.ConfigPath, "absdatabase.sqlite")

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
		if !dbConnected {
			// Try to reconnect if not connected yet
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
			log.Printf("[Status] DB not connected.")
			http.Error(w, `{"error": "Database not connected"}`, http.StatusInternalServerError)
			return
		}

		isInit, err := HasRootUser(db)
		if err != nil {
			log.Printf("[Status] Failed to check root user: %v", err)
			http.Error(w, `{"error": "Failed to check status"}`, http.StatusInternalServerError)
			return
		}

		settings, err := GetServerSettings(db)
		if err != nil {
			log.Printf("[Status] Failed to get server settings: %v", err)
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
		w.Header().Set("Content-Security-Policy", "frame-ancestors 'self'")
		w.Header().Set("Referrer-Policy", "no-referrer")
		json.NewEncoder(w).Encode(payload)
	})

	// Auth & User endpoints
	mux.HandleFunc(cfg.RouterBasePath+"/login", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			handleLogin(db)(w, r)
		} else if r.Method == http.MethodGet {
			http.ServeFile(w, r, filepath.Join(appRoot, "client", "dist", "index.html"))
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc(cfg.RouterBasePath+"/logout", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			handleLogout(db)(w, r)
		} else if r.Method == http.MethodGet {
			http.ServeFile(w, r, filepath.Join(appRoot, "client", "dist", "index.html"))
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc(cfg.RouterBasePath+"/init", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			handleInit(db)(w, r)
		} else if r.Method == http.MethodGet {
			http.ServeFile(w, r, filepath.Join(appRoot, "client", "dist", "index.html"))
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc(cfg.RouterBasePath+"/auth/logout", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			handleLogout(db)(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc(cfg.RouterBasePath+"/auth/refresh", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			handleRefresh(db)(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc(cfg.RouterBasePath+"/api/authorize", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			AuthMiddleware(db, getTokenSecret(db), handleAuthorize(db)).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc(cfg.RouterBasePath+"/api/users/online", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			AuthMiddleware(db, getTokenSecret(db), handleGetOnlineUsers(db)).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})

	handleUsersDispatch := func(db *sql.DB) http.HandlerFunc {
		usersHandler := handleGetUsers(db)
		crudHandler := handleUserCRUD(db)
		return func(w http.ResponseWriter, r *http.Request) {
			pathWithoutPrefix := strings.TrimPrefix(r.URL.Path, cfg.RouterBasePath)
			if pathWithoutPrefix == "/api/users" || pathWithoutPrefix == "/api/users/" {
				if r.Method == http.MethodGet {
					usersHandler(w, r)
					return
				}
			}
			crudHandler(w, r)
		}
	}

	mux.HandleFunc(cfg.RouterBasePath+"/api/users", func(w http.ResponseWriter, r *http.Request) {
		AuthMiddleware(db, getTokenSecret(db), handleUsersDispatch(db)).ServeHTTP(w, r)
	})
	mux.HandleFunc(cfg.RouterBasePath+"/api/users/", func(w http.ResponseWriter, r *http.Request) {
		AuthMiddleware(db, getTokenSecret(db), handleUsersDispatch(db)).ServeHTTP(w, r)
	})

	// Server Settings endpoints
	mux.HandleFunc(cfg.RouterBasePath+"/api/settings", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			AuthMiddleware(db, getTokenSecret(db), handleGetServerSettings(db)).ServeHTTP(w, r)
		} else if r.Method == http.MethodPatch {
			AuthMiddleware(db, getTokenSecret(db), handleUpdateServerSettings(db)).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc(cfg.RouterBasePath+"/api/sorting-prefixes", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch {
			AuthMiddleware(db, getTokenSecret(db), handleUpdateSortingPrefixes(db)).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc(cfg.RouterBasePath+"/api/auth-settings", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			AuthMiddleware(db, getTokenSecret(db), handleGetAuthSettings(db)).ServeHTTP(w, r)
		} else if r.Method == http.MethodPatch {
			AuthMiddleware(db, getTokenSecret(db), handleUpdateAuthSettings(db)).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc(cfg.RouterBasePath+"/api/search/providers", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			AuthMiddleware(db, getTokenSecret(db), handleGetMetadataProviders(db)).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc(cfg.RouterBasePath+"/api/custom-metadata-providers", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			AuthMiddleware(db, getTokenSecret(db), handleGetCustomMetadataProviders(db)).ServeHTTP(w, r)
		} else if r.Method == http.MethodPost {
			AuthMiddleware(db, getTokenSecret(db), handleCreateCustomMetadataProvider(db)).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc(cfg.RouterBasePath+"/api/custom-metadata-providers/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			AuthMiddleware(db, getTokenSecret(db), handleDeleteCustomMetadataProvider(db)).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})

	// Stubs for ApiKeys, Notifications, Email settings, and Feeds
	mux.HandleFunc(cfg.RouterBasePath+"/api/api-keys", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			AuthMiddleware(db, getTokenSecret(db), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`{"apiKeys":[]}`))
			})).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc(cfg.RouterBasePath+"/api/notifications", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			AuthMiddleware(db, getTokenSecret(db), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`{"data":null,"settings":{"appriseApiUrl":null,"maxNotificationQueue":25,"maxFailedAttempts":5,"notifications":[]}}`))
			})).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc(cfg.RouterBasePath+"/api/emails/settings", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			AuthMiddleware(db, getTokenSecret(db), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`{"settings":{"host":"","port":465,"secure":true,"rejectUnauthorized":true,"user":"","pass":"","testAddress":"","fromAddress":"","ereaderDevices":[]}}`))
			})).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc(cfg.RouterBasePath+"/api/feeds", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			AuthMiddleware(db, getTokenSecret(db), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`{"feeds":[]}`))
			})).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})

	// Backup endpoints
	mux.HandleFunc(cfg.RouterBasePath+"/api/backups", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			AuthMiddleware(db, getTokenSecret(db), handleGetBackups(db, cfg.MetadataPath)).ServeHTTP(w, r)
		} else if r.Method == http.MethodPost {
			AuthMiddleware(db, getTokenSecret(db), handleCreateBackup(db, cfg.ConfigPath, cfg.MetadataPath)).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc(cfg.RouterBasePath+"/api/backups/path", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			AuthMiddleware(db, getTokenSecret(db), handleUpdateBackupPath(db, cfg.MetadataPath)).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc(cfg.RouterBasePath+"/api/backups/upload", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			AuthMiddleware(db, getTokenSecret(db), handleUploadBackup(db, cfg.MetadataPath)).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc(cfg.RouterBasePath+"/api/backups/", func(w http.ResponseWriter, r *http.Request) {
		subPath := strings.TrimPrefix(r.URL.Path, cfg.RouterBasePath+"/api/backups/")
		parts := strings.Split(subPath, "/")
		if len(parts) == 1 && parts[0] != "" {
			if r.Method == http.MethodDelete {
				AuthMiddleware(db, getTokenSecret(db), handleDeleteBackup(db, cfg.MetadataPath)).ServeHTTP(w, r)
				return
			}
		} else if len(parts) == 2 && parts[1] == "download" {
			if r.Method == http.MethodGet {
				AuthMiddleware(db, getTokenSecret(db), handleDownloadBackup(db, cfg.MetadataPath)).ServeHTTP(w, r)
				return
			}
		} else if len(parts) == 2 && parts[1] == "apply" {
			if r.Method == http.MethodPost {
				AuthMiddleware(db, getTokenSecret(db), handleApplyBackup(db, cfg.ConfigPath, cfg.MetadataPath, func() {
					log.Printf("[Backup Apply] Restart/reload triggered.")
				})).ServeHTTP(w, r)
				return
			}
		}
		http.NotFound(w, r)
	})

	// /api/me endpoints
	mux.HandleFunc(cfg.RouterBasePath+"/api/me", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			AuthMiddleware(db, getTokenSecret(db), handleGetMe(db)).ServeHTTP(w, r)
			return
		}
		http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
	})
	mux.HandleFunc(cfg.RouterBasePath+"/api/me/", func(w http.ResponseWriter, r *http.Request) {
		subPath := strings.TrimPrefix(r.URL.Path, cfg.RouterBasePath+"/api/me/")
		parts := strings.Split(subPath, "/")

		if len(parts) == 1 && parts[0] == "password" {
			if r.Method == http.MethodPost {
				AuthMiddleware(db, getTokenSecret(db), handleUpdateMePassword(db)).ServeHTTP(w, r)
				return
			}
		} else if len(parts) == 1 && parts[0] == "items-in-progress" {
			if r.Method == http.MethodGet {
				AuthMiddleware(db, getTokenSecret(db), handleGetAllLibraryItemsInProgress(db)).ServeHTTP(w, r)
				return
			}
		} else if len(parts) == 2 && parts[0] == "progress" {
			if r.Method == http.MethodGet {
				AuthMiddleware(db, getTokenSecret(db), handleGetMeProgress(db)).ServeHTTP(w, r)
				return
			} else if r.Method == http.MethodPatch || r.Method == http.MethodPost {
				AuthMiddleware(db, getTokenSecret(db), handleCreateUpdateMeProgress(db)).ServeHTTP(w, r)
				return
			} else if r.Method == http.MethodDelete {
				AuthMiddleware(db, getTokenSecret(db), handleRemoveMeProgress(db)).ServeHTTP(w, r)
				return
			}
		} else if len(parts) == 3 && parts[0] == "progress" && parts[2] == "hide-from-continue-listening" {
			if r.Method == http.MethodPatch {
				AuthMiddleware(db, getTokenSecret(db), handleHideMeProgressFromContinueListening(db)).ServeHTTP(w, r)
				return
			}
		} else if len(parts) == 3 && parts[0] == "series" && parts[2] == "remove" {
			if r.Method == http.MethodPost {
				AuthMiddleware(db, getTokenSecret(db), handleRemoveSeriesFromContinueListening(db)).ServeHTTP(w, r)
				return
			}
		} else if len(parts) == 3 && parts[0] == "series" && parts[2] == "readd" {
			if r.Method == http.MethodPost {
				AuthMiddleware(db, getTokenSecret(db), handleReaddSeriesFromContinueListening(db)).ServeHTTP(w, r)
				return
			}
		} else if len(parts) == 3 && parts[0] == "item" && parts[2] == "bookmark" {
			if r.Method == http.MethodPost {
				AuthMiddleware(db, getTokenSecret(db), handleMeCreateBookmark(db)).ServeHTTP(w, r)
				return
			} else if r.Method == http.MethodPatch {
				AuthMiddleware(db, getTokenSecret(db), handleMeUpdateBookmark(db)).ServeHTTP(w, r)
				return
			} else if r.Method == http.MethodDelete {
				AuthMiddleware(db, getTokenSecret(db), handleMeRemoveBookmark(db)).ServeHTTP(w, r)
				return
			}
		}

		http.NotFound(w, r)
	})

	// Tag / Genre / Misc endpoints
	mux.HandleFunc(cfg.RouterBasePath+"/api/tags", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			AuthMiddleware(db, getTokenSecret(db), handleGetAllTags(db)).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc(cfg.RouterBasePath+"/api/tags/rename", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			AuthMiddleware(db, getTokenSecret(db), handleRenameTag(db)).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc(cfg.RouterBasePath+"/api/tags/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			AuthMiddleware(db, getTokenSecret(db), handleDeleteTag(db)).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc(cfg.RouterBasePath+"/api/genres", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			AuthMiddleware(db, getTokenSecret(db), handleGetAllGenres(db)).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc(cfg.RouterBasePath+"/api/genres/rename", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			AuthMiddleware(db, getTokenSecret(db), handleRenameGenre(db)).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc(cfg.RouterBasePath+"/api/genres/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			AuthMiddleware(db, getTokenSecret(db), handleDeleteGenre(db)).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc(cfg.RouterBasePath+"/api/stats/year/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			AuthMiddleware(db, getTokenSecret(db), handleGetAdminStatsForYear(db)).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc(cfg.RouterBasePath+"/api/logger-data", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			AuthMiddleware(db, getTokenSecret(db), handleGetLoggerData(db)).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc(cfg.RouterBasePath+"/api/validate-cron", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			AuthMiddleware(db, getTokenSecret(db), http.HandlerFunc(handleValidateCron)).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc(cfg.RouterBasePath+"/api/watcher/update", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			AuthMiddleware(db, getTokenSecret(db), http.HandlerFunc(handleWatcherUpdate)).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc(cfg.RouterBasePath+"/api/filesystem", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			AuthMiddleware(db, getTokenSecret(db), handleGetFilesystem(appRoot)).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc(cfg.RouterBasePath+"/api/filesystem/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			AuthMiddleware(db, getTokenSecret(db), handleGetFilesystem(appRoot)).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc(cfg.RouterBasePath+"/api/filesystem/pathexists", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			AuthMiddleware(db, getTokenSecret(db), handleCheckPathExists(db)).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc(cfg.RouterBasePath+"/api/tasks", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			AuthMiddleware(db, getTokenSecret(db), handleGetTasks(db)).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc(cfg.RouterBasePath+"/api/tasks/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			AuthMiddleware(db, getTokenSecret(db), handleGetTasks(db)).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})

	// Libraries API (native Go implementation)
	mux.HandleFunc(cfg.RouterBasePath+"/api/libraries", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[Go] %s /api/libraries", r.Method)
		if r.Method == http.MethodGet {
			AuthMiddleware(db, getTokenSecret(db), http.HandlerFunc(handleGetLibraries(db))).ServeHTTP(w, r)
		} else if r.Method == http.MethodPost {
			AuthMiddleware(db, getTokenSecret(db), http.HandlerFunc(handleCreateLibrary(db))).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc(cfg.RouterBasePath+"/api/libraries/", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[Go] %s %s", r.Method, r.URL.Path)

		// strip RouterBasePath + "/api/libraries/"
		subPath := strings.TrimPrefix(r.URL.Path, cfg.RouterBasePath+"/api/libraries/")
		if subPath == "" {
			if r.Method == http.MethodGet {
				AuthMiddleware(db, getTokenSecret(db), http.HandlerFunc(handleGetLibraries(db))).ServeHTTP(w, r)
				return
			}
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
			return
		}

		parts := strings.Split(subPath, "/")
		if len(parts) == 1 && parts[0] != "" {
			libraryID := parts[0]
			if r.Method == http.MethodGet {
				AuthMiddleware(db, getTokenSecret(db), http.HandlerFunc(handleGetLibraryByID(db, libraryID))).ServeHTTP(w, r)
				return
			} else if r.Method == http.MethodPatch {
				AuthMiddleware(db, getTokenSecret(db), http.HandlerFunc(handleUpdateLibrary(db, libraryID))).ServeHTTP(w, r)
				return
			} else if r.Method == http.MethodDelete {
				AuthMiddleware(db, getTokenSecret(db), http.HandlerFunc(handleDeleteLibrary(db, libraryID))).ServeHTTP(w, r)
				return
			}
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
			return
		} else if len(parts) == 2 && parts[1] == "items" {
			libraryID := parts[0]
			if r.Method == http.MethodGet {
				AuthMiddleware(db, getTokenSecret(db), http.HandlerFunc(handleGetLibraryItems(db, libraryID))).ServeHTTP(w, r)
				return
			}
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
			return
		} else if len(parts) == 2 && parts[1] == "filterdata" {
			libraryID := parts[0]
			if r.Method == http.MethodGet {
				AuthMiddleware(db, getTokenSecret(db), http.HandlerFunc(handleGetLibraryFilterData(db, libraryID))).ServeHTTP(w, r)
				return
			}
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
			return
		} else if len(parts) == 2 && parts[1] == "playlists" {
			libraryID := parts[0]
			if r.Method == http.MethodGet {
				AuthMiddleware(db, getTokenSecret(db), http.HandlerFunc(handleGetLibraryPlaylists(db, libraryID))).ServeHTTP(w, r)
				return
			}
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
			return
		} else if len(parts) == 2 && parts[1] == "collections" {
			libraryID := parts[0]
			if r.Method == http.MethodGet {
				AuthMiddleware(db, getTokenSecret(db), http.HandlerFunc(handleGetLibraryCollections(db, libraryID))).ServeHTTP(w, r)
				return
			}
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
			return
		} else if len(parts) == 2 && parts[1] == "opml" {
			libraryID := parts[0]
			if r.Method == http.MethodGet {
				AuthMiddleware(db, getTokenSecret(db), http.HandlerFunc(handleGetLibraryOPML(db, libraryID))).ServeHTTP(w, r)
				return
			}
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
			return
		} else if len(parts) == 2 && parts[1] == "scan" {
			libraryID := parts[0]
			if r.Method == http.MethodPost {
				AuthMiddleware(db, getTokenSecret(db), http.HandlerFunc(handleScanLibrary(db, libraryID))).ServeHTTP(w, r)
				return
			}
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
			return
		}

		http.NotFound(w, r)
	})

	// Playlists API
	mux.HandleFunc(cfg.RouterBasePath+"/api/playlists", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			AuthMiddleware(db, getTokenSecret(db), http.HandlerFunc(handleGetPlaylists(db))).ServeHTTP(w, r)
		} else if r.Method == http.MethodPost {
			AuthMiddleware(db, getTokenSecret(db), http.HandlerFunc(handleCreatePlaylist(db))).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc(cfg.RouterBasePath+"/api/playlists/", func(w http.ResponseWriter, r *http.Request) {
		pathWithoutPrefix := strings.TrimPrefix(r.URL.Path, cfg.RouterBasePath)
		id := strings.TrimPrefix(pathWithoutPrefix, "/api/playlists/")
		if id == "" {
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodGet {
			AuthMiddleware(db, getTokenSecret(db), http.HandlerFunc(handleGetPlaylist(db, id))).ServeHTTP(w, r)
		} else if r.Method == http.MethodPatch {
			AuthMiddleware(db, getTokenSecret(db), http.HandlerFunc(handleUpdatePlaylist(db, id))).ServeHTTP(w, r)
		} else if r.Method == http.MethodDelete {
			AuthMiddleware(db, getTokenSecret(db), http.HandlerFunc(handleDeletePlaylist(db, id))).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})

	// Collections API
	mux.HandleFunc(cfg.RouterBasePath+"/api/collections", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			AuthMiddleware(db, getTokenSecret(db), http.HandlerFunc(handleGetCollections(db))).ServeHTTP(w, r)
		} else if r.Method == http.MethodPost {
			AuthMiddleware(db, getTokenSecret(db), http.HandlerFunc(handleCreateCollection(db))).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc(cfg.RouterBasePath+"/api/collections/", func(w http.ResponseWriter, r *http.Request) {
		pathWithoutPrefix := strings.TrimPrefix(r.URL.Path, cfg.RouterBasePath)
		id := strings.TrimPrefix(pathWithoutPrefix, "/api/collections/")
		if id == "" {
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodGet {
			AuthMiddleware(db, getTokenSecret(db), http.HandlerFunc(handleGetCollection(db, id))).ServeHTTP(w, r)
		} else if r.Method == http.MethodPatch {
			AuthMiddleware(db, getTokenSecret(db), http.HandlerFunc(handleUpdateCollection(db, id))).ServeHTTP(w, r)
		} else if r.Method == http.MethodDelete {
			AuthMiddleware(db, getTokenSecret(db), http.HandlerFunc(handleDeleteCollection(db, id))).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})

	// Shares API
	mux.HandleFunc(cfg.RouterBasePath+"/api/share/mediaitem", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			AuthMiddleware(db, getTokenSecret(db), http.HandlerFunc(handleCreateShare(db))).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc(cfg.RouterBasePath+"/api/share/mediaitem/", func(w http.ResponseWriter, r *http.Request) {
		pathWithoutPrefix := strings.TrimPrefix(r.URL.Path, cfg.RouterBasePath)
		id := strings.TrimPrefix(pathWithoutPrefix, "/api/share/mediaitem/")
		if id == "" {
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodDelete {
			AuthMiddleware(db, getTokenSecret(db), http.HandlerFunc(handleDeleteShare(db, id))).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})

	// Search API
	mux.HandleFunc(cfg.RouterBasePath+"/api/search/books", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			AuthMiddleware(db, getTokenSecret(db), http.HandlerFunc(handleSearchBooks(db))).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc(cfg.RouterBasePath+"/api/search/podcast", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			AuthMiddleware(db, getTokenSecret(db), http.HandlerFunc(handleSearchPodcasts(db))).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})


	// RSS Feed API
	mux.HandleFunc(cfg.RouterBasePath+"/feed/", func(w http.ResponseWriter, r *http.Request) {
		initManagers(db)
		pathWithoutPrefix := strings.TrimPrefix(r.URL.Path, cfg.RouterBasePath)
		subPath := strings.TrimPrefix(pathWithoutPrefix, "/feed/")
		parts := strings.Split(subPath, "/")
		if len(parts) == 0 || parts[0] == "" {
			http.NotFound(w, r)
			return
		}
		slug := parts[0]
		globalFeedManager.ServeRSSFeed(slug).ServeHTTP(w, r)
	})

	// OIDC API
	mux.HandleFunc(cfg.RouterBasePath+"/auth/openid", func(w http.ResponseWriter, r *http.Request) {
		s, err := getOIDCSettings(db)
		if err != nil || s.IssuerURL == "" {
			log.Printf("[OIDC Login] Error getting OIDC settings or not configured: %v", err)
			http.Error(w, "OIDC is not configured or settings error", http.StatusBadRequest)
			return
		}
		globalOIDCHandlerMu.Lock()
		globalOIDCHandler = auth.NewOIDCHandler(s)
		globalOIDCHandlerMu.Unlock()
		globalOIDCHandler.HandleLogin(w, r)
	})
	mux.HandleFunc(cfg.RouterBasePath+"/auth/openid/callback", func(w http.ResponseWriter, r *http.Request) {
		handleOIDCCallback(db)(w, r)
	})
	mux.HandleFunc(cfg.RouterBasePath+"/auth/openid/mobile-redirect", func(w http.ResponseWriter, r *http.Request) {
		handleOIDCCallback(db)(w, r)
	})

	// Cover and download endpoints (native Go implementation)
	mux.HandleFunc(cfg.RouterBasePath+"/api/items/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if strings.HasSuffix(path, "/cover") {
			serveCover(db, cfg.MetadataPath)(w, r)
			return
		}
		if strings.HasSuffix(path, "/download") {
			AuthMiddleware(db, getTokenSecret(db), http.HandlerFunc(serveDownload(db))).ServeHTTP(w, r)
			return
		}

		log.Printf("[Backend] 404 Not Found: %s %s", r.Method, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error": "API route not found"}`))
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
			log.Printf("[Backend] 404 Not Found: %s %s", r.Method, r.URL.Path)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"error": "API route not found"}`))
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

func serveCover(db *sql.DB, metadataPath string) http.HandlerFunc {
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
				http.NotFound(w, r)
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

		// Cache miss fallback: serve the raw cover natively
		log.Printf("[Cover] Cache miss for %s. Serving raw cover.", cacheFilename)
		coverPath, err := GetCoverPath(db, itemID)
		if err != nil || coverPath == "" {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, coverPath)
	}
}

func serveDownload(db *sql.DB) http.HandlerFunc {
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
			log.Printf("[Download] Failed to get library item info: %v", err)
			http.Error(w, `{"error": "Library item not found"}`, http.StatusNotFound)
			return
		}

		log.Printf("[Download] User %s requested download for item %s (isFile: %t)", user.Username, itemID, info.IsFile)

		if info.IsFile {
			w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filepath.Base(info.RelPath)))
			http.ServeFile(w, r, info.Path)
			return
		}

		// Directory zip downloads: zip on-the-fly in Go
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q.zip", filepath.Base(info.Path)))
		zw := zip.NewWriter(w)
		defer zw.Close()

		filepath.Walk(info.Path, func(path string, fileInfo os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if fileInfo.IsDir() {
				return nil
			}
			rel, err := filepath.Rel(info.Path, path)
			if err != nil {
				return err
			}
			f, err := os.Open(path)
			if err != nil {
				return err
			}
			defer f.Close()

			header, err := zip.FileInfoHeader(fileInfo)
			if err != nil {
				return err
			}
			header.Name = rel
			header.Method = zip.Deflate

			writer, err := zw.CreateHeader(header)
			if err != nil {
				return err
			}
			_, err = io.Copy(writer, f)
			return err
		})
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

func handleGetLibraryByID(db *sql.DB, libraryID string) http.HandlerFunc {
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
			fd, err := getLibraryFilterDataGo(db, libraryID)
			if err != nil {
				log.Printf("[Library getFilterData] Error: %v", err)
				http.Error(w, `{"error": "Failed to load filter data"}`, http.StatusInternalServerError)
				return
			}
			lib, err := GetLibraryByID(db, libraryID)
			if err != nil || lib == nil {
				http.Error(w, `{"error": "Library not found"}`, http.StatusNotFound)
				return
			}
			libBytes, _ := json.Marshal(lib)
			var libMap map[string]interface{}
			json.Unmarshal(libBytes, &libMap)
			libMap["filterdata"] = fd
			libMap["issues"] = fd.NumIssues

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(libMap)
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

func handleGetLibraryFilterData(db *sql.DB, libraryID string) http.HandlerFunc {
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

		fd, err := getLibraryFilterDataGo(db, libraryID)
		if err != nil {
			log.Printf("[Library getFilterData] Error: %v", err)
			http.Error(w, `{"error": "Failed to load filter data"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(fd)
	}
}

var initManagersOnce sync.Once

func initManagers(db *sql.DB) {
	initManagersOnce.Do(func() {
		globalShareManager = share.NewShareManager(db)
		globalPlaylistManager = playlist.NewPlaylistManager(db)
		globalFeedManager = feed.NewFeedManager(db)
		globalFinder = finders.NewFinder([]providers.Provider{
			&providers.AudibleProvider{},
			&providers.AudnexusProvider{},
			&providers.GoogleBooksProvider{},
			&providers.ITunesProvider{},
			&providers.OpenLibraryProvider{},
		})
	})
}

func parseMsFromDBStr(s string) int64 {
	layouts := []string{
		"2006-01-02 15:04:05.000 +00:00",
		"2006-01-02T15:04:05.000Z",
		"2006-01-02 15:04:05.000000 +00:00",
		"2006-01-02 15:04:05.000",
		"2006-01-02 15:04:05",
		time.RFC3339,
	}
	for _, l := range layouts {
		if t, err := time.Parse(l, s); err == nil {
			return t.UnixNano() / int64(time.Millisecond)
		}
	}
	return 0
}

func hasColumn(ctx context.Context, db *sql.DB, tableName, columnName string) bool {
	query := fmt.Sprintf("PRAGMA table_info(%s)", tableName)
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return false
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name string
		var typeStr string
		var notnull int
		var dfltVal interface{}
		var pk int
		if err := rows.Scan(&cid, &name, &typeStr, &notnull, &dfltVal, &pk); err != nil {
			log.Printf("[Main] Failed to scan table column info: %v", err)
			continue
		}
		if strings.EqualFold(name, columnName) {
			return true
		}
	}
	if err := rows.Err(); err != nil {
		log.Printf("[Main] Table info iteration error for table %s: %v", tableName, err)
	}
	return false
}

func queryPlaylistsForUserAndLibrary(ctx context.Context, db *sql.DB, userID, libraryID string) ([]map[string]interface{}, error) {
	query := "SELECT id, userId, name, libraryId, description, createdAt, updatedAt FROM playlists WHERE userId = ?"
	var args []interface{}
	args = append(args, userID)
	if libraryID != "" {
		query += " AND (libraryId = ? OR libraryId IS NULL)"
		args = append(args, libraryID)
	}

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var playlists []map[string]interface{}
	for rows.Next() {
		var id, uID, name string
		var libID, desc sql.NullString
		var createdAtStr, updatedAtStr string
		if err := rows.Scan(&id, &uID, &name, &libID, &desc, &createdAtStr, &updatedAtStr); err != nil {
			return nil, err
		}

		p := map[string]interface{}{
			"id":        id,
			"userId":    uID,
			"name":      name,
			"libraryId": libID.String,
			"createdAt": parseMsFromDBStr(createdAtStr),
			"updatedAt": parseMsFromDBStr(updatedAtStr),
		}
		if desc.Valid {
			p["description"] = desc.String
		} else {
			p["description"] = nil
		}
		playlists = append(playlists, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for _, p := range playlists {
		pID := p["id"].(string)
		itemRows, err := db.QueryContext(ctx, `SELECT mediaItemId FROM playlistMediaItems WHERE playlistId = ? ORDER BY "order" ASC`, pID)
		if err != nil {
			return nil, err
		}
		var items []string
		for itemRows.Next() {
			var itemID string
			if err := itemRows.Scan(&itemID); err != nil {
				itemRows.Close()
				return nil, err
			}
			items = append(items, itemID)
		}
		if err := itemRows.Err(); err != nil {
			itemRows.Close()
			return nil, err
		}
		itemRows.Close()
		p["itemIds"] = items
	}

	return playlists, nil
}

func queryCollectionsForLibrary(ctx context.Context, db *sql.DB, libraryID string) ([]map[string]interface{}, error) {
	hasDisplayOrder := hasColumn(ctx, db, "collections", "displayOrder")
	var query string
	var args []interface{}
	if libraryID != "" {
		if hasDisplayOrder {
			query = "SELECT id, name, description, libraryId, displayOrder, createdAt, updatedAt FROM collections WHERE libraryId = ?"
		} else {
			query = "SELECT id, name, description, libraryId, createdAt, updatedAt FROM collections WHERE libraryId = ?"
		}
		args = append(args, libraryID)
	} else {
		if hasDisplayOrder {
			query = "SELECT id, name, description, libraryId, displayOrder, createdAt, updatedAt FROM collections"
		} else {
			query = "SELECT id, name, description, libraryId, createdAt, updatedAt FROM collections"
		}
	}

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var collections []map[string]interface{}
	for rows.Next() {
		var id, name string
		var description, libraryIDCol sql.NullString
		var createdAtStr, updatedAtStr string
		var displayOrder int

		var err error
		if hasDisplayOrder {
			err = rows.Scan(&id, &name, &description, &libraryIDCol, &displayOrder, &createdAtStr, &updatedAtStr)
		} else {
			err = rows.Scan(&id, &name, &description, &libraryIDCol, &createdAtStr, &updatedAtStr)
		}
		if err != nil {
			return nil, err
		}

		c := map[string]interface{}{
			"id":          id,
			"name":        name,
			"description": description.String,
			"libraryId":   libraryIDCol.String,
			"createdAt":   parseMsFromDBStr(createdAtStr),
			"updatedAt":   parseMsFromDBStr(updatedAtStr),
		}
		if hasDisplayOrder {
			c["displayOrder"] = displayOrder
		}
		collections = append(collections, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for _, c := range collections {
		cID := c["id"].(string)
		itemRows, err := db.QueryContext(ctx, `SELECT bookId FROM collectionBooks WHERE collectionId = ? ORDER BY "order" ASC`, cID)
		if err != nil {
			return nil, err
		}
		var items []string
		for itemRows.Next() {
			var bookID string
			if err := itemRows.Scan(&bookID); err != nil {
				itemRows.Close()
				return nil, err
			}
			items = append(items, bookID)
		}
		if err := itemRows.Err(); err != nil {
			itemRows.Close()
			return nil, err
		}
		itemRows.Close()
		c["books"] = items
		c["itemIds"] = items
	}

	return collections, nil
}

func handleGetLibraryPlaylists(db *sql.DB, libraryID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userSess := r.Context().Value(UserContextKey).(*UserSession)
		initManagers(db)

		playlists, err := queryPlaylistsForUserAndLibrary(r.Context(), db, userSess.ID, libraryID)
		if err != nil {
			log.Printf("[Playlist] handleGetLibraryPlaylists failed: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		payload := map[string]interface{}{
			"results": playlists,
			"total":   len(playlists),
			"limit":   0,
			"page":    0,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(payload)
	}
}

func handleGetLibraryCollections(db *sql.DB, libraryID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		initManagers(db)

		collections, err := queryCollectionsForLibrary(r.Context(), db, libraryID)
		if err != nil {
			log.Printf("[Collection] handleGetLibraryCollections failed: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		payload := map[string]interface{}{
			"results": collections,
			"total":   len(collections),
			"limit":   0,
			"page":    0,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(payload)
	}
}

func handleGetLibraryOPML(db *sql.DB, libraryID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userSess := r.Context().Value(UserContextKey).(*UserSession)
		initManagers(db)

		opmlText, err := globalFeedManager.GenerateOPML(r.Context(), userSess.ID, libraryID)
		if err != nil {
			log.Printf("[Feed] GenerateOPML failed: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/xml")
		w.Write([]byte(opmlText))
	}
}

func handleGetPlaylists(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userSess := r.Context().Value(UserContextKey).(*UserSession)
		initManagers(db)

		playlists, err := queryPlaylistsForUserAndLibrary(r.Context(), db, userSess.ID, "")
		if err != nil {
			log.Printf("[Playlist] handleGetPlaylists failed: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		payload := map[string]interface{}{
			"playlists": playlists,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(payload)
	}
}

func handleGetPlaylist(db *sql.DB, id string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		initManagers(db)

		p, err := globalPlaylistManager.GetPlaylist(r.Context(), id)
		if err != nil {
			if err == sql.ErrNoRows {
				http.NotFound(w, r)
				return
			}
			log.Printf("[Playlist] GetPlaylist failed: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(p)
	}
}

func handleCreatePlaylist(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userSess := r.Context().Value(UserContextKey).(*UserSession)
		initManagers(db)

		var req struct {
			Name  string   `json:"name"`
			Items []string `json:"items"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		p := &playlist.Playlist{
			ID:      uuid.New().String(),
			UserID:  userSess.ID,
			Name:    req.Name,
			ItemIDs: req.Items,
		}

		if err := globalPlaylistManager.CreatePlaylist(r.Context(), p); err != nil {
			log.Printf("[Playlist] Create failed: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(p)
	}
}

func handleUpdatePlaylist(db *sql.DB, id string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		initManagers(db)

		p, err := globalPlaylistManager.GetPlaylist(r.Context(), id)
		if err != nil {
			if err == sql.ErrNoRows {
				http.NotFound(w, r)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		var req struct {
			Name  string   `json:"name"`
			Items []string `json:"items"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if req.Name != "" {
			p.Name = req.Name
		}
		if req.Items != nil {
			p.ItemIDs = req.Items
		}

		if err := globalPlaylistManager.UpdatePlaylist(r.Context(), p); err != nil {
			log.Printf("[Playlist] Update failed: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(p)
	}
}

func handleDeletePlaylist(db *sql.DB, id string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		initManagers(db)

		if err := globalPlaylistManager.DeletePlaylist(r.Context(), id); err != nil {
			log.Printf("[Playlist] Delete failed: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"success": true}`))
	}
}

func handleGetCollections(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		initManagers(db)

		collections, err := queryCollectionsForLibrary(r.Context(), db, "")
		if err != nil {
			log.Printf("[Collection] handleGetCollections failed: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		payload := map[string]interface{}{
			"collections": collections,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(payload)
	}
}

func handleGetCollection(db *sql.DB, id string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		initManagers(db)

		c, err := globalPlaylistManager.GetCollection(r.Context(), id)
		if err != nil {
			if err == sql.ErrNoRows {
				http.NotFound(w, r)
				return
			}
			log.Printf("[Collection] GetCollection failed: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(c)
	}
}

func handleCreateCollection(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		initManagers(db)

		var req struct {
			Name        string   `json:"name"`
			Description string   `json:"description"`
			LibraryID   string   `json:"libraryId"`
			Books       []string `json:"books"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		c := &playlist.Collection{
			ID:          uuid.New().String(),
			Name:        req.Name,
			Description: req.Description,
			LibraryID:   req.LibraryID,
			ItemIDs:     req.Books,
		}

		if err := globalPlaylistManager.CreateCollection(r.Context(), c); err != nil {
			log.Printf("[Collection] Create failed: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(c)
	}
}

func handleUpdateCollection(db *sql.DB, id string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		initManagers(db)

		c, err := globalPlaylistManager.GetCollection(r.Context(), id)
		if err != nil {
			if err == sql.ErrNoRows {
				http.NotFound(w, r)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		var req struct {
			Name        string   `json:"name"`
			Description string   `json:"description"`
			LibraryID   string   `json:"libraryId"`
			Books       []string `json:"books"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if req.Name != "" {
			c.Name = req.Name
		}
		if req.Description != "" {
			c.Description = req.Description
		}
		if req.LibraryID != "" {
			c.LibraryID = req.LibraryID
		}
		if req.Books != nil {
			c.ItemIDs = req.Books
		}

		if err := globalPlaylistManager.UpdateCollection(r.Context(), c); err != nil {
			log.Printf("[Collection] Update failed: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(c)
	}
}

func handleDeleteCollection(db *sql.DB, id string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		initManagers(db)

		if err := globalPlaylistManager.DeleteCollection(r.Context(), id); err != nil {
			log.Printf("[Collection] Delete failed: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"success": true}`))
	}
}

func handleCreateShare(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userSess := r.Context().Value(UserContextKey).(*UserSession)
		initManagers(db)

		var req struct {
			Slug           string `json:"slug"`
			ExpiresAt      int64  `json:"expiresAt"`
			MediaItemID    string `json:"mediaItemId"`
			MediaItemType  string `json:"mediaItemType"`
			IsDownloadable bool   `json:"isDownloadable"`
			Password       string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		var libraryItemID string
		err := db.QueryRowContext(r.Context(), "SELECT id FROM libraryItems WHERE mediaId = ?", req.MediaItemID).Scan(&libraryItemID)
		if err != nil {
			err = db.QueryRowContext(r.Context(), "SELECT id FROM libraryItems WHERE id = ?", req.MediaItemID).Scan(&libraryItemID)
			if err != nil {
				log.Printf("[Share] Failed to find libraryItem for mediaItemId %s: %v", req.MediaItemID, err)
				http.Error(w, "Media item not found", http.StatusNotFound)
				return
			}
		}

		var expiresTime time.Time
		if req.ExpiresAt > 0 {
			expiresTime = time.Unix(req.ExpiresAt/1000, (req.ExpiresAt%1000)*1000000)
		}

		var pash string
		if req.Password != "" {
			hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
			if err == nil {
				pash = string(hash)
			}
		}

		s := &share.ShareLink{
			ID:             req.Slug,
			LibraryItemID:  libraryItemID,
			CreatedBy:      userSess.ID,
			ExpiresAt:      expiresTime,
			IsDownloadable: req.IsDownloadable,
			PasswordHash:   pash,
		}

		if err := globalShareManager.CreateShare(r.Context(), s); err != nil {
			log.Printf("[Share] CreateShare failed: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		resPayload := map[string]interface{}{
			"id":             s.ID,
			"slug":           s.ID,
			"libraryItemId":  s.LibraryItemID,
			"mediaItemId":    req.MediaItemID,
			"mediaItemType":  req.MediaItemType,
			"userId":         s.CreatedBy,
			"expiresAt":      nil,
			"isDownloadable": s.IsDownloadable,
			"createdAt":      s.CreatedAt.UnixNano() / int64(time.Millisecond),
			"updatedAt":      s.UpdatedAt.UnixNano() / int64(time.Millisecond),
		}
		if !s.ExpiresAt.IsZero() {
			resPayload["expiresAt"] = req.ExpiresAt
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(resPayload)
	}
}

func handleDeleteShare(db *sql.DB, id string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		initManagers(db)

		if err := globalShareManager.DeleteShare(r.Context(), id); err != nil {
			log.Printf("[Share] DeleteShare failed: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

func handleSearchBooks(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		initManagers(db)

		q := r.URL.Query()
		provider := q.Get("provider")
		if provider == "" {
			provider = "google"
		}
		title := q.Get("title")
		author := q.Get("author")

		queryStr := title
		if author != "" {
			if queryStr != "" {
				queryStr += " " + author
			} else {
				queryStr = author
			}
		}

		results, err := globalFinder.SearchBooks(r.Context(), provider, queryStr)
		if err != nil {
			log.Printf("[Search] SearchBooks failed: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(results)
	}
}

func handleSearchPodcasts(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		initManagers(db)

		q := r.URL.Query()
		term := q.Get("term")

		results, err := globalFinder.SearchPodcasts(r.Context(), "itunes", term)
		if err != nil {
			log.Printf("[Search] SearchPodcasts failed: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(results)
	}
}

func handleGetSearchProviders(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{
		"providers": {
			"books": [
				{"value": "google", "text": "Google Books"},
				{"value": "itunes", "text": "iTunes"},
				{"value": "openlibrary", "text": "Open Library"},
				{"value": "audible", "text": "Audible.com"},
				{"value": "audnexus", "text": "Audnexus"}
			],
			"booksCovers": [
				{"value": "best", "text": "Best"},
				{"value": "google", "text": "Google Books"},
				{"value": "itunes", "text": "iTunes"},
				{"value": "openlibrary", "text": "Open Library"},
				{"value": "audible", "text": "Audible.com"},
				{"value": "audnexus", "text": "Audnexus"},
				{"value": "all", "text": "All"}
			],
			"podcasts": [
				{"value": "itunes", "text": "iTunes"}
			]
		}
	}`))
}

func getOIDCSettings(db *sql.DB) (auth.OIDCSettings, error) {
	var valStr string
	err := db.QueryRow("SELECT value FROM settings WHERE key = 'server-settings'").Scan(&valStr)
	if err != nil {
		return auth.OIDCSettings{}, err
	}

	var settingsMap map[string]interface{}
	if err := json.Unmarshal([]byte(valStr), &settingsMap); err != nil {
		return auth.OIDCSettings{}, err
	}

	s := auth.OIDCSettings{
		IssuerURL:    getString(settingsMap["authOpenIDIssuerURL"]),
		ClientID:     getString(settingsMap["authOpenIDClientID"]),
		ClientSecret: getString(settingsMap["authOpenIDClientSecret"]),
		RedirectURL:  getString(settingsMap["authOpenIDRedirectURL"]),
	}

	if settingsMap["authOpenIDAutoRegister"] != nil {
		if val, ok := settingsMap["authOpenIDAutoRegister"].(bool); ok {
			s.AutoRegister = val
		}
	}
	if settingsMap["authOpenIDMatchExistingBy"] != nil {
		s.MatchExistingBy = getString(settingsMap["authOpenIDMatchExistingBy"])
	}
	if settingsMap["authOpenIDMobileRedirectURIs"] != nil {
		if list, ok := settingsMap["authOpenIDMobileRedirectURIs"].([]interface{}); ok {
			for _, item := range list {
				s.MobileRedirectURIs = append(s.MobileRedirectURIs, getString(item))
			}
		}
	}
	if settingsMap["authOpenIDGroupClaim"] != nil {
		s.GroupClaim = getString(settingsMap["authOpenIDGroupClaim"])
	}
	if settingsMap["authOpenIDAdvancedPermsClaim"] != nil {
		s.AdvancedPermsClaim = getString(settingsMap["authOpenIDAdvancedPermsClaim"])
	}
	if settingsMap["authOpenIDSubfolderForRedirectURLs"] != nil {
		if val, ok := settingsMap["authOpenIDSubfolderForRedirectURLs"].(bool); ok {
			s.SubfolderForRedirectURLs = val
		}
	}

	return s, nil
}

func getString(val interface{}) string {
	if val == nil {
		return ""
	}
	if s, ok := val.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", val)
}

func handleOIDCCallback(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		globalOIDCHandlerMu.RLock()
		handler := globalOIDCHandler
		globalOIDCHandlerMu.RUnlock()

		if handler == nil {
			http.Error(w, "No active OIDC session", http.StatusBadRequest)
			return
		}

		claims, err := handler.HandleCallback(w, r)
		if err != nil {
			log.Printf("[OIDC Callback] Error: %v", err)
			http.Error(w, fmt.Sprintf("Authentication failed: %v", err), http.StatusUnauthorized)
			return
		}

		if claims == nil {
			return
		}

		s, err := getOIDCSettings(db)
		if err != nil {
			http.Error(w, "Failed to load OIDC settings", http.StatusInternalServerError)
			return
		}

		u, err := findUserFromOpenIdUserInfo(r.Context(), db, claims, s.MatchExistingBy)
		if err != nil {
			log.Printf("[OIDC Callback] User match error: %v", err)
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}

		if u == nil {
			if !s.AutoRegister {
				http.Error(w, "Auto-registration is disabled and no matching user was found", http.StatusUnauthorized)
				return
			}

			u, err = createUserFromOpenIdUserInfo(r.Context(), db, claims, getTokenSecret(db))
			if err != nil {
				log.Printf("[OIDC Callback] User registration failed: %v", err)
				http.Error(w, "Failed to register user", http.StatusInternalServerError)
				return
			}
		}

		var cbURL string
		if cookie, err := r.Cookie("auth_cb"); err == nil {
			cbURL = cookie.Value
		}

		tokenString := u.Token

		if cbURL != "" {
			redirectURL := cbURL + "?setToken=" + url.QueryEscape(tokenString) + "&accessToken=" + url.QueryEscape(tokenString)
			if stateCookie, err := r.Cookie("auth_state"); err == nil {
				redirectURL += "&state=" + url.QueryEscape(stateCookie.Value)
			}
			http.Redirect(w, r, redirectURL, http.StatusFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"user": map[string]interface{}{
				"id":          u.ID,
				"username":    u.Username,
				"email":       u.Email,
				"type":        u.Type,
				"token":       tokenString,
				"isActive":    u.IsActive,
				"isLocked":    u.IsLocked,
				"permissions": json.RawMessage(u.Permissions),
			},
		})
	}
}
