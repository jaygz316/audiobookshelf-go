package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"audiobookshelf/internal/utils"
)

// handleServeEbook serves the EPUB/PDF ebook file
func handleServeEbook(db *sql.DB, itemID string, fileID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Go] GET /api/items/%s/ebook (fileID=%s)", itemID, fileID)

		var mediaID, mediaType string
		err := db.QueryRow("SELECT mediaId, mediaType FROM libraryItems WHERE id = ?", itemID).Scan(&mediaID, &mediaType)
		if err != nil || mediaType != "book" {
			http.NotFound(w, r)
			return
		}

		var ebookFileBytes []byte
		err = db.QueryRow("SELECT ebookFile FROM books WHERE id = ?", mediaID).Scan(&ebookFileBytes)
		if err != nil || len(ebookFileBytes) == 0 {
			http.NotFound(w, r)
			return
		}

		var ebook struct {
			EbookFormat string `json:"ebookFormat"`
			Metadata    struct {
				Path string `json:"path"`
			} `json:"metadata"`
		}
		if err := json.Unmarshal(ebookFileBytes, &ebook); err != nil {
			http.Error(w, "invalid ebook metadata", http.StatusInternalServerError)
			return
		}

		filePath := ebook.Metadata.Path
		if _, err := os.Stat(filePath); err != nil {
			log.Warnf("[Go] Ebook file not found: %s", filePath)
			http.NotFound(w, r)
			return
		}

		if !utils.IsSafeFilePath(db, MetadataPath, filePath) {
			log.Warnf("[Go] Ebook file path traversal blocked: %s", filePath)
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		ext := strings.ToLower(filepath.Ext(filePath))
		switch ext {
		case ".epub":
			w.Header().Set("Content-Type", "application/epub+zip")
		case ".pdf":
			w.Header().Set("Content-Type", "application/pdf")
		case ".mobi":
			w.Header().Set("Content-Type", "application/x-mobipocket-ebook")
		case ".cbz":
			w.Header().Set("Content-Type", "application/x-cbz")
		case ".cbr":
			w.Header().Set("Content-Type", "application/x-cbr")
		}

		http.ServeFile(w, r, filePath)
	}
}
