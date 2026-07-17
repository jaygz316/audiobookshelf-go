package feed

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
)

func (m *FeedManager) checkUserAccess(ctx context.Context, userID, libraryID string) (bool, error) {
	var userType string
	var isActive int
	var permissionsStr sql.NullString
	err := m.db.QueryRowContext(ctx, "SELECT type, isActive, permissions FROM users WHERE id = ?", userID).Scan(&userType, &isActive, &permissionsStr)
	if err == sql.ErrNoRows {
		return false, fmt.Errorf("user not found: %w", err)
	}
	if err != nil {
		return false, fmt.Errorf("query user permissions: %w", err)
	}
	if isActive == 0 {
		return false, fmt.Errorf("user is inactive")
	}
	if userType == "root" || userType == "admin" {
		return true, nil
	}
	if !permissionsStr.Valid || permissionsStr.String == "" {
		return false, nil
	}

	type userPermissions struct {
		AccessAllLibraries  *bool    `json:"accessAllLibraries"`
		LibrariesAccessible []string `json:"librariesAccessible"`
	}
	var perm userPermissions
	if err := json.Unmarshal([]byte(permissionsStr.String), &perm); err != nil {
		return false, fmt.Errorf("unmarshal user permissions: %w", err)
	}

	if perm.AccessAllLibraries != nil && *perm.AccessAllLibraries {
		return true, nil
	}
	for _, lid := range perm.LibrariesAccessible {
		if lid == libraryID {
			return true, nil
		}
	}
	return false, nil
}

func hasColumn(ctx context.Context, db *sql.DB, tableName, columnName string) bool {
	rows, err := db.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info(%s)", tableName))
	if err != nil {
		return false
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, dType string
		var notnull int
		var dfltVal sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &dType, &notnull, &dfltVal, &pk); err == nil {
			if name == columnName {
				return true
			}
		}
	}
	if err := rows.Err(); err != nil {
		return false
	}
	return false
}
