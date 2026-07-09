package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
)

// OpenLibraryProvider searches OpenLibrary API.
type OpenLibraryProvider struct{}

func (p *OpenLibraryProvider) Name() string {
	return "openlibrary"
}

type openLibraryDoc struct {
	Key              string   `json:"key"`
	Title            string   `json:"title"`
	Subtitle         string   `json:"subtitle"`
	AuthorName       []string `json:"author_name"`
	CoverEditionKey  string   `json:"cover_edition_key"`
	FirstPublishYear int      `json:"first_publish_year"`
	ISBN             []string `json:"isbn"`
	Language         []string `json:"language"`
	Publisher        []string `json:"publisher"`
}

type openLibraryResponse struct {
	Docs []*openLibraryDoc `json:"docs"`
}

type openLibraryWorksData struct {
	Covers           []int       `json:"covers"`
	FirstPublishDate string      `json:"first_publish_date"`
	Description      interface{} `json:"description"`
}

func parsePublishYear(firstPublishYear int, firstPublishDate string) string {
	if firstPublishYear > 0 {
		return strconv.Itoa(firstPublishYear)
	}
	if firstPublishDate != "" {
		parts := strings.Split(firstPublishDate, "-")
		if len(parts) > 0 {
			if _, err := strconv.Atoi(parts[0]); err == nil {
				return parts[0]
			}
		}
	}
	return ""
}

func (p *OpenLibraryProvider) getWorksData(ctx context.Context, worksKey string) (*openLibraryWorksData, error) {
	urlStr := fmt.Sprintf("https://openlibrary.org%s.json", worksKey)
	resp, err := getWithRetry(ctx, safeHTTPClient, urlStr)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("open library works request returned status %d", resp.StatusCode)
	}

	var data openLibraryWorksData
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	return &data, nil
}

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

			results[idx] = &MetadataResult{
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

func (p *OpenLibraryProvider) SearchPodcasts(ctx context.Context, query string) ([]*MetadataResult, error) {
	return nil, nil
}

// IsbnLookup searches for works detailed info using an ISBN.
func (p *OpenLibraryProvider) IsbnLookup(ctx context.Context, isbn string) (interface{}, error) {
	urlStr := fmt.Sprintf("https://openlibrary.org/isbn/%s.json", url.PathEscape(isbn))
	resp, err := getWithRetry(ctx, safeHTTPClient, urlStr)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("isbn lookup returned status %d", resp.StatusCode)
	}

	var result interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	return result, nil
}
