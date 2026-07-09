package handlers

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"time"

	idb "audiobookshelf/internal/db"

	"github.com/google/uuid"
)

// handleGetMetadataProviders maps to GET /api/search/providers
func handleGetMetadataProviders(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[Go] GET /api/search/providers")

		customBookProviders, customPodcastProviders, err := queryCustomMetadataProviders(db)
		if err != nil {
			log.Printf("[Settings] Failed to query custom metadata providers: %v", err)
		}

		response := buildMetadataProvidersResponse(customBookProviders, customPodcastProviders)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}
}

// handleGetCustomMetadataProviders maps to GET /api/custom-metadata-providers
func handleGetCustomMetadataProviders(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[Go] GET /api/custom-metadata-providers")

		rows, err := db.Query("SELECT id, name, mediaType, url, authHeaderValue, extraData, createdAt, updatedAt FROM customMetadataProviders")
		if err != nil {
			http.Error(w, `{"error": "Internal Error"}`, http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var list []map[string]interface{}
		for rows.Next() {
			var id, name, mediaType, url string
			var authHeaderVal, extraData, createdAt, updatedAt sql.NullString
			if err := rows.Scan(&id, &name, &mediaType, &url, &authHeaderVal, &extraData, &createdAt, &updatedAt); err != nil {
				log.Printf("[Settings] Failed to scan custom metadata provider: %v", err)
				http.Error(w, `{"error": "Internal Error"}`, http.StatusInternalServerError)
				return
			}
			m := map[string]interface{}{
				"id":        id,
				"name":      name,
				"mediaType": mediaType,
				"url":       url,
				"slug":      "custom-" + id,
			}
			if authHeaderVal.Valid {
				m["authHeaderValue"] = authHeaderVal.String
			} else {
				m["authHeaderValue"] = nil
			}
			list = append(list, m)
		}
		if err := rows.Err(); err != nil {
			log.Printf("[Settings] Custom metadata providers query iteration error: %v", err)
			http.Error(w, `{"error": "Internal Error"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"providers": list,
		})
	}
}

// handleCreateCustomMetadataProvider maps to POST /api/custom-metadata-providers
func handleCreateCustomMetadataProvider(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[Go] POST /api/custom-metadata-providers")

		var body struct {
			Name            string  `json:"name"`
			URL             string  `json:"url"`
			MediaType       string  `json:"mediaType"`
			AuthHeaderValue *string `json:"authHeaderValue"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, `{"error": "Invalid payload"}`, http.StatusBadRequest)
			return
		}

		if body.Name == "" || body.URL == "" || body.MediaType == "" {
			http.Error(w, `{"error": "Name, url and mediaType are required"}`, http.StatusBadRequest)
			return
		}

		if body.MediaType != "book" && body.MediaType != "podcast" {
			http.Error(w, `{"error": "mediaType must be book or podcast"}`, http.StatusBadRequest)
			return
		}

		id := uuid.New().String()
		nowStr := idb.TimeToDBStr(time.Now())

		var authVal interface{} = nil
		if body.AuthHeaderValue != nil && *body.AuthHeaderValue != "" {
			authVal = *body.AuthHeaderValue
		}

		_, err := db.Exec("INSERT INTO customMetadataProviders (id, name, mediaType, url, authHeaderValue, extraData, createdAt, updatedAt) VALUES (?, ?, ?, ?, ?, '{}', ?, ?)",
			id, body.Name, body.MediaType, body.URL, authVal, nowStr, nowStr)
		if err != nil {
			log.Printf("[Custom Provider] Creation failed: %v", err)
			http.Error(w, `{"error": "Failed to create custom provider"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"provider": map[string]interface{}{
				"id":              id,
				"name":            body.Name,
				"mediaType":       body.MediaType,
				"url":             body.URL,
				"authHeaderValue": authVal,
				"slug":            "custom-" + id,
			},
		})
	}
}

// handleDeleteCustomMetadataProvider maps to DELETE /api/custom-metadata-providers/:id
func handleDeleteCustomMetadataProvider(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := trimPathPrefix(r.URL.Path, "/api/custom-metadata-providers/")
		log.Printf("[Go] DELETE /api/custom-metadata-providers/%s", id)

		// Delete from customMetadataProviders
		_, err := db.Exec("DELETE FROM customMetadataProviders WHERE id = ?", id)
		if err != nil {
			http.Error(w, `{"error": "Failed to delete"}`, http.StatusInternalServerError)
			return
		}

		// Fallback libraries to default providers
		slug := "custom-" + id
		_, _ = db.Exec("UPDATE libraries SET provider = 'google' WHERE provider = ? AND mediaType = 'book'", slug)
		_, _ = db.Exec("UPDATE libraries SET provider = 'itunes' WHERE provider = ? AND mediaType = 'podcast'", slug)

		w.WriteHeader(http.StatusOK)
	}
}

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
			log.Printf("[Settings] Failed to scan custom metadata provider: %v", err)
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
