package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// ITunesProvider searches iTunes API.
type ITunesProvider struct{}

func (p *ITunesProvider) Name() string {
	return "itunes"
}

type iTunesResponse struct {
	Results []map[string]interface{} `json:"results"`
}

func getCoverArtwork(data map[string]interface{}) string {
	if val, ok := data["artworkUrl600"].(string); ok && val != "" {
		return val
	}

	type artwork struct {
		url  string
		size int
	}
	var artworks []artwork

	for k, v := range data {
		if strings.HasPrefix(k, "artworkUrl") {
			sizeStr := strings.TrimPrefix(k, "artworkUrl")
			if size, err := strconv.Atoi(sizeStr); err == nil {
				if urlStr, ok := v.(string); ok && urlStr != "" {
					artworks = append(artworks, artwork{url: urlStr, size: size})
				}
			}
		}
	}

	if len(artworks) == 0 {
		return ""
	}

	sort.Slice(artworks, func(i, j int) bool {
		return artworks[i].size < artworks[j].size
	})

	for _, a := range artworks {
		if a.size > 600 {
			return a.url
		}
	}

	for _, a := range artworks {
		sizePattern := fmt.Sprintf("%dx%dbb", a.size, a.size)
		if strings.Contains(a.url, sizePattern) {
			return strings.Replace(a.url, sizePattern, "600x600bb", 1)
		}
	}

	return artworks[len(artworks)-1].url
}

func (p *ITunesProvider) search(ctx context.Context, term, media, entity string) ([]map[string]interface{}, error) {
	params := url.Values{}
	params.Set("term", term)
	params.Set("media", media)
	params.Set("entity", entity)

	urlStr := fmt.Sprintf("https://itunes.apple.com/search?%s", params.Encode())

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
		return nil, fmt.Errorf("itunes api returned status %d", resp.StatusCode)
	}

	var iTunesResp iTunesResponse
	if err := json.NewDecoder(resp.Body).Decode(&iTunesResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return iTunesResp.Results, nil
}

func (p *ITunesProvider) SearchBooks(ctx context.Context, query string) ([]*MetadataResult, error) {
	if query == "" {
		return nil, nil
	}

	results, err := p.search(ctx, query, "audiobook", "audiobook")
	if err != nil {
		return nil, fmt.Errorf("itunes search failed: %w", err)
	}

	var cleaned []*MetadataResult
	for _, data := range results {
		title, _ := data["collectionName"].(string)

		artistName, _ := data["artistName"].(string)
		authorRaw := strings.ReplaceAll(artistName, " & ", ", ")
		var authors []string
		if authorRaw != "" {
			for _, a := range strings.Split(authorRaw, ", ") {
				trimmed := strings.TrimSpace(a)
				if trimmed != "" {
					authors = append(authors, trimmed)
				}
			}
		}

		desc, _ := data["description"].(string)

		publishedYear := ""
		if releaseDate, ok := data["releaseDate"].(string); ok && releaseDate != "" {
			parts := strings.Split(releaseDate, "-")
			if len(parts) > 0 {
				publishedYear = parts[0]
			}
		}

		publisher, _ := data["publisher"].(string)

		cleaned = append(cleaned, &MetadataResult{
			Title:         title,
			Authors:       authors,
			Description:   cleanDescription(desc),
			PublishedYear: publishedYear,
			Publisher:     publisher,
			CoverURL:      getCoverArtwork(data),
		})
	}

	return cleaned, nil
}

func (p *ITunesProvider) SearchPodcasts(ctx context.Context, query string) ([]*MetadataResult, error) {
	if query == "" {
		return nil, nil
	}

	results, err := p.search(ctx, query, "podcast", "podcast")
	if err != nil {
		return nil, fmt.Errorf("itunes search failed: %w", err)
	}

	var cleaned []*MetadataResult
	for _, data := range results {
		title, _ := data["collectionName"].(string)

		artistName, _ := data["artistName"].(string)
		var authors []string
		if artistName != "" {
			authors = append(authors, artistName)
		}

		desc, _ := data["description"].(string)

		publishedYear := ""
		if releaseDate, ok := data["releaseDate"].(string); ok && releaseDate != "" {
			parts := strings.Split(releaseDate, "-")
			if len(parts) > 0 {
				publishedYear = parts[0]
			}
		}

		cleaned = append(cleaned, &MetadataResult{
			Title:         title,
			Authors:       authors,
			Description:   cleanDescription(desc),
			PublishedYear: publishedYear,
			CoverURL:      getCoverArtwork(data),
		})
	}

	return cleaned, nil
}
