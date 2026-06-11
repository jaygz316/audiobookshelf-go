package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"audiobookshelf/internal/playlist"

	"github.com/google/uuid"

	"audiobookshelf/internal/core")

func parseMsFromDBStr(s string) int64 {
	if s == "" {
		return 0
	}
	// Try parsing as float first because SQLite sometimes stores decimal strings
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return int64(f)
	}
	if val, err := strconv.ParseInt(s, 10, 64); err == nil {
		return val
	}
	// Fallback to try parsing as RFC3339 if it's a date string
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UnixNano() / int64(time.Millisecond)
	}
	return 0
}

func hasColumn(ctx context.Context, db *sql.DB, tableName, columnName string) bool {
	query := `PRAGMA table_info(` + tableName + `)`
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return false
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name, dfltVal string
		var typeVal string
		var notnull, pk int
		if err := rows.Scan(&cid, &name, &typeVal, &notnull, &dfltVal, &pk); err == nil {
			if strings.EqualFold(name, columnName) {
				return true
			}
		}
	}
	return false
}

func queryPlaylistsForUserAndLibrary(ctx context.Context, db *sql.DB, userID, libraryID string) ([]map[string]interface{}, error) {
	query := "SELECT id, userId, name, libraryId, description, createdAt, updatedAt FROM playlists WHERE userId = ?"
	var args []interface{}
	args = append(args, userID)
	if libraryID != "" {
		query += " AND (libraryId = ? OR libraryId IS NULL)"
		args = append(args, libraryID)
	}

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var playlists []map[string]interface{}
	for rows.Next() {
		var id, uID, name string
		var libID, desc sql.NullString
		var createdAtStr, updatedAtStr string
		if err := rows.Scan(&id, &uID, &name, &libID, &desc, &createdAtStr, &updatedAtStr); err != nil {
			return nil, err
		}

		p := map[string]interface{}{
			"id":        id,
			"userId":    uID,
			"name":      name,
			"libraryId": libID.String,
			"createdAt": parseMsFromDBStr(createdAtStr),
			"updatedAt": parseMsFromDBStr(updatedAtStr),
		}
		if desc.Valid {
			p["description"] = desc.String
		} else {
			p["description"] = nil
		}
		playlists = append(playlists, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for _, p := range playlists {
		pID := p["id"].(string)
		itemRows, err := db.QueryContext(ctx, `SELECT mediaItemId FROM playlistMediaItems WHERE playlistId = ? ORDER BY "order" ASC`, pID)
		if err != nil {
			return nil, err
		}
		var items []string
		for itemRows.Next() {
			var itemID string
			if err := itemRows.Scan(&itemID); err != nil {
				itemRows.Close()
				return nil, err
			}
			items = append(items, itemID)
		}
		if err := itemRows.Err(); err != nil {
			itemRows.Close()
			return nil, err
		}
		itemRows.Close()
		p["itemIds"] = items
	}

	return playlists, nil
}

func queryCollectionsForLibrary(ctx context.Context, db *sql.DB, libraryID string) ([]map[string]interface{}, error) {
	hasDisplayOrder := hasColumn(ctx, db, "collections", "displayOrder")
	var query string
	var args []interface{}
	if libraryID != "" {
		if hasDisplayOrder {
			query = "SELECT id, name, description, libraryId, displayOrder, createdAt, updatedAt FROM collections WHERE libraryId = ?"
		} else {
			query = "SELECT id, name, description, libraryId, createdAt, updatedAt FROM collections WHERE libraryId = ?"
		}
		args = append(args, libraryID)
	} else {
		if hasDisplayOrder {
			query = "SELECT id, name, description, libraryId, displayOrder, createdAt, updatedAt FROM collections"
		} else {
			query = "SELECT id, name, description, libraryId, createdAt, updatedAt FROM collections"
		}
	}

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var collections []map[string]interface{}
	for rows.Next() {
		var id, name string
		var description, libraryIDCol sql.NullString
		var createdAtStr, updatedAtStr string
		var displayOrder int

		var err error
		if hasDisplayOrder {
			err = rows.Scan(&id, &name, &description, &libraryIDCol, &displayOrder, &createdAtStr, &updatedAtStr)
		} else {
			err = rows.Scan(&id, &name, &description, &libraryIDCol, &createdAtStr, &updatedAtStr)
		}
		if err != nil {
			return nil, err
		}

		c := map[string]interface{}{
			"id":          id,
			"name":        name,
			"description": description.String,
			"libraryId":   libraryIDCol.String,
			"createdAt":   parseMsFromDBStr(createdAtStr),
			"updatedAt":   parseMsFromDBStr(updatedAtStr),
		}
		if hasDisplayOrder {
			c["displayOrder"] = displayOrder
		}
		collections = append(collections, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for _, c := range collections {
		cID := c["id"].(string)
		itemRows, err := db.QueryContext(ctx, `SELECT bookId FROM collectionBooks WHERE collectionId = ? ORDER BY "order" ASC`, cID)
		if err != nil {
			return nil, err
		}
		var items []string
		for itemRows.Next() {
			var bookID string
			if err := itemRows.Scan(&bookID); err != nil {
				itemRows.Close()
				return nil, err
			}
			items = append(items, bookID)
		}
		if err := itemRows.Err(); err != nil {
			itemRows.Close()
			return nil, err
		}
		itemRows.Close()
		c["books"] = items
		c["itemIds"] = items
	}

	return collections, nil
}

func handleGetLibraryPlaylists(db *sql.DB, libraryID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userSess := r.Context().Value(core.UserContextKey).(*core.UserSession)
		initManagers(db)

		playlists, err := queryPlaylistsForUserAndLibrary(r.Context(), db, userSess.ID, libraryID)
		if err != nil {
			log.Printf("[Playlist] handleGetLibraryPlaylists failed: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		payload := map[string]interface{}{
			"results": playlists,
			"total":   len(playlists),
			"limit":   0,
			"page":    0,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(payload)
	}
}

func handleGetLibraryCollections(db *sql.DB, libraryID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		initManagers(db)

		collections, err := queryCollectionsForLibrary(r.Context(), db, libraryID)
		if err != nil {
			log.Printf("[Collection] handleGetLibraryCollections failed: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		payload := map[string]interface{}{
			"results": collections,
			"total":   len(collections),
			"limit":   0,
			"page":    0,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(payload)
	}
}

func handleGetLibraryOPML(db *sql.DB, libraryID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userSess := r.Context().Value(core.UserContextKey).(*core.UserSession)
		initManagers(db)

		opmlText, err := globalFeedManager.GenerateOPML(r.Context(), userSess.ID, libraryID)
		if err != nil {
			log.Printf("[Feed] GenerateOPML failed: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/xml")
		w.Write([]byte(opmlText))
	}
}

func handleGetPlaylists(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userSess := r.Context().Value(core.UserContextKey).(*core.UserSession)
		initManagers(db)

		playlists, err := queryPlaylistsForUserAndLibrary(r.Context(), db, userSess.ID, "")
		if err != nil {
			log.Printf("[Playlist] handleGetPlaylists failed: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		payload := map[string]interface{}{
			"playlists": playlists,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(payload)
	}
}

func handleGetPlaylist(db *sql.DB, id string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userSess := r.Context().Value(core.UserContextKey).(*core.UserSession)
		initManagers(db)

		p, err := globalPlaylistManager.GetPlaylist(r.Context(), id)
		if err != nil {
			if err == sql.ErrNoRows {
				http.NotFound(w, r)
				return
			}
			log.Printf("[Playlist] GetPlaylist failed: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if userSess.Type != "root" && userSess.Type != "admin" && userSess.ID != p.UserID {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(p)
	}
}

func handleCreatePlaylist(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userSess := r.Context().Value(core.UserContextKey).(*core.UserSession)
		initManagers(db)

		var req struct {
			Name  string   `json:"name"`
			Items []string `json:"items"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		p := &playlist.Playlist{
			ID:      uuid.New().String(),
			UserID:  userSess.ID,
			Name:    req.Name,
			ItemIDs: req.Items,
		}

		if err := globalPlaylistManager.CreatePlaylist(r.Context(), p); err != nil {
			log.Printf("[Playlist] Create failed: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(p)
	}
}

func handleUpdatePlaylist(db *sql.DB, id string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userSess := r.Context().Value(core.UserContextKey).(*core.UserSession)
		initManagers(db)

		p, err := globalPlaylistManager.GetPlaylist(r.Context(), id)
		if err != nil {
			if err == sql.ErrNoRows {
				http.NotFound(w, r)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if userSess.Type != "root" && userSess.Type != "admin" && userSess.ID != p.UserID {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		var req struct {
			Name  string   `json:"name"`
			Items []string `json:"items"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if req.Name != "" {
			p.Name = req.Name
		}
		if req.Items != nil {
			p.ItemIDs = req.Items
		}

		if err := globalPlaylistManager.UpdatePlaylist(r.Context(), p); err != nil {
			log.Printf("[Playlist] Update failed: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(p)
	}
}

func handleDeletePlaylist(db *sql.DB, id string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userSess := r.Context().Value(core.UserContextKey).(*core.UserSession)
		initManagers(db)

		p, err := globalPlaylistManager.GetPlaylist(r.Context(), id)
		if err != nil {
			if err == sql.ErrNoRows {
				http.NotFound(w, r)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if userSess.Type != "root" && userSess.Type != "admin" && userSess.ID != p.UserID {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		if err := globalPlaylistManager.DeletePlaylist(r.Context(), id); err != nil {
			log.Printf("[Playlist] Delete failed: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"success": true}`))
	}
}

func handleGetCollections(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		initManagers(db)

		collections, err := queryCollectionsForLibrary(r.Context(), db, "")
		if err != nil {
			log.Printf("[Collection] handleGetCollections failed: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		payload := map[string]interface{}{
			"collections": collections,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(payload)
	}
}

func handleGetCollection(db *sql.DB, id string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		initManagers(db)

		c, err := globalPlaylistManager.GetCollection(r.Context(), id)
		if err != nil {
			if err == sql.ErrNoRows {
				http.NotFound(w, r)
				return
			}
			log.Printf("[Collection] GetCollection failed: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(c)
	}
}

func handleCreateCollection(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userSess := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if userSess.Type != "root" && userSess.Type != "admin" {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}
		initManagers(db)

		var req struct {
			Name        string   `json:"name"`
			Description string   `json:"description"`
			LibraryID   string   `json:"libraryId"`
			Books       []string `json:"books"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		c := &playlist.Collection{
			ID:          uuid.New().String(),
			Name:        req.Name,
			Description: req.Description,
			LibraryID:   req.LibraryID,
			ItemIDs:     req.Books,
		}

		if err := globalPlaylistManager.CreateCollection(r.Context(), c); err != nil {
			log.Printf("[Collection] Create failed: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(c)
	}
}

func handleUpdateCollection(db *sql.DB, id string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userSess := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if userSess.Type != "root" && userSess.Type != "admin" {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}
		initManagers(db)

		c, err := globalPlaylistManager.GetCollection(r.Context(), id)
		if err != nil {
			if err == sql.ErrNoRows {
				http.NotFound(w, r)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		var req struct {
			Name        string   `json:"name"`
			Description string   `json:"description"`
			LibraryID   string   `json:"libraryId"`
			Books       []string `json:"books"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if req.Name != "" {
			c.Name = req.Name
		}
		if req.Description != "" {
			c.Description = req.Description
		}
		if req.LibraryID != "" {
			c.LibraryID = req.LibraryID
		}
		if req.Books != nil {
			c.ItemIDs = req.Books
		}

		if err := globalPlaylistManager.UpdateCollection(r.Context(), c); err != nil {
			log.Printf("[Collection] Update failed: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(c)
	}
}

func handleDeleteCollection(db *sql.DB, id string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userSess := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if userSess.Type != "root" && userSess.Type != "admin" {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}
		initManagers(db)

		if err := globalPlaylistManager.DeleteCollection(r.Context(), id); err != nil {
			log.Printf("[Collection] Delete failed: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"success": true}`))
	}
}
