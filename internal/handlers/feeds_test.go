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

func TestFeedsHandlers(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Insert test data
	_, err := db.Exec(`INSERT INTO users (id, username, type, isActive, permissions) VALUES ('user1', 'adminuser', 'admin', 1, '{}')`)
	if err != nil {
		t.Fatalf("Failed to insert test user: %v", err)
	}

	_, err = db.Exec(`INSERT INTO libraryItems (id, title, mediaType, mediaId) VALUES ('item1', 'Test Book Title', 'book', 'book1')`)
	if err != nil {
		t.Fatalf("Failed to insert library item: %v", err)
	}

	adminSession := &core.UserSession{
		ID:       "user1",
		Username: "adminuser",
		Type:     "admin",
		IsActive: true,
	}

	regularSession := &core.UserSession{
		ID:       "user2",
		Username: "regularuser",
		Type:     "user",
		IsActive: true,
	}

	t.Run("Create Feed", func(t *testing.T) {
		reqPayload := CreateFeedRequest{
			EntityID: "item1",
			Type:     "book",
		}
		body, _ := json.Marshal(reqPayload)
		req := httptest.NewRequest("POST", "/api/feeds", bytes.NewReader(body))
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, adminSession))
		rr := httptest.NewRecorder()

		handler := handleCreateFeed(db)
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusCreated {
			t.Errorf("Expected status 201, got %d. Body: %s", rr.Code, rr.Body.String())
		}

		var resp FeedResponse
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to unmarshal response: %v", err)
		}

		if resp.EntityID != "item1" {
			t.Errorf("Expected EntityID 'item1', got %s", resp.EntityID)
		}
		if resp.Type != "book" {
			t.Errorf("Expected Type 'book', got %s", resp.Type)
		}
		if resp.ID == "" {
			t.Errorf("Expected non-empty Feed ID")
		}
		if resp.Title != "Test Book Title" {
			t.Errorf("Expected Title 'Test Book Title', got %s", resp.Title)
		}
	})

	t.Run("Create Feed Unauthorized & Forbidden", func(t *testing.T) {
		// Unauthorized
		reqPayload := CreateFeedRequest{
			EntityID: "item1",
			Type:     "book",
		}
		body, _ := json.Marshal(reqPayload)
		req := httptest.NewRequest("POST", "/api/feeds", bytes.NewReader(body))
		rr := httptest.NewRecorder()
		handleCreateFeed(db).ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("Expected status 401, got %d", rr.Code)
		}

		// Forbidden
		req = httptest.NewRequest("POST", "/api/feeds", bytes.NewReader(body))
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, regularSession))
		rr = httptest.NewRecorder()
		handleCreateFeed(db).ServeHTTP(rr, req)
		if rr.Code != http.StatusForbidden {
			t.Errorf("Expected status 403, got %d", rr.Code)
		}
	})

	t.Run("Get Feeds", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/feeds", nil)
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, adminSession))
		rr := httptest.NewRecorder()

		handler := handleGetFeeds(db)
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", rr.Code)
		}

		var resp map[string][]FeedResponse
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to unmarshal response: %v", err)
		}

		if len(resp["feeds"]) != 1 {
			t.Fatalf("Expected 1 feed, got %d", len(resp["feeds"]))
		}

		f := resp["feeds"][0]
		if f.EntityID != "item1" {
			t.Errorf("Expected EntityID 'item1', got %s", f.EntityID)
		}
		if f.Title != "Test Book Title" {
			t.Errorf("Expected Title 'Test Book Title', got %s", f.Title)
		}
	})

	t.Run("Delete Feed", func(t *testing.T) {
		// First get the feed ID
		req := httptest.NewRequest("GET", "/api/feeds", nil)
		req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, adminSession))
		rr := httptest.NewRecorder()
		handleGetFeeds(db).ServeHTTP(rr, req)

		var resp map[string][]FeedResponse
		json.Unmarshal(rr.Body.Bytes(), &resp)
		feedID := resp["feeds"][0].ID

		// Delete
		deleteReq := httptest.NewRequest("DELETE", "/api/feeds/"+feedID, nil)
		deleteReq = deleteReq.WithContext(context.WithValue(deleteReq.Context(), core.UserContextKey, adminSession))
		deleteRR := httptest.NewRecorder()
		handleDeleteFeed(db).ServeHTTP(deleteRR, deleteReq)

		if deleteRR.Code != http.StatusNoContent {
			t.Errorf("Expected status 204, got %d. Body: %s", deleteRR.Code, deleteRR.Body.String())
		}

		// Verify deleted
		checkReq := httptest.NewRequest("GET", "/api/feeds", nil)
		checkReq = checkReq.WithContext(context.WithValue(checkReq.Context(), core.UserContextKey, adminSession))
		checkRR := httptest.NewRecorder()
		handleGetFeeds(db).ServeHTTP(checkRR, checkReq)

		var checkResp map[string][]FeedResponse
		json.Unmarshal(checkRR.Body.Bytes(), &checkResp)
		if len(checkResp["feeds"]) != 0 {
			t.Errorf("Expected 0 feeds, got %d", len(checkResp["feeds"]))
		}
	})
}
