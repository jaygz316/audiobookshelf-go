package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"sync"
	"syscall"

	"audiobookshelf/internal/auth"
	"audiobookshelf/internal/feed"
	"audiobookshelf/internal/finders"
	"audiobookshelf/internal/playlist"
	"audiobookshelf/internal/providers"
	"audiobookshelf/internal/share"

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

var managersMu sync.Mutex

func initManagers(db *sql.DB) {
	managersMu.Lock()
	defer managersMu.Unlock()

	if db == nil {
		log.Println("[Warning] initManagers: database connection is nil. Deferring initialization.")
		return
	}

	if globalFinder == nil {
		globalFinder = finders.NewFinder([]providers.Provider{
			&providers.AudibleProvider{},
			&providers.AudnexusProvider{},
			&providers.GoogleBooksProvider{},
			&providers.ITunesProvider{},
			&providers.OpenLibraryProvider{},
		})
	}

	if globalShareManager == nil {
		globalShareManager = share.NewShareManager(db)
	}
	if globalPlaylistManager == nil {
		globalPlaylistManager = playlist.NewPlaylistManager(db)
	}
	if globalFeedManager == nil {
		globalFeedManager = feed.NewFeedManager(db)
	}
}

func reinitManagers(db *sql.DB) {
	managersMu.Lock()
	defer managersMu.Unlock()

	if db == nil {
		log.Println("[Warning] reinitManagers: database connection is nil.")
		return
	}

	log.Println("[Info] reinitManagers: updating database connection for managers.")
	globalShareManager = share.NewShareManager(db)
	globalPlaylistManager = playlist.NewPlaylistManager(db)
	globalFeedManager = feed.NewFeedManager(db)
}

func getTokenSecret(db *sql.DB) string {
	if envSecret := os.Getenv("JWT_SECRET_KEY"); envSecret != "" {
		return envSecret
	}
	if cachedSecret != "" {
		return cachedSecret
	}
	if db == nil {
		return ""
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
	log.SetOutput(&LogWriter{Stdout: os.Stdout})
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
	routerBasePath = path.Clean("/" + routerBasePath)

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
