package finders

import (
	"context"
	"fmt"
	"strings"
	"sync"

	log "audiobookshelf/internal/logger"
	"audiobookshelf/internal/providers"
	"golang.org/x/sync/errgroup"
)

// SearchBooks searches for books using a specified provider (or all providers if providerName is empty/all).
func (f *Finder) SearchBooks(ctx context.Context, providerName, query string) ([]*providers.MetadataResult, error) {
	providerName = strings.ToLower(providerName)
	if providerName == "all" || providerName == "" {
		return f.searchAllBooks(ctx, query)
	}

	var p providers.Provider
	var ok bool

	if strings.HasPrefix(providerName, "custom-") {
		var err error
		p, err = f.getCustomProvider(ctx, providerName)
		if err != nil {
			return nil, err
		}
	} else {
		p, ok = f.providers[providerName]
		if !ok {
			if providerName == "fanlab" {
				p, ok = f.providers["fantlab"]
			} else if strings.HasPrefix(providerName, "audible.") {
				p, ok = f.providers["audible"]
			}
		}
		if !ok {
			return nil, fmt.Errorf("provider %q not found", providerName)
		}
	}

	return p.SearchBooks(ctx, query)
}

func (f *Finder) searchAllBooks(ctx context.Context, query string) ([]*providers.MetadataResult, error) {
	var (
		mu      sync.Mutex
		results []*providers.MetadataResult
	)

	g, egCtx := errgroup.WithContext(ctx)

	var allProvs []providers.Provider
	for _, p := range f.providers {
		allProvs = append(allProvs, p)
	}

	if customProvs, err := f.getCustomProviders(ctx, "book"); err == nil {
		allProvs = append(allProvs, customProvs...)
	} else {
		log.Printf("[Finders] searchAllBooks: failed to fetch custom providers: %v", err)
	}

	for _, p := range allProvs {
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
