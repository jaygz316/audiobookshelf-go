package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

// AudibleProvider searches Audible.com API.
type AudibleProvider struct{}

func (p *AudibleProvider) Name() string {
	return "audible"
}

type audibleProduct struct {
	ASIN string `json:"asin"`
}

type audibleCatalogResponse struct {
	Products []audibleProduct `json:"products"`
}

func (p *AudibleProvider) asinSearch(ctx context.Context, asin string) (*audnexusBookDetails, error) {
	urlStr := fmt.Sprintf("https://api.audnex.us/books/%s", url.PathEscape(strings.ToUpper(asin)))
	resp, err := getWithRetry(ctx, safeHTTPClient, urlStr)
	if err != nil {
		return nil, fmt.Errorf("get request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("audnexus returned status %d", resp.StatusCode)
	}

	var details audnexusBookDetails
	if err := json.NewDecoder(resp.Body).Decode(&details); err != nil {
		return nil, fmt.Errorf("failed to decode audnexus response: %w", err)
	}
	return &details, nil
}

func (p *AudibleProvider) SearchBooks(ctx context.Context, query string) ([]*MetadataResult, error) {
	if query == "" {
		return nil, nil
	}

	if isValidASIN(query) {
		book, err := p.asinSearch(ctx, query)
		if err != nil {
			return nil, fmt.Errorf("asin search failed: %w", err)
		}
		if book != nil {
			return []*MetadataResult{cleanAudnexusBook(book)}, nil
		}
		return nil, nil
	}

	escapedQuery := url.QueryEscape(query)
	urlStr := fmt.Sprintf("https://api.audible.com/1.0/catalog/products?num_results=10&products_sort_by=Relevance&title=%s", escapedQuery)

	req, err := http.NewRequestWithContext(ctx, "GET", urlStr, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := safeHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("audible api returned status %d", resp.StatusCode)
	}

	var catResp audibleCatalogResponse
	if err := json.NewDecoder(resp.Body).Decode(&catResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	products := catResp.Products
	if len(products) > 10 {
		products = products[:10]
	}

	results := make([]*MetadataResult, len(products))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 5)

	for i, prod := range products {
		wg.Add(1)
		go func(idx int, asin string) {
			defer wg.Done()
			select {
			case <-ctx.Done():
				return
			case sem <- struct{}{}:
			}
			defer func() { <-sem }()

			book, err := p.asinSearch(ctx, asin)
			if err == nil && book != nil {
				results[idx] = cleanAudnexusBook(book)
			}
		}(i, prod.ASIN)
	}
	wg.Wait()

	var cleanedResults []*MetadataResult
	for _, r := range results {
		if r != nil {
			cleanedResults = append(cleanedResults, r)
		}
	}

	return cleanedResults, nil
}

func (p *AudibleProvider) SearchPodcasts(ctx context.Context, query string) ([]*MetadataResult, error) {
	return nil, nil
}
