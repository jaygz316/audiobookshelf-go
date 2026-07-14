package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"audiobookshelf/internal/core"
)

func TestCustomMetadataProvidersEndpoints(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Setup user session for AuthMiddleware
	rootSession := &core.UserSession{
		ID:       "root-user",
		Username: "root",
		Type:     "root",
		IsActive: true,
	}

	// 1. Initially get custom metadata providers (should be empty)
	t.Run("GetInitialCustomProviders", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/custom-metadata-providers", nil)
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, rootSession))
		rr := httptest.NewRecorder()

		handleGetCustomMetadataProviders(db).ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected status OK, got %d", rr.Code)
		}

		var resp map[string]interface{}
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to parse JSON response: %v", err)
		}

		provs, ok := resp["providers"].([]interface{})
		if !ok {
			t.Fatalf("Expected providers array in response")
		}
		if len(provs) != 0 {
			t.Errorf("Expected 0 custom providers initially, got %d", len(provs))
		}
	})

	// 2. Create a custom metadata provider
	var createdID string
	t.Run("CreateCustomProvider", func(t *testing.T) {
		payload := `{
			"name": "My Custom Book Provider",
			"url": "https://api.custom.com/search",
			"mediaType": "book",
			"authHeaderValue": "Bearer my-secret-token"
		}`
		req := httptest.NewRequest("POST", "/api/custom-metadata-providers", bytes.NewBufferString(payload))
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, rootSession))
		rr := httptest.NewRecorder()

		handleCreateCustomMetadataProvider(db).ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected status OK, got %d: %s", rr.Code, rr.Body.String())
		}

		var resp map[string]interface{}
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to parse JSON response: %v", err)
		}

		prov, ok := resp["provider"].(map[string]interface{})
		if !ok {
			t.Fatalf("Expected provider object in response")
		}

		if prov["name"] != "My Custom Book Provider" {
			t.Errorf("Expected name 'My Custom Book Provider', got %q", prov["name"])
		}

		id, ok := prov["id"].(string)
		if !ok || id == "" {
			t.Fatalf("Expected non-empty ID in response")
		}
		createdID = id
	})

	// 3. Verify it is listed in active metadata providers
	t.Run("GetActiveProviders", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/search/providers", nil)
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, rootSession))
		rr := httptest.NewRecorder()

		handleGetMetadataProviders(db).ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected status OK, got %d", rr.Code)
		}

		var resp map[string]interface{}
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to parse JSON response: %v", err)
		}

		providers, ok := resp["providers"].(map[string]interface{})
		if !ok {
			t.Fatalf("Expected providers map")
		}

		books, ok := providers["books"].([]interface{})
		if !ok {
			t.Fatalf("Expected books array")
		}

		// Find our custom provider in books list
		found := false
		expectedVal := "custom-" + createdID
		for _, b := range books {
			bMap, ok := b.(map[string]interface{})
			if ok && bMap["value"] == expectedVal {
				found = true
				if bMap["text"] != "My Custom Book Provider" {
					t.Errorf("Expected text 'My Custom Book Provider', got %q", bMap["text"])
				}
				break
			}
		}
		if !found {
			t.Errorf("Expected custom provider %q to be in books list", expectedVal)
		}
	})

	// 4. Delete the custom metadata provider
	t.Run("DeleteCustomProvider", func(t *testing.T) {
		req := httptest.NewRequest("DELETE", "/api/custom-metadata-providers/"+createdID, nil)
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, rootSession))
		rr := httptest.NewRecorder()

		handleDeleteCustomMetadataProvider(db).ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected status OK, got %d: %s", rr.Code, rr.Body.String())
		}

		// Verify it was deleted from db
		var count int
		err := db.QueryRow("SELECT COUNT(*) FROM customMetadataProviders WHERE id = ?", createdID).Scan(&count)
		if err != nil {
			t.Fatalf("Failed to query db: %v", err)
		}
		if count != 0 {
			t.Errorf("Expected custom provider to be deleted, but count is %d", count)
		}
	})

	// 5. Verify non-admin is forbidden
	t.Run("ForbiddenForNonAdmin", func(t *testing.T) {
		nonAdminSession := &core.UserSession{
			ID:       "normal-user",
			Username: "user",
			Type:     "user",
			IsActive: true,
		}

		// GET
		req := httptest.NewRequest("GET", "/api/custom-metadata-providers", nil)
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, nonAdminSession))
		rr := httptest.NewRecorder()
		handleGetCustomMetadataProviders(db).ServeHTTP(rr, req)
		if rr.Code != http.StatusForbidden {
			t.Errorf("Expected GET to return 403 Forbidden, got %d", rr.Code)
		}

		// POST
		payload := `{"name":"Forbidden Provider","url":"http://test.com","mediaType":"book"}`
		req = httptest.NewRequest("POST", "/api/custom-metadata-providers", bytes.NewBufferString(payload))
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, nonAdminSession))
		rr = httptest.NewRecorder()
		handleCreateCustomMetadataProvider(db).ServeHTTP(rr, req)
		if rr.Code != http.StatusForbidden {
			t.Errorf("Expected POST to return 403 Forbidden, got %d", rr.Code)
		}

		// DELETE
		req = httptest.NewRequest("DELETE", "/api/custom-metadata-providers/xyz", nil)
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, nonAdminSession))
		rr = httptest.NewRecorder()
		handleDeleteCustomMetadataProvider(db).ServeHTTP(rr, req)
		if rr.Code != http.StatusForbidden {
			t.Errorf("Expected DELETE to return 403 Forbidden, got %d", rr.Code)
		}
	})
}
