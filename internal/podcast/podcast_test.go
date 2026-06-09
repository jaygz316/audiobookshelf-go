package podcast

import (
	"context"
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/doyensec/safeurl"
	_ "modernc.org/sqlite"
)

var dbCounter int64

// setupTestDB creates an in-memory SQLite database.
// If hasExtraColumns is true, it defines all optional columns in podcastEpisodes.
// Otherwise, it defines only the minimal core columns.
func setupTestDB(t *testing.T, hasExtraColumns bool) *sql.DB {
	id := atomic.AddInt64(&dbCounter, 1)
	dsn := fmt.Sprintf("file:podcastmemdb%d?mode=memory&cache=shared", id)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	db.SetMaxIdleConns(2)

	schemas := []string{
		`CREATE TABLE podcasts (
			id TEXT PRIMARY KEY,
			title TEXT,
			feedURL TEXT,
			autoDownloadEpisodes INTEGER,
			maxEpisodesToKeep INTEGER,
			maxNewEpisodesToDownload INTEGER
		)`,
		`CREATE TABLE libraryItems (
			id TEXT PRIMARY KEY,
			path TEXT,
			mediaId TEXT,
			mediaType TEXT
		)`,
	}

	var episodeSchema string
	if hasExtraColumns {
		episodeSchema = `CREATE TABLE podcastEpisodes (
			id TEXT PRIMARY KEY,
			podcastId TEXT,
			title TEXT,
			audioFile TEXT,
			pubDate TEXT,
			description TEXT,
			season TEXT,
			episode TEXT,
			episodeType TEXT,
			publishedAt TEXT,
			enclosureURL TEXT
		)`
	} else {
		episodeSchema = `CREATE TABLE podcastEpisodes (
			id TEXT PRIMARY KEY,
			podcastId TEXT,
			title TEXT,
			audioFile TEXT
		)`
	}
	schemas = append(schemas, episodeSchema)

	for _, schema := range schemas {
		if _, err := db.Exec(schema); err != nil {
			db.Close()
			t.Fatalf("failed to create schema: %v", err)
		}
	}
	return db
}

// configureTestClient extracts ports from mock server URLs and configures
// the safeurl client to allow them, along with loopback addresses.
func configureTestClient(t *testing.T, m *PodcastManager, urls ...string) {
	var ports []int
	for _, uStr := range urls {
		u, err := url.Parse(uStr)
		if err != nil {
			t.Fatalf("failed to parse url %q: %v", uStr, err)
		}
		portStr := u.Port()
		if portStr == "" {
			if u.Scheme == "https" {
				ports = append(ports, 443)
			} else {
				ports = append(ports, 80)
			}
			continue
		}
		port, err := strconv.Atoi(portStr)
		if err != nil {
			t.Fatalf("invalid port in url %q: %v", uStr, err)
		}
		ports = append(ports, port)
	}

	tr := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}

	config := safeurl.GetConfigBuilder().
		SetAllowedIPs("127.0.0.1", "::1").
		SetAllowedPorts(ports...).
		SetTransport(tr).
		Build()
	m.client = safeurl.Client(config)
}

const feedUTF8 = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:itunes="http://www.itunes.com/dtds/podcast-1.0.dtd" xmlns:content="http://purl.org/rss/1.0/modules/content/">
<channel>
	<title>Test UTF-8 Feed</title>
	<author>Test Author</author>
	<description>A UTF-8 Description with nice chars like ñ and á</description>
	<item>
		<title>Episode 1</title>
		<description>Description of Episode 1</description>
		<content:encoded><![CDATA[<p>Content of Episode 1</p>]]></content:encoded>
		<pubDate>Mon, 08 Jun 2026 12:00:00 +0000</pubDate>
		<enclosure url="http://example.com/ep1.mp3" length="12345" type="audio/mpeg" />
		<itunes:duration>01:30:00</itunes:duration>
	</item>
	<item>
		<title>Episode 2</title>
		<description>Description of Episode 2</description>
		<pubDate>2026-06-08 12:00:00</pubDate>
		<enclosure url="http://example.com/ep2.mp3" length="54321" type="audio/mpeg" />
		<itunes:duration>900</itunes:duration>
	</item>
</channel>
</rss>`

// feedISOLatin1Bytes contains ISO-8859-1 encoded XML data.
// \xe9 is é, \xf1 is ñ, \xfc is ü in Latin-1.
var feedISOLatin1Bytes = []byte("<?xml version=\"1.0\" encoding=\"utf-8\"?>\n" +
	"<rss version=\"2.0\">\n" +
	"<channel>\n" +
	"\t<title>ISO Feed \xe9</title>\n" +
	"\t<author>Author \xf1</author>\n" +
	"\t<description>Description</description>\n" +
	"\t<item>\n" +
	"\t\t<title>Episode \xfc</title>\n" +
	"\t\t<description>Desc</description>\n" +
	"\t\t<enclosure url=\"http://example.com/ep.mp3\" length=\"123\" type=\"audio/mpeg\"/>\n" +
	"\t</item>\n" +
	"</channel>\n" +
	"</rss>")

func TestFetchFeed_UTF8(t *testing.T) {
	db := setupTestDB(t, false)
	defer db.Close()
	m := NewPodcastManager(db)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(feedUTF8))
	}))
	defer server.Close()

	configureTestClient(t, m, server.URL)

	ctx := context.Background()
	feed, err := m.FetchFeed(ctx, server.URL)
	if err != nil {
		t.Fatalf("FetchFeed failed: %v", err)
	}

	if feed.Title != "Test UTF-8 Feed" {
		t.Errorf("expected Title %q, got %q", "Test UTF-8 Feed", feed.Title)
	}
	if feed.Author != "Test Author" {
		t.Errorf("expected Author %q, got %q", "Test Author", feed.Author)
	}
	if feed.Description != "A UTF-8 Description with nice chars like ñ and á" {
		t.Errorf("expected Description %q, got %q", "A UTF-8 Description with nice chars like ñ and á", feed.Description)
	}

	if len(feed.Episodes) != 2 {
		t.Fatalf("expected 2 episodes, got %d", len(feed.Episodes))
	}

	ep1 := feed.Episodes[0]
	if ep1.Title != "Episode 1" {
		t.Errorf("expected ep1 title 'Episode 1', got %q", ep1.Title)
	}
	if ep1.Description != "<p>Content of Episode 1</p>" {
		t.Errorf("expected ep1 description (from content:encoded) '<p>Content of Episode 1</p>', got %q", ep1.Description)
	}
	if ep1.EnclosureURL != "http://example.com/ep1.mp3" {
		t.Errorf("expected ep1 enclosure URL, got %q", ep1.EnclosureURL)
	}
	if ep1.Duration != 5400.0 {
		t.Errorf("expected ep1 duration 5400 (from 01:30:00), got %f", ep1.Duration)
	}
	if ep1.PublishedAt == "" {
		t.Error("expected ep1 publishedAt to be parsed and formatted")
	}

	ep2 := feed.Episodes[1]
	if ep2.Duration != 900.0 {
		t.Errorf("expected ep2 duration 900.0, got %f", ep2.Duration)
	}
}

func TestFetchFeed_ISO88591(t *testing.T) {
	db := setupTestDB(t, false)
	defer db.Close()
	m := NewPodcastManager(db)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml; charset=iso-8859-1")
		w.WriteHeader(http.StatusOK)
		w.Write(feedISOLatin1Bytes)
	}))
	defer server.Close()

	configureTestClient(t, m, server.URL)

	ctx := context.Background()
	feed, err := m.FetchFeed(ctx, server.URL)
	if err != nil {
		t.Fatalf("FetchFeed failed: %v", err)
	}

	if feed.Title != "ISO Feed é" {
		t.Errorf("expected Title %q, got %q", "ISO Feed é", feed.Title)
	}
	if feed.Author != "Author ñ" {
		t.Errorf("expected Author %q, got %q", "Author ñ", feed.Author)
	}

	if len(feed.Episodes) != 1 {
		t.Fatalf("expected 1 episode, got %d", len(feed.Episodes))
	}

	ep := feed.Episodes[0]
	if ep.Title != "Episode ü" {
		t.Errorf("expected episode Title %q, got %q", "Episode ü", ep.Title)
	}
}

func TestFetchFeed_Fallback(t *testing.T) {
	db := setupTestDB(t, false)
	defer db.Close()
	m := NewPodcastManager(db)

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(feedUTF8))
	}))
	defer server.Close()

	httpURL := strings.Replace(server.URL, "https://", "http://", 1)

	var ports []int
	u, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("failed to parse url: %v", err)
	}
	port, _ := strconv.Atoi(u.Port())
	ports = append(ports, port)

	tr := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}

	// Allow only the "https" scheme.
	// This forces the "http://" request to fail immediately due to safeurl scheme validation,
	// triggering the HTTP-to-HTTPS redirect fallback block, which upgrades the request to "https://",
	// which is allowed and succeeds.
	config := safeurl.GetConfigBuilder().
		SetAllowedIPs("127.0.0.1", "::1").
		SetAllowedPorts(ports...).
		SetAllowedSchemes("https").
		SetTransport(tr).
		Build()
	m.client = safeurl.Client(config)

	ctx := context.Background()
	feed, err := m.FetchFeed(ctx, httpURL)
	if err != nil {
		t.Fatalf("FetchFeed fallback failed: %v", err)
	}

	if feed.Title != "Test UTF-8 Feed" {
		t.Errorf("expected title 'Test UTF-8 Feed', got %q", feed.Title)
	}
}

func TestDownloadEpisode(t *testing.T) {
	db := setupTestDB(t, false)
	defer db.Close()
	m := NewPodcastManager(db)

	audioContent := []byte("fake-mp3-stream-data")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/mpeg")
		w.WriteHeader(http.StatusOK)
		w.Write(audioContent)
	}))
	defer server.Close()

	configureTestClient(t, m, server.URL)

	tempDir := t.TempDir()
	destPath := filepath.Join(tempDir, "ep1.mp3")

	ctx := context.Background()
	err := m.DownloadEpisode(ctx, server.URL, destPath)
	if err != nil {
		t.Fatalf("DownloadEpisode failed: %v", err)
	}

	data, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("failed to read downloaded file: %v", err)
	}

	if string(data) != string(audioContent) {
		t.Errorf("downloaded content mismatch, expected %q, got %q", string(audioContent), string(data))
	}
}

func TestSyncAllFeeds_StandardSchema(t *testing.T) {
	db := setupTestDB(t, false)
	defer db.Close()
	m := NewPodcastManager(db)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(feedUTF8))
	}))
	defer server.Close()

	configureTestClient(t, m, server.URL)

	_, err := db.Exec(`
		INSERT INTO podcasts (id, title, feedURL, autoDownloadEpisodes, maxEpisodesToKeep, maxNewEpisodesToDownload)
		VALUES (?, ?, ?, ?, ?, ?)
	`, "pod-std", "Std Podcast", server.URL, 0, 0, 0)
	if err != nil {
		t.Fatalf("failed to insert podcast: %v", err)
	}

	ctx := context.Background()
	err = m.SyncAllFeeds(ctx)
	if err != nil {
		t.Fatalf("SyncAllFeeds failed: %v", err)
	}

	rows, err := db.Query("SELECT id, podcastId, title, audioFile FROM podcastEpisodes WHERE podcastId = ?", "pod-std")
	if err != nil {
		t.Fatalf("query podcastEpisodes failed: %v", err)
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var id, podId, title, audioFile string
		if err := rows.Scan(&id, &podId, &title, &audioFile); err != nil {
			t.Fatalf("scan podcast episode failed: %v", err)
		}
		count++
	}
	if count != 2 {
		t.Errorf("expected 2 episodes inserted, got %d", count)
	}
}

func TestSyncAllFeeds_FullSchema(t *testing.T) {
	db := setupTestDB(t, true)
	defer db.Close()
	m := NewPodcastManager(db)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(feedUTF8))
	}))
	defer server.Close()

	configureTestClient(t, m, server.URL)

	_, err := db.Exec(`
		INSERT INTO podcasts (id, title, feedURL, autoDownloadEpisodes, maxEpisodesToKeep, maxNewEpisodesToDownload)
		VALUES (?, ?, ?, ?, ?, ?)
	`, "pod-full", "Full Podcast", server.URL, 0, 0, 0)
	if err != nil {
		t.Fatalf("failed to insert podcast: %v", err)
	}

	ctx := context.Background()
	err = m.SyncAllFeeds(ctx)
	if err != nil {
		t.Fatalf("SyncAllFeeds failed: %v", err)
	}

	rows, err := db.Query("SELECT id, podcastId, title, pubDate, description, enclosureURL FROM podcastEpisodes WHERE podcastId = ?", "pod-full")
	if err != nil {
		t.Fatalf("query podcastEpisodes failed: %v", err)
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var id, podId, title, pubDate, description, encURL string
		if err := rows.Scan(&id, &podId, &title, &pubDate, &description, &encURL); err != nil {
			t.Fatalf("scan podcast episode failed: %v", err)
		}
		count++
		if title == "Episode 1" {
			if !strings.Contains(pubDate, "2026-06-08T12:00:00") {
				t.Errorf("expected parsed pubDate, got %q", pubDate)
			}
			if description != "<p>Content of Episode 1</p>" {
				t.Errorf("expected description, got %q", description)
			}
			if encURL != "http://example.com/ep1.mp3" {
				t.Errorf("expected enclosureURL, got %q", encURL)
			}
		}
	}
	if count != 2 {
		t.Errorf("expected 2 episodes inserted, got %d", count)
	}
}

func TestSyncAllFeeds_AutoDownload(t *testing.T) {
	db := setupTestDB(t, true)
	defer db.Close()
	m := NewPodcastManager(db)

	audioContent := []byte("audio-file-data-stream")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/mpeg")
		w.WriteHeader(http.StatusOK)
		w.Write(audioContent)
	}))
	defer server.Close()

	feedWithEnclosure := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
<channel>
	<title>Download Feed</title>
	<item>
		<title>Downloadable Episode</title>
		<enclosure url="%s" length="123" type="audio/mpeg" />
		<pubDate>Mon, 08 Jun 2026 12:00:00 +0000</pubDate>
	</item>
</channel>
</rss>`, server.URL)

	feedServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(feedWithEnclosure))
	}))
	defer feedServer.Close()

	configureTestClient(t, m, server.URL, feedServer.URL)

	podID := "pod-dl"
	_, err := db.Exec(`
		INSERT INTO podcasts (id, title, feedURL, autoDownloadEpisodes, maxEpisodesToKeep, maxNewEpisodesToDownload)
		VALUES (?, ?, ?, ?, ?, ?)
	`, podID, "Download Podcast", feedServer.URL, 1, 0, 0)
	if err != nil {
		t.Fatalf("failed to insert podcast: %v", err)
	}

	tempLibDir := t.TempDir()

	_, err = db.Exec(`
		INSERT INTO libraryItems (id, path, mediaId, mediaType)
		VALUES (?, ?, ?, ?)
	`, "lib-item-1", tempLibDir, podID, "podcast")
	if err != nil {
		t.Fatalf("failed to insert library item: %v", err)
	}

	expectedDestName := sanitizeFilename("Downloadable Episode") + ".mp3"
	duplicateCheckPath := filepath.Join(tempLibDir, expectedDestName)
	err = os.WriteFile(duplicateCheckPath, []byte("pre-existing-content"), 0644)
	if err != nil {
		t.Fatalf("failed to write pre-existing file: %v", err)
	}

	ctx := context.Background()
	err = m.SyncAllFeeds(ctx)
	if err != nil {
		t.Fatalf("SyncAllFeeds failed: %v", err)
	}

	files, err := os.ReadDir(tempLibDir)
	if err != nil {
		t.Fatalf("failed to read library dir: %v", err)
	}

	var downloadedFile string
	for _, f := range files {
		if f.Name() != expectedDestName {
			downloadedFile = filepath.Join(tempLibDir, f.Name())
		}
	}

	if downloadedFile == "" {
		t.Fatal("expected duplicate-resolved file to be downloaded, but found none")
	}

	dlContent, err := os.ReadFile(downloadedFile)
	if err != nil {
		t.Fatalf("failed to read downloaded file: %v", err)
	}
	if string(dlContent) != string(audioContent) {
		t.Errorf("downloaded file content mismatch, expected %q, got %q", string(audioContent), string(dlContent))
	}

	var audioFileJSON string
	err = db.QueryRow("SELECT audioFile FROM podcastEpisodes WHERE podcastId = ?", podID).Scan(&audioFileJSON)
	if err != nil {
		t.Fatalf("failed to query episode audioFile: %v", err)
	}

	var audioFileMap map[string]interface{}
	err = json.Unmarshal([]byte(audioFileJSON), &audioFileMap)
	if err != nil {
		t.Fatalf("failed to unmarshal audioFile JSON %q: %v", audioFileJSON, err)
	}

	meta, ok := audioFileMap["metadata"].(map[string]interface{})
	if !ok {
		t.Fatalf("invalid audioFile JSON structure, missing metadata: %s", audioFileJSON)
	}

	if meta["path"] != downloadedFile {
		t.Errorf("expected path %q in DB, got %q", downloadedFile, meta["path"])
	}
	if int64(meta["size"].(float64)) != int64(len(audioContent)) {
		t.Errorf("expected size %d, got %v", len(audioContent), meta["size"])
	}
}

func TestSyncAllFeeds_LimitNewEpisodes(t *testing.T) {
	db := setupTestDB(t, false)
	defer db.Close()
	m := NewPodcastManager(db)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(feedUTF8))
	}))
	defer server.Close()

	configureTestClient(t, m, server.URL)

	_, err := db.Exec(`
		INSERT INTO podcasts (id, title, feedURL, autoDownloadEpisodes, maxEpisodesToKeep, maxNewEpisodesToDownload)
		VALUES (?, ?, ?, ?, ?, ?)
	`, "pod-limit", "Limit Podcast", server.URL, 0, 0, 1)
	if err != nil {
		t.Fatalf("failed to insert podcast: %v", err)
	}

	ctx := context.Background()
	err = m.SyncAllFeeds(ctx)
	if err != nil {
		t.Fatalf("SyncAllFeeds failed: %v", err)
	}

	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM podcastEpisodes WHERE podcastId = ?", "pod-limit").Scan(&count)
	if err != nil {
		t.Fatalf("query count failed: %v", err)
	}

	if count != 1 {
		t.Errorf("expected exactly 1 episode to be inserted due to limit, got %d", count)
	}
}

func TestScheduleRefresh(t *testing.T) {
	db := setupTestDB(t, false)
	defer db.Close()
	m := NewPodcastManager(db)

	var reqCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&reqCount, 1)
		w.Header().Set("Content-Type", "application/rss+xml")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(feedUTF8))
	}))
	defer server.Close()

	configureTestClient(t, m, server.URL)

	_, err := db.Exec(`
		INSERT INTO podcasts (id, title, feedURL, autoDownloadEpisodes, maxEpisodesToKeep, maxNewEpisodesToDownload)
		VALUES (?, ?, ?, ?, ?, ?)
	`, "pod-refresh", "Refresh Podcast", server.URL, 0, 0, 0)
	if err != nil {
		t.Fatalf("failed to insert podcast: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = m.ScheduleRefresh(ctx, "* * * * *")
	if err != nil {
		t.Fatalf("ScheduleRefresh failed: %v", err)
	}

	start := time.Now()
	for {
		if atomic.LoadInt32(&reqCount) > 0 {
			break
		}
		if time.Since(start) > 1500*time.Millisecond {
			t.Fatal("timed out waiting for background sync to trigger")
		}
		time.Sleep(10 * time.Millisecond)
	}

	cancel()
	time.Sleep(50 * time.Millisecond)
}

func TestParseCronToDuration(t *testing.T) {
	tests := []struct {
		expr     string
		expected time.Duration
	}{
		{"* * * * *", 1 * time.Minute},
		{"*/5 * * * *", 5 * time.Minute},
		{"5 * * * *", 1 * time.Hour},
		{"5 */3 * * *", 3 * time.Hour},
		{"5 2 * * *", 24 * time.Hour},
		{"invalid", 1 * time.Hour},
		{"*", 1 * time.Hour},
	}

	for _, tc := range tests {
		t.Run(tc.expr, func(t *testing.T) {
			res := parseCronToDuration(tc.expr)
			if res != tc.expected {
				t.Errorf("expected %v for expr %q, got %v", tc.expected, tc.expr, res)
			}
		})
	}
}
