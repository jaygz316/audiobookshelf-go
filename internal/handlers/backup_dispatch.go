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
)

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

						if GetGlobalDB() != nil {
							GetGlobalDB().Close()
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
