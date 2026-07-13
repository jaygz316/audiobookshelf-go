package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// CustomProvider implements the Provider interface for user-configured external metadata providers.
type CustomProvider struct {
	id              string
	name            string
	mediaType       string
	targetURL       string
	authHeaderValue string
}

// NewCustomProvider creates a new CustomProvider.
func NewCustomProvider(id, name, mediaType, targetURL, authHeaderValue string) *CustomProvider {
	return &CustomProvider{
		id:              id,
		name:            name,
		mediaType:       mediaType,
		targetURL:       targetURL,
		authHeaderValue: authHeaderValue,
	}
}

// Name returns the custom provider name.
func (c *CustomProvider) Name() string {
	return "custom-" + c.id
}

// SearchBooks queries the custom provider for books.
func (c *CustomProvider) SearchBooks(ctx context.Context, query string) ([]*MetadataResult, error) {
	if query == "" {
		return nil, nil
	}

	u, err := url.Parse(c.targetURL)
	if err != nil {
		return nil, fmt.Errorf("invalid custom provider URL: %w", err)
	}

	q := u.Query()
	q.Set("query", query)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if c.authHeaderValue != "" {
		req.Header.Set("Authorization", c.authHeaderValue)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Audiobookshelf-go Custom Metadata Provider)")

	httpClient := &http.Client{}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request to custom provider failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("custom provider returned status %d", resp.StatusCode)
	}

	var result struct {
		Matches []struct {
			Title         string   `json:"title"`
			Subtitle      string   `json:"subtitle"`
			Author        string   `json:"author"`
			Narrator      string   `json:"narrator"`
			Publisher     string   `json:"publisher"`
			PublishedYear string   `json:"publishedYear"`
			Description   string   `json:"description"`
			Cover         string   `json:"cover"`
			ISBN          string   `json:"isbn"`
			ASIN          string   `json:"asin"`
			Genres        []string `json:"genres"`
			Tags          []string `json:"tags"`
			Language      string   `json:"language"`
		} `json:"matches"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	var results []*MetadataResult
	for _, m := range result.Matches {
		var authors []string
		if m.Author != "" {
			authors = []string{m.Author}
		}
		var narrators []string
		if m.Narrator != "" {
			narrators = []string{m.Narrator}
		}

		results = append(results, &MetadataResult{
			Title:         m.Title,
			Subtitle:      m.Subtitle,
			Authors:       authors,
			Narrators:     narrators,
			Publisher:     m.Publisher,
			PublishedYear: m.PublishedYear,
			Description:   cleanDescription(m.Description),
			Language:      toTitle(m.Language),
			ISBN:          m.ISBN,
			ASIN:          m.ASIN,
			CoverURL:      m.Cover,
		})
	}

	return results, nil
}

// SearchPodcasts queries the custom provider for podcasts.
func (c *CustomProvider) SearchPodcasts(ctx context.Context, query string) ([]*MetadataResult, error) {
	if query == "" {
		return nil, nil
	}
	if c.mediaType != "podcast" {
		return nil, fmt.Errorf("provider %q is configured for books, not podcasts", c.name)
	}

	u, err := url.Parse(c.targetURL)
	if err != nil {
		return nil, fmt.Errorf("invalid custom provider URL: %w", err)
	}

	q := u.Query()
	q.Set("query", query)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if c.authHeaderValue != "" {
		req.Header.Set("Authorization", c.authHeaderValue)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Audiobookshelf-go Custom Metadata Provider)")

	httpClient := &http.Client{}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request to custom provider failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("custom provider returned status %d", resp.StatusCode)
	}

	var result struct {
		Matches []struct {
			Title         string `json:"title"`
			Subtitle      string `json:"subtitle"`
			Author        string `json:"author"`
			Publisher     string `json:"publisher"`
			PublishedYear string `json:"publishedYear"`
			Description   string `json:"description"`
			Cover         string `json:"cover"`
			Language      string `json:"language"`
		} `json:"matches"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	var results []*MetadataResult
	for _, m := range result.Matches {
		var authors []string
		if m.Author != "" {
			authors = []string{m.Author}
		}

		results = append(results, &MetadataResult{
			Title:         m.Title,
			Subtitle:      m.Subtitle,
			Authors:       authors,
			Publisher:     m.Publisher,
			PublishedYear: m.PublishedYear,
			Description:   cleanDescription(m.Description),
			Language:      toTitle(m.Language),
			CoverURL:      m.Cover,
		})
	}

	return results, nil
}
