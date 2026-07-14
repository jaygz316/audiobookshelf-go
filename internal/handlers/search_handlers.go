package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"encoding/json"
	"net/http"
)

func handleSearchBooks(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		initManagers(db)

		q := r.URL.Query()
		provider := q.Get("provider")
		if provider == "" {
			provider = "google"
		}
		title := q.Get("title")
		author := q.Get("author")

		queryStr := title
		if author != "" {
			if queryStr != "" {
				queryStr += " " + author
			} else {
				queryStr = author
			}
		}

		results, err := globalFinder.SearchBooks(r.Context(), provider, queryStr)
		if err != nil {
			log.Printf("[Search] SearchBooks failed: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(results)
	}
}

func handleSearchPodcasts(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		initManagers(db)

		q := r.URL.Query()
		term := q.Get("term")

		results, err := globalFinder.SearchPodcasts(r.Context(), "itunes", term)
		if err != nil {
			log.Printf("[Search] SearchPodcasts failed: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(results)
	}
}

func handleGetSearchProviders(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{
		"providers": {
			"books": [
				{"value": "google", "text": "Google Books"},
				{"value": "itunes", "text": "iTunes"},
				{"value": "openlibrary", "text": "Open Library"},
				{"value": "audible", "text": "Audible.com"},
				{"value": "fantlab", "text": "FantLab.ru"},
				{"value": "audnexus", "text": "Audnexus"}
			],
			"booksCovers": [
				{"value": "best", "text": "Best"},
				{"value": "google", "text": "Google Books"},
				{"value": "itunes", "text": "iTunes"},
				{"value": "openlibrary", "text": "Open Library"},
				{"value": "audible", "text": "Audible.com"},
				{"value": "fantlab", "text": "FantLab.ru"},
				{"value": "audnexus", "text": "Audnexus"},
				{"value": "all", "text": "All"}
			],
			"podcasts": [
				{"value": "itunes", "text": "iTunes"}
			]
		}
	}`))
}

func handleSearchAuthors(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		initManagers(db)

		q := r.URL.Query()
		provider := q.Get("provider")
		if provider == "" {
			provider = "audnexus"
		}
		name := q.Get("name")
		if name == "" {
			name = q.Get("q")
		}

		if name == "" {
			http.Error(w, `{"error": "name parameter is required"}`, http.StatusBadRequest)
			return
		}

		results, err := globalFinder.SearchAuthors(r.Context(), provider, name)
		if err != nil {
			log.Printf("[Search] SearchAuthors failed: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(results)
	}
}
