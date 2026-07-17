package podcast

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	log "audiobookshelf/internal/logger"
)

func parseDurationToSeconds(durationStr string) float64 {
	durationStr = strings.TrimSpace(durationStr)
	if durationStr == "" {
		return 0
	}
	if s, err := strconv.ParseFloat(durationStr, 64); err == nil {
		return s
	}
	parts := strings.Split(durationStr, ":")
	if len(parts) == 1 {
		s, _ := strconv.ParseFloat(parts[0], 64)
		return s
	} else if len(parts) == 2 {
		m, _ := strconv.ParseFloat(parts[0], 64)
		s, _ := strconv.ParseFloat(parts[1], 64)
		return m*60 + s
	} else if len(parts) == 3 {
		h, _ := strconv.ParseFloat(parts[0], 64)
		m, _ := strconv.ParseFloat(parts[1], 64)
		s, _ := strconv.ParseFloat(parts[2], 64)
		return h*3600 + m*60 + s
	}
	return 0
}

func parseTime(s string) time.Time {
	layouts := []string{
		time.RFC1123Z,
		time.RFC1123,
		time.RFC3339,
		"2006-01-02 15:04:05.000",
		"2006-01-02 15:04:05",
		"2006-01-02",
	}
	for _, l := range layouts {
		if t, err := time.Parse(l, s); err == nil {
			return t
		}
	}
	if ms, err := strconv.ParseInt(s, 10, 64); err == nil {
		return time.Unix(ms/1000, (ms%1000)*1000000)
	}
	return time.Time{}
}

func sanitizeFilename(name string) string {
	reg := regexp.MustCompile(`[\\/:*?"<>|]`)
	safe := reg.ReplaceAllString(name, "_")
	return strings.TrimSpace(safe)
}

func hasColumn(ctx context.Context, db *sql.DB, tableName, columnName string) bool {
	rows, err := db.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info(%s)", tableName))
	if err != nil {
		log.Printf("[Podcast] PRAGMA table_info query failed: %v", err)
		return false
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, dType string
		var notnull int
		var dfltVal sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &dType, &notnull, &dfltVal, &pk); err == nil {
			if strings.EqualFold(name, columnName) {
				return true
			}
		} else {
			log.Printf("[Podcast] Scan column in PRAGMA table_info failed: %v", err)
		}
	}
	if err := rows.Err(); err != nil {
		log.Printf("[Podcast] PRAGMA table_info rows iteration failed: %v", err)
	}
	return false
}

func (m *PodcastManager) autoDownloadEpisode(ctx context.Context, p podcastInfo, ep *PodcastEpisode, libraryItemPath string) string {
	if p.AutoDownload != 1 || libraryItemPath == "" || ep.EnclosureURL == "" {
		return ""
	}
	ext := ".mp3"
	if parsedExt := filepath.Ext(ep.EnclosureURL); parsedExt != "" {
		if idx := strings.Index(parsedExt, "?"); idx != -1 {
			parsedExt = parsedExt[:idx]
		}
		if len(parsedExt) <= 5 {
			ext = parsedExt
		}
	}

	destFilename := sanitizeFilename(ep.Title) + ext
	destPath := filepath.Join(libraryItemPath, destFilename)

	if _, err := os.Stat(destPath); err == nil {
		destFilename = sanitizeFilename(ep.Title) + "_" + uuid.New().String()[:8] + ext
		destPath = filepath.Join(libraryItemPath, destFilename)
	}

	err := m.DownloadEpisode(ctx, ep.EnclosureURL, destPath)
	if err == nil {
		return destPath
	}
	log.Printf("[Podcast] Failed to download episode %q from %s: %v", ep.Title, ep.EnclosureURL, err)
	return ""
}

func buildAudioFileJSON(downloadedPath string, duration float64) string {
	if downloadedPath == "" {
		return "{}"
	}
	fi, err := os.Stat(downloadedPath)
	var size int64
	if err == nil {
		size = fi.Size()
	}

	audioFileMap := map[string]interface{}{
		"duration": duration,
		"mimeType": "audio/mpeg",
		"metadata": map[string]interface{}{
			"path":     downloadedPath,
			"filename": filepath.Base(downloadedPath),
			"size":     size,
		},
	}
	if b, err := json.Marshal(audioFileMap); err == nil {
		return string(b)
	}
	return "{}"
}
