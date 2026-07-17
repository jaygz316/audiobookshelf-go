package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestOpenLibraryStress_Concurrency verifies that calling SearchBooks concurrently
// under a race detector (-race flag) does not trigger any race conditions.
func TestOpenLibraryStress_Concurrency(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case path == "/search.json":
			// Return a mock search response with 3 docs
			w.Write([]byte(`{
				"docs": [
					{"key": "/works/OL1", "title": "Book 1", "author_name": ["Author A"]},
					{"key": "/works/OL2", "title": "Book 2", "author_name": ["Author B"]},
					{"key": "/works/OL3", "title": "Book 3", "author_name": ["Author C"]}
				]
			}`))
		case strings.HasSuffix(path, ".json") && strings.Contains(path, "/works/"):
			// Return mock works details
			w.Write([]byte(`{"covers": [123], "first_publish_date": "2020", "description": "Works Description"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	setupTestClient(t, &mockTransport{TargetURL: server.URL})
	provider := &OpenLibraryProvider{}

	const concurrencyCount = 20
	var wg sync.WaitGroup
	wg.Add(concurrencyCount)

	for i := 0; i < concurrencyCount; i++ {
		go func(idx int) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			results, err := provider.SearchBooks(ctx, fmt.Sprintf("Query%d", idx))
			if err != nil {
				t.Errorf("Goroutine %d: unexpected error: %v", idx, err)
				return
			}
			if len(results) != 3 {
				t.Errorf("Goroutine %d: expected 3 results, got %d", idx, len(results))
				return
			}
		}(i)
	}

	wg.Wait()
}

// TestOpenLibraryStress_EdgeCases tests various edge cases in OpenLibrary parsing and filtering.
func TestOpenLibraryStress_EdgeCases(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case path == "/search.json":
			// Return 15 docs to test the limit slicing to 10
			var docs []map[string]interface{}
			for i := 1; i <= 15; i++ {
				docs = append(docs, map[string]interface{}{
					"key":                fmt.Sprintf("/works/OL%d", i),
					"title":              fmt.Sprintf("Book %d", i),
					"author_name":        []string{fmt.Sprintf("Author %d", i)},
					"cover_edition_key":  fmt.Sprintf("OLID%d", i),
					"first_publish_year": 2000 + i,
					"isbn":               []string{fmt.Sprintf("isbn-%d", i)},
					"language":           []string{"english"},
					"publisher":          []string{fmt.Sprintf("Publisher %d", i)},
				})
			}
			resp := map[string]interface{}{"docs": docs}
			json.NewEncoder(w).Encode(resp)

		case strings.HasSuffix(path, ".json") && strings.Contains(path, "/works/"):
			// Return varying works details based on ID
			workID := strings.TrimSuffix(strings.TrimPrefix(path, "/works/OL"), ".json")
			id, _ := strconv.Atoi(workID)

			switch id {
			case 1:
				// Description is a string
				w.Write([]byte(`{"covers": [1001], "first_publish_date": "2001-01-01", "description": "Simple string description"}`))
			case 2:
				// Description is an object
				w.Write([]byte(`{"covers": [1002], "first_publish_date": "2002-02-02", "description": {"type": "/type/text", "value": "Object description value"}}`))
			case 3:
				// Empty description
				w.Write([]byte(`{"covers": [], "first_publish_date": "", "description": null}`))
			default:
				w.Write([]byte(`{"covers": [0], "first_publish_date": "2020", "description": ""}`))
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	setupTestClient(t, &mockTransport{TargetURL: server.URL})
	provider := &OpenLibraryProvider{}

	ctx := context.Background()
	results, err := provider.SearchBooks(ctx, "test query")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should be sliced to exactly 10
	if len(results) != 10 {
		t.Errorf("expected exactly 10 results, got %d", len(results))
	}

	// Verify first item (string description)
	if results[0].Description != "Simple string description" {
		t.Errorf("expected 'Simple string description', got '%s'", results[0].Description)
	}
	if results[0].PublishedYear != "2001" {
		t.Errorf("expected published year '2001', got '%s'", results[0].PublishedYear)
	}

	// Verify second item (map description)
	if results[1].Description != "Object description value" {
		t.Errorf("expected 'Object description value', got '%s'", results[1].Description)
	}
	if results[1].PublishedYear != "2002" {
		t.Errorf("expected published year '2002', got '%s'", results[1].PublishedYear)
	}

	// Verify third item (empty description)
	if results[2].Description != "" {
		t.Errorf("expected empty description, got '%s'", results[2].Description)
	}
}

// TestOpenLibraryStress_ContextTimeout verifies that OpenLibraryProvider handles
// context cancellation and timeouts properly.
func TestOpenLibraryStress_ContextTimeout(t *testing.T) {
	t.Run("Context cancelled before search query", func(t *testing.T) {
		provider := &OpenLibraryProvider{}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := provider.SearchBooks(ctx, "query")
		if err == nil {
			t.Error("expected error for cancelled context, got nil")
		}
	})

	t.Run("Context timeout during search query request", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(100 * time.Millisecond)
			w.Write([]byte(`{"docs": []}`))
		}))
		defer server.Close()

		setupTestClient(t, &mockTransport{TargetURL: server.URL})
		provider := &OpenLibraryProvider{}

		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()

		_, err := provider.SearchBooks(ctx, "query")
		if err == nil {
			t.Error("expected error due to timeout, got nil")
		}
	})

	t.Run("Context cancellation during detail fetches", func(t *testing.T) {
		// The search response returns 3 items. The mock server delays on detail fetches.
		var blockSearch sync.WaitGroup
		blockSearch.Add(1)

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path := r.URL.Path
			if path == "/search.json" {
				w.Write([]byte(`{
					"docs": [
						{"key": "/works/OL1", "title": "Book 1"},
						{"key": "/works/OL2", "title": "Book 2"},
						{"key": "/works/OL3", "title": "Book 3"},
						{"key": "/works/OL4", "title": "Book 4"},
						{"key": "/works/OL5", "title": "Book 5"},
						{"key": "/works/OL6", "title": "Book 6"},
						{"key": "/works/OL7", "title": "Book 7"},
						{"key": "/works/OL8", "title": "Book 8"},
						{"key": "/works/OL9", "title": "Book 9"},
						{"key": "/works/OL10", "title": "Book 10"}
					]
				}`))
				return
			}
			if strings.HasSuffix(path, ".json") && strings.Contains(path, "/works/") {
				// Simulating slow network response to allow cancellation to trigger
				blockSearch.Wait()
				w.Write([]byte(`{"covers": [123]}`))
				return
			}
		}))
		defer server.Close()

		setupTestClient(t, &mockTransport{TargetURL: server.URL})
		provider := &OpenLibraryProvider{}

		ctx, cancel := context.WithCancel(context.Background())

		// Cancel the context after a brief moment in a goroutine
		go func() {
			time.Sleep(50 * time.Millisecond)
			cancel()
			blockSearch.Done() // Unblock the HTTP handler
		}()

		// Since context is canceled, SearchBooks should return, potentially with fewer/no results,
		// and won't crash.
		results, _ := provider.SearchBooks(ctx, "query")
		// The goroutines will select <-ctx.Done() and exit, returning nil results, which are cleaned.
		if len(results) != 0 {
			t.Logf("got %d results after cancellation (could be partial depending on schedule)", len(results))
		}
	})
}

// TestOpenLibraryStress_MalformedResponses ensures that the provider handles bad server output safely.
func TestOpenLibraryStress_MalformedResponses(t *testing.T) {
	t.Run("search query invalid JSON", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{docs: [invalid`))
		}))
		defer server.Close()

		setupTestClient(t, &mockTransport{TargetURL: server.URL})
		provider := &OpenLibraryProvider{}

		_, err := provider.SearchBooks(context.Background(), "query")
		if err == nil {
			t.Error("expected error decoding malformed search response, got nil")
		}
	})

	t.Run("search query status 500 error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		setupTestClient(t, &mockTransport{TargetURL: server.URL})
		provider := &OpenLibraryProvider{}

		_, err := provider.SearchBooks(context.Background(), "query")
		if err == nil {
			t.Error("expected error for 500 status, got nil")
		}
	})

	t.Run("detail fetch status 500 error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path := r.URL.Path
			if path == "/search.json" {
				w.Write([]byte(`{"docs": [{"key": "/works/OL1", "title": "Book 1"}]}`))
				return
			}
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		setupTestClient(t, &mockTransport{TargetURL: server.URL})
		provider := &OpenLibraryProvider{}

		// Should succeed by falling back to empty worksData (suppressing detail errors)
		results, err := provider.SearchBooks(context.Background(), "query")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 1 || results[0].Title != "Book 1" {
			t.Errorf("expected 1 result with Title 'Book 1', got %v", results)
		}
	})

	t.Run("detail fetch invalid JSON", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path := r.URL.Path
			if path == "/search.json" {
				w.Write([]byte(`{"docs": [{"key": "/works/OL1", "title": "Book 1"}]}`))
				return
			}
			w.Write([]byte(`{bad json`))
		}))
		defer server.Close()

		setupTestClient(t, &mockTransport{TargetURL: server.URL})
		provider := &OpenLibraryProvider{}

		// Should succeed by falling back to empty worksData (suppressing detail errors)
		results, err := provider.SearchBooks(context.Background(), "query")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 1 || results[0].Title != "Book 1" {
			t.Errorf("expected 1 result with Title 'Book 1', got %v", results)
		}
	})
}

// TestOpenLibraryStress_IsbnLookup verifies that IsbnLookup behaves correctly under stress, concurrency, and error conditions.
func TestOpenLibraryStress_IsbnLookup(t *testing.T) {
	t.Run("concurrency", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"title": "ISBN Book"}`))
		}))
		defer server.Close()

		setupTestClient(t, &mockTransport{TargetURL: server.URL})
		provider := &OpenLibraryProvider{}

		const concurrencyCount = 20
		var wg sync.WaitGroup
		wg.Add(concurrencyCount)

		for i := 0; i < concurrencyCount; i++ {
			go func(idx int) {
				defer wg.Done()
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()

				res, err := provider.IsbnLookup(ctx, fmt.Sprintf("123456789%d", idx))
				if err != nil {
					t.Errorf("unexpected error: %v", err)
					return
				}
				if res == nil {
					t.Errorf("expected non-nil response")
					return
				}
			}(i)
		}
		wg.Wait()
	})

	t.Run("context cancelled", func(t *testing.T) {
		provider := &OpenLibraryProvider{}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := provider.IsbnLookup(ctx, "1234567890")
		if err == nil {
			t.Error("expected error for cancelled context, got nil")
		}
	})

	t.Run("context timeout", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(100 * time.Millisecond)
			w.Write([]byte(`{"title": "ISBN Book"}`))
		}))
		defer server.Close()

		setupTestClient(t, &mockTransport{TargetURL: server.URL})
		provider := &OpenLibraryProvider{}

		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()

		_, err := provider.IsbnLookup(ctx, "1234567890")
		if err == nil {
			t.Error("expected error due to timeout, got nil")
		}
	})

	t.Run("404 not found", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		setupTestClient(t, &mockTransport{TargetURL: server.URL})
		provider := &OpenLibraryProvider{}

		res, err := provider.IsbnLookup(context.Background(), "1234567890")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res != nil {
			t.Errorf("expected nil result, got %v", res)
		}
	})

	t.Run("500 error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		setupTestClient(t, &mockTransport{TargetURL: server.URL})
		provider := &OpenLibraryProvider{}

		_, err := provider.IsbnLookup(context.Background(), "1234567890")
		if err == nil {
			t.Error("expected error for 500 status, got nil")
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{invalid`))
		}))
		defer server.Close()

		setupTestClient(t, &mockTransport{TargetURL: server.URL})
		provider := &OpenLibraryProvider{}

		_, err := provider.IsbnLookup(context.Background(), "1234567890")
		if err == nil {
			t.Error("expected error decoding malformed response, got nil")
		}
	})
}

// TestOpenLibrary_SearchPodcasts verifies that SearchPodcasts returns (nil, nil) since podcasts are not supported.
func TestOpenLibrary_SearchPodcasts(t *testing.T) {
	provider := &OpenLibraryProvider{}
	res, err := provider.SearchPodcasts(context.Background(), "query")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res != nil {
		t.Errorf("expected nil results, got %v", res)
	}
}

// TestOpenLibrary_SearchBooksEmptyQuery verifies that SearchBooks returns (nil, nil) for an empty query.
func TestOpenLibrary_SearchBooksEmptyQuery(t *testing.T) {
	provider := &OpenLibraryProvider{}
	res, err := provider.SearchBooks(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res != nil {
		t.Errorf("expected nil results, got %v", res)
	}
}
