package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// AudnexusProvider searches Audnexus API.
type AudnexusProvider struct{}

func (p *AudnexusProvider) Name() string {
	return "audnexus"
}

type audnexusAuthorOrNarrator struct {
	Name string `json:"name"`
}

type audnexusBookDetails struct {
	Title         string                     `json:"title"`
	Subtitle      string                     `json:"subtitle"`
	ASIN          string                     `json:"asin"`
	Authors       []audnexusAuthorOrNarrator `json:"authors"`
	Narrators     []audnexusAuthorOrNarrator `json:"narrators"`
	PublisherName string                     `json:"publisherName"`
	Summary       string                     `json:"summary"`
	ReleaseDate   string                     `json:"releaseDate"`
	Image         string                     `json:"image"`
	Language      string                     `json:"language"`
	ISBN          string                     `json:"isbn"`
}

type AudnexusAuthorASIN struct {
	ASIN string `json:"asin"`
	Name string `json:"name"`
}

type AudnexusAuthorDetails struct {
	ASIN        string `json:"asin"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Image       string `json:"image"`
}

func cleanAudnexusBook(item *audnexusBookDetails) *MetadataResult {
	var authors []string
	for _, a := range item.Authors {
		if a.Name != "" {
			authors = append(authors, a.Name)
		}
	}

	var narrators []string
	for _, n := range item.Narrators {
		if n.Name != "" {
			narrators = append(narrators, n.Name)
		}
	}

	publishedYear := ""
	if item.ReleaseDate != "" {
		parts := strings.Split(item.ReleaseDate, "-")
		if len(parts) > 0 {
			publishedYear = parts[0]
		}
	}

	lang := item.Language
	if lang != "" {
		lang = toTitle(lang)
	}

	return &MetadataResult{
		Title:         item.Title,
		Subtitle:      item.Subtitle,
		Authors:       authors,
		Narrators:     narrators,
		Publisher:     item.PublisherName,
		PublishedYear: publishedYear,
		Description:   item.Summary,
		Language:      lang,
		ISBN:          item.ISBN,
		ASIN:          item.ASIN,
		CoverURL:      item.Image,
	}
}

func (p *AudnexusProvider) asinSearch(ctx context.Context, asin string) (*audnexusBookDetails, error) {
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
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	return &details, nil
}

func (p *AudnexusProvider) SearchBooks(ctx context.Context, query string) ([]*MetadataResult, error) {
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
	}
	// PORT: Audnexus only supports book lookup by ASIN directly.
	return nil, nil
}

func (p *AudnexusProvider) SearchPodcasts(ctx context.Context, query string) ([]*MetadataResult, error) {
	return nil, nil
}

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

func levenshteinDistance(s1, s2 string) int {
	s1 = strings.ToLower(s1)
	s2 = strings.ToLower(s2)

	r1 := []rune(s1)
	r2 := []rune(s2)

	len1 := len(r1)
	len2 := len(r2)

	column := make([]int, len1+1)
	for y := 1; y <= len1; y++ {
		column[y] = y
	}

	for x := 1; x <= len2; x++ {
		column[0] = x
		lastkey := x - 1
		for y := 1; y <= len1; y++ {
			oldkey := column[y]
			var incr int
			if r1[y-1] != r2[x-1] {
				incr = 1
			}
			column[y] = minInt(column[y]+1, column[y-1]+1, lastkey+incr)
			lastkey = oldkey
		}
	}

	return column[len1]
}

func minInt(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}
