package providers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/doyensec/safeurl"
)

// mockTransport intercepts outgoing requests and routes them to httptest.Server.
type mockTransport struct {
	TargetURL   string
	ForceStatus int
	ForceRetry  int
	retryCount  int
}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if m.ForceRetry > 0 && m.retryCount < m.ForceRetry {
		m.retryCount++
		resp := &http.Response{
			StatusCode: http.StatusTooManyRequests,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("Rate limit")),
		}
		resp.Header.Set("Retry-After", "0")
		return resp, nil
	}

	if m.ForceStatus > 0 {
		return &http.Response{
			StatusCode: m.ForceStatus,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("Error")),
		}, nil
	}

	target, err := url.Parse(m.TargetURL)
	if err != nil {
		return nil, err
	}

	req.Header.Set("X-Original-Host", req.URL.Host)
	req.Header.Set("X-Original-URL", req.URL.String())

	req.URL.Scheme = target.Scheme
	req.URL.Host = target.Host

	return http.DefaultTransport.RoundTrip(req)
}

func setupTestClient(t *testing.T, transport http.RoundTripper) {
	orig := safeHTTPClient
	config := safeurl.GetConfigBuilder().Build()
	client := safeurl.Client(config)
	client.Client = &http.Client{
		Transport: transport,
	}
	safeHTTPClient = client
	t.Cleanup(func() {
		safeHTTPClient = orig
	})
}

func mockHTTPHandler(w http.ResponseWriter, r *http.Request) {
	origHost := r.Header.Get("X-Original-Host")
	if origHost == "" {
		origHost = r.Host
	}

	path := r.URL.Path

	switch {
	case strings.Contains(origHost, "audible.com"):
		{
			title := r.URL.Query().Get("title")
			if title == "error" {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			if title == "invalid-json" {
				w.Write([]byte("invalid json"))
				return
			}
			resp := audibleCatalogResponse{
				Products: []audibleProduct{
					{ASIN: "B01N22S8W8"},
				},
			}
			json.NewEncoder(w).Encode(resp)
		}

	case strings.Contains(origHost, "audnex.us"):
		{
			if strings.HasSuffix(path, "/chapters") {
				w.Write([]byte(`{"chapters": [{"title": "Chapter 1"}]}`))
				return
			}
			if strings.HasPrefix(path, "/books/") {
				asin := strings.TrimPrefix(path, "/books/")
				if asin == "NOTFOUND" {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				if asin == "ERROR" {
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				resp := audnexusBookDetails{
					Title:         "The Way of Kings",
					Subtitle:      "Book One",
					ASIN:          asin,
					Authors:       []audnexusAuthorOrNarrator{{Name: "Brandon Sanderson"}},
					Narrators:     []audnexusAuthorOrNarrator{{Name: "Michael Kramer"}},
					PublisherName: "Macmillan Audio",
					Summary:       "Roshar is a world of stone.",
					ReleaseDate:   "2010-08-31",
					Image:         "https://images.com/cover.jpg",
					Language:      "english",
					ISBN:          "9780765326355",
				}
				json.NewEncoder(w).Encode(resp)
				return
			}
			if path == "/authors" {
				name := r.URL.Query().Get("name")
				if name == "notfound" {
					w.Write([]byte("[]"))
					return
				}
				w.Write([]byte(`[{"asin": "B000AP76L6", "name": "Brandon Sanderson"}]`))
				return
			}
			if strings.HasPrefix(path, "/authors/") {
				asin := strings.TrimPrefix(path, "/authors/")
				if asin == "NOTFOUND" {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				resp := AudnexusAuthorDetails{
					ASIN:        asin,
					Name:        "Brandon Sanderson",
					Description: "Fantasy author",
					Image:       "https://images.com/author.jpg",
				}
				json.NewEncoder(w).Encode(resp)
				return
			}
		}

	case strings.Contains(origHost, "googleapis.com"):
		{
			q := r.URL.Query().Get("q")
			if q == "invalid-json" {
				w.Write([]byte("invalid-json"))
				return
			}
			if q == "error" {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			resp := googleSearchResponse{
				Items: []googleVolumeItem{
					{
						ID: "12345",
						VolumeInfo: googleVolumeInfo{
							Title:     "The Hobbit",
							Authors:   []string{"J.R.R. Tolkien"},
							Publisher: "Allen & Unwin",
							ImageLinks: googleImageLinks{
								Thumbnail: "http://books.google.com/hobbit.jpg",
							},
							Language: "en",
						},
					},
				},
			}
			json.NewEncoder(w).Encode(resp)
		}

	case strings.Contains(origHost, "apple.com"):
		{
			term := r.URL.Query().Get("term")
			if term == "invalid-json" {
				w.Write([]byte("invalid-json"))
				return
			}
			if term == "error" {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			media := r.URL.Query().Get("media")
			if media == "podcast" {
				w.Write([]byte(`{"results": [{"collectionName": "Podcast Title", "artistName": "Podcast Artist", "description": "Podcast Desc"}]}`))
			} else {
				w.Write([]byte(`{"results": [{"collectionName": "Book Title", "artistName": "Book Artist", "description": "Book Desc"}]}`))
			}
		}

	case strings.Contains(origHost, "openlibrary.org"):
		{
			if strings.Contains(path, "/isbn/") {
				isbn := strings.TrimSuffix(strings.TrimPrefix(path, "/isbn/"), ".json")
				if isbn == "NOTFOUND" {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				w.Write([]byte(`{"title": "ISBN Book"}`))
				return
			}
			if path == "/search.json" {
				title := r.URL.Query().Get("title")
				if title == "invalid-json" {
					w.Write([]byte("invalid-json"))
					return
				}
				if title == "error" {
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				w.Write([]byte(`{"docs": [{"key": "/works/OL123W", "title": "OL Title", "author_name": ["OL Author"], "cover_edition_key": "OLID123"}]}`))
				return
			}
			if strings.HasSuffix(path, ".json") {
				w.Write([]byte(`{"covers": [12345], "first_publish_date": "1950", "description": "Works Description"}`))
				return
			}
		}
	}
}

func TestAudibleProvider(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(mockHTTPHandler))
	defer server.Close()

	setupTestClient(t, &mockTransport{TargetURL: server.URL})

	provider := &AudibleProvider{}

	t.Run("Name", func(t *testing.T) {
		if provider.Name() != "audible" {
			t.Errorf("expected audible, got %s", provider.Name())
		}
	})

	t.Run("SearchBooks by Query", func(t *testing.T) {
		results, err := provider.SearchBooks(context.Background(), "Stormlight")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}
		res := results[0]
		if res.Title != "The Way of Kings" {
			t.Errorf("expected 'The Way of Kings', got '%s'", res.Title)
		}
		if len(res.Authors) == 0 || res.Authors[0] != "Brandon Sanderson" {
			t.Errorf("expected Brandon Sanderson, got %v", res.Authors)
		}
		if len(res.Narrators) == 0 || res.Narrators[0] != "Michael Kramer" {
			t.Errorf("expected Michael Kramer, got %v", res.Narrators)
		}
		if res.PublishedYear != "2010" {
			t.Errorf("expected 2010, got %s", res.PublishedYear)
		}
		if res.Language != "English" {
			t.Errorf("expected English, got %s", res.Language)
		}
	})

	t.Run("SearchBooks by ASIN", func(t *testing.T) {
		results, err := provider.SearchBooks(context.Background(), "B01N22S8W8")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}
		if results[0].Title != "The Way of Kings" {
			t.Errorf("expected 'The Way of Kings', got '%s'", results[0].Title)
		}
	})

	t.Run("SearchBooks empty query", func(t *testing.T) {
		results, err := provider.SearchBooks(context.Background(), "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if results != nil {
			t.Errorf("expected nil results for empty query")
		}
	})

	t.Run("SearchBooks error response", func(t *testing.T) {
		_, err := provider.SearchBooks(context.Background(), "error")
		if err == nil {
			t.Error("expected error but got nil")
		}
	})

	t.Run("SearchBooks invalid json", func(t *testing.T) {
		_, err := provider.SearchBooks(context.Background(), "invalid-json")
		if err == nil {
			t.Error("expected error decoding invalid json")
		}
	})

	t.Run("SearchPodcasts", func(t *testing.T) {
		results, err := provider.SearchPodcasts(context.Background(), "test")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if results != nil {
			t.Errorf("expected nil results for podcasts")
		}
	})
}

func TestAudnexusProvider(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(mockHTTPHandler))
	defer server.Close()

	setupTestClient(t, &mockTransport{TargetURL: server.URL})

	provider := &AudnexusProvider{}

	t.Run("Name", func(t *testing.T) {
		if provider.Name() != "audnexus" {
			t.Errorf("expected audnexus, got %s", provider.Name())
		}
	})

	t.Run("SearchBooks by ASIN", func(t *testing.T) {
		results, err := provider.SearchBooks(context.Background(), "B01N22S8W8")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}
		if results[0].Title != "The Way of Kings" {
			t.Errorf("expected 'The Way of Kings', got '%s'", results[0].Title)
		}
	})

	t.Run("SearchBooks by query non-ASIN", func(t *testing.T) {
		results, err := provider.SearchBooks(context.Background(), "not_an_asin")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if results != nil {
			t.Errorf("expected nil results for non-ASIN search")
		}
	})

	t.Run("SearchPodcasts", func(t *testing.T) {
		results, err := provider.SearchPodcasts(context.Background(), "test")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if results != nil {
			t.Errorf("expected nil results for podcasts")
		}
	})

	t.Run("AuthorASINsRequest", func(t *testing.T) {
		asins, err := provider.AuthorASINsRequest(context.Background(), "Brandon Sanderson", "us")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(asins) != 1 || asins[0].ASIN != "B000AP76L6" {
			t.Errorf("expected ASIN B000AP76L6, got %v", asins)
		}
	})

	t.Run("AuthorRequest", func(t *testing.T) {
		details, err := provider.AuthorRequest(context.Background(), "B000AP76L6", "us")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if details.Name != "Brandon Sanderson" {
			t.Errorf("expected Brandon Sanderson, got %s", details.Name)
		}
	})

	t.Run("FindAuthorByASIN", func(t *testing.T) {
		details, err := provider.FindAuthorByASIN(context.Background(), "B000AP76L6", "us")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if details.Name != "Brandon Sanderson" {
			t.Errorf("expected Brandon Sanderson, got %s", details.Name)
		}
	})

	t.Run("FindAuthorByName", func(t *testing.T) {
		details, err := provider.FindAuthorByName(context.Background(), "Brandon Sanderson", "us", 3)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if details.Name != "Brandon Sanderson" {
			t.Errorf("expected Brandon Sanderson, got %s", details.Name)
		}
	})

	t.Run("GetChaptersByASIN", func(t *testing.T) {
		chapters, err := provider.GetChaptersByASIN(context.Background(), "B01N22S8W8", "us")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if chapters == nil {
			t.Fatal("expected chapters list, got nil")
		}
	})
}

func TestGoogleBooksProvider(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(mockHTTPHandler))
	defer server.Close()

	setupTestClient(t, &mockTransport{TargetURL: server.URL})

	provider := &GoogleBooksProvider{}

	t.Run("Name", func(t *testing.T) {
		if provider.Name() != "google" {
			t.Errorf("expected google, got %s", provider.Name())
		}
	})

	t.Run("SearchBooks", func(t *testing.T) {
		results, err := provider.SearchBooks(context.Background(), "Hobbit")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}
		res := results[0]
		if res.Title != "The Hobbit" {
			t.Errorf("expected 'The Hobbit', got '%s'", res.Title)
		}
		if len(res.Authors) == 0 || res.Authors[0] != "J.R.R. Tolkien" {
			t.Errorf("expected J.R.R. Tolkien, got %v", res.Authors)
		}
		if res.Language != "En" {
			t.Errorf("expected En, got %s", res.Language)
		}
		if res.CoverURL != "https://books.google.com/hobbit.jpg" {
			t.Errorf("expected https version of cover URL, got %s", res.CoverURL)
		}
	})

	t.Run("SearchPodcasts", func(t *testing.T) {
		results, err := provider.SearchPodcasts(context.Background(), "test")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if results != nil {
			t.Errorf("expected nil results for podcasts")
		}
	})
}

func TestITunesProvider(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(mockHTTPHandler))
	defer server.Close()

	setupTestClient(t, &mockTransport{TargetURL: server.URL})

	provider := &ITunesProvider{}

	t.Run("Name", func(t *testing.T) {
		if provider.Name() != "itunes" {
			t.Errorf("expected itunes, got %s", provider.Name())
		}
	})

	t.Run("SearchBooks", func(t *testing.T) {
		results, err := provider.SearchBooks(context.Background(), "Book Title")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}
		res := results[0]
		if res.Title != "Book Title" {
			t.Errorf("expected 'Book Title', got '%s'", res.Title)
		}
		if len(res.Authors) == 0 || res.Authors[0] != "Book Artist" {
			t.Errorf("expected 'Book Artist', got %v", res.Authors)
		}
	})

	t.Run("SearchPodcasts", func(t *testing.T) {
		results, err := provider.SearchPodcasts(context.Background(), "Podcast Title")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}
		res := results[0]
		if res.Title != "Podcast Title" {
			t.Errorf("expected 'Podcast Title', got '%s'", res.Title)
		}
	})
}

func TestOpenLibraryProvider(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(mockHTTPHandler))
	defer server.Close()

	setupTestClient(t, &mockTransport{TargetURL: server.URL})

	provider := &OpenLibraryProvider{}

	t.Run("Name", func(t *testing.T) {
		if provider.Name() != "openlibrary" {
			t.Errorf("expected openlibrary, got %s", provider.Name())
		}
	})

	t.Run("SearchBooks", func(t *testing.T) {
		results, err := provider.SearchBooks(context.Background(), "OL Title")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}
		res := results[0]
		if res.Title != "OL Title" {
			t.Errorf("expected 'OL Title', got '%s'", res.Title)
		}
		if len(res.Authors) == 0 || res.Authors[0] != "OL Author" {
			t.Errorf("expected 'OL Author', got %v", res.Authors)
		}
	})

	t.Run("IsbnLookup", func(t *testing.T) {
		res, err := provider.IsbnLookup(context.Background(), "1234567890")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res == nil {
			t.Fatal("expected non-nil response")
		}
	})
}

func TestBoundaryCases(t *testing.T) {
	t.Run("isValidASIN", func(t *testing.T) {
		if !isValidASIN("B01N22S8W8") {
			t.Error("expected B01N22S8W8 to be valid ASIN")
		}
		if isValidASIN("invalid") {
			t.Error("expected invalid to be invalid ASIN")
		}
	})

	t.Run("cleanDescription", func(t *testing.T) {
		cleaned := cleanDescription("<p>Hello &amp; World</p>")
		if cleaned != "Hello & World" {
			t.Errorf("expected 'Hello & World', got '%s'", cleaned)
		}
	})

	t.Run("toTitle", func(t *testing.T) {
		cases := []struct {
			input, expected string
		}{
			{"english", "English"},
			{"american english", "American English"},
			{"", ""},
			{"en-us", "En-us"},
		}
		for _, c := range cases {
			actual := toTitle(c.input)
			if actual != c.expected {
				t.Errorf("toTitle(%q) = %q; expected %q", c.input, actual, c.expected)
			}
		}
	})

	t.Run("getCoverArtwork sizes", func(t *testing.T) {
		data := map[string]interface{}{
			"artworkUrl100": "http://example.com/100x100bb.jpg",
		}
		cover := getCoverArtwork(data)
		if cover != "http://example.com/600x600bb.jpg" {
			t.Errorf("expected artwork resized to 600x600bb, got %s", cover)
		}
	})

	t.Run("getWithRetry 429 retries", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(mockHTTPHandler))
		defer server.Close()

		// Force two 429 requests, then let it succeed
		transport := &mockTransport{
			TargetURL:  server.URL,
			ForceRetry: 2,
		}
		setupTestClient(t, transport)

		resp, err := getWithRetry(context.Background(), safeHTTPClient, "https://api.audnex.us/books/B01N22S8W8")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200 OK, got %d", resp.StatusCode)
		}
		if transport.retryCount != 2 {
			t.Errorf("expected 2 retries, got %d", transport.retryCount)
		}
	})

	t.Run("network timeout / context cancellation", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(mockHTTPHandler))
		defer server.Close()

		setupTestClient(t, &mockTransport{TargetURL: server.URL})

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // cancel immediately

		provider := &AudnexusProvider{}
		_, err := provider.SearchBooks(ctx, "B01N22S8W8")
		if err == nil {
			t.Error("expected context canceled error, got nil")
		}
	})
}
