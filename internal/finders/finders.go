package finders

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"regexp"
	"strings"
	"sync"

	"audiobookshelf/internal/providers"
	"golang.org/x/sync/errgroup"
)

// Finder coordinates search requests across all registered providers.
type Finder struct {
	db        *sql.DB
	providers map[string]providers.Provider
}

// NewFinder initializes the finder with active provider backends.
func NewFinder(db *sql.DB, provs []providers.Provider) *Finder {
	m := make(map[string]providers.Provider)
	for _, p := range provs {
		if p != nil {
			m[strings.ToLower(p.Name())] = p
		}
	}
	return &Finder{db: db, providers: m}
}

// getCustomProvider retrieves a custom metadata provider's config from the database.
func (f *Finder) getCustomProvider(ctx context.Context, providerName string) (providers.Provider, error) {
	if f.db == nil {
		return nil, fmt.Errorf("database connection not available in finder")
	}

	id := strings.TrimPrefix(providerName, "custom-")

	var name, mediaType, urlStr string
	var authHeaderVal sql.NullString
	err := f.db.QueryRowContext(ctx, "SELECT name, mediaType, url, authHeaderValue FROM customMetadataProviders WHERE id = ?", id).
		Scan(&name, &mediaType, &urlStr, &authHeaderVal)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("custom provider %q not found in database", providerName)
	} else if err != nil {
		return nil, fmt.Errorf("failed to query custom provider: %w", err)
	}

	var authStr string
	if authHeaderVal.Valid {
		authStr = authHeaderVal.String
	}

	return providers.NewCustomProvider(id, name, mediaType, urlStr, authStr), nil
}

// getCustomProviders retrieves all custom metadata providers of the given mediaType from the database.
func (f *Finder) getCustomProviders(ctx context.Context, mediaType string) ([]providers.Provider, error) {
	if f.db == nil {
		return nil, nil
	}

	rows, err := f.db.QueryContext(ctx, "SELECT id, name, mediaType, url, authHeaderValue FROM customMetadataProviders WHERE mediaType = ?", mediaType)
	if err != nil {
		return nil, fmt.Errorf("failed to query custom providers: %w", err)
	}
	defer rows.Close()

	var list []providers.Provider
	for rows.Next() {
		var id, name, mType, urlStr string
		var authHeaderVal sql.NullString
		if err := rows.Scan(&id, &name, &mType, &urlStr, &authHeaderVal); err != nil {
			log.Printf("[Finders] Failed to scan custom provider: %v", err)
			continue
		}
		var authStr string
		if authHeaderVal.Valid {
			authStr = authHeaderVal.String
		}
		list = append(list, providers.NewCustomProvider(id, name, mType, urlStr, authStr))
	}
	return list, rows.Err()
}

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

// SearchPodcasts searches for podcasts using a specified provider.
func (f *Finder) SearchPodcasts(ctx context.Context, providerName, query string) ([]*providers.MetadataResult, error) {
	providerName = strings.ToLower(providerName)
	if providerName == "all" || providerName == "" {
		return f.searchAllPodcasts(ctx, query)
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

	return p.SearchPodcasts(ctx, query)
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

func (f *Finder) searchAllPodcasts(ctx context.Context, query string) ([]*providers.MetadataResult, error) {
	var (
		mu      sync.Mutex
		results []*providers.MetadataResult
	)

	g, egCtx := errgroup.WithContext(ctx)

	var allProvs []providers.Provider
	for _, p := range f.providers {
		allProvs = append(allProvs, p)
	}

	if customProvs, err := f.getCustomProviders(ctx, "podcast"); err == nil {
		allProvs = append(allProvs, customProvs...)
	} else {
		log.Printf("[Finders] searchAllPodcasts: failed to fetch custom providers: %v", err)
	}

	for _, p := range allProvs {
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

var asinRegex = regexp.MustCompile(`^[A-Z0-9]{10}$`)

func isValidASIN(str string) bool {
	return asinRegex.MatchString(strings.ToUpper(str))
}

// SearchAuthors searches for authors using Audnexus (or future providers).
func (f *Finder) SearchAuthors(ctx context.Context, providerName, query string) ([]*providers.AudnexusAuthorDetails, error) {
	// Only audnexus supports author matching right now
	prov, ok := f.providers["audnexus"]
	if !ok {
		return nil, fmt.Errorf("audnexus provider not registered")
	}
	audnexus, ok := prov.(*providers.AudnexusProvider)
	if !ok {
		return nil, fmt.Errorf("failed to cast to AudnexusProvider")
	}

	trimmedQuery := strings.TrimSpace(query)
	if isValidASIN(trimmedQuery) {
		details, err := audnexus.AuthorRequest(ctx, strings.ToUpper(trimmedQuery), "")
		if err == nil && details != nil {
			return []*providers.AudnexusAuthorDetails{details}, nil
		}
	}

	asins, err := audnexus.AuthorASINsRequest(ctx, query, "")
	if err != nil {
		return nil, err
	}

	if len(asins) > 5 {
		asins = asins[:5]
	}

	results := make([]*providers.AudnexusAuthorDetails, len(asins))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 3)

	for i, a := range asins {
		wg.Add(1)
		go func(idx int, authorName, asin string) {
			defer wg.Done()
			select {
			case <-ctx.Done():
				return
			case sem <- struct{}{}:
			}
			defer func() { <-sem }()

			details, err := audnexus.AuthorRequest(ctx, asin, "")
			if err == nil && details != nil {
				results[idx] = details
			} else {
				// fallback with name/asin if details request failed
				results[idx] = &providers.AudnexusAuthorDetails{
					ASIN: asin,
					Name: authorName,
				}
			}
		}(i, a.Name, a.ASIN)
	}
	wg.Wait()

	var cleaned []*providers.AudnexusAuthorDetails
	for _, r := range results {
		if r != nil {
			cleaned = append(cleaned, r)
		}
	}
	return cleaned, nil
}

// Providers returns the map of registered providers.
func (f *Finder) Providers() map[string]providers.Provider {
	return f.providers
}


