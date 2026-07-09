package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// GoogleBooksProvider searches Google Books API.
type GoogleBooksProvider struct{}

func (p *GoogleBooksProvider) Name() string {
	return "google"
}

type googleIndustryIdentifier struct {
	Type       string `json:"type"`
	Identifier string `json:"identifier"`
}

type googleImageLinks struct {
	ExtraLarge     string `json:"extraLarge"`
	Large          string `json:"large"`
	Medium         string `json:"medium"`
	Small          string `json:"small"`
	Thumbnail      string `json:"thumbnail"`
	SmallThumbnail string `json:"smallThumbnail"`
}

type googleVolumeInfo struct {
	Title               string                     `json:"title"`
	Subtitle            string                     `json:"subtitle"`
	Authors             []string                   `json:"authors"`
	Publisher           string                     `json:"publisher"`
	PublishedDate       string                     `json:"publishedDate"`
	Description         string                     `json:"description"`
	IndustryIdentifiers []googleIndustryIdentifier `json:"industryIdentifiers"`
	Categories          []string                   `json:"categories"`
	ImageLinks          googleImageLinks           `json:"imageLinks"`
	Language            string                     `json:"language"`
}

type googleVolumeItem struct {
	ID         string           `json:"id"`
	VolumeInfo googleVolumeInfo `json:"volumeInfo"`
}

type googleSearchResponse struct {
	Items []googleVolumeItem `json:"items"`
}

func (p *GoogleBooksProvider) SearchBooks(ctx context.Context, query string) ([]*MetadataResult, error) {
	if query == "" {
		return nil, nil
	}

	escapedQuery := url.QueryEscape(query)
	urlStr := fmt.Sprintf("https://www.googleapis.com/books/v1/volumes?q=%s", escapedQuery)

	resp, err := getWithRetry(ctx, safeHTTPClient, urlStr)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("google books api returned status %d", resp.StatusCode)
	}

	var searchResp googleSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	var results []*MetadataResult
	for _, item := range searchResp.Items {
		info := item.VolumeInfo

		// Extract ISBN
		isbn := ""
		for _, identifier := range info.IndustryIdentifiers {
			if identifier.Type == "ISBN_13" {
				isbn = identifier.Identifier
				break
			}
		}
		if isbn == "" {
			for _, identifier := range info.IndustryIdentifiers {
				if identifier.Type == "ISBN_10" {
					isbn = identifier.Identifier
					break
				}
			}
		}

		// Select largest cover
		cover := ""
		if info.ImageLinks.ExtraLarge != "" {
			cover = info.ImageLinks.ExtraLarge
		} else if info.ImageLinks.Large != "" {
			cover = info.ImageLinks.Large
		} else if info.ImageLinks.Medium != "" {
			cover = info.ImageLinks.Medium
		} else if info.ImageLinks.Small != "" {
			cover = info.ImageLinks.Small
		} else if info.ImageLinks.Thumbnail != "" {
			cover = info.ImageLinks.Thumbnail
		} else if info.ImageLinks.SmallThumbnail != "" {
			cover = info.ImageLinks.SmallThumbnail
		}

		if cover != "" {
			cover = strings.Replace(cover, "http:", "https:", 1)
		}

		publishedYear := ""
		if info.PublishedDate != "" {
			parts := strings.Split(info.PublishedDate, "-")
			if len(parts) > 0 {
				publishedYear = parts[0]
			}
		}

		lang := info.Language
		if lang != "" {
			lang = toTitle(lang)
		}

		results = append(results, &MetadataResult{
			Title:         info.Title,
			Subtitle:      info.Subtitle,
			Authors:       info.Authors,
			Publisher:     info.Publisher,
			PublishedYear: publishedYear,
			Description:   info.Description,
			Language:      lang,
			ISBN:          isbn,
			CoverURL:      cover,
		})
	}

	return results, nil
}

func (p *GoogleBooksProvider) SearchPodcasts(ctx context.Context, query string) ([]*MetadataResult, error) {
	return nil, nil
}
