package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"audiobookshelf/internal/core"
	"audiobookshelf/internal/providers"
	"audiobookshelf/internal/utils"
)

// handleMatchAuthor handles POST /api/authors/{id}/match
func handleMatchAuthor(db *sql.DB, cfg *core.Config, authorID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Go] POST /api/authors/%s/match", authorID)

		userVal := r.Context().Value(core.UserContextKey)
		if userVal == nil {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}
		user := userVal.(*core.UserSession)
		if user.Type != "root" && user.Type != "admin" {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		var payload struct {
			ASIN     string `json:"asin"`
			Provider string `json:"provider"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
			return
		}

		if payload.ASIN == "" {
			http.Error(w, `{"error": "asin parameter is required"}`, http.StatusBadRequest)
			return
		}

		initManagers(db)
		prov, ok := globalFinder.Providers()["audnexus"]
		if !ok {
			http.Error(w, `{"error": "audnexus provider not registered"}`, http.StatusInternalServerError)
			return
		}
		audnexus, ok := prov.(*providers.AudnexusProvider)
		if !ok {
			http.Error(w, `{"error": "failed to cast to AudnexusProvider"}`, http.StatusInternalServerError)
			return
		}

		details, err := audnexus.AuthorRequest(r.Context(), payload.ASIN, "")
		if err != nil {
			log.Errorf("[MatchAuthor] AuthorRequest failed: %v", err)
			http.Error(w, fmt.Sprintf("Failed to fetch author details from provider: %v", err), http.StatusInternalServerError)
			return
		}
		if details == nil {
			http.Error(w, `{"error": "Author details not found on provider"}`, http.StatusNotFound)
			return
		}

		// Download author image if present
		localImagePath := ""
		if details.Image != "" {
			// Ensure metadata/authors folder exists
			authorsDir := filepath.Join(cfg.MetadataPath, "authors")
			if err := os.MkdirAll(authorsDir, 0755); err == nil {
				imgBytes, downloadErr := providers.DownloadURL(r.Context(), details.Image)
				if downloadErr == nil {
					destFile := filepath.Join(authorsDir, authorID+".jpg")
					if !utils.IsSafeFilePath(db, cfg.MetadataPath, destFile) {
						log.Warnf("[MatchAuthor] Blocked unsafe author image path traversal: %s", destFile)
						http.Error(w, "Forbidden", http.StatusForbidden)
						return
					}
					if writeErr := os.WriteFile(destFile, imgBytes, 0644); writeErr == nil {
						localImagePath = "authors/" + authorID + ".jpg"
					} else {
						log.Warnf("[MatchAuthor] Warning: failed to write author image file: %v", writeErr)
					}
				} else {
					log.Warnf("[MatchAuthor] Warning: failed to download author image: %v", downloadErr)
				}
			} else {
				log.Warnf("[MatchAuthor] Warning: failed to create authors metadata dir: %v", err)
			}
		}

		err = updateAuthorMatchInDB(db, authorID, details.Name, details.ASIN, details.Description, localImagePath)
		if err != nil {
			http.Error(w, "Failed to update database: "+err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"updated": true}`))
	}
}
