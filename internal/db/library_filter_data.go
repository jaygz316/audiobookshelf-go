package db

import (
	"database/sql"
	"sort"
)

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
		if err := queryPodcastFilterData(db, libraryID, fd, genresSet, tagsSet, languagesSet); err != nil {
			return nil, err
		}
	} else {
		if err := queryBookFilterData(db, libraryID, fd, genresSet, tagsSet, narratorsSet, languagesSet, publishersSet, decadesSet); err != nil {
			return nil, err
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
