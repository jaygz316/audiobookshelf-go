package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"audiobookshelf/internal/core"
)

func TestRoutingMeProgressChallenger(t *testing.T) {
	cfg := &core.Config{
		RouterBasePath: "",
		ConfigPath:     t.TempDir(),
		MetadataPath:   t.TempDir(),
	}

	handler := SetupHandler(nil, cfg, false, ".", "2.35.1")

	tests := []struct {
		name           string
		method         string
		path           string
		expectedStatus int
	}{
		// 1. Items in progress
		{
			name:           "GET items-in-progress should reach handler (500 database not connected)",
			method:         "GET",
			path:           "/api/me/items-in-progress",
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name:           "POST items-in-progress should be 404 (method not allowed/matched)",
			method:         "POST",
			path:           "/api/me/items-in-progress",
			expectedStatus: http.StatusNotFound,
		},

		// 2. GET/PATCH/POST/DELETE progress by libraryItemID
		{
			name:           "GET progress libraryItemID should reach handler",
			method:         "GET",
			path:           "/api/me/progress/library-item-123",
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name:           "PATCH progress libraryItemID should reach handler",
			method:         "PATCH",
			path:           "/api/me/progress/library-item-123",
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name:           "POST progress libraryItemID should reach handler",
			method:         "POST",
			path:           "/api/me/progress/library-item-123",
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name:           "DELETE progress libraryItemID should reach handler",
			method:         "DELETE",
			path:           "/api/me/progress/library-item-123",
			expectedStatus: http.StatusInternalServerError,
		},

		// 3. GET/PATCH/POST/DELETE progress by libraryItemID + episodeID
		{
			name:           "GET progress libraryItemID/episodeID should reach handler",
			method:         "GET",
			path:           "/api/me/progress/library-item-123/episode-456",
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name:           "PATCH progress libraryItemID/episodeID should reach handler",
			method:         "PATCH",
			path:           "/api/me/progress/library-item-123/episode-456",
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name:           "POST progress libraryItemID/episodeID should reach handler",
			method:         "POST",
			path:           "/api/me/progress/library-item-123/episode-456",
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name:           "DELETE progress libraryItemID/episodeID should reach handler",
			method:         "DELETE",
			path:           "/api/me/progress/library-item-123/episode-456",
			expectedStatus: http.StatusInternalServerError,
		},

		// 4. Hide/remove from continue listening
		{
			name:           "GET hide-from-continue-listening should reach handler",
			method:         "GET",
			path:           "/api/me/progress/progress-123/hide-from-continue-listening",
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name:           "PATCH hide-from-continue-listening should reach handler",
			method:         "PATCH",
			path:           "/api/me/progress/progress-123/hide-from-continue-listening",
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name:           "GET remove-from-continue-listening should reach handler",
			method:         "GET",
			path:           "/api/me/progress/progress-123/remove-from-continue-listening",
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name:           "PATCH remove-from-continue-listening should reach handler",
			method:         "PATCH",
			path:           "/api/me/progress/progress-123/remove-from-continue-listening",
			expectedStatus: http.StatusInternalServerError,
		},

		// 5. Series continue listening
		{
			name:           "POST series remove should reach handler",
			method:         "POST",
			path:           "/api/me/series/series-123/remove",
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name:           "POST series readd should reach handler",
			method:         "POST",
			path:           "/api/me/series/series-123/readd",
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name:           "GET series remove should be 404 (method not allowed/matched)",
			method:         "GET",
			path:           "/api/me/series/series-123/remove",
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "GET series readd should be 404 (method not allowed/matched)",
			method:         "GET",
			path:           "/api/me/series/series-123/readd",
			expectedStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d. Body: %s", tt.expectedStatus, rr.Code, rr.Body.String())
			}
		})
	}
}
