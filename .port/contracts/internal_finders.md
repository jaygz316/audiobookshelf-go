# Package internal/finders

This package coordinates searching across metadata providers.

## Go Signatures

```go
package finders

import (
	"context"
	"audiobookshelf/internal/providers"
)

type Finder struct {
	providers map[string]providers.Provider
}

// NewFinder initializes the finder with active provider backends.
func NewFinder(provs []providers.Provider) *Finder

// SearchBooks searches for books using a specified provider (or all providers if providerName is empty/all).
func (f *Finder) SearchBooks(ctx context.Context, providerName, query string) ([]*providers.MetadataResult, error)

// SearchPodcasts searches for podcasts using a specified provider.
func (f *Finder) SearchPodcasts(ctx context.Context, providerName, query string) ([]*providers.MetadataResult, error)
```

## Behavioral Notes
- **NewFinder**: Registers metadata providers into an internal map keyed by their provider name.
- **SearchBooks/SearchPodcasts**: Maps `providerName` to a registered provider. If `providerName` is "all" or blank, runs searches across all registered providers concurrently using a `errgroup.Group` or goroutines, consolidating the results.
