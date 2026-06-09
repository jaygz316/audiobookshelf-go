package finders

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"

	"audiobookshelf/internal/providers"
	"golang.org/x/sync/errgroup"
)

// Finder coordinates search requests across all registered providers.
type Finder struct {
	providers map[string]providers.Provider
}

// NewFinder initializes the finder with active provider backends.
func NewFinder(provs []providers.Provider) *Finder {
	m := make(map[string]providers.Provider)
	for _, p := range provs {
		if p != nil {
			m[strings.ToLower(p.Name())] = p
		}
	}
	return &Finder{providers: m}
}

// SearchBooks searches for books using a specified provider (or all providers if providerName is empty/all).
func (f *Finder) SearchBooks(ctx context.Context, providerName, query string) ([]*providers.MetadataResult, error) {
	providerName = strings.ToLower(providerName)
	if providerName == "all" || providerName == "" {
		return f.searchAllBooks(ctx, query)
	}

	p, ok := f.providers[providerName]
	if !ok {
		// PORT: Fallback for region-specific Audible provider names like "audible.ca"
		if strings.HasPrefix(providerName, "audible.") {
			p, ok = f.providers["audible"]
		}
	}
	if !ok {
		return nil, fmt.Errorf("provider %q not found", providerName)
	}
	return p.SearchBooks(ctx, query)
}

// SearchPodcasts searches for podcasts using a specified provider.
func (f *Finder) SearchPodcasts(ctx context.Context, providerName, query string) ([]*providers.MetadataResult, error) {
	providerName = strings.ToLower(providerName)
	if providerName == "all" || providerName == "" {
		return f.searchAllPodcasts(ctx, query)
	}

	p, ok := f.providers[providerName]
	if !ok {
		// PORT: Fallback for region-specific Audible provider names like "audible.ca"
		if strings.HasPrefix(providerName, "audible.") {
			p, ok = f.providers["audible"]
		}
	}
	if !ok {
		return nil, fmt.Errorf("provider %q not found", providerName)
	}
	return p.SearchPodcasts(ctx, query)
}

func (f *Finder) searchAllBooks(ctx context.Context, query string) ([]*providers.MetadataResult, error) {
	var (
		mu      sync.Mutex
		results []*providers.MetadataResult
	)

	g, egCtx := errgroup.WithContext(ctx)

	for _, p := range f.providers {
		prov := p
		g.Go(func() error {
			res, err := prov.SearchBooks(egCtx, query)
			if err != nil {
				// PORT: In concurrent search, fail-silent behavior allows results from other
				// healthy providers to return. We log the error but return nil.
				if egCtx.Err() == nil {
					log.Printf("[Finders] provider %s SearchBooks failed: %v", prov.Name(), err)
				}
				return nil
			}
			if len(res) > 0 {
				mu.Lock()
				results = append(results, res...)
				mu.Unlock()
			}
			return nil
		})
	}

	_ = g.Wait()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return results, nil
}

func (f *Finder) searchAllPodcasts(ctx context.Context, query string) ([]*providers.MetadataResult, error) {
	var (
		mu      sync.Mutex
		results []*providers.MetadataResult
	)

	g, egCtx := errgroup.WithContext(ctx)

	for _, p := range f.providers {
		prov := p
		g.Go(func() error {
			res, err := prov.SearchPodcasts(egCtx, query)
			if err != nil {
				// PORT: In concurrent search, fail-silent behavior allows results from other
				// healthy providers to return. We log the error but return nil.
				if egCtx.Err() == nil {
					log.Printf("[Finders] provider %s SearchPodcasts failed: %v", prov.Name(), err)
				}
				return nil
			}
			if len(res) > 0 {
				mu.Lock()
				results = append(results, res...)
				mu.Unlock()
			}
			return nil
		})
	}

	_ = g.Wait()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return results, nil
}
