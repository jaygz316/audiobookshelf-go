package utils

import (
	"database/sql"
	"testing"
)

func TestReplaceInJSONArray(t *testing.T) {
	tests := []struct {
		name          string
		jsonStr       sql.NullString
		oldVal        string
		newVal        string
		expectedStr   string
		expectedFound bool
	}{
		{
			name:          "null jsonStr",
			jsonStr:       sql.NullString{String: "", Valid: false},
			oldVal:        "old",
			newVal:        "new",
			expectedStr:   "[]",
			expectedFound: false,
		},
		{
			name:          "empty jsonStr",
			jsonStr:       sql.NullString{String: "", Valid: true},
			oldVal:        "old",
			newVal:        "new",
			expectedStr:   "[]",
			expectedFound: false,
		},
		{
			name:          "null string jsonStr",
			jsonStr:       sql.NullString{String: "null", Valid: true},
			oldVal:        "old",
			newVal:        "new",
			expectedStr:   "[]",
			expectedFound: false,
		},
		{
			name:          "invalid jsonStr",
			jsonStr:       sql.NullString{String: "{invalid}", Valid: true},
			oldVal:        "old",
			newVal:        "new",
			expectedStr:   "{invalid}",
			expectedFound: false,
		},
		{
			name:          "empty array",
			jsonStr:       sql.NullString{String: "[]", Valid: true},
			oldVal:        "old",
			newVal:        "new",
			expectedStr:   "[]",
			expectedFound: false,
		},
		{
			name:          "simple replace",
			jsonStr:       sql.NullString{String: `["old"]`, Valid: true},
			oldVal:        "old",
			newVal:        "new",
			expectedStr:   `["new"]`,
			expectedFound: true,
		},
		{
			name:          "replace in multiple values",
			jsonStr:       sql.NullString{String: `["old","other"]`, Valid: true},
			oldVal:        "old",
			newVal:        "new",
			expectedStr:   `["new","other"]`,
			expectedFound: true,
		},
		{
			name:          "replace when newVal already exists",
			jsonStr:       sql.NullString{String: `["old","new"]`, Valid: true},
			oldVal:        "old",
			newVal:        "new",
			expectedStr:   `["new"]`,
			expectedFound: true,
		},
		{
			name:          "replace when newVal already exists, keeping order",
			jsonStr:       sql.NullString{String: `["old","other","new"]`, Valid: true},
			oldVal:        "old",
			newVal:        "new",
			expectedStr:   `["other","new"]`,
			expectedFound: true,
		},
		{
			name:          "old value not found",
			jsonStr:       sql.NullString{String: `["other"]`, Valid: true},
			oldVal:        "old",
			newVal:        "new",
			expectedStr:   `["other"]`,
			expectedFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotStr, gotFound := ReplaceInJSONArray(tt.jsonStr, tt.oldVal, tt.newVal)
			if gotFound != tt.expectedFound {
				t.Errorf("ReplaceInJSONArray() gotFound = %v, want %v", gotFound, tt.expectedFound)
			}
			if gotStr != tt.expectedStr {
				t.Errorf("ReplaceInJSONArray() gotStr = %q, want %q", gotStr, tt.expectedStr)
			}
		})
	}
}

func TestRemoveFromJSONArray(t *testing.T) {
	tests := []struct {
		name          string
		jsonStr       sql.NullString
		valToRemove   string
		expectedStr   string
		expectedFound bool
	}{
		{
			name:          "null jsonStr",
			jsonStr:       sql.NullString{String: "", Valid: false},
			valToRemove:   "val",
			expectedStr:   "[]",
			expectedFound: false,
		},
		{
			name:          "empty jsonStr",
			jsonStr:       sql.NullString{String: "", Valid: true},
			valToRemove:   "val",
			expectedStr:   "[]",
			expectedFound: false,
		},
		{
			name:          "invalid jsonStr",
			jsonStr:       sql.NullString{String: "{invalid}", Valid: true},
			valToRemove:   "val",
			expectedStr:   "{invalid}",
			expectedFound: false,
		},
		{
			name:          "val found and removed",
			jsonStr:       sql.NullString{String: `["val","other"]`, Valid: true},
			valToRemove:   "val",
			expectedStr:   `["other"]`,
			expectedFound: true,
		},
		{
			name:          "val not found",
			jsonStr:       sql.NullString{String: `["other"]`, Valid: true},
			valToRemove:   "val",
			expectedStr:   `["other"]`,
			expectedFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotStr, gotFound := RemoveFromJSONArray(tt.jsonStr, tt.valToRemove)
			if gotFound != tt.expectedFound {
				t.Errorf("RemoveFromJSONArray() gotFound = %v, want %v", gotFound, tt.expectedFound)
			}
			if gotStr != tt.expectedStr {
				t.Errorf("RemoveFromJSONArray() gotStr = %q, want %q", gotStr, tt.expectedStr)
			}
		})
	}
}
