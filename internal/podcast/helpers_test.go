package podcast

import (
	"crypto/tls"
	"database/sql"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"sync/atomic"
	"testing"

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
			maxNewEpisodesToDownload INTEGER,
			autoDeletePlayed INTEGER
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
			enclosureURL TEXT,
			imageURL TEXT
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
		<itunes:season>3</itunes:season>
		<itunes:episode>12</itunes:episode>
		<itunes:episodeType>bonus</itunes:episodeType>
		<itunes:image href="http://example.com/ep1-cover.jpg" />
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
