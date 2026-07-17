package db

import (
	"encoding/json"
)

// JsonArrayToCommaString converts a JSON array of strings to a comma-separated string.
func JsonArrayToCommaString(jsonBytes []byte) string {
	if len(jsonBytes) == 0 {
		return ""
	}
	var arr []string
	if err := json.Unmarshal(jsonBytes, &arr); err != nil {
		return ""
	}
	result := ""
	for i, s := range arr {
		if i > 0 {
			result += ", "
		}
		result += s
	}
	return result
}
