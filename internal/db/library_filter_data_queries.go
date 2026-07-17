package db

import (
	"database/sql"
	"encoding/json"
	"sort"
	"strconv"
	"strings"
)

func queryPodcastFilterData(db *sql.DB, libraryID string, fd *LibraryFilterData, genresSet, tagsSet, languagesSet map[string]bool) error {
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

	return db.QueryRow("SELECT COUNT(*) FROM libraryItems WHERE libraryId = ?", libraryID).Scan(&fd.PodcastCount)
}

func queryBookFilterData(
	db *sql.DB,
	libraryID string,
	fd *LibraryFilterData,
	genresSet, tagsSet, narratorsSet, languagesSet, publishersSet, decadesSet map[string]bool,
) error {
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

	return nil
}
