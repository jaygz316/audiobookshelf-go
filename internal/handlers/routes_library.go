package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"net/http"

	"audiobookshelf/internal/core"
)

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
	mux.HandleFunc(joinPath(cfg.RouterBasePath, "/api/podcasts/opml/export"), func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			AuthMiddlewareWrapper(db, handleExportOPML(db)).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
	})

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
