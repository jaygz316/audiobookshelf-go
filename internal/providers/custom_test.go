package providers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCustomProviderSearchBooks(t *testing.T) {
	// 1. Mock server that returns the expected custom metadata format
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify query parameter
		query := r.URL.Query().Get("query")
		if query != "The Hobbit" {
			t.Errorf("Expected query 'The Hobbit', got %q", query)
		}

		// Verify auth header
		authHeader := r.Header.Get("Authorization")
		if authHeader != "Bearer my-secret-token" {
			t.Errorf("Expected Authorization 'Bearer my-secret-token', got %q", authHeader)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		// Respond with matching specification
		response := map[string]interface{}{
			"matches": []map[string]interface{}{
				{
					"title":         "The Hobbit",
					"subtitle":      "There and Back Again",
					"author":        "J.R.R. Tolkien",
					"narrator":      "Rob Inglis",
					"publisher":     "George Allen & Unwin",
					"publishedYear": "1937",
					"description":   "A classic fantasy novel.",
					"cover":         "https://example.com/hobbit.jpg",
					"isbn":          "9780007440832",
					"language":      "english",
				},
			},
		}
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	// 2. Instantiate CustomProvider
	provider := NewCustomProvider("test-uuid", "My Mock Provider", "book", server.URL, "Bearer my-secret-token")

	// 3. Perform search
	results, err := provider.SearchBooks(context.Background(), "The Hobbit")
	if err != nil {
		t.Fatalf("SearchBooks failed: %v", err)
	}

	// 4. Assertions
	if len(results) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(results))
	}

	res := results[0]
	if res.Title != "The Hobbit" {
		t.Errorf("Expected Title 'The Hobbit', got %q", res.Title)
	}
	if res.Subtitle != "There and Back Again" {
		t.Errorf("Expected Subtitle 'There and Back Again', got %q", res.Subtitle)
	}
	if len(res.Authors) != 1 || res.Authors[0] != "J.R.R. Tolkien" {
		t.Errorf("Expected Author 'J.R.R. Tolkien', got %v", res.Authors)
	}
	if len(res.Narrators) != 1 || res.Narrators[0] != "Rob Inglis" {
		t.Errorf("Expected Narrator 'Rob Inglis', got %v", res.Narrators)
	}
	if res.Publisher != "George Allen & Unwin" {
		t.Errorf("Expected Publisher 'George Allen & Unwin', got %q", res.Publisher)
	}
	if res.PublishedYear != "1937" {
		t.Errorf("Expected PublishedYear '1937', got %q", res.PublishedYear)
	}
	if res.Description != "A classic fantasy novel." {
		t.Errorf("Expected Description 'A classic fantasy novel.', got %q", res.Description)
	}
	if res.CoverURL != "https://example.com/hobbit.jpg" {
		t.Errorf("Expected CoverURL 'https://example.com/hobbit.jpg', got %q", res.CoverURL)
	}
	if res.ISBN != "9780007440832" {
		t.Errorf("Expected ISBN '9780007440832', got %q", res.ISBN)
	}
	if res.Language != "English" {
		t.Errorf("Expected Language 'English', got %q", res.Language)
	}
}

func TestCustomProviderSearchPodcasts(t *testing.T) {
	// 1. Mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		response := map[string]interface{}{
			"matches": []map[string]interface{}{
				{
					"title":       "Science VS",
					"author":      "Gimlet",
					"description": "A science podcast.",
					"cover":       "https://example.com/science.jpg",
					"language":    "english",
				},
			},
		}
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	// 2. Instantiate CustomProvider for podcasts
	provider := NewCustomProvider("test-uuid-2", "My Podcast Provider", "podcast", server.URL, "")

	// 3. Perform search
	results, err := provider.SearchPodcasts(context.Background(), "Science VS")
	if err != nil {
		t.Fatalf("SearchPodcasts failed: %v", err)
	}

	// 4. Assertions
	if len(results) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(results))
	}

	res := results[0]
	if res.Title != "Science VS" {
		t.Errorf("Expected Title 'Science VS', got %q", res.Title)
	}
	if len(res.Authors) != 1 || res.Authors[0] != "Gimlet" {
		t.Errorf("Expected Author 'Gimlet', got %v", res.Authors)
	}
	if res.Description != "A science podcast." {
		t.Errorf("Expected Description 'A science podcast.', got %q", res.Description)
	}
	if res.CoverURL != "https://example.com/science.jpg" {
		t.Errorf("Expected CoverURL 'https://example.com/science.jpg', got %q", res.CoverURL)
	}
	if res.Language != "English" {
		t.Errorf("Expected Language 'English', got %q", res.Language)
	}
}
