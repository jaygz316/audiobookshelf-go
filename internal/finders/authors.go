package finders

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"audiobookshelf/internal/providers"
)

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
