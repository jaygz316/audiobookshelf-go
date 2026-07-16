package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"flag"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"audiobookshelf/internal/core"
	ihls "audiobookshelf/internal/hls"
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
				AuthMiddlewareWrapper(db, http.HandlerFunc(handleGetAuthorImage(db, cfg.MetadataPath, authorID))).ServeHTTP(w, r)
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
