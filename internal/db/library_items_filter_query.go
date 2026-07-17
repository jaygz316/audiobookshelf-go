package db

import (
	"database/sql"
	"fmt"
	"strings"
)

func buildFilteredItemsWhere(options GetFilteredLibraryItemsOptions) (string, []interface{}) {
	var conds []string
	var args []interface{}

	conds = append(conds, "li.libraryId = ?")
	args = append(args, options.LibraryID)

	if len(options.Include) > 0 {
		var placeholders []string
		for _, inc := range options.Include {
			placeholders = append(placeholders, "?")
			args = append(args, inc)
		}
		conds = append(conds, fmt.Sprintf("li.id IN (%s)", strings.Join(placeholders, ", ")))
	}

	var tableAlias string
	if options.MediaType == "book" {
		tableAlias = "b"
	} else {
		tableAlias = "p"
	}

	permCond, permArgs := getUserPermissionWhere(options.User, tableAlias)
	if permCond != "" {
		conds = append(conds, permCond)
		args = append(args, permArgs...)
	}

	filterCond, filterArgs := getFilterWhere(options.FilterBy, options.MediaType, tableAlias, "li", options.User.ID)
	if filterCond != "" {
		conds = append(conds, filterCond)
		args = append(args, filterArgs...)
	}

	if options.Search != "" {
		searchTerm := "%" + options.Search + "%"
		if options.MediaType == "book" {
			conds = append(conds, "(b.title LIKE ? OR li.authorNamesFirstLast LIKE ? OR b.subtitle LIKE ? OR b.description LIKE ?)")
			args = append(args, searchTerm, searchTerm, searchTerm, searchTerm)
		} else {
			conds = append(conds, "(p.title LIKE ? OR p.author LIKE ? OR p.description LIKE ?)")
			args = append(args, searchTerm, searchTerm, searchTerm)
		}
	}

	whereClause := "WHERE " + strings.Join(conds, " AND ")
	return whereClause, args
}

func executeFilteredItemsCount(db *sql.DB, mediaType string, whereClause string, args []interface{}) (int, error) {
	var countQuery string
	if mediaType == "book" {
		countQuery = fmt.Sprintf("SELECT COUNT(*) FROM libraryItems li JOIN books b ON li.mediaId = b.id AND li.mediaType = 'book' %s", whereClause)
	} else {
		countQuery = fmt.Sprintf("SELECT COUNT(*) FROM libraryItems li JOIN podcasts p ON li.mediaId = p.id AND li.mediaType = 'podcast' %s", whereClause)
	}

	var total int
	err := db.QueryRow(countQuery, args...).Scan(&total)
	if err != nil {
		return 0, err
	}
	return total, nil
}

func buildFilteredItemsSelectQuery(options GetFilteredLibraryItemsOptions, whereClause string, sortingIgnorePrefix bool, args *[]interface{}) string {
	orderClause := "ORDER BY " + getSortOrder(options.SortBy, options.SortDesc, sortingIgnorePrefix, options.MediaType, options.User.ID, args)

	limitOffsetClause := ""
	if options.Limit > 0 {
		limitOffsetClause = fmt.Sprintf("LIMIT %d OFFSET %d", options.Limit, options.Page*options.Limit)
	}

	var selectQuery string
	if options.MediaType == "book" {
		selectQuery = fmt.Sprintf(`
			SELECT 
				li.id, li.ino, li.path, li.relPath, li.isFile, li.mtime, li.ctime, li.birthtime, li.createdAt, li.updatedAt, li.isMissing, li.isInvalid, li.mediaType, li.mediaId, li.size, li.libraryFolderId, li.authorNamesFirstLast, li.authorNamesLastFirst,
				b.id, b.title, b.titleIgnorePrefix, b.subtitle, b.publishedYear, b.publishedDate, b.publisher, b.description, b.isbn, b.asin, b.language, b.explicit, b.abridged, b.coverPath, b.duration, b.narrators, b.audioFiles, b.ebookFile, b.chapters, b.tags, b.genres, b.lockedFields
			FROM libraryItems li
			JOIN books b ON li.mediaId = b.id AND li.mediaType = 'book'
			%s
			%s
			%s
		`, whereClause, orderClause, limitOffsetClause)
	} else {
		selectQuery = fmt.Sprintf(`
			SELECT 
				li.id, li.ino, li.path, li.relPath, li.isFile, li.mtime, li.ctime, li.birthtime, li.createdAt, li.updatedAt, li.isMissing, li.isInvalid, li.mediaType, li.mediaId, li.size, li.libraryFolderId,
				p.id, p.title, p.titleIgnorePrefix, p.author, p.releaseDate, p.feedURL, p.imageURL, p.description, p.itunesPageURL, p.itunesId, p.itunesArtistId, p.language, p.podcastType, p.explicit, p.autoDownloadEpisodes, p.autoDownloadSchedule, p.lastEpisodeCheck, p.maxEpisodesToKeep, p.maxNewEpisodesToDownload, p.coverPath, p.tags, p.genres, p.numEpisodes, p.skipIntroDuration, p.skipOutroDuration, p.autoDeletePlayed, p.lockedFields
			FROM libraryItems li
			JOIN podcasts p ON li.mediaId = p.id AND li.mediaType = 'podcast'
			%s
			%s
			%s
		`, whereClause, orderClause, limitOffsetClause)
	}
	return selectQuery
}
