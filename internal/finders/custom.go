package finders

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	log "audiobookshelf/internal/logger"
	"audiobookshelf/internal/providers"
)

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
