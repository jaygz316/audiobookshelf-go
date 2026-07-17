package finders

import (
	"database/sql"
	"strings"

	"audiobookshelf/internal/providers"
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

// Providers returns the map of registered providers.
func (f *Finder) Providers() map[string]providers.Provider {
	return f.providers
}
