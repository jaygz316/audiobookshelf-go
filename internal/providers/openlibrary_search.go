package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"sync"
)

// SearchBooks searches the OpenLibrary catalog for books matching query.
func (p *OpenLibraryProvider) SearchBooks(ctx context.Context, query string) ([]*MetadataResult, error) {
	if query == "" {
		return nil, nil
	}

	escapedQuery := url.QueryEscape(query)
	urlStr := fmt.Sprintf("https://openlibrary.org/search.json?title=%s&fields=key,title,subtitle,author_name,cover_edition_key,first_publish_year,isbn,language,publisher", escapedQuery)

	resp, err := getWithRetry(ctx, safeHTTPClient, urlStr)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("open library api returned status %d", resp.StatusCode)
	}

	var searchResp openLibraryResponse
	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	docs := searchResp.Docs
	if len(docs) > 10 {
		docs = docs[:10]
	}

	results := make([]*MetadataResult, len(docs))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 5)

	for i, doc := range docs {
		wg.Add(1)
		go func(idx int, d *openLibraryDoc) {
			defer wg.Done()
			select {
			case <-ctx.Done():
				return
			case sem <- struct{}{}:
			}
			defer func() { <-sem }()

			results[idx] = p.processBookResult(ctx, d)
		}(i, doc)
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

// processBookResult processes a single book document from OpenLibrary search results.
func (p *OpenLibraryProvider) processBookResult(ctx context.Context, d *openLibraryDoc) *MetadataResult {
	worksData, err := p.getWorksData(ctx, d.Key)
	if err != nil {
		// PORT: Suppress detail lookup error, fallback to catalog info
		worksData = &openLibraryWorksData{}
	}

	var coverImages []string
	for _, c := range worksData.Covers {
		if c > 0 {
			coverImages = append(coverImages, "https://covers.openlibrary.org/b/id/"+strconv.Itoa(c)+"-L.jpg")
		}
	}

	var description string
	if worksData.Description != nil {
		if str, ok := worksData.Description.(string); ok {
			description = cleanDescription(str)
		} else if m, ok := worksData.Description.(map[string]interface{}); ok {
			if val, ok := m["value"].(string); ok {
				description = cleanDescription(val)
			}
		}
	}

	coverURL := ""
	if d.CoverEditionKey != "" {
		coverURL = "https://covers.openlibrary.org/b/OLID/" + d.CoverEditionKey + "-L.jpg"
	} else if len(coverImages) > 0 {
		coverURL = coverImages[0]
	}

	publisher := ""
	if len(d.Publisher) > 0 {
		publisher = d.Publisher[0]
	}

	language := ""
	if len(d.Language) > 0 {
		language = toTitle(d.Language[0])
	}

	isbn := ""
	if len(d.ISBN) > 0 {
		isbn = d.ISBN[0]
	}

	return &MetadataResult{
		Title:         d.Title,
		Subtitle:      d.Subtitle,
		Authors:       d.AuthorName,
		PublishedYear: parsePublishYear(d.FirstPublishYear, worksData.FirstPublishDate),
		Publisher:     publisher,
		Language:      language,
		ISBN:          isbn,
		Description:   description,
		CoverURL:      coverURL,
	}
}
