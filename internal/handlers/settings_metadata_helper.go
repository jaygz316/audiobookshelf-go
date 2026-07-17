package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
)

// queryCustomMetadataProviders fetches and organizes custom metadata providers by mediaType
func queryCustomMetadataProviders(db *sql.DB) (books []map[string]interface{}, podcasts []map[string]interface{}, err error) {
	rows, err := db.Query("SELECT id, name, mediaType FROM customMetadataProviders")
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var id, name, mediaType string
		if err := rows.Scan(&id, &name, &mediaType); err != nil {
			log.Errorf("[Settings] Failed to scan custom metadata provider: %v", err)
			continue
		}
		p := map[string]interface{}{
			"value": "custom-" + id,
			"text":  name,
		}
		if mediaType == "book" {
			books = append(books, p)
		} else if mediaType == "podcast" {
			podcasts = append(podcasts, p)
		}
	}
	err = rows.Err()
	return books, podcasts, err
}

// buildMetadataProvidersResponse formats metadata provider details into the target JSON response
func buildMetadataProvidersResponse(customBooks, customPodcasts []map[string]interface{}) map[string]interface{} {
	providerMap := map[string]string{
		"google":          "Google Books",
		"itunes":          "iTunes",
		"openlibrary":     "Open Library",
		"fantlab":         "FantLab.ru",
		"audiobookcovers": "AudiobookCovers.com",
		"audible":         "Audible.com",
		"audnexus":        "Audnexus",
		"best":            "Best",
		"all":             "All",
	}

	formatProvider := func(p string) map[string]string {
		text := p
		if t, ok := providerMap[p]; ok {
			text = t
		}
		return map[string]string{
			"value": p,
			"text":  text,
		}
	}

	bookProvidersList := []string{"google", "openlibrary", "itunes", "audible", "fantlab", "audnexus"}
	bookCoversProvidersList := []string{"google", "openlibrary", "itunes", "audible", "fantlab", "audnexus", "audiobookcovers"}

	var booksProviders []map[string]interface{}
	for _, p := range bookProvidersList {
		m := make(map[string]interface{})
		for k, v := range formatProvider(p) {
			m[k] = v
		}
		booksProviders = append(booksProviders, m)
	}
	for _, cp := range customBooks {
		booksProviders = append(booksProviders, cp)
	}

	var booksCoversProviders []map[string]interface{}
	booksCoversProviders = append(booksCoversProviders, map[string]interface{}{"value": "best", "text": "Best"})
	for _, p := range bookCoversProvidersList {
		m := make(map[string]interface{})
		for k, v := range formatProvider(p) {
			m[k] = v
		}
		booksCoversProviders = append(booksCoversProviders, m)
	}
	for _, cp := range customBooks {
		booksCoversProviders = append(booksCoversProviders, cp)
	}
	booksCoversProviders = append(booksCoversProviders, map[string]interface{}{"value": "all", "text": "All"})

	var podcastsProviders []map[string]interface{}
	podcastsProviders = append(podcastsProviders, map[string]interface{}{"value": "itunes", "text": "iTunes"})
	for _, cp := range customPodcasts {
		podcastsProviders = append(podcastsProviders, cp)
	}

	return map[string]interface{}{
		"providers": map[string]interface{}{
			"books":       booksProviders,
			"booksCovers": booksCoversProviders,
			"podcasts":    podcastsProviders,
		},
	}
}
