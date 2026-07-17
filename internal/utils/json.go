package utils

import (
	"database/sql"
	"encoding/json"
)

// ReplaceInJSONArray replaces oldVal with newVal in a JSON array string.
func ReplaceInJSONArray(jsonStr sql.NullString, oldVal, newVal string) (string, bool) {
	if !jsonStr.Valid || jsonStr.String == "" || jsonStr.String == "null" {
		return "[]", false
	}
	var arr []string
	if err := json.Unmarshal([]byte(jsonStr.String), &arr); err != nil {
		return jsonStr.String, false
	}
	found := false
	newArr := []string{}
	for _, val := range arr {
		if val == oldVal {
			found = true
			alreadyHasNew := false
			for _, v := range arr {
				if v == newVal {
					alreadyHasNew = true
					break
				}
			}
			if !alreadyHasNew {
				newArr = append(newArr, newVal)
			}
		} else {
			newArr = append(newArr, val)
		}
	}
	if !found {
		return jsonStr.String, false
	}
	res, _ := json.Marshal(newArr)
	return string(res), true
}

// RemoveFromJSONArray removes valToRemove from a JSON array string.
func RemoveFromJSONArray(jsonStr sql.NullString, valToRemove string) (string, bool) {
	if !jsonStr.Valid || jsonStr.String == "" || jsonStr.String == "null" {
		return "[]", false
	}
	var arr []string
	if err := json.Unmarshal([]byte(jsonStr.String), &arr); err != nil {
		return jsonStr.String, false
	}
	found := false
	newArr := []string{}
	for _, val := range arr {
		if val == valToRemove {
			found = true
		} else {
			newArr = append(newArr, val)
		}
	}
	if !found {
		return jsonStr.String, false
	}
	res, _ := json.Marshal(newArr)
	return string(res), true
}
