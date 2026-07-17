package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// OpenLibraryProvider searches OpenLibrary API.
type OpenLibraryProvider struct{}

// Name returns the provider's identifier.
func (p *OpenLibraryProvider) Name() string {
	return "openlibrary"
}

// SearchPodcasts is not supported by OpenLibrary.
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
