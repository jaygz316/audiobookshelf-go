package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// parsePublishYear parses the published year from publish year or date.
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

// getWorksData retrieves detailed works data from OpenLibrary.
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
