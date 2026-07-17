package db

import (
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
