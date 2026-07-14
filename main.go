package main

import (
	"database/sql"
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"syscall"

	ibackup "audiobookshelf/internal/backup"
	"audiobookshelf/internal/core"
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

func getTokenSecret(database *sql.DB) string {
	if envSecret := os.Getenv("JWT_SECRET_KEY"); envSecret != "" {
		return envSecret
	}
	if cachedSecret != "" {
		return cachedSecret
	}
	if database == nil {
		return ""
	}
	settings, err := db.GetServerSettings(database)
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
	db, err := db.InitDB(dbPath)
	var dbConnected bool
	if err != nil {
		log.Printf("Warning: Failed to connect to SQLite database: %v. Node.js server might initialize it.", err)
	} else {
		defer db.Close()
		log.Printf("Successfully connected to SQLite database: %s", dbPath)
		dbConnected = true
		globalDB = db
		ibackup.InitScheduler(db, cfg.ConfigPath, cfg.MetadataPath)
	}

	handler := handlers.SetupHandler(db, cfg, dbConnected, appRoot, version)

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

func parseConfig() *core.Config {
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

	routerBasePath, exists := os.LookupEnv("ROUTER_BASE_PATH")
	if !exists {
		routerBasePath = "/audiobookshelf"
	}
	routerBasePath = path.Clean("/" + routerBasePath)

	return &core.Config{
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
