package handlers

import (
	"database/sql"
	"io/fs"
	"net/http"
	"strings"
	"sync"

	"audiobookshelf/internal/core"
	iscanner "audiobookshelf/internal/scanner"
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

var (
	ActiveHandler *SwappableHandler
	globalCfg     *core.Config
	globalAppRoot string
	globalVersion string
)

type SwappableHandler struct {
	mu      sync.RWMutex
	handler http.Handler
}

func (s *SwappableHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	h := s.handler
	s.mu.RUnlock()
	h.ServeHTTP(w, r)
}

func (s *SwappableHandler) Swap(h http.Handler) {
	s.mu.Lock()
	s.handler = h
	s.mu.Unlock()
}

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
	SetGlobalDB(db)
	globalCfg = cfg
	globalAppRoot = appRoot
	globalVersion = version

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
	finalHandler := MetricsMiddleware(handlerWithLogging)

	if ActiveHandler == nil {
		ActiveHandler = &SwappableHandler{handler: finalHandler}
	} else {
		ActiveHandler.Swap(finalHandler)
	}
	return ActiveHandler
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
	mux.Handle(joinPath(cfg.RouterBasePath, "/metrics"), AuthMiddlewareWrapper(db, handleMetrics(db)))
}
