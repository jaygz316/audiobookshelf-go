package feed

import (
	"net/http"
	"os"
	"strings"

	"audiobookshelf/internal/utils"
)

// Media file serving (handles HTTP range requests automatically via ServeFile)
func (m *FeedManager) serveFeedItem(w http.ResponseWriter, r *http.Request, itemID string, entityType string) {
	ctx := r.Context()
	reqPath := r.URL.Path

	itemIdx := strings.Index(reqPath, "/item/")
	if itemIdx == -1 {
		http.NotFound(w, r)
		return
	}
	sub := reqPath[itemIdx+len("/item/"):]
	parts := strings.Split(sub, "/")
	if len(parts) == 0 {
		http.NotFound(w, r)
		return
	}
	episodeID := parts[0]

	var filePath string
	var mimeType string
	var err error

	switch entityType {
	case "playlist":
		filePath, mimeType, err = m.getPlaylistItemPath(ctx, itemID, episodeID)
	case "collection":
		filePath, mimeType, err = m.getCollectionItemPath(ctx, itemID, episodeID)
	case "series":
		filePath, mimeType, err = m.getSeriesItemPath(ctx, itemID, episodeID)
	default:
		filePath, mimeType, err = m.getLibraryItemPath(ctx, itemID, episodeID, entityType)
	}

	if err != nil || filePath == "" {
		http.NotFound(w, r)
		return
	}

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		http.NotFound(w, r)
		return
	}

	if mimeType != "" {
		w.Header().Set("Content-Type", mimeType)
	}

	if !utils.IsSafeFilePath(m.db, m.metadataPath, filePath) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	http.ServeFile(w, r, filePath)
}
