package db

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"audiobookshelf/internal/core"
)

type GetFilteredLibraryItemsOptions struct {
	LibraryID      string
	User           *core.UserSession
	FilterBy       string
	SortBy         string
	SortDesc       bool
	Limit          int
	Page           int
	CollapseSeries bool
	Include        []string
	MediaType      string
	Minified       bool
	Search         string
}

type LibraryFilterData struct {
	Authors          []map[string]string `json:"authors"`
	Genres           []string            `json:"genres"`
	Tags             []string            `json:"tags"`
	Series           []map[string]string `json:"series"`
	Narrators        []string            `json:"narrators"`
	Languages        []string            `json:"languages"`
	Publishers       []string            `json:"publishers"`
	PublishedDecades []string            `json:"publishedDecades"`
	BookCount        int                 `json:"bookCount"`
	AuthorCount      int                 `json:"authorCount"`
	SeriesCount      int                 `json:"seriesCount"`
	PodcastCount     int                 `json:"podcastCount"`
	NumIssues        int                 `json:"numIssues"`
}

func decodeFilterValue(s string) string {
	decoded, err := url.QueryUnescape(s)
	if err != nil {
		decoded = s
	}
	data, err := base64.StdEncoding.DecodeString(decoded)
	if err != nil {
		return decoded
	}
	return string(data)
}

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
		if value == "in-progress" {
			return fmt.Sprintf("EXISTS (SELECT 1 FROM mediaProgresses mp WHERE mp.userId = ? AND mp.mediaItemId = %s.id AND mp.isFinished = 0 AND mp.currentTime > 0)", tableAlias), []interface{}{userID}
		} else if value == "finished" {
			return fmt.Sprintf("EXISTS (SELECT 1 FROM mediaProgresses mp WHERE mp.userId = ? AND mp.mediaItemId = %s.id AND mp.isFinished = 1)", tableAlias), []interface{}{userID}
		} else if value == "not-started" {
			return fmt.Sprintf("NOT EXISTS (SELECT 1 FROM mediaProgresses mp WHERE mp.userId = ? AND mp.mediaItemId = %s.id AND (mp.isFinished = 1 OR mp.currentTime > 0))", tableAlias), []interface{}{userID}
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

func GetLibraryFilterDataGo(db *sql.DB, libraryID string) (*LibraryFilterData, error) {
	var mediaType string
	err := db.QueryRow("SELECT mediaType FROM libraries WHERE id = ?", libraryID).Scan(&mediaType)
	if err != nil {
		return nil, err
	}

	fd := &LibraryFilterData{
		Authors:          []map[string]string{},
		Genres:           []string{},
		Tags:             []string{},
		Series:           []map[string]string{},
		Narrators:        []string{},
		Languages:        []string{},
		Publishers:       []string{},
		PublishedDecades: []string{},
	}

	genresSet := make(map[string]bool)
	tagsSet := make(map[string]bool)
	narratorsSet := make(map[string]bool)
	languagesSet := make(map[string]bool)
	publishersSet := make(map[string]bool)
	decadesSet := make(map[string]bool)

	if mediaType == "podcast" {
		rows, err := db.Query(`SELECT p.tags, p.genres, p.language 
			FROM podcasts p JOIN libraryItems li ON li.mediaId = p.id WHERE li.libraryId = ?`, libraryID)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var tagsStr, genresStr, langStr sql.NullString
				if err := rows.Scan(&tagsStr, &genresStr, &langStr); err == nil {
					if tagsStr.Valid && tagsStr.String != "" {
						var arr []string
						if json.Unmarshal([]byte(tagsStr.String), &arr) == nil {
							for _, v := range arr {
								if v != "" {
									tagsSet[v] = true
								}
							}
						}
					}
					if genresStr.Valid && genresStr.String != "" {
						var arr []string
						if json.Unmarshal([]byte(genresStr.String), &arr) == nil {
							for _, v := range arr {
								if v != "" {
									genresSet[v] = true
								}
							}
						}
					}
					if langStr.Valid && langStr.String != "" {
						languagesSet[langStr.String] = true
					}
				}
			}
		}

		db.QueryRow("SELECT COUNT(*) FROM libraryItems WHERE libraryId = ?", libraryID).Scan(&fd.PodcastCount)

	} else {
		rows, err := db.Query(`SELECT b.tags, b.genres, b.narrators, b.publisher, b.publishedYear, b.language, li.isMissing, li.isInvalid 
			FROM books b JOIN libraryItems li ON li.mediaId = b.id WHERE li.libraryId = ?`, libraryID)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var tagsStr, genresStr, narrStr, pubStr, langStr sql.NullString
				var pubYear sql.NullInt64
				var isMissingVal, isInvalidVal int
				if err := rows.Scan(&tagsStr, &genresStr, &narrStr, &pubStr, &pubYear, &langStr, &isMissingVal, &isInvalidVal); err == nil {
					if isMissingVal != 0 || isInvalidVal != 0 {
						fd.NumIssues++
					}
					if tagsStr.Valid && tagsStr.String != "" {
						var arr []string
						if json.Unmarshal([]byte(tagsStr.String), &arr) == nil {
							for _, v := range arr {
								if v != "" {
									tagsSet[v] = true
								}
							}
						}
					}
					if genresStr.Valid && genresStr.String != "" {
						var arr []string
						if json.Unmarshal([]byte(genresStr.String), &arr) == nil {
							for _, v := range arr {
								if v != "" {
									genresSet[v] = true
								}
							}
						}
					}
					if narrStr.Valid && narrStr.String != "" {
						var arr []string
						if json.Unmarshal([]byte(narrStr.String), &arr) == nil {
							for _, v := range arr {
								if v != "" {
									narratorsSet[v] = true
								}
							}
						}
					}
					if pubStr.Valid && pubStr.String != "" {
						publishersSet[pubStr.String] = true
					}
					if langStr.Valid && langStr.String != "" {
						languagesSet[langStr.String] = true
					}
					if pubYear.Valid && pubYear.Int64 > 0 && pubYear.Int64 < 3000 {
						decade := (pubYear.Int64 / 10) * 10
						decadesSet[strconv.FormatInt(decade, 10)] = true
					}
				}
			}
		}

		db.QueryRow("SELECT COUNT(*) FROM libraryItems WHERE libraryId = ?", libraryID).Scan(&fd.BookCount)
		db.QueryRow("SELECT COUNT(*) FROM series WHERE libraryId = ?", libraryID).Scan(&fd.SeriesCount)
		db.QueryRow("SELECT COUNT(*) FROM authors WHERE libraryId = ?", libraryID).Scan(&fd.AuthorCount)

		// Get authors list
		authRows, err := db.Query("SELECT id, name FROM authors WHERE libraryId = ?", libraryID)
		if err == nil {
			defer authRows.Close()
			for authRows.Next() {
				var id, name string
				if err := authRows.Scan(&id, &name); err == nil {
					fd.Authors = append(fd.Authors, map[string]string{"id": id, "name": name})
				}
			}
			sort.Slice(fd.Authors, func(i, j int) bool {
				return strings.ToLower(fd.Authors[i]["name"]) < strings.ToLower(fd.Authors[j]["name"])
			})
		}

		// Get series list
		serRows, err := db.Query("SELECT id, name FROM series WHERE libraryId = ?", libraryID)
		if err == nil {
			defer serRows.Close()
			for serRows.Next() {
				var id, name string
				if err := serRows.Scan(&id, &name); err == nil {
					fd.Series = append(fd.Series, map[string]string{"id": id, "name": name})
				}
			}
			sort.Slice(fd.Series, func(i, j int) bool {
				return strings.ToLower(fd.Series[i]["name"]) < strings.ToLower(fd.Series[j]["name"])
			})
		}
	}

	for k := range genresSet {
		fd.Genres = append(fd.Genres, k)
	}
	sort.Strings(fd.Genres)

	for k := range tagsSet {
		fd.Tags = append(fd.Tags, k)
	}
	sort.Strings(fd.Tags)

	for k := range narratorsSet {
		fd.Narrators = append(fd.Narrators, k)
	}
	sort.Strings(fd.Narrators)

	for k := range languagesSet {
		fd.Languages = append(fd.Languages, k)
	}
	sort.Strings(fd.Languages)

	for k := range publishersSet {
		fd.Publishers = append(fd.Publishers, k)
	}
	sort.Strings(fd.Publishers)

	for k := range decadesSet {
		fd.PublishedDecades = append(fd.PublishedDecades, k)
	}
	sort.Strings(fd.PublishedDecades)

	return fd, nil
}
