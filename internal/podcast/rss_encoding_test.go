package podcast

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseRSS_EncodingFallbacks(t *testing.T) {
	// Case 1: XML with encoding="iso-8859-1" in prologue and Latin-1 byte (0xe9 = é)
	xmlISO := []byte(`<?xml version="1.0" encoding="iso-8859-1"?>
<rss version="2.0">
<channel>
	<title>ISO Title ` + string([]byte{0xe9}) + `</title>
	<item>
		<title>Episode 1</title>
		<enclosure url="http://example.com/ep1.mp3" type="audio/mpeg"/>
	</item>
</channel>
</rss>`)

	feed, err := parseRSS(xmlISO)
	if err != nil {
		t.Fatalf("Failed parsing ISO-8859-1 XML: %v", err)
	}
	expectedISO := "ISO Title \u00e9"
	if feed.Title != expectedISO {
		t.Errorf("Expected title %q, got %q", expectedISO, feed.Title)
	}

	// Case 2: XML with encoding="windows-1252"
	xmlWin := []byte(`<?xml version="1.0" encoding="windows-1252"?>
<rss version="2.0">
<channel>
	<title>Windows Title ` + string([]byte{0xe9}) + `</title>
	<item>
		<title>Episode 1</title>
		<enclosure url="http://example.com/ep1.mp3" type="audio/mpeg"/>
	</item>
</channel>
</rss>`)

	feed2, err := parseRSS(xmlWin)
	if err != nil {
		t.Fatalf("Failed parsing Windows-1252 XML: %v", err)
	}
	expectedWin := "Windows Title \u00e9"
	if feed2.Title != expectedWin {
		t.Errorf("Expected title %q, got %q", expectedWin, feed2.Title)
	}
}

func TestFetchFeed_ISO88591_WithISODeclaration(t *testing.T) {
	db := setupTestDB(t, false)
	defer db.Close()
	m := NewPodcastManager(db)

	xmlISO := []byte(`<?xml version="1.0" encoding="iso-8859-1"?>
<rss version="2.0">
<channel>
	<title>ISO Title ` + string([]byte{0xe9}) + `</title>
	<item>
		<title>Episode 1</title>
		<enclosure url="http://example.com/ep1.mp3" type="audio/mpeg"/>
	</item>
</channel>
</rss>`)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml; charset=iso-8859-1")
		w.WriteHeader(http.StatusOK)
		w.Write(xmlISO)
	}))
	defer server.Close()

	configureTestClient(t, m, server.URL)

	ctx := context.Background()
	feed, err := m.FetchFeed(ctx, server.URL)
	if err != nil {
		t.Fatalf("FetchFeed failed: %v", err)
	}

	// Verify that the title is correctly single-decoded into "ISO Title é"
	expectedISO := "ISO Title \u00e9"
	if feed.Title != expectedISO {
		t.Errorf("Expected title %q, got %q", expectedISO, feed.Title)
	}
}

func TestFetchFeed_Windows1252_WithDeclaration(t *testing.T) {
	db := setupTestDB(t, false)
	defer db.Close()
	m := NewPodcastManager(db)

	xmlWin := []byte(`<?xml version="1.0" encoding="windows-1252"?>
<rss version="2.0">
<channel>
	<title>Windows Title ` + string([]byte{0xe9}) + `</title>
	<item>
		<title>Episode 1</title>
		<enclosure url="http://example.com/ep1.mp3" type="audio/mpeg"/>
	</item>
</channel>
</rss>`)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml; charset=windows-1252")
		w.WriteHeader(http.StatusOK)
		w.Write(xmlWin)
	}))
	defer server.Close()

	configureTestClient(t, m, server.URL)

	ctx := context.Background()
	feed, err := m.FetchFeed(ctx, server.URL)
	if err != nil {
		t.Fatalf("FetchFeed failed: %v", err)
	}

	expectedWin := "Windows Title \u00e9"
	if feed.Title != expectedWin {
		t.Errorf("Expected title %q, got %q", expectedWin, feed.Title)
	}
}
