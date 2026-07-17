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

type FantLabProvider struct{}

func (p *FantLabProvider) Name() string {
	return "fantlab"
}

func (p *FantLabProvider) SearchBooks(ctx context.Context, query string) ([]*MetadataResult, error) {
	if query == "" {
		return nil, nil
	}

	escapedQuery := url.QueryEscape(query)
	urlStr := fmt.Sprintf("https://api.fantlab.ru/search-works?q=%s&page=1&onlymatches=1", escapedQuery)

	resp, err := getWithRetry(ctx, safeHTTPClient, urlStr)
	if err != nil {
		return nil, fmt.Errorf("search works failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fantlab search returned status %d", resp.StatusCode)
	}

	var searchItems []fantLabSearchItem
	if err := json.NewDecoder(resp.Body).Decode(&searchItems); err != nil {
		return nil, fmt.Errorf("failed to decode search results: %w", err)
	}

	// Limit to first 10 items to prevent flooding
	if len(searchItems) > 10 {
		searchItems = searchItems[:10]
	}

	results := make([]*MetadataResult, len(searchItems))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 3) // Concurrency limit

	for i, item := range searchItems {
		wg.Add(1)
		go func(idx int, sItem fantLabSearchItem) {
			defer wg.Done()

			if filterWorkTypes[sItem.WorkTypeID] {
				return
			}

			select {
			case <-ctx.Done():
				return
			case sem <- struct{}{}:
			}
			defer func() { <-sem }()

			extendedUrl := fmt.Sprintf("https://api.fantlab.ru/work/%d/extended", sItem.WorkID)
			extResp, err := getWithRetry(ctx, safeHTTPClient, extendedUrl)
			if err != nil {
				return
			}
			defer extResp.Body.Close()

			if extResp.StatusCode != http.StatusOK {
				return
			}

			var bookData fantLabWorkExtended
			if err := json.NewDecoder(extResp.Body).Decode(&bookData); err != nil {
				return
			}

			// Clean book data
			var authorNames []string
			for _, au := range bookData.Authors {
				trimmed := strings.TrimSpace(au.Name)
				if trimmed != "" {
					authorNames = append(authorNames, trimmed)
				}
			}

			coverImg, isbn := p.tryGetCoverFromEditions(ctx, bookData.EditionsBlocks)
			if coverImg == "" {
				coverImg = bookData.Image
			}

			coverURL := ""
			if coverImg != "" {
				coverURL = "https://fantlab.ru" + coverImg
			}

			subtitle := ""
			if len(bookData.WorkNameAlts) > 0 {
				subtitle = bookData.WorkNameAlts[0]
			}

			publishedYear := ""
			if bookData.WorkYear > 0 {
				publishedYear = strconv.Itoa(bookData.WorkYear)
			}

			results[idx] = &MetadataResult{
				Title:         bookData.WorkName,
				Subtitle:      subtitle,
				Authors:       authorNames,
				PublishedYear: publishedYear,
				Description:   bookData.WorkDescription,
				CoverURL:      coverURL,
				ISBN:          isbn,
			}
		}(i, item)
	}
	wg.Wait()

	var cleaned []*MetadataResult
	for _, r := range results {
		if r != nil {
			cleaned = append(cleaned, r)
		}
	}
	return cleaned, nil
}

func (p *FantLabProvider) SearchPodcasts(ctx context.Context, query string) ([]*MetadataResult, error) {
	return nil, nil
}
