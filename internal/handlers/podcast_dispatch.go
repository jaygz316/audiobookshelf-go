package handlers

import (
	"database/sql"
	"net/http"
	"strings"

	"audiobookshelf/internal/core"
)

func handlePodcastsDispatch(db *sql.DB, cfg *core.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		initManagers(db)
		subPath := strings.TrimPrefix(r.URL.Path, joinPath(cfg.RouterBasePath, "/api/podcasts/"))
		parts := strings.Split(subPath, "/")

		if len(parts) >= 1 && parts[0] == "feed" {
			if r.Method == http.MethodPost {
				AuthMiddlewareWrapper(db, handleGetPodcastFeed(db)).ServeHTTP(w, r)
				return
			}
		}
		if len(parts) >= 2 && parts[0] == "opml" && parts[1] == "parse" {
			if r.Method == http.MethodPost {
				AuthMiddlewareWrapper(db, handleParseOPML(db)).ServeHTTP(w, r)
				return
			}
		}
		if len(parts) >= 2 && parts[0] == "opml" && parts[1] == "export" {
			if r.Method == http.MethodGet {
				AuthMiddlewareWrapper(db, handleExportOPML(db)).ServeHTTP(w, r)
				return
			}
		}
		if len(parts) >= 2 && parts[0] == "opml" && parts[1] == "create" {
			if r.Method == http.MethodPost {
				AuthMiddlewareWrapper(db, handleBulkCreatePodcasts(db)).ServeHTTP(w, r)
				return
			}
		}

		if len(parts) >= 3 && parts[1] == "episodes" && parts[2] == "progress" {
			id := parts[0]
			if r.Method == http.MethodPost || r.Method == http.MethodPatch {
				AuthMiddlewareWrapper(db, handleBulkUpdateEpisodesProgress(db, id)).ServeHTTP(w, r)
				return
			}
		}
		if len(parts) >= 3 && parts[1] == "episodes" && parts[2] == "delete" {
			id := parts[0]
			if r.Method == http.MethodPost || r.Method == http.MethodDelete {
				AuthMiddlewareWrapper(db, handleDeleteEpisodes(db, id)).ServeHTTP(w, r)
				return
			}
		}

		if len(parts) >= 2 {
			id := parts[0]
			action := parts[1]

			switch action {
			case "checknew":
				if r.Method == http.MethodGet {
					AuthMiddlewareWrapper(db, handleCheckNewEpisodes(db, id)).ServeHTTP(w, r)
					return
				}
			case "clear-queue":
				if r.Method == http.MethodGet {
					AuthMiddlewareWrapper(db, handleClearEpisodeQueue(db, id)).ServeHTTP(w, r)
					return
				}
			case "downloads":
				if r.Method == http.MethodGet {
					AuthMiddlewareWrapper(db, handleGetEpisodeDownloads(db, id)).ServeHTTP(w, r)
					return
				}
			case "search-episode":
				if r.Method == http.MethodGet {
					AuthMiddlewareWrapper(db, handleSearchEpisode(db, id)).ServeHTTP(w, r)
					return
				}
			case "download-episodes":
				if r.Method == http.MethodPost {
					AuthMiddlewareWrapper(db, handleDownloadEpisodes(db, id)).ServeHTTP(w, r)
					return
				}
			case "delete-episodes":
				if r.Method == http.MethodPost || r.Method == http.MethodDelete {
					AuthMiddlewareWrapper(db, handleDeleteEpisodes(db, id)).ServeHTTP(w, r)
					return
				}
			case "progress-episodes":
				if r.Method == http.MethodPost || r.Method == http.MethodPatch {
					AuthMiddlewareWrapper(db, handleBulkUpdateEpisodesProgress(db, id)).ServeHTTP(w, r)
					return
				}
			case "match-episodes":
				if r.Method == http.MethodPost {
					AuthMiddlewareWrapper(db, handleMatchEpisodes(db, id)).ServeHTTP(w, r)
					return
				}
			case "episode":
				if len(parts) == 3 {
					episodeId := parts[2]
					if r.Method == http.MethodGet {
						AuthMiddlewareWrapper(db, handleGetEpisode(db, id, episodeId)).ServeHTTP(w, r)
						return
					} else if r.Method == http.MethodPatch {
						AuthMiddlewareWrapper(db, handleUpdateEpisode(db, id, episodeId)).ServeHTTP(w, r)
						return
					} else if r.Method == http.MethodDelete {
						AuthMiddlewareWrapper(db, handleDeleteEpisode(db, id, episodeId)).ServeHTTP(w, r)
						return
					}
				}
			}
		}

		http.NotFound(w, r)
	}
}
