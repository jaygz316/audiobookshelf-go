package finders

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/doyensec/safeurl"

	"audiobookshelf/internal/providers"
)

type testMockTransport struct {
	TargetURL string
}

func (m *testMockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	target, err := url.Parse(m.TargetURL)
	if err != nil {
		return nil, err
	}
	req.URL.Scheme = target.Scheme
	req.URL.Host = target.Host
	return http.DefaultTransport.RoundTrip(req)
}

func TestSearchAuthors(t *testing.T) {
	// 1. Setup Mock HTTP Server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if strings.HasPrefix(r.URL.Path, "/authors") {
			// Check if query parameter "name" is set (AuthorASINsRequest)
			name := r.URL.Query().Get("name")
			if name != "" {
				if name == "Stephen King" {
					w.WriteHeader(http.StatusOK)
					w.Write([]byte(`[{"asin": "B000AP9U0C", "name": "Stephen King"}]`))
					return
				}
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`[]`))
				return
			}

			// Single author query (AuthorRequest)
			if strings.HasSuffix(r.URL.Path, "/B000AP9U0C") {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{
					"asin": "B000AP9U0C",
					"name": "Stephen King",
					"description": "Master of Horror",
					"image": "http://mockserver/king.jpg"
				}`))
				return
			}

			if strings.HasSuffix(r.URL.Path, "/B000000000") {
				w.WriteHeader(http.StatusNotFound)
				return
			}
		}

		http.NotFound(w, r)
	}))
	defer server.Close()

	// 2. Setup mock client for safeurl
	transport := &testMockTransport{TargetURL: server.URL}
	config := safeurl.GetConfigBuilder().Build()
	mockWrappedClient := safeurl.Client(config)
	mockWrappedClient.Client = &http.Client{
		Transport: transport,
	}

	// 3. Set custom client and defer restore
	// Note: We don't have a clean way to get the old one, but we can set a new one or keep reference.
	// We will set the mock client.
	providers.SetSafeHTTPClientForTest(mockWrappedClient)
	defer func() {
		// Restore to default client (which is constructed from default safeurl settings)
		restoredConfig := safeurl.GetConfigBuilder().Build()
		providers.SetSafeHTTPClientForTest(safeurl.Client(restoredConfig))
	}()

	// 4. Initialize Finder with Audnexus provider
	audnexusProv := &providers.AudnexusProvider{}
	finder := NewFinder(nil, []providers.Provider{audnexusProv})

	t.Run("Success author search by name", func(t *testing.T) {
		res, err := finder.SearchAuthors(context.Background(), "audnexus", "Stephen King")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(res) != 1 {
			t.Fatalf("expected 1 result, got %d", len(res))
		}
		if res[0].ASIN != "B000AP9U0C" || res[0].Name != "Stephen King" || res[0].Description != "Master of Horror" {
			t.Errorf("unexpected details: %+v", res[0])
		}
	})

	t.Run("Success author search by ASIN", func(t *testing.T) {
		res, err := finder.SearchAuthors(context.Background(), "audnexus", "B000AP9U0C")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(res) != 1 {
			t.Fatalf("expected 1 result, got %d", len(res))
		}
		if res[0].ASIN != "B000AP9U0C" || res[0].Name != "Stephen King" || res[0].Description != "Master of Horror" {
			t.Errorf("unexpected details: %+v", res[0])
		}
	})

	t.Run("Audnexus provider not registered returns error", func(t *testing.T) {
		emptyFinder := NewFinder(nil, nil)
		_, err := emptyFinder.SearchAuthors(context.Background(), "audnexus", "Stephen King")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		expectedErr := "audnexus provider not registered"
		if err.Error() != expectedErr {
			t.Errorf("expected error %q, got %q", expectedErr, err.Error())
		}
	})

	t.Run("Context cancellation handles gracefully", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := finder.SearchAuthors(ctx, "audnexus", "Stephen King")
		// Since context is cancelled, request should fail
		if err == nil {
			t.Error("expected error due to cancelled context, got nil")
		}
	})

	t.Run("Concurrency with multiple ASINs and fallback", func(t *testing.T) {
		// Mock server that returns multiple ASINs
		server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if r.URL.Query().Get("name") == "Many" {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`[
					{"asin": "B000AP9U0C", "name": "Stephen King"},
					{"asin": "B000000000", "name": "Unknown Author"}
				]`))
				return
			}
			if strings.HasSuffix(r.URL.Path, "/B000AP9U0C") {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{"asin": "B000AP9U0C", "name": "Stephen King"}`))
				return
			}
			if strings.HasSuffix(r.URL.Path, "/B000000000") {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			http.NotFound(w, r)
		}))
		defer server2.Close()

		transport2 := &testMockTransport{TargetURL: server2.URL}
		mockWrappedClient2 := safeurl.Client(config)
		mockWrappedClient2.Client = &http.Client{Transport: transport2}

		providers.SetSafeHTTPClientForTest(mockWrappedClient2)

		res, err := finder.SearchAuthors(context.Background(), "audnexus", "Many")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(res) != 2 {
			t.Fatalf("expected 2 results (one successful, one fallback), got %d", len(res))
		}

		// One should be Stephen King (successful), the other should be Unknown Author (fallback)
		var kingFound, unknownFound bool
		for _, details := range res {
			if details.ASIN == "B000AP9U0C" && details.Name == "Stephen King" {
				kingFound = true
			}
			if details.ASIN == "B000000000" && details.Name == "Unknown Author" {
				unknownFound = true
			}
		}

		if !kingFound {
			t.Error("expected to find Stephen King in results")
		}
		if !unknownFound {
			t.Error("expected to find fallback Unknown Author in results")
		}
	})
}

func TestSearchAuthorsContextDoneBeforeSemaphore(t *testing.T) {
	// 1. Mock server returning multiple ASINs
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("name") == "Slow" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`[
				{"asin": "B000AP9U01", "name": "Author 1"},
				{"asin": "B000AP9U02", "name": "Author 2"},
				{"asin": "B000AP9U03", "name": "Author 3"},
				{"asin": "B000AP9U04", "name": "Author 4"}
			]`))
			return
		}
		// Introduce delay for details requests
		time.Sleep(50 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"asin": "B000AP9U01", "name": "Author 1"}`))
	}))
	defer server.Close()

	transport := &testMockTransport{TargetURL: server.URL}
	config := safeurl.GetConfigBuilder().Build()
	mockWrappedClient := safeurl.Client(config)
	mockWrappedClient.Client = &http.Client{Transport: transport}

	providers.SetSafeHTTPClientForTest(mockWrappedClient)
	defer func() {
		restoredConfig := safeurl.GetConfigBuilder().Build()
		providers.SetSafeHTTPClientForTest(safeurl.Client(restoredConfig))
	}()

	audnexusProv := &providers.AudnexusProvider{}
	finder := NewFinder(nil, []providers.Provider{audnexusProv})

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel the context very quickly during execution to test the select <-ctx.Done() path
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	res, err := finder.SearchAuthors(ctx, "audnexus", "Slow")
	// The request itself should return some results or error, but let's check it doesn't panic
	if err == nil && len(res) == 4 {
		// If it succeeded completely before cancellation, that's fine, but let's check it's clean
		t.Log("Search completed before cancellation")
	} else {
		t.Logf("Search cancelled as expected, got error: %v, results count: %d", err, len(res))
	}
}
