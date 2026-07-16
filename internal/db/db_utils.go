package db

import (
	"database/sql"
	"encoding/json"
)

// jsonUnmarshalSafe unmarshals JSON safely, returning false on error.
func jsonUnmarshalSafe(data []byte, v interface{}) bool {
	return json.Unmarshal(data, v) == nil
}

// parseEpochMillis delegates to internal/db.
func parseEpochMillis(s string) int64 {
	return ParseEpochMillis(s)
}

// jsonArrayToCommaString delegates to internal/db.
func jsonArrayToCommaString(jsonBytes []byte) string {
	return JsonArrayToCommaString(jsonBytes)
}

func TableExistsTx(tx *sql.Tx, tableName string) bool {
	var name string
	err := tx.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", tableName).Scan(&name)
	return err == nil && name == tableName
}

func tableExistsTx(tx *sql.Tx, tableName string) bool {
	return TableExistsTx(tx, tableName)
}

// nullIfEmpty returns nil if s is empty, otherwise returns a pointer to s.
func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
