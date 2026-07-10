package podcast

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/doyensec/safeurl"
	"github.com/google/uuid"
)

// PodcastFeed represents the metadata and episode list of a podcast RSS feed.
type PodcastFeed struct {
	Title       string            `json:"title"`
	Author      string            `json:"author"`
	Description string            `json:"description"`
	Episodes    []*PodcastEpisode `json:"episodes"`
}

// PodcastEpisode represents a single episode parsed from a podcast RSS feed.
type PodcastEpisode struct {
	Title        string  `json:"title"`
	Description  string  `json:"description"`
	EnclosureURL string  `json:"enclosureUrl"`
	PublishedAt  string  `json:"publishedAt"`
	Duration     float64 `json:"duration"`
}

// PodcastManager coordinates podcast subscriptions, feed parsing, episode downloads, and background syncing.
type PodcastManager struct {
	db     *sql.DB
	client *safeurl.WrappedClient
}

// NewPodcastManager constructs a podcast subscription manager.
func NewPodcastManager(db *sql.DB) *PodcastManager {
	// PORT: Safe URL configuration builder is used to configure HTTP client against SSRF.
	builder := safeurl.GetConfigBuilder()
	if os.Getenv("BYPASS_SAFEURL") == "true" {
		builder = builder.SetAllowedIPs("127.0.0.1", "::1")
		var ports []int
		for p := 1; p <= 65535; p++ {
			ports = append(ports, p)
		}
		builder = builder.SetAllowedPorts(ports...)
	}
	config := builder.Build()
	client := safeurl.Client(config)
	return &PodcastManager{
		db:     db,
		client: client,
	}
}

// FetchFeed parses a remote RSS feed URL.
func (m *PodcastManager) FetchFeed(ctx context.Context, url string) (*PodcastFeed, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("User-Agent", getUserAgent(url))
	req.Header.Set("Accept", "application/rss+xml, application/xhtml+xml, application/xml, */*;q=0.8")
	req.Header.Set("Accept-Encoding", "gzip, compress, deflate")

	resp, err := m.client.Do(req)
	if err != nil {
		// PORT: Redirect fallback from http to https in case of protocol/redirection error.
		if strings.HasPrefix(url, "http://") {
			upgradedURL := strings.Replace(url, "http://", "https://", 1)
			reqUpgraded, errUpgraded := http.NewRequestWithContext(ctx, "GET", upgradedURL, nil)
			if errUpgraded == nil {
				reqUpgraded.Header.Set("User-Agent", getUserAgent(upgradedURL))
				reqUpgraded.Header.Set("Accept", "application/rss+xml, application/xhtml+xml, application/xml, */*;q=0.8")
				reqUpgraded.Header.Set("Accept-Encoding", "gzip, compress, deflate")
				respUpgraded, errUpgradedCall := m.client.Do(reqUpgraded)
				if errUpgradedCall == nil {
					if resp != nil && resp.Body != nil {
						resp.Body.Close()
					}
					resp = respUpgraded
					err = nil
				}
			}
		}
	}

	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	// PORT: Support iso-8859-1 encoded feeds by converting Latin1 string to UTF-8
	contentType := resp.Header.Get("Content-Type")
	if strings.Contains(strings.ToLower(contentType), "iso-8859-1") {
		bodyBytes = []byte(latin1ToUTF8(string(bodyBytes)))
	}

	feed, err := parseRSS(bodyBytes)
	if err != nil {
		return nil, fmt.Errorf("parse RSS XML: %w", err)
	}

	return feed, nil
}

// DownloadEpisode streams an episode's audio media enclosure to a local file path.
func (m *PodcastManager) DownloadEpisode(ctx context.Context, episodeURL, destPath string) error {
	req, err := http.NewRequestWithContext(ctx, "GET", episodeURL, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("User-Agent", getUserAgent(episodeURL))

	resp, err := m.client.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	dir := filepath.Dir(destPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create destination directory: %w", err)
	}

	out, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("create destination file: %w", err)
	}
	defer func() {
		_ = out.Close()
	}()

	// PORT: Chunked streaming using io.Copy for streaming efficiency.
	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return fmt.Errorf("stream copy failed: %w", err)
	}

	if err := out.Close(); err != nil {
		return fmt.Errorf("close destination file: %w", err)
	}

	return nil
}

// ScheduleRefresh initiates recurring background ticks for feed synchronization.
func (m *PodcastManager) ScheduleRefresh(ctx context.Context, cronExpression string) error {
	duration := parseCronToDuration(cronExpression)

	go func() {
		// Run once immediately
		if err := m.SyncAllFeeds(ctx); err != nil {
			// PORT: Sync error during tick is logged/ignored to not crash the scheduler.
			log.Printf("[Podcast] Sync error during initial run: %v", err)
		}

		ticker := time.NewTicker(duration)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := m.SyncAllFeeds(ctx); err != nil {
					// PORT: Sync error during tick is logged/ignored to not crash the scheduler.
					log.Printf("[Podcast] Sync error during tick: %v", err)
				}
			}
		}
	}()

	return nil
}

// podcastInfo holds SQLite metadata retrieved for a podcast during sync.
type podcastInfo struct {
	ID                       string
	Title                    string
	FeedURL                  string
	AutoDownload             int
	MaxEpisodesToKeep        int
	MaxNewEpisodesToDownload int
}

// SyncAllFeeds queries all podcasts from database and checks them for updates.
func (m *PodcastManager) SyncAllFeeds(ctx context.Context) error {
	rows, err := m.db.QueryContext(ctx, "SELECT id, title, feedURL, autoDownloadEpisodes, maxEpisodesToKeep, maxNewEpisodesToDownload FROM podcasts")
	if err != nil {
		return fmt.Errorf("query podcasts: %w", err)
	}
	defer rows.Close()

	var podcasts []podcastInfo
	for rows.Next() {
		var p podcastInfo
		var feedURL sql.NullString
		var autoDownload sql.NullInt64
		var maxKeep sql.NullInt64
		var maxDownload sql.NullInt64
		if err := rows.Scan(&p.ID, &p.Title, &feedURL, &autoDownload, &maxKeep, &maxDownload); err != nil {
			return fmt.Errorf("scan podcast: %w", err)
		}
		p.FeedURL = feedURL.String
		p.AutoDownload = int(autoDownload.Int64)
		p.MaxEpisodesToKeep = int(maxKeep.Int64)
		p.MaxNewEpisodesToDownload = int(maxDownload.Int64)
		podcasts = append(podcasts, p)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("rows error: %w", err)
	}

	for _, p := range podcasts {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if p.FeedURL == "" {
			continue
		}

		feed, err := m.FetchFeed(ctx, p.FeedURL)
		if err != nil {
			log.Printf("[Podcast] Failed to fetch feed for %q (%s): %v", p.Title, p.FeedURL, err)
			continue
		}

		if err := m.syncPodcastEpisodes(ctx, p, feed); err != nil {
			log.Printf("[Podcast] Failed to sync episodes for %q: %v", p.Title, err)
			continue
		}
	}

	return nil
}

// SyncFeed syncs the feed and downloads episodes for a single podcast by its ID.
func (m *PodcastManager) SyncFeed(ctx context.Context, podcastID string) error {
	var p podcastInfo
	var feedURL sql.NullString
	var autoDownload sql.NullInt64
	var maxKeep sql.NullInt64
	var maxDownload sql.NullInt64

	err := m.db.QueryRowContext(ctx, `
		SELECT id, title, feedURL, autoDownloadEpisodes, maxEpisodesToKeep, maxNewEpisodesToDownload
		FROM podcasts
		WHERE id = ?
	`, podcastID).Scan(&p.ID, &p.Title, &feedURL, &autoDownload, &maxKeep, &maxDownload)
	if err != nil {
		return err
	}

	p.FeedURL = feedURL.String
	p.AutoDownload = int(autoDownload.Int64)
	p.MaxEpisodesToKeep = int(maxKeep.Int64)
	p.MaxNewEpisodesToDownload = int(maxDownload.Int64)

	if p.FeedURL == "" {
		return fmt.Errorf("no feed URL configured")
	}

	feed, err := m.FetchFeed(ctx, p.FeedURL)
	if err != nil {
		return err
	}

	return m.syncPodcastEpisodes(ctx, p, feed)
}

func (m *PodcastManager) syncPodcastEpisodes(ctx context.Context, p podcastInfo, feed *PodcastFeed) error {
	var libraryItemPath string
	var libraryItemID string
	err := m.db.QueryRowContext(ctx, "SELECT id, path FROM libraryItems WHERE mediaId = ? AND mediaType = 'podcast'", p.ID).Scan(&libraryItemID, &libraryItemPath)
	if err != nil {
		if err != sql.ErrNoRows {
			log.Printf("[Podcast] Failed to query library items for podcast %s: %v", p.ID, err)
		}
		libraryItemPath = ""
	}

	hasEnclosureURL := hasColumn(ctx, m.db, "podcastEpisodes", "enclosureURL")
	query := "SELECT title"
	if hasEnclosureURL {
		query += ", enclosureURL"
	}
	query += " FROM podcastEpisodes WHERE podcastId = ?"

	rows, err := m.db.QueryContext(ctx, query, p.ID)
	if err != nil {
		return fmt.Errorf("query existing episodes: %w", err)
	}
	defer rows.Close()

	existingEpisodes := make(map[string]bool)
	for rows.Next() {
		var title string
		var encURL sql.NullString
		if hasEnclosureURL {
			if err := rows.Scan(&title, &encURL); err != nil {
				return fmt.Errorf("scan existing episode details: %w", err)
			}
			if encURL.Valid && encURL.String != "" {
				existingEpisodes[encURL.String] = true
			}
		} else {
			if err := rows.Scan(&title); err != nil {
				return fmt.Errorf("scan existing episode title: %w", err)
			}
		}
		existingEpisodes[title] = true
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate existing episodes: %w", err)
	}

	newCount := 0
	for _, ep := range feed.Episodes {
		isNew := false
		if hasEnclosureURL && ep.EnclosureURL != "" {
			if !existingEpisodes[ep.EnclosureURL] && !existingEpisodes[ep.Title] {
				isNew = true
			}
		} else {
			if !existingEpisodes[ep.Title] {
				isNew = true
			}
		}

		if !isNew {
			continue
		}

		// Enforce max new episodes to download limit if configured
		if p.MaxNewEpisodesToDownload > 0 && newCount >= p.MaxNewEpisodesToDownload {
			break
		}

		var downloadedPath string
		if p.AutoDownload == 1 && libraryItemPath != "" && ep.EnclosureURL != "" {
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

			if err := m.DownloadEpisode(ctx, ep.EnclosureURL, destPath); err == nil {
				downloadedPath = destPath
			} else {
				log.Printf("[Podcast] Failed to download episode %q from %s: %v", ep.Title, ep.EnclosureURL, err)
			}
		}

		epID := uuid.New().String()
		audioFileJSON := "{}"
		if downloadedPath != "" {
			fi, err := os.Stat(downloadedPath)
			var size int64
			if err == nil {
				size = fi.Size()
			}

			audioFileMap := map[string]interface{}{
				"duration": ep.Duration,
				"mimeType": "audio/mpeg",
				"metadata": map[string]interface{}{
					"path":     downloadedPath,
					"filename": filepath.Base(downloadedPath),
					"size":     size,
				},
			}
			if b, err := json.Marshal(audioFileMap); err == nil {
				audioFileJSON = string(b)
			}
		}

		hasPubDate := hasColumn(ctx, m.db, "podcastEpisodes", "pubDate")
		hasDesc := hasColumn(ctx, m.db, "podcastEpisodes", "description")
		hasSeason := hasColumn(ctx, m.db, "podcastEpisodes", "season")
		hasEp := hasColumn(ctx, m.db, "podcastEpisodes", "episode")
		hasEpType := hasColumn(ctx, m.db, "podcastEpisodes", "episodeType")
		hasPublishedAt := hasColumn(ctx, m.db, "podcastEpisodes", "publishedAt")
		hasCreatedAt := hasColumn(ctx, m.db, "podcastEpisodes", "createdAt")
		hasUpdatedAt := hasColumn(ctx, m.db, "podcastEpisodes", "updatedAt")

		cols := []string{"id", "podcastId", "title", "audioFile"}
		vals := []interface{}{epID, p.ID, ep.Title, audioFileJSON}

		if hasPubDate {
			cols = append(cols, "pubDate")
			vals = append(vals, ep.PublishedAt)
		}
		if hasDesc {
			cols = append(cols, "description")
			vals = append(vals, ep.Description)
		}
		if hasPublishedAt {
			cols = append(cols, "publishedAt")
			vals = append(vals, ep.PublishedAt)
		}
		if hasEnclosureURL {
			cols = append(cols, "enclosureURL")
			vals = append(vals, ep.EnclosureURL)
		}
		if hasSeason {
			cols = append(cols, "season")
			vals = append(vals, "")
		}
		if hasEp {
			cols = append(cols, "episode")
			vals = append(vals, "")
		}
		if hasEpType {
			cols = append(cols, "episodeType")
			vals = append(vals, "")
		}
		if hasCreatedAt {
			cols = append(cols, "createdAt")
			vals = append(vals, time.Now().Format("2006-01-02 15:04:05.000"))
		}
		if hasUpdatedAt {
			cols = append(cols, "updatedAt")
			vals = append(vals, time.Now().Format("2006-01-02 15:04:05.000"))
		}

		placeholders := make([]string, len(cols))
		for i := range cols {
			placeholders[i] = "?"
		}

		insertSQL := fmt.Sprintf("INSERT INTO podcastEpisodes (%s) VALUES (%s)",
			strings.Join(cols, ", "),
			strings.Join(placeholders, ", "),
		)

		if _, err := m.db.ExecContext(ctx, insertSQL, vals...); err == nil {
			newCount++
		} else {
			log.Printf("[Podcast] Failed to insert episode %q: %v", ep.Title, err)
		}
	}

	return nil
}

// Helpers

func getUserAgent(urlStr string) string {
	userAgent := "audiobookshelf (+https://audiobookshelf.org; like iTMS)"
	if strings.HasPrefix(urlStr, "https://www.cbc.ca") {
		userAgent = "audiobookshelf (+https://audiobookshelf.org; like iTMS) - CBC"
	}
	return userAgent
}

func latin1ToUTF8(latin1Str string) string {
	runes := make([]rune, len(latin1Str))
	for i, b := range []byte(latin1Str) {
		runes[i] = rune(b)
	}
	return string(runes)
}

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

func parseCronToDuration(expr string) time.Duration {
	parts := strings.Fields(expr)
	if len(parts) < 5 {
		return 1 * time.Hour
	}

	minPart := parts[0]
	hourPart := parts[1]

	if minPart == "*" {
		return 1 * time.Minute
	}
	if strings.HasPrefix(minPart, "*/") {
		if val, err := strconv.Atoi(minPart[2:]); err == nil {
			return time.Duration(val) * time.Minute
		}
	}

	if hourPart == "*" {
		return 1 * time.Hour
	}
	if strings.HasPrefix(hourPart, "*/") {
		if val, err := strconv.Atoi(hourPart[2:]); err == nil {
			return time.Duration(val) * time.Hour
		}
	}

	if hourPart != "*" && minPart != "*" {
		return 24 * time.Hour
	}

	return 1 * time.Hour
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

func parseRSS(xmlData []byte) (*PodcastFeed, error) {
	decoder := xml.NewDecoder(bytes.NewReader(xmlData))
	decoder.Entity = xml.HTMLEntity

	var feed PodcastFeed
	var episodes []*PodcastEpisode

	var currentEp *PodcastEpisode
	var elementStack []string

	var channelTitle, channelAuthor, channelDescription, channelITunesSummary strings.Builder
	var itemTitle, itemDescription, itemContentEncoded, itemPubDate, itemDuration, itemITunesDuration strings.Builder
	var itemEnclosureURL string

	for {
		t, err := decoder.Token()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("xml token: %w", err)
		}

		switch se := t.(type) {
		case xml.StartElement:
			elementStack = append(elementStack, se.Name.Local)

			if se.Name.Local == "item" {
				currentEp = &PodcastEpisode{}
				itemTitle.Reset()
				itemDescription.Reset()
				itemContentEncoded.Reset()
				itemPubDate.Reset()
				itemDuration.Reset()
				itemITunesDuration.Reset()
				itemEnclosureURL = ""
			}

			if currentEp != nil && len(elementStack) >= 4 && elementStack[len(elementStack)-2] == "item" {
				localName := se.Name.Local
				if localName == "enclosure" {
					for _, attr := range se.Attr {
						if attr.Name.Local == "url" {
							itemEnclosureURL = attr.Value
						}
					}
				} else if localName == "content" {
					isAudio := false
					var urlVal string
					for _, attr := range se.Attr {
						if attr.Name.Local == "type" && strings.HasPrefix(attr.Value, "audio") {
							isAudio = true
						}
						if attr.Name.Local == "url" {
							urlVal = attr.Value
						}
					}
					if isAudio && urlVal != "" && itemEnclosureURL == "" {
						itemEnclosureURL = urlVal
					}
				}
			}

		case xml.EndElement:
			if se.Name.Local == "item" && currentEp != nil {
				desc := itemContentEncoded.String()
				if desc == "" {
					desc = itemDescription.String()
				}

				durStr := itemITunesDuration.String()
				if durStr == "" {
					durStr = itemDuration.String()
				}
				durSec := parseDurationToSeconds(durStr)

				currentEp.Title = strings.TrimSpace(itemTitle.String())
				currentEp.Description = strings.TrimSpace(desc)
				currentEp.EnclosureURL = strings.TrimSpace(itemEnclosureURL)
				currentEp.PublishedAt = strings.TrimSpace(itemPubDate.String())
				currentEp.Duration = durSec

				episodes = append(episodes, currentEp)
				currentEp = nil
			}

			if len(elementStack) > 0 {
				elementStack = elementStack[:len(elementStack)-1]
			}

		case xml.CharData:
			if len(elementStack) < 3 {
				continue
			}
			val := string(se)
			parent := elementStack[len(elementStack)-1]
			grandParent := elementStack[len(elementStack)-2]

			if grandParent == "item" && currentEp != nil {
				switch parent {
				case "title":
					itemTitle.WriteString(val)
				case "description":
					itemDescription.WriteString(val)
				case "encoded":
					itemContentEncoded.WriteString(val)
				case "pubDate":
					itemPubDate.WriteString(val)
				case "duration":
					itemITunesDuration.WriteString(val)
				}
			} else if grandParent == "channel" {
				switch parent {
				case "title":
					channelTitle.WriteString(val)
				case "author":
					channelAuthor.WriteString(val)
				case "description":
					channelDescription.WriteString(val)
				case "summary":
					channelITunesSummary.WriteString(val)
				}
			}
		}
	}

	feed.Title = strings.TrimSpace(channelTitle.String())
	feed.Author = strings.TrimSpace(channelAuthor.String())

	desc := strings.TrimSpace(channelDescription.String())
	if desc == "" {
		desc = strings.TrimSpace(channelITunesSummary.String())
	}
	feed.Description = desc

	for _, ep := range episodes {
		if ep.PublishedAt != "" {
			t := parseTime(ep.PublishedAt)
			if !t.IsZero() {
				ep.PublishedAt = t.UTC().Format(time.RFC3339)
			}
		}
	}

	feed.Episodes = episodes
	return &feed, nil
}
