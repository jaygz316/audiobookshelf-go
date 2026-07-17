package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"

	"audiobookshelf/internal/core"
)

// handleGetMeListeningStats handles GET /api/me/listening-stats
func handleGetMeListeningStats(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userSess, ok := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if !ok {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}

		stats, err := getUserListeningStats(db, userSess.ID)
		if err != nil {
			log.Errorf("[Listening Stats] Failed to query stats: %v", err)
			http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(stats)
	}
}

// handleGetMeListeningSessions handles GET /api/me/listening-sessions
func handleGetMeListeningSessions(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userSess, ok := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if !ok {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}

		page := 0
		if pVal := r.URL.Query().Get("page"); pVal != "" {
			if p, err := strconv.Atoi(pVal); err == nil {
				page = p
			}
		}
		itemsPerPage := 10
		if limitVal := r.URL.Query().Get("itemsPerPage"); limitVal != "" {
			if limit, err := strconv.Atoi(limitVal); err == nil {
				itemsPerPage = limit
			}
		}

		mediaItemID := r.URL.Query().Get("mediaItemId")
		libraryItemID := r.URL.Query().Get("libraryItemId")
		if libraryItemID == "" {
			libraryItemID = r.URL.Query().Get("itemId")
		}

		sessions, err := handleGetUserListeningSessions(db, userSess.ID, page, itemsPerPage, mediaItemID, libraryItemID)
		if err != nil {
			log.Errorf("[Listening Sessions] Failed to query sessions: %v", err)
			http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(sessions)
	}
}

// handleGetServerListeningStats handles GET /api/server-listening-stats
func handleGetServerListeningStats(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userSess, ok := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if !ok {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}

		if userSess.Type != "root" && userSess.Type != "admin" {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		stats, err := getServerListeningStats(db)
		if err != nil {
			log.Errorf("[Server Listening Stats] Failed to query stats: %v", err)
			http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(stats)
	}
}

// handleGetServerListeningSessions handles GET /api/server-listening-sessions
func handleGetServerListeningSessions(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userSess, ok := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if !ok {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}

		if userSess.Type != "root" && userSess.Type != "admin" {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		page := 0
		if pVal := r.URL.Query().Get("page"); pVal != "" {
			if p, err := strconv.Atoi(pVal); err == nil {
				page = p
			}
		}
		itemsPerPage := 10
		if limitVal := r.URL.Query().Get("itemsPerPage"); limitVal != "" {
			if limit, err := strconv.Atoi(limitVal); err == nil {
				itemsPerPage = limit
			}
		}

		mediaItemID := r.URL.Query().Get("mediaItemId")
		libraryItemID := r.URL.Query().Get("libraryItemId")
		if libraryItemID == "" {
			libraryItemID = r.URL.Query().Get("itemId")
		}

		sessions, err := handleGetUserListeningSessions(db, "", page, itemsPerPage, mediaItemID, libraryItemID)
		if err != nil {
			log.Errorf("[Listening Sessions] Failed to query server sessions: %v", err)
			http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(sessions)
	}
}
