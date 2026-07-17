package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// AuthorASINsRequest gets author ASIN info by author name.
func (p *AudnexusProvider) AuthorASINsRequest(ctx context.Context, name, region string) ([]*AudnexusAuthorASIN, error) {
	params := url.Values{}
	params.Set("name", name)
	if region != "" {
		params.Set("region", region)
	}

	urlStr := fmt.Sprintf("https://api.audnex.us/authors?%s", params.Encode())
	resp, err := getWithRetry(ctx, safeHTTPClient, urlStr)
	if err != nil {
		return nil, fmt.Errorf("get request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("audnexus author request returned status %d", resp.StatusCode)
	}

	var results []*AudnexusAuthorASIN
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	return results, nil
}

// AuthorRequest queries detailed author metadata from Audnexus.
func (p *AudnexusProvider) AuthorRequest(ctx context.Context, asin, region string) (*AudnexusAuthorDetails, error) {
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

	urlStr := fmt.Sprintf("https://api.audnex.us/authors/%s%s", url.PathEscape(strings.ToUpper(asin)), queryStr)
	resp, err := getWithRetry(ctx, safeHTTPClient, urlStr)
	if err != nil {
		return nil, fmt.Errorf("get request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("audnexus author details returned status %d", resp.StatusCode)
	}

	var details AudnexusAuthorDetails
	if err := json.NewDecoder(resp.Body).Decode(&details); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	return &details, nil
}

// FindAuthorByASIN resolves author details by ASIN.
func (p *AudnexusProvider) FindAuthorByASIN(ctx context.Context, asin, region string) (*AudnexusAuthorDetails, error) {
	return p.AuthorRequest(ctx, asin, region)
}

// FindAuthorByName searches for author by name and finds closest Levenshtein match.
func (p *AudnexusProvider) FindAuthorByName(ctx context.Context, name, region string, maxLevenshtein int) (*AudnexusAuthorDetails, error) {
	authorAsins, err := p.AuthorASINsRequest(ctx, name, region)
	if err != nil {
		return nil, fmt.Errorf("failed to search author ASINs: %w", err)
	}

	var closestMatch *AudnexusAuthorASIN
	bestDist := -1

	for _, item := range authorAsins {
		dist := levenshteinDistance(item.Name, name)
		if bestDist == -1 || dist < bestDist {
			bestDist = dist
			closestMatch = item
		}
	}

	if closestMatch == nil || (maxLevenshtein >= 0 && bestDist > maxLevenshtein) {
		return nil, nil
	}

	return p.AuthorRequest(ctx, closestMatch.ASIN, region)
}
