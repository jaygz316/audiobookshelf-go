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

// TestFantLabStress_Concurrency verifies that calling SearchBooks concurrently
// under a race detector (-race flag) does not trigger any race conditions.
func TestFantLabStress_Concurrency(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case strings.HasPrefix(path, "/search-works"):
			w.Write([]byte(`[{"work_id": 100, "work_type_id": 1}, {"work_id": 200, "work_type_id": 2}]`))
		case strings.HasPrefix(path, "/work/"):
			// Return extended details
			workIDStr := strings.TrimSuffix(strings.TrimPrefix(path, "/work/"), "/extended")
			w.Write([]byte(fmt.Sprintf(`{
				"work_id": %s,
				"work_name": "Book %s",
				"work_name_alts": ["Alt %s"],
				"work_year": 2020,
				"work_description": "Description %s",
				"image": "/img_%s.jpg",
				"authors": [{"name": "Author %s"}],
				"editions_blocks": {
					"30": {
						"list": [{"edition_id": 1000, "isbn": "isbn-%s"}]
					}
				}
			}`, workIDStr, workIDStr, workIDStr, workIDStr, workIDStr, workIDStr, workIDStr)))
		case strings.HasPrefix(path, "/edition/"):
			w.Write([]byte(`{"image": "/ed_cover.jpg"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	setupTestClient(t, &mockTransport{TargetURL: server.URL})
	provider := &FantLabProvider{}

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
			if len(results) == 0 {
				t.Errorf("Goroutine %d: expected results, got none", idx)
				return
			}
		}(i)
	}

	wg.Wait()
}

// TestFantLabStress_EdgeCases tests various edge cases in parsing and filtering.
func TestFantLabStress_EdgeCases(t *testing.T) {
	// We want to test:
	// 1. Search items limit (> 10 items in search response, sliced to 10).
	// 2. Filter work types (items with WorkTypeID in filterWorkTypes map are skipped).
	// 3. Author names are trimmed, and empty names are omitted.
	// 4. editions_blocks: fallback to block "10" if "30" is not present.
	// 5. editions_blocks: empty block lists or missing keys.
	// 6. Work year is 0, alts are empty.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case strings.HasPrefix(path, "/search-works"):
			// Return 15 search results.
			// ID 1 to 5: filtered types (e.g. type 7).
			// ID 6 to 15: normal types (type 1).
			var items []map[string]interface{}
			for i := 1; i <= 15; i++ {
				typeID := 1
				if i <= 5 {
					typeID = 7 // in filterWorkTypes
				}
				items = append(items, map[string]interface{}{
					"work_id":      i,
					"work_type_id": typeID,
				})
			}
			json.NewEncoder(w).Encode(items)

		case strings.HasPrefix(path, "/work/"):
			workIDStr := strings.TrimSuffix(strings.TrimPrefix(path, "/work/"), "/extended")
			workID, _ := strconv.Atoi(workIDStr)

			var authors []map[string]string
			var editionsBlocks map[string]interface{}
			var image string
			var workYear int
			var alts []string

			switch workID {
			case 6:
				// Empty and whitespace author name
				authors = []map[string]string{
					{"name": "   "},
					{"name": "Arkady Strugatsky"},
					{"name": ""},
				}
				// "30" editions block present but empty list
				editionsBlocks = map[string]interface{}{
					"30": map[string]interface{}{
						"list": []interface{}{},
					},
				}
				image = "/work6.jpg"
				workYear = 0 // empty year check
			case 7:
				// Fallback to "10" block
				authors = []map[string]string{{"name": "Some Author"}}
				editionsBlocks = map[string]interface{}{
					"10": map[string]interface{}{
						"list": []map[string]interface{}{
							{"edition_id": 701, "isbn": "isbn-701"},
							{"edition_id": 702, "isbn": "isbn-702"},
						},
					},
				}
				alts = []string{"Alt Title 7"}
				workYear = 2021
			default:
				// normal cases
				authors = []map[string]string{{"name": "Author"}}
				workYear = 2022
			}

			w.Write([]byte(fmt.Sprintf(`{
				"work_id": %d,
				"work_name": "Work %d",
				"work_name_alts": %s,
				"work_year": %d,
				"work_description": "Desc",
				"image": %q,
				"authors": %s,
				"editions_blocks": %s
			}`, workID, workID, marshalJSON(alts), workYear, image, marshalJSON(authors), marshalJSON(editionsBlocks))))

		case strings.HasPrefix(path, "/edition/"):
			editionIDStr := strings.TrimPrefix(path, "/edition/")
			if editionIDStr == "702" {
				w.Write([]byte(`{"image": "/ed_cover702.jpg"}`))
			} else {
				w.WriteHeader(http.StatusNotFound)
			}

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	setupTestClient(t, &mockTransport{TargetURL: server.URL})
	provider := &FantLabProvider{}

	results, err := provider.SearchBooks(context.Background(), "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// In search works response:
	// Total items returned: 15.
	// But it limits to first 10 items to prevent flooding: `searchItems = searchItems[:10]`.
	// The first 10 items correspond to IDs 1 to 10.
	// IDs 1 to 5 have type 7 (filtered). They are skipped.
	// IDs 6 to 10 have type 1 (not filtered). They should be processed.
	// IDs 11 to 15 are excluded because of the slice limit of 10.
	// So we expect exactly 5 results (IDs 6, 7, 8, 9, 10).
	if len(results) != 5 {
		t.Fatalf("expected 5 results, got %d", len(results))
	}

	// Verify ID 6 result details
	// Title: "Work 6", authors trimmed (only "Arkady Strugatsky"), empty cover fallback to image="/work6.jpg" -> "https://fantlab.ru/work6.jpg"
	var r6 *MetadataResult
	for _, r := range results {
		if r.Title == "Work 6" {
			r6 = r
			break
		}
	}
	if r6 == nil {
		t.Fatal("expected to find result for Work 6")
	}
	if len(r6.Authors) != 1 || r6.Authors[0] != "Arkady Strugatsky" {
		t.Errorf("Work 6: expected only 'Arkady Strugatsky', got %v", r6.Authors)
	}
	if r6.CoverURL != "https://fantlab.ru/work6.jpg" {
		t.Errorf("Work 6: expected cover fallback to work image, got %q", r6.CoverURL)
	}
	if r6.PublishedYear != "" {
		t.Errorf("Work 6: expected empty PublishedYear, got %q", r6.PublishedYear)
	}

	// Verify ID 7 result details
	// Should have used edition 702 (last element of list in block "10").
	// Cover: "/ed_cover702.jpg" -> "https://fantlab.ru/ed_cover702.jpg"
	// ISBN: "isbn-702"
	// Subtitle: "Alt Title 7"
	// PublishedYear: "2021"
	var r7 *MetadataResult
	for _, r := range results {
		if r.Title == "Work 7" {
			r7 = r
			break
		}
	}
	if r7 == nil {
		t.Fatal("expected to find result for Work 7")
	}
	if r7.CoverURL != "https://fantlab.ru/ed_cover702.jpg" {
		t.Errorf("Work 7: expected cover from edition 702, got %q", r7.CoverURL)
	}
	if r7.ISBN != "isbn-702" {
		t.Errorf("Work 7: expected ISBN 'isbn-702', got %q", r7.ISBN)
	}
	if r7.Subtitle != "Alt Title 7" {
		t.Errorf("Work 7: expected subtitle 'Alt Title 7', got %q", r7.Subtitle)
	}
	if r7.PublishedYear != "2021" {
		t.Errorf("Work 7: expected published year '2021', got %q", r7.PublishedYear)
	}
}

// TestFantLabStress_ContextTimeout verifies context cancellation/timeout handling.
func TestFantLabStress_ContextTimeout(t *testing.T) {
	t.Run("Context cancelled before search query", func(t *testing.T) {
		provider := &FantLabProvider{}
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // cancel immediately

		_, err := provider.SearchBooks(ctx, "test")
		if err == nil {
			t.Error("expected error due to cancelled context, got nil")
		}
	})

	t.Run("Context timeout during search query request", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(100 * time.Millisecond) // slow server
			w.Write([]byte(`[]`))
		}))
		defer server.Close()

		setupTestClient(t, &mockTransport{TargetURL: server.URL})
		provider := &FantLabProvider{}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()

		_, err := provider.SearchBooks(ctx, "test")
		if err == nil {
			t.Error("expected error due to timeout, got nil")
		} else if !strings.Contains(err.Error(), "context deadline exceeded") && !strings.Contains(err.Error(), "canceled") {
			t.Errorf("expected context error, got: %v", err)
		}
	})

	t.Run("Context cancellation during detail fetches", func(t *testing.T) {
		// Main search returns immediately, but detail fetches block/delay.
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path := r.URL.Path
			if strings.HasPrefix(path, "/search-works") {
				w.Write([]byte(`[{"work_id": 1, "work_type_id": 1}, {"work_id": 2, "work_type_id": 1}]`))
				return
			}
			if strings.HasPrefix(path, "/work/") {
				time.Sleep(200 * time.Millisecond) // slow response
				w.Write([]byte(`{}`))
				return
			}
		}))
		defer server.Close()

		setupTestClient(t, &mockTransport{TargetURL: server.URL})
		provider := &FantLabProvider{}

		ctx, cancel := context.WithCancel(context.Background())
		// Cancel the context after a tiny delay so search query completes, but details block.
		go func() {
			time.Sleep(15 * time.Millisecond)
			cancel()
		}()

		results, err := provider.SearchBooks(ctx, "test")
		// SearchBooks does not fail entirely if detail fetches fail/cancel, it just returns completed results.
		// Since detail fetches were cancelled, we expect 0 results.
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 0 {
			t.Errorf("expected 0 results due to cancellation, got %d", len(results))
		}
	})
}

// TestFantLabStress_MalformedResponses tests the robustness of the parser against invalid and error responses.
func TestFantLabStress_MalformedResponses(t *testing.T) {
	t.Run("search query invalid JSON", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{invalid json`))
		}))
		defer server.Close()

		setupTestClient(t, &mockTransport{TargetURL: server.URL})
		provider := &FantLabProvider{}

		_, err := provider.SearchBooks(context.Background(), "test")
		if err == nil {
			t.Error("expected error for malformed search JSON, got nil")
		}
	})

	t.Run("search query status 500 error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		setupTestClient(t, &mockTransport{TargetURL: server.URL})
		provider := &FantLabProvider{}

		_, err := provider.SearchBooks(context.Background(), "test")
		if err == nil {
			t.Error("expected error for HTTP 500, got nil")
		}
	})

	t.Run("detail fetch status 500 error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path := r.URL.Path
			if strings.HasPrefix(path, "/search-works") {
				w.Write([]byte(`[{"work_id": 1, "work_type_id": 1}]`))
			} else {
				w.WriteHeader(http.StatusInternalServerError)
			}
		}))
		defer server.Close()

		setupTestClient(t, &mockTransport{TargetURL: server.URL})
		provider := &FantLabProvider{}

		results, err := provider.SearchBooks(context.Background(), "test")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Fails silently for that item, returns empty slice.
		if len(results) != 0 {
			t.Errorf("expected 0 results, got %d", len(results))
		}
	})

	t.Run("detail fetch invalid JSON", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path := r.URL.Path
			if strings.HasPrefix(path, "/search-works") {
				w.Write([]byte(`[{"work_id": 1, "work_type_id": 1}]`))
			} else {
				w.Write([]byte(`{invalid`))
			}
		}))
		defer server.Close()

		setupTestClient(t, &mockTransport{TargetURL: server.URL})
		provider := &FantLabProvider{}

		results, err := provider.SearchBooks(context.Background(), "test")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 0 {
			t.Errorf("expected 0 results, got %d", len(results))
		}
	})

	t.Run("edition fetch 500 error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path := r.URL.Path
			switch {
			case strings.HasPrefix(path, "/search-works"):
				w.Write([]byte(`[{"work_id": 1, "work_type_id": 1}]`))
			case strings.HasPrefix(path, "/work/"):
				w.Write([]byte(`{
					"work_id": 1,
					"work_name": "Book 1",
					"image": "/img1.jpg",
					"editions_blocks": {
						"30": {
							"list": [{"edition_id": 1000, "isbn": "isbn-1000"}]
						}
					}
				}`))
			case strings.HasPrefix(path, "/edition/"):
				w.WriteHeader(http.StatusInternalServerError)
			}
		}))
		defer server.Close()

		setupTestClient(t, &mockTransport{TargetURL: server.URL})
		provider := &FantLabProvider{}

		results, err := provider.SearchBooks(context.Background(), "test")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}
		res := results[0]
		// Cover should fallback to work image
		if res.CoverURL != "https://fantlab.ru/img1.jpg" {
			t.Errorf("expected cover URL fallback, got %q", res.CoverURL)
		}
		if res.ISBN != "isbn-1000" {
			t.Errorf("expected ISBN isbn-1000, got %q", res.ISBN)
		}
	})

	t.Run("edition fetch invalid JSON", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path := r.URL.Path
			switch {
			case strings.HasPrefix(path, "/search-works"):
				w.Write([]byte(`[{"work_id": 1, "work_type_id": 1}]`))
			case strings.HasPrefix(path, "/work/"):
				w.Write([]byte(`{
					"work_id": 1,
					"work_name": "Book 1",
					"image": "/img1.jpg",
					"editions_blocks": {
						"30": {
							"list": [{"edition_id": 1000, "isbn": "isbn-1000"}]
						}
					}
				}`))
			case strings.HasPrefix(path, "/edition/"):
				w.Write([]byte(`{invalid`))
			}
		}))
		defer server.Close()

		setupTestClient(t, &mockTransport{TargetURL: server.URL})
		provider := &FantLabProvider{}

		results, err := provider.SearchBooks(context.Background(), "test")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}
		res := results[0]
		// Cover should fallback to work image
		if res.CoverURL != "https://fantlab.ru/img1.jpg" {
			t.Errorf("expected cover URL fallback, got %q", res.CoverURL)
		}
	})
}

// helper function to marshal JSON string or return blank
func marshalJSON(v interface{}) string {
	if v == nil {
		return "null"
	}
	b, _ := json.Marshal(v)
	return string(b)
}
