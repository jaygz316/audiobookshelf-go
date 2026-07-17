package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"net/http"
	"strings"

	"audiobookshelf/internal/core"
	iscanner "audiobookshelf/internal/scanner"
	isocket "audiobookshelf/internal/socket"
)

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
	case "search":
		if r.Method == http.MethodGet {
			AuthMiddlewareWrapper(db, http.HandlerFunc(HandleSearchLibrary(db, libraryID))).ServeHTTP(w, r)
		} else {
			http.Error(w, `{"error": "Method Not Allowed"}`, http.StatusMethodNotAllowed)
		}
		return true
	case "items":
		if r.Method == http.MethodGet {
			AuthMiddlewareWrapper(db, http.HandlerFunc(HandleGetLibraryItems(db, libraryID))).ServeHTTP(w, r)
		} else if r.Method == http.MethodPost {
			AuthMiddlewareWrapper(db, handleUpload(db)).ServeHTTP(w, r)
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
