package main

import (
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	ibackup "audiobookshelf/internal/backup"
	"audiobookshelf/internal/db"
	"audiobookshelf/internal/handlers"
	"audiobookshelf/internal/logger"
	"audiobookshelf/internal/watcher"

	_ "modernc.org/sqlite"
)

//go:embed frontend
var frontendFS embed.FS

//go:embed docs
var docsFS embed.FS

var subFS fs.FS

var cachedSecret string
var globalDB *sql.DB

func init() {
	// Register font MIME types to ensure proper browser rendering of icons.
	_ = mime.AddExtensionType(".woff", "font/woff")
	_ = mime.AddExtensionType(".woff2", "font/woff2")
	_ = mime.AddExtensionType(".ttf", "font/ttf")
}

func main() {
	logFormat := os.Getenv("LOG_FORMAT")
	if logFormat == "" {
		logFormat = "json"
	}
	logLevel := os.Getenv("LOG_LEVEL")
	if logLevel == "" {
		logLevel = "info"
	}
	logger.InitLogger(logFormat, logLevel)

	cfg := parseConfig()

	var err error
	subFS, err = fs.Sub(frontendFS, "frontend")
	if err != nil {
		log.Fatalf("Failed to initialize embedded frontend filesystem: %v", err)
	}
	handlers.SetSubFS(subFS)

	subDocs, err := fs.Sub(docsFS, "docs")
	if err == nil {
		handlers.SetDocsFS(subDocs)
	} else {
		log.Printf("Warning: Failed to initialize embedded docs filesystem: %v", err)
	}

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
	var sqlDB *sql.DB
	for i := 1; i <= 30; i++ {
		sqlDB, err = db.InitDB(dbPath)
		if err == nil {
			break
		}
		log.Printf("Warning: Failed to connect to SQLite database (attempt %d/30): %v. Retrying in 2s...", i, err)
		time.Sleep(2 * time.Second)
	}

	var dbConnected bool
	if err != nil {
		log.Printf("Warning: Failed to connect to SQLite database: %v. Node.js server might initialize it.", err)
	} else {
		defer sqlDB.Close()
		log.Printf("Successfully connected to SQLite database: %s", dbPath)
		dbConnected = true
		globalDB = sqlDB
		ibackup.InitScheduler(sqlDB, cfg.ConfigPath, cfg.MetadataPath)
	}

	handler := handlers.SetupHandler(sqlDB, cfg, dbConnected, appRoot, version)

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
		handlers.ShutdownStreamManager()
		if watcher.GlobalWatcher != nil {
			if err := watcher.GlobalWatcher.Close(); err != nil {
				log.Printf("[Watcher] Error closing watcher: %v", err)
			}
		}
		if ibackup.GlobalScheduler != nil {
			ibackup.GlobalScheduler.Stop()
		}
		srv.Close()
	}()

	log.Printf("Go Gateway listening on http://%s", serverAddr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server ListenAndServe failed: %v", err)
	}
	log.Printf("Go Gateway stopped.")
}
