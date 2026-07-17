package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"net/http"
	"strings"

	"audiobookshelf/internal/core"
	ihls "audiobookshelf/internal/hls"
)

func HandleItemsDispatch(db *sql.DB, cfg *core.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if r.Method == http.MethodGet && strings.HasSuffix(path, "/cover") {
			AuthMiddlewareWrapper(db, serveCover(db, cfg.MetadataPath)).ServeHTTP(w, r)
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
			} else if r.Method == http.MethodDelete {
				AuthMiddlewareWrapper(db, http.HandlerFunc(handleDeleteLibraryItemByID(db, itemID))).ServeHTTP(w, r)
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
		} else if len(parts) == 2 && parts[1] == "cover" {
			itemID := parts[0]
			if r.Method == http.MethodPost {
				AuthMiddlewareWrapper(db, http.HandlerFunc(handleUploadCover(db, cfg, itemID))).ServeHTTP(w, r)
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
