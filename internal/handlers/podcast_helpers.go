package handlers

import (
	"database/sql"
	"fmt"
	"strings"
)

func sanitizeFilename(name string) string {
	invalid := []string{"/", "\\", "?", "%", "*", ":", "|", "\"", "<", ">", "."}
	res := name
	for _, char := range invalid {
		res = strings.ReplaceAll(res, char, "")
	}
	res = strings.TrimSpace(res)
	if res == "" {
		res = "unnamed"
	}
	return res
}

func getTableColumnsTx(tx *sql.Tx, tableName string) map[string]bool {
	cols := make(map[string]bool)
	rows, err := tx.Query(fmt.Sprintf("PRAGMA table_info(%s)", tableName))
	if err != nil {
		return cols
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pkey int
		var dfltVal interface{}
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dfltVal, &pkey); err == nil {
			cols[name] = true
		}
	}
	return cols
}

func getTableColumns(db *sql.DB, tableName string) map[string]bool {
	cols := make(map[string]bool)
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", tableName))
	if err != nil {
		return cols
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pkey int
		var dfltVal interface{}
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dfltVal, &pkey); err == nil {
			cols[name] = true
		}
	}
	return cols
}

func explicitInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
