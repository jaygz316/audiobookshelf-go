package handlers

import (
	"database/sql"
	"encoding/json"
	"sort"
	"strings"
)

type TagResult struct {
	Name     string `json:"name"`
	NumItems int    `json:"numItems"`
}

type GenreResult struct {
	Name     string `json:"name"`
	NumItems int    `json:"numItems"`
}

type NarratorResult struct {
	Name     string `json:"name"`
	NumBooks int    `json:"numBooks"`
}

func searchTaxonomy(db *sql.DB, libraryID string, q string, limit int) ([]TagResult, []GenreResult, []NarratorResult) {
	tagsMap := make(map[string]int)
	genresMap := make(map[string]int)
	narratorsMap := make(map[string]int)

	rowsBooks, err := db.Query(`
		SELECT b.tags, b.genres, b.narrators
		FROM books b
		JOIN libraryItems li ON li.mediaId = b.id AND li.mediaType = 'book'
		WHERE li.libraryId = ?
	`, libraryID)
	if err == nil {
		defer rowsBooks.Close()
		for rowsBooks.Next() {
			var tagsStr, genresStr, narrStr sql.NullString
			if err := rowsBooks.Scan(&tagsStr, &genresStr, &narrStr); err == nil {
				if tagsStr.Valid && tagsStr.String != "" {
					var arr []string
					if json.Unmarshal([]byte(tagsStr.String), &arr) == nil {
						for _, v := range arr {
							if v != "" {
								tagsMap[v]++
							}
						}
					}
				}
				if genresStr.Valid && genresStr.String != "" {
					var arr []string
					if json.Unmarshal([]byte(genresStr.String), &arr) == nil {
						for _, v := range arr {
							if v != "" {
								genresMap[v]++
							}
						}
					}
				}
				if narrStr.Valid && narrStr.String != "" {
					var arr []string
					if json.Unmarshal([]byte(narrStr.String), &arr) == nil {
						for _, v := range arr {
							if v != "" {
								narratorsMap[v]++
							}
						}
					}
				}
			}
		}
	}

	rowsPodcasts, err := db.Query(`
		SELECT p.tags, p.genres
		FROM podcasts p
		JOIN libraryItems li ON li.mediaId = p.id AND li.mediaType = 'podcast'
		WHERE li.libraryId = ?
	`, libraryID)
	if err == nil {
		defer rowsPodcasts.Close()
		for rowsPodcasts.Next() {
			var tagsStr, genresStr sql.NullString
			if err := rowsPodcasts.Scan(&tagsStr, &genresStr); err == nil {
				if tagsStr.Valid && tagsStr.String != "" {
					var arr []string
					if json.Unmarshal([]byte(tagsStr.String), &arr) == nil {
						for _, v := range arr {
							if v != "" {
								tagsMap[v]++
							}
						}
					}
				}
				if genresStr.Valid && genresStr.String != "" {
					var arr []string
					if json.Unmarshal([]byte(genresStr.String), &arr) == nil {
						for _, v := range arr {
							if v != "" {
								genresMap[v]++
							}
						}
					}
				}
			}
		}
	}

	var matchedTags []TagResult
	qLower := strings.ToLower(q)
	for name, count := range tagsMap {
		if strings.Contains(strings.ToLower(name), qLower) {
			matchedTags = append(matchedTags, TagResult{Name: name, NumItems: count})
		}
	}
	sort.Slice(matchedTags, func(i, j int) bool {
		if matchedTags[i].NumItems == matchedTags[j].NumItems {
			return strings.ToLower(matchedTags[i].Name) < strings.ToLower(matchedTags[j].Name)
		}
		return matchedTags[i].NumItems > matchedTags[j].NumItems
	})
	if len(matchedTags) > limit {
		matchedTags = matchedTags[:limit]
	}

	var matchedGenres []GenreResult
	for name, count := range genresMap {
		if strings.Contains(strings.ToLower(name), qLower) {
			matchedGenres = append(matchedGenres, GenreResult{Name: name, NumItems: count})
		}
	}
	sort.Slice(matchedGenres, func(i, j int) bool {
		if matchedGenres[i].NumItems == matchedGenres[j].NumItems {
			return strings.ToLower(matchedGenres[i].Name) < strings.ToLower(matchedGenres[j].Name)
		}
		return matchedGenres[i].NumItems > matchedGenres[j].NumItems
	})
	if len(matchedGenres) > limit {
		matchedGenres = matchedGenres[:limit]
	}

	var matchedNarrators []NarratorResult
	for name, count := range narratorsMap {
		if strings.Contains(strings.ToLower(name), qLower) {
			matchedNarrators = append(matchedNarrators, NarratorResult{Name: name, NumBooks: count})
		}
	}
	sort.Slice(matchedNarrators, func(i, j int) bool {
		if matchedNarrators[i].NumBooks == matchedNarrators[j].NumBooks {
			return strings.ToLower(matchedNarrators[i].Name) < strings.ToLower(matchedNarrators[j].Name)
		}
		return matchedNarrators[i].NumBooks > matchedNarrators[j].NumBooks
	})
	if len(matchedNarrators) > limit {
		matchedNarrators = matchedNarrators[:limit]
	}

	return matchedTags, matchedGenres, matchedNarrators
}
