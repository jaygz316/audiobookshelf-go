package podcast

import (
	"context"
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/doyensec/safeurl"
)

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

	if ep1.Season != "3" {
		t.Errorf("expected ep1 Season '3', got %q", ep1.Season)
	}
	if ep1.Episode != "12" {
		t.Errorf("expected ep1 Episode '12', got %q", ep1.Episode)
	}
	if ep1.EpisodeType != "bonus" {
		t.Errorf("expected ep1 EpisodeType 'bonus', got %q", ep1.EpisodeType)
	}
	if ep1.ImageURL != "http://example.com/ep1-cover.jpg" {
		t.Errorf("expected ep1 ImageURL 'http://example.com/ep1-cover.jpg', got %q", ep1.ImageURL)
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
