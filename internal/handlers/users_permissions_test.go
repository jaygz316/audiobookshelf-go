package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"audiobookshelf/internal/core"
)

func TestGranularPermissions_Upload(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// 1. User without upload permission
	sessNoUpload := &core.UserSession{
		ID:        "user-1",
		Username:  "no-upload",
		Type:      "user",
		IsActive:  true,
		CanUpload: false,
	}

	req := httptest.NewRequest("POST", "/api/upload", nil)
	req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, sessNoUpload))
	rr := httptest.NewRecorder()

	handleUpload(db).ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("Expected 403 Forbidden for no-upload, got %d", rr.Code)
	}

	// 2. User with upload permission
	sessUpload := &core.UserSession{
		ID:        "user-2",
		Username:  "upload",
		Type:      "user",
		IsActive:  true,
		CanUpload: true,
	}

	req2 := httptest.NewRequest("POST", "/api/upload", nil)
	req2 = req2.WithContext(context.WithValue(req2.Context(), core.UserContextKey, sessUpload))
	rr2 := httptest.NewRecorder()

	handleUpload(db).ServeHTTP(rr2, req2)
	// We expect bad request or similar instead of 403 Forbidden, because there's no multipart form payload
	if rr2.Code == http.StatusForbidden {
		t.Errorf("Expected not forbidden for user with upload permission, got %d", rr2.Code)
	}
}

func TestGranularPermissions_DeleteLibrary(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// 1. User without delete permission
	sessNoDelete := &core.UserSession{
		ID:        "user-1",
		Username:  "no-delete",
		Type:      "user",
		IsActive:  true,
		CanDelete: false,
	}

	req := httptest.NewRequest("DELETE", "/api/libraries/lib-1", nil)
	req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, sessNoDelete))
	rr := httptest.NewRecorder()

	HandleDeleteLibrary(db, "lib-1").ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("Expected 403 Forbidden for no-delete, got %d", rr.Code)
	}

	// 2. User with delete permission
	sessDelete := &core.UserSession{
		ID:        "user-2",
		Username:  "delete",
		Type:      "user",
		IsActive:  true,
		CanDelete: true,
	}

	req2 := httptest.NewRequest("DELETE", "/api/libraries/lib-1", nil)
	req2 = req2.WithContext(context.WithValue(req2.Context(), core.UserContextKey, sessDelete))
	rr2 := httptest.NewRecorder()

	HandleDeleteLibrary(db, "lib-1").ServeHTTP(rr2, req2)
	// We expect library not found (404) or similar since the DB is empty, but definitely not 403 Forbidden!
	if rr2.Code == http.StatusForbidden {
		t.Errorf("Expected not forbidden for user with delete permission, got %d", rr2.Code)
	}
}

func TestGranularPermissions_UpdateItem(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// 1. User without update permission
	sessNoUpdate := &core.UserSession{
		ID:        "user-1",
		Username:  "no-update",
		Type:      "user",
		IsActive:  true,
		CanUpdate: false,
	}

	req := httptest.NewRequest("PATCH", "/api/items/item-1", nil)
	req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, sessNoUpdate))
	rr := httptest.NewRecorder()

	handleUpdateLibraryItemByID(db, "item-1").ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("Expected 403 Forbidden for no-update, got %d", rr.Code)
	}

	// 2. User with update permission
	sessUpdate := &core.UserSession{
		ID:        "user-2",
		Username:  "update",
		Type:      "user",
		IsActive:  true,
		CanUpdate: true,
	}

	req2 := httptest.NewRequest("PATCH", "/api/items/item-1", nil)
	req2 = req2.WithContext(context.WithValue(req2.Context(), core.UserContextKey, sessUpdate))
	rr2 := httptest.NewRecorder()

	handleUpdateLibraryItemByID(db, "item-1").ServeHTTP(rr2, req2)
	// We expect 404 (Not Found) or similar since item-1 doesn't exist, but not 403 Forbidden!
	if rr2.Code == http.StatusForbidden {
		t.Errorf("Expected not forbidden for user with update permission, got %d", rr2.Code)
	}
}

func TestGranularPermissions_AccessRss(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// 1. User without accessRss permission
	sessNoRss := &core.UserSession{
		ID:           "user-1",
		Username:     "no-rss",
		Type:         "user",
		IsActive:     true,
		CanAccessRss: false,
	}

	req := httptest.NewRequest("GET", "/api/feeds", nil)
	req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, sessNoRss))
	rr := httptest.NewRecorder()

	handleGetFeeds(db).ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("Expected 403 Forbidden for no-rss, got %d", rr.Code)
	}

	// 2. User with accessRss permission
	sessRss := &core.UserSession{
		ID:           "user-2",
		Username:     "rss",
		Type:         "user",
		IsActive:     true,
		CanAccessRss: true,
	}

	req2 := httptest.NewRequest("GET", "/api/feeds", nil)
	req2 = req2.WithContext(context.WithValue(req2.Context(), core.UserContextKey, sessRss))
	rr2 := httptest.NewRecorder()

	handleGetFeeds(db).ServeHTTP(rr2, req2)
	// We expect 200 OK (empty list)
	if rr2.Code != http.StatusOK {
		t.Errorf("Expected 200 OK for user with accessRss permission, got %d", rr2.Code)
	}
}

func TestGranularPermissions_CreateShares(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// 1. User without createShares permission
	sessNoShares := &core.UserSession{
		ID:              "user-1",
		Username:        "no-shares",
		Type:            "user",
		IsActive:        true,
		CanCreateShares: false,
	}

	req := httptest.NewRequest("POST", "/api/share/mediaitem", nil)
	req = req.WithContext(context.WithValue(req.Context(), core.UserContextKey, sessNoShares))
	rr := httptest.NewRecorder()

	handleCreateShare(db).ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("Expected 403 Forbidden for no-shares, got %d", rr.Code)
	}

	// 2. User with createShares permission
	sessShares := &core.UserSession{
		ID:              "user-2",
		Username:        "shares",
		Type:            "user",
		IsActive:        true,
		CanCreateShares: true,
	}

	req2 := httptest.NewRequest("POST", "/api/share/mediaitem", nil)
	req2 = req2.WithContext(context.WithValue(req2.Context(), core.UserContextKey, sessShares))
	rr2 := httptest.NewRecorder()

	handleCreateShare(db).ServeHTTP(rr2, req2)
	// We expect bad request or similar instead of 403 Forbidden, because there's no body payload
	if rr2.Code == http.StatusForbidden {
		t.Errorf("Expected not forbidden for user with createShares permission, got %d", rr2.Code)
	}
}
