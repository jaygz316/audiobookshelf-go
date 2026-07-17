package handlers

import (
	"context"
	"database/sql"
	"strings"
)

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

func queryCollectionsForLibrary(ctx context.Context, db *sql.DB, libraryID string) ([]map[string]interface{}, error) {
	hasDisplayOrder := hasColumn(ctx, db, "collections", "displayOrder")
	var query string
	var args []interface{}
	if libraryID != "" {
		if hasDisplayOrder {
			query = "SELECT id, name, description, libraryId, displayOrder, isSmart, rules, createdAt, updatedAt FROM collections WHERE libraryId = ?"
		} else {
			query = "SELECT id, name, description, libraryId, isSmart, rules, createdAt, updatedAt FROM collections WHERE libraryId = ?"
		}
		args = append(args, libraryID)
	} else {
		if hasDisplayOrder {
			query = "SELECT id, name, description, libraryId, displayOrder, isSmart, rules, createdAt, updatedAt FROM collections"
		} else {
			query = "SELECT id, name, description, libraryId, isSmart, rules, createdAt, updatedAt FROM collections"
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
		var isSmartVal sql.NullInt64
		var rulesVal sql.NullString

		var err error
		if hasDisplayOrder {
			err = rows.Scan(&id, &name, &description, &libraryIDCol, &displayOrder, &isSmartVal, &rulesVal, &createdAtStr, &updatedAtStr)
		} else {
			err = rows.Scan(&id, &name, &description, &libraryIDCol, &isSmartVal, &rulesVal, &createdAtStr, &updatedAtStr)
		}
		if err != nil {
			return nil, err
		}

		c := map[string]interface{}{
			"id":          id,
			"name":        name,
			"description": description.String,
			"libraryId":   libraryIDCol.String,
			"isSmart":     isSmartVal.Int64 != 0,
			"rules":       rulesVal.String,
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
		isSmart := c["isSmart"].(bool)
		rules := c["rules"].(string)
		var items []string

		if isSmart {
			initManagers(db)
			dynamicIDs, err := globalPlaylistManager.ResolveSmartCollectionItems(ctx, c["libraryId"].(string), rules)
			if err != nil {
				return nil, err
			}
			items = dynamicIDs
		} else {
			itemRows, err := db.QueryContext(ctx, `SELECT bookId FROM collectionBooks WHERE collectionId = ? ORDER BY "order" ASC`, cID)
			if err != nil {
				return nil, err
			}
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
		}
		c["books"] = items
		c["itemIds"] = items
	}

	return collections, nil
}
