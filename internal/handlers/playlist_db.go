package handlers

import (
	"context"
	"database/sql"
	"strconv"
	"time"
)

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
