package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// GetChaptersByASIN resolves chapters of an audiobook from Audnexus.
func (p *AudnexusProvider) GetChaptersByASIN(ctx context.Context, asin, region string) (interface{}, error) {
	if !isValidASIN(asin) {
		return nil, fmt.Errorf("invalid ASIN: %s", asin)
	}

	params := url.Values{}
	if region != "" {
		params.Set("region", region)
	}

	queryStr := ""
	if len(params) > 0 {
		queryStr = "?" + params.Encode()
	}

	urlStr := fmt.Sprintf("https://api.audnex.us/books/%s/chapters%s", url.PathEscape(strings.ToUpper(asin)), queryStr)
	resp, err := getWithRetry(ctx, safeHTTPClient, urlStr)
	if err != nil {
		return nil, fmt.Errorf("get request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("audnexus chapters returned status %d", resp.StatusCode)
	}

	var result interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	return result, nil
}
