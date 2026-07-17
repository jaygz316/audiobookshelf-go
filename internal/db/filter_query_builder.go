package db

import (
	"fmt"
	"strings"

	"audiobookshelf/internal/core"
)

func getUserPermissionWhere(user *core.UserSession, tableAlias string) (string, []interface{}) {
	var conds []string
	var args []interface{}

	// Explicit content restriction
	if !user.CanAccessExplicitContent {
		conds = append(conds, fmt.Sprintf("%s.explicit = 0", tableAlias))
	}

	// Tag restriction
	if !user.AccessAllTags && len(user.ItemTagsSelected) > 0 {
		placeholders := ""
		for i, tag := range user.ItemTagsSelected {
			if i > 0 {
				placeholders += ","
			}
			placeholders += "?"
			args = append(args, tag)
		}
		if user.SelectedTagsNotAccessible {
			conds = append(conds, fmt.Sprintf("(SELECT count(*) FROM json_each(%s.tags) WHERE json_valid(%s.tags) AND json_each.value IN (%s)) = 0", tableAlias, tableAlias, placeholders))
		} else {
			conds = append(conds, fmt.Sprintf("(SELECT count(*) FROM json_each(%s.tags) WHERE json_valid(%s.tags) AND json_each.value IN (%s)) >= 1", tableAlias, tableAlias, placeholders))
		}
	}

	if len(conds) == 0 {
		return "", nil
	}

	return strings.Join(conds, " AND "), args
}

func getFilterWhere(filterBy string, mediaType string, tableAlias string, liAlias string, userID string) (string, []interface{}) {
	if filterBy == "" {
		return "", nil
	}
	parts := strings.SplitN(filterBy, ".", 2)
	group := parts[0]
	var value string
	if len(parts) == 2 {
		value = decodeFilterValue(parts[1])
	}

	switch group {
	case "authors":
		if mediaType == "book" {
			return fmt.Sprintf("%s.id IN (SELECT bookId FROM bookAuthors WHERE authorId = ?)", tableAlias), []interface{}{value}
		}
	case "series":
		if mediaType == "book" {
			if value == "no-series" {
				return fmt.Sprintf("NOT EXISTS (SELECT 1 FROM bookSeries bs WHERE bs.bookId = %s.id)", tableAlias), nil
			}
			return fmt.Sprintf("%s.id IN (SELECT bookId FROM bookSeries WHERE seriesId = ?)", tableAlias), []interface{}{value}
		}
	case "genres":
		return fmt.Sprintf("EXISTS (SELECT 1 FROM json_each(%s.genres) WHERE json_valid(%s.genres) AND json_each.value = ?)", tableAlias, tableAlias), []interface{}{value}
	case "tags":
		return fmt.Sprintf("EXISTS (SELECT 1 FROM json_each(%s.tags) WHERE json_valid(%s.tags) AND json_each.value = ?)", tableAlias, tableAlias), []interface{}{value}
	case "narrators":
		if mediaType == "book" {
			return fmt.Sprintf("EXISTS (SELECT 1 FROM json_each(%s.narrators) WHERE json_valid(%s.narrators) AND json_each.value = ?)", tableAlias, tableAlias), []interface{}{value}
		}
	case "languages":
		return fmt.Sprintf("%s.language = ?", tableAlias), []interface{}{value}
	case "publishers":
		if mediaType == "book" {
			return fmt.Sprintf("%s.publisher = ?", tableAlias), []interface{}{value}
		}
	case "progress":
		var idField string
		if mediaType == "podcast" {
			idField = "mp.podcastId"
		} else {
			idField = "mp.mediaItemId"
		}
		if value == "in-progress" {
			return fmt.Sprintf("EXISTS (SELECT 1 FROM mediaProgresses mp WHERE mp.userId = ? AND %s = %s.id AND mp.isFinished = 0 AND mp.currentTime > 0)", idField, tableAlias), []interface{}{userID}
		} else if value == "finished" {
			return fmt.Sprintf("EXISTS (SELECT 1 FROM mediaProgresses mp WHERE mp.userId = ? AND %s = %s.id AND mp.isFinished = 1)", idField, tableAlias), []interface{}{userID}
		} else if value == "not-started" {
			return fmt.Sprintf("NOT EXISTS (SELECT 1 FROM mediaProgresses mp WHERE mp.userId = ? AND %s = %s.id AND (mp.isFinished = 1 OR mp.currentTime > 0))", idField, tableAlias), []interface{}{userID}
		}
	case "missing":
		return fmt.Sprintf("(%s.isMissing = 1 OR %s.isInvalid = 1)", liAlias, liAlias), nil
	case "decades":
		if mediaType == "book" {
			var startYear int
			_, err := fmt.Sscanf(value, "%d", &startYear)
			if err == nil {
				return fmt.Sprintf("CAST(%s.publishedYear AS INTEGER) >= ? AND CAST(%s.publishedYear AS INTEGER) < ?", tableAlias, tableAlias), []interface{}{startYear, startYear + 10}
			}
		}
	case "years":
		if mediaType == "book" {
			return fmt.Sprintf("%s.publishedYear = ?", tableAlias), []interface{}{value}
		}
	case "duration":
		if value == "under-1h" {
			return fmt.Sprintf("%s.duration < 3600", tableAlias), nil
		} else if value == "1h-5h" {
			return fmt.Sprintf("%s.duration >= 3600 AND %s.duration < 18000", tableAlias, tableAlias), nil
		} else if value == "5h-10h" {
			return fmt.Sprintf("%s.duration >= 18000 AND %s.duration < 36000", tableAlias, tableAlias), nil
		} else if value == "over-10h" {
			return fmt.Sprintf("%s.duration >= 36000", tableAlias), nil
		}
	case "folder":
		return fmt.Sprintf("%s.libraryFolderId = ?", liAlias), []interface{}{value}
	}
	return "", nil
}
