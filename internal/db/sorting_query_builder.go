package db

import (
	"fmt"
)

func getSortOrder(sortBy string, sortDesc bool, sortingIgnorePrefix bool, mediaType string, userID string, args *[]interface{}) string {
	dir := "ASC"
	if sortDesc {
		dir = "DESC"
	}

	titleCol := "li.title"
	if sortingIgnorePrefix {
		titleCol = "li.titleIgnorePrefix"
	}

	switch sortBy {
	case "addedAt":
		return fmt.Sprintf("li.createdAt %s", dir)
	case "size":
		return fmt.Sprintf("li.size %s", dir)
	case "birthtimeMs":
		return fmt.Sprintf("li.birthtime %s", dir)
	case "mtimeMs":
		return fmt.Sprintf("li.mtime %s", dir)
	case "media.duration":
		if mediaType == "book" {
			return fmt.Sprintf("b.duration %s", dir)
		}
	case "media.metadata.publishedYear":
		if mediaType == "book" {
			return fmt.Sprintf("CAST(b.publishedYear AS INTEGER) %s", dir)
		}
	case "media.metadata.authorNameLF":
		if mediaType == "book" {
			return fmt.Sprintf("li.authorNamesLastFirst COLLATE NOCASE %s, %s COLLATE NOCASE %s", dir, titleCol, dir)
		}
	case "media.metadata.authorName":
		if mediaType == "book" {
			return fmt.Sprintf("li.authorNamesFirstLast COLLATE NOCASE %s, %s COLLATE NOCASE %s", dir, titleCol, dir)
		}
	case "media.metadata.title":
		return fmt.Sprintf("%s COLLATE NOCASE %s", titleCol, dir)
	case "sequence":
		nullDir := "ASC NULLS LAST"
		if sortDesc {
			nullDir = "DESC NULLS FIRST"
		}
		if mediaType == "book" {
			return fmt.Sprintf("CAST((SELECT sequence FROM bookSeries bs WHERE bs.bookId = b.id LIMIT 1) AS FLOAT) %s", nullDir)
		}
	case "progress":
		nullDir := "ASC NULLS LAST"
		if sortDesc {
			nullDir = "DESC NULLS FIRST"
		}
		if mediaType == "book" {
			*args = append(*args, userID)
			return fmt.Sprintf("(SELECT mp.updatedAt FROM mediaProgresses mp WHERE mp.mediaItemId = b.id AND mp.userId = ?) %s", nullDir)
		} else if mediaType == "podcast" {
			*args = append(*args, userID)
			return fmt.Sprintf("(SELECT MAX(mp.updatedAt) FROM mediaProgresses mp WHERE mp.podcastId = p.id AND mp.userId = ?) %s", nullDir)
		}
	case "media.metadata.author":
		if mediaType == "podcast" {
			return fmt.Sprintf("p.author COLLATE NOCASE %s", dir)
		}
	case "media.numTracks":
		if mediaType == "podcast" {
			return fmt.Sprintf("p.numEpisodes %s", dir)
		}
	case "random":
		return "random()"
	}

	return fmt.Sprintf("%s COLLATE NOCASE %s", titleCol, dir)
}
