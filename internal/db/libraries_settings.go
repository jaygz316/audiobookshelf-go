package db

import (
	"fmt"
)

func getDefaultSettings(mediaType string) map[string]interface{} {
	if mediaType == "podcast" {
		return map[string]interface{}{
			"coverAspectRatio":              float64(1),
			"disableWatcher":                false,
			"autoScanCronExpression":        nil,
			"podcastSearchRegion":           "us",
			"markAsFinishedPercentComplete": nil,
			"markAsFinishedTimeRemaining":   float64(10),
		}
	}
	return map[string]interface{}{
		"coverAspectRatio":                   float64(1),
		"disableWatcher":                     false,
		"autoScanCronExpression":             nil,
		"skipMatchingMediaWithAsin":          false,
		"skipMatchingMediaWithIsbn":          false,
		"audiobooksOnly":                     false,
		"epubsAllowScriptedContent":          false,
		"hideSingleBookSeries":               false,
		"onlyShowLaterBooksInContinueSeries": false,
		"metadataPrecedence": []interface{}{
			"folderStructure", "audioMetatags", "nfoFile", "txtFiles", "opfFile", "absMetadata",
		},
		"markAsFinishedPercentComplete": nil,
		"markAsFinishedTimeRemaining":   float64(10),
	}
}

func mergeSettings(mediaType string, inputSettings map[string]interface{}) (map[string]interface{}, error) {
	settings := getDefaultSettings(mediaType)
	if inputSettings == nil {
		return settings, nil
	}

	for k, v := range inputSettings {
		if _, exists := settings[k]; !exists {
			continue
		}

		if v == nil {
			settings[k] = nil
			continue
		}

		if k == "metadataPrecedence" {
			arr, ok := v.([]interface{})
			if !ok {
				return nil, fmt.Errorf("settings \"metadataPrecedence\" must be an array")
			}
			for _, item := range arr {
				if _, ok := item.(string); !ok {
					return nil, fmt.Errorf("settings \"metadataPrecedence\" array elements must be strings")
				}
			}
			settings[k] = arr
		} else if k == "autoScanCronExpression" || k == "podcastSearchRegion" {
			if _, ok := v.(string); !ok {
				return nil, fmt.Errorf("settings \"%s\" must be a string", k)
			}
			settings[k] = v
		} else if k == "markAsFinishedPercentComplete" || k == "markAsFinishedTimeRemaining" {
			val, ok := v.(float64)
			if !ok {
				return nil, fmt.Errorf("setting \"%s\" must be a number", k)
			}
			if k == "markAsFinishedPercentComplete" {
				if val < 0 || val > 100 {
					return nil, fmt.Errorf("setting \"%s\" must be between 0 and 100", k)
				}
			} else if k == "markAsFinishedTimeRemaining" {
				if val < 0 {
					return nil, fmt.Errorf("setting \"%s\" must be greater than or equal to 0", k)
				}
			}
			settings[k] = val
		} else {
			switch settings[k].(type) {
			case bool:
				if _, ok := v.(bool); !ok {
					return nil, fmt.Errorf("setting \"%s\" must be of type bool", k)
				}
			case float64:
				if _, ok := v.(float64); !ok {
					return nil, fmt.Errorf("setting \"%s\" must be of type number", k)
				}
			case string:
				if _, ok := v.(string); !ok {
					return nil, fmt.Errorf("setting \"%s\" must be of type string", k)
				}
			}
			settings[k] = v
		}
	}
	return settings, nil
}
