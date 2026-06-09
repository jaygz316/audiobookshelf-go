# Package internal/providers

This package provides clients for querying external metadata providers (Audible, Google Books, Open Library, Audnexus, iTunes).

## Go Signatures

```go
package providers

import (
	"context"
)

type MetadataResult struct {
	Title         string   `json:"title"`
	Subtitle      string   `json:"subtitle"`
	Authors       []string `json:"authors"`
	Narrators     []string `json:"narrators"`
	Publisher     string   `json:"publisher"`
	PublishedYear string   `json:"publishedYear"`
	Description   string   `json:"description"`
	Language      string   `json:"language"`
	ISBN          string   `json:"isbn"`
	ASIN          string   `json:"asin"`
	CoverURL      string   `json:"coverUrl"`
}

type Provider interface {
	Name() string
	SearchBooks(ctx context.Context, query string) ([]*MetadataResult, error)
	SearchPodcasts(ctx context.Context, query string) ([]*MetadataResult, error)
}

// AudibleProvider searches Audible.com API.
type AudibleProvider struct{}
func (p *AudibleProvider) Name() string
func (p *AudibleProvider) SearchBooks(ctx context.Context, query string) ([]*MetadataResult, error)
func (p *AudibleProvider) SearchPodcasts(ctx context.Context, query string) ([]*MetadataResult, error)

// GoogleBooksProvider searches Google Books API.
type GoogleBooksProvider struct{}
func (p *GoogleBooksProvider) Name() string
func (p *GoogleBooksProvider) SearchBooks(ctx context.Context, query string) ([]*MetadataResult, error)
func (p *GoogleBooksProvider) SearchPodcasts(ctx context.Context, query string) ([]*MetadataResult, error)

// OpenLibraryProvider searches OpenLibrary API.
type OpenLibraryProvider struct{}
func (p *OpenLibraryProvider) Name() string
func (p *OpenLibraryProvider) SearchBooks(ctx context.Context, query string) ([]*MetadataResult, error)
func (p *OpenLibraryProvider) SearchPodcasts(ctx context.Context, query string) ([]*MetadataResult, error)

// AudnexusProvider searches Audnexus API.
type AudnexusProvider struct{}
func (p *AudnexusProvider) Name() string
func (p *AudnexusProvider) SearchBooks(ctx context.Context, query string) ([]*MetadataResult, error)
func (p *AudnexusProvider) SearchPodcasts(ctx context.Context, query string) ([]*MetadataResult, error)

// ITunesProvider searches iTunes API.
type ITunesProvider struct{}
func (p *ITunesProvider) Name() string
func (p *ITunesProvider) SearchBooks(ctx context.Context, query string) ([]*MetadataResult, error)
func (p *ITunesProvider) SearchPodcasts(ctx context.Context, query string) ([]*MetadataResult, error)
```

## Behavioral Notes
- **SearchBooks/SearchPodcasts**: Should perform GET HTTP queries to the external APIs, handle network timeouts using context deadlines, parse JSON/XML responses, and return standardized `MetadataResult` items.
- **Provider Names**: Constant strings: "audible", "google", "openlibrary", "audnexus", "itunes".
