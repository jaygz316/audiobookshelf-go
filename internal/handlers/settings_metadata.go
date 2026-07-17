package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/url"
	"time"

	"audiobookshelf/internal/core"
	idb "audiobookshelf/internal/db"

	"github.com/google/uuid"
)

// handleGetMetadataProviders maps to GET /api/search/providers
func handleGetMetadataProviders(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Go] GET /api/search/providers")

		customBookProviders, customPodcastProviders, err := queryCustomMetadataProviders(db)
		if err != nil {
			log.Errorf("[Settings] Failed to query custom metadata providers: %v", err)
		}

		response := buildMetadataProvidersResponse(customBookProviders, customPodcastProviders)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}
}

// handleGetCustomMetadataProviders maps to GET /api/custom-metadata-providers
func handleGetCustomMetadataProviders(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Go] GET /api/custom-metadata-providers")
		userSess, ok := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if !ok || !userSess.IsAdminOrUp() {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		rows, err := db.Query("SELECT id, name, mediaType, url, authHeaderValue, extraData, createdAt, updatedAt FROM customMetadataProviders")
		if err != nil {
			http.Error(w, `{"error": "Internal Error"}`, http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		list := []map[string]interface{}{}
		for rows.Next() {
			var id, name, mediaType, url string
			var authHeaderVal, extraData, createdAt, updatedAt sql.NullString
			if err := rows.Scan(&id, &name, &mediaType, &url, &authHeaderVal, &extraData, &createdAt, &updatedAt); err != nil {
				log.Errorf("[Settings] Failed to scan custom metadata provider: %v", err)
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
			log.Errorf("[Settings] Custom metadata providers query iteration error: %v", err)
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
		log.Infof("[Go] POST /api/custom-metadata-providers")
		userSess, ok := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if !ok || !userSess.IsAdminOrUp() {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

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

		parsedURL, err := url.Parse(body.URL)
		if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
			http.Error(w, `{"error": "url must be a valid http or https link"}`, http.StatusBadRequest)
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

		_, err = db.Exec("INSERT INTO customMetadataProviders (id, name, mediaType, url, authHeaderValue, extraData, createdAt, updatedAt) VALUES (?, ?, ?, ?, ?, '{}', ?, ?)",
			id, body.Name, body.MediaType, body.URL, authVal, nowStr, nowStr)
		if err != nil {
			log.Errorf("[Custom Provider] Creation failed: %v", err)
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
		log.Infof("[Go] DELETE /api/custom-metadata-providers/%s", id)
		userSess, ok := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if !ok || !userSess.IsAdminOrUp() {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

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
