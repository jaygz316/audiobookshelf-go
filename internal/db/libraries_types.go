package db

import (
	"encoding/json"
)

type LibraryFolderJSON struct {
	ID        string `json:"id"`
	FullPath  string `json:"fullPath"`
	LibraryID string `json:"libraryId"`
	AddedAt   int64  `json:"addedAt"`
}

type LibraryJSON struct {
	ID              string               `json:"id"`
	Name            string               `json:"name"`
	Folders         []*LibraryFolderJSON `json:"folders"`
	DisplayOrder    int                  `json:"displayOrder"`
	Icon            string               `json:"icon"`
	MediaType       string               `json:"mediaType"`
	Provider        string               `json:"provider"`
	Settings        json.RawMessage      `json:"settings"`
	LastScan        *int64               `json:"lastScan"`
	LastScanVersion string               `json:"lastScanVersion"`
	CreatedAt       int64                `json:"createdAt"`
	LastUpdate      int64                `json:"lastUpdate"`
	Stats           *LibraryStats        `json:"stats,omitempty"`
}

type GenreWithCount struct {
	Genre string `json:"genre"`
	Count int    `json:"count"`
}

type AuthorWithCount struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type MinLibraryItem struct {
	ID       string  `json:"id"`
	Title    string  `json:"title"`
	Duration float64 `json:"duration,omitempty"`
	Size     int64   `json:"size,omitempty"`
}

type LibraryStats struct {
	TotalSize        int64             `json:"totalSize"`
	TotalDuration    float64           `json:"totalDuration"`
	NumAudioFiles    int               `json:"numAudioFiles"`
	NumAudioTracks   int               `json:"numAudioTracks"`
	TotalItems       int               `json:"totalItems"`
	TotalAuthors     int               `json:"totalAuthors"`
	GenresWithCount  []GenreWithCount  `json:"genresWithCount"`
	AuthorsWithCount []AuthorWithCount `json:"authorsWithCount"`
	LongestItems     []MinLibraryItem  `json:"longestItems"`
	LargestItems     []MinLibraryItem  `json:"largestItems"`
}

type CreateFolderPayload struct {
	Path     string `json:"path"`
	FullPath string `json:"fullPath"`
}

type CreateLibraryPayload struct {
	Name      string                 `json:"name"`
	Folders   []CreateFolderPayload  `json:"folders"`
	MediaType string                 `json:"mediaType"`
	Icon      string                 `json:"icon"`
	Provider  string                 `json:"provider"`
	Settings  map[string]interface{} `json:"settings"`
}

type UpdateFolderPayload struct {
	ID       string `json:"id"`
	Path     string `json:"path"`
	FullPath string `json:"fullPath"`
}

type UpdateLibraryPayload struct {
	Name         *string                `json:"name"`
	Provider     *string                `json:"provider"`
	MediaType    *string                `json:"mediaType"`
	Icon         *string                `json:"icon"`
	DisplayOrder *int                   `json:"displayOrder"`
	Settings     map[string]interface{} `json:"settings"`
	Folders      []UpdateFolderPayload  `json:"folders"`
}
