package db

type LibraryItemDownloadInfo struct {
	Path    string
	RelPath string
	IsFile  bool
}

type LibraryItemMinifiedJSON struct {
	ID               string      `json:"id"`
	Ino              string      `json:"ino"`
	OldLibraryItemID *string     `json:"oldLibraryItemId"`
	LibraryID        string      `json:"libraryId"`
	FolderID         string      `json:"folderId"`
	Path             string      `json:"path"`
	RelPath          string      `json:"relPath"`
	IsFile           bool        `json:"isFile"`
	MtimeMs          int64       `json:"mtimeMs"`
	CtimeMs          int64       `json:"ctimeMs"`
	BirthtimeMs      int64       `json:"birthtimeMs"`
	AddedAt          int64       `json:"addedAt"`
	UpdatedAt        int64       `json:"updatedAt"`
	IsMissing        bool        `json:"isMissing"`
	IsInvalid        bool        `json:"isInvalid"`
	MediaType        string      `json:"mediaType"`
	Media            interface{} `json:"media"`
	NumFiles         int         `json:"numFiles"`
	Size             int64       `json:"size"`
}

type BookMinifiedJSON struct {
	ID            string                `json:"id"`
	Metadata      *BookMetadataMinified `json:"metadata"`
	CoverPath     *string               `json:"coverPath"`
	Tags          []string              `json:"tags"`
	NumTracks     int                   `json:"numTracks"`
	NumAudioFiles int                   `json:"numAudioFiles"`
	NumChapters   int                   `json:"numChapters"`
	Duration      float64               `json:"duration"`
	Size          int64                 `json:"size"`
	EbookFormat   *string               `json:"ebookFormat"`
	AudioFiles    []interface{}         `json:"audioFiles,omitempty"`
}

type BookSeriesMinifiedJSON struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Sequence string `json:"sequence"`
}

type BookMetadataMinified struct {
	Title             string                    `json:"title"`
	TitleIgnorePrefix string                    `json:"titleIgnorePrefix"`
	Subtitle          *string                   `json:"subtitle"`
	AuthorName        string                    `json:"authorName"`
	AuthorNameLF      string                    `json:"authorNameLF"`
	NarratorName      string                    `json:"narratorName"`
	SeriesName        string                    `json:"seriesName"`
	SeriesSequence    *string                   `json:"seriesSequence"`
	Series            []*BookSeriesMinifiedJSON `json:"series"`
	Genres            []string                  `json:"genres"`
	PublishedYear     *string                   `json:"publishedYear"`
	PublishedDate     *string                   `json:"publishedDate"`
	Publisher         *string                   `json:"publisher"`
	Description       *string                   `json:"description"`
	Isbn              *string                   `json:"isbn"`
	Asin              *string                   `json:"asin"`
	Language          *string                   `json:"language"`
	Explicit          bool                      `json:"explicit"`
	Abridged          bool                      `json:"abridged"`
	LockedFields      []string                  `json:"lockedFields"`
}

type PodcastMinifiedJSON struct {
	ID                       string              `json:"id"`
	Metadata                 *PodcastMetadataMin `json:"metadata"`
	CoverPath                *string             `json:"coverPath"`
	Tags                     []string            `json:"tags"`
	NumEpisodes              int                 `json:"numEpisodes"`
	AutoDownloadEpisodes     bool                `json:"autoDownloadEpisodes"`
	AutoDownloadSchedule     *string             `json:"autoDownloadSchedule"`
	LastEpisodeCheck         *int64              `json:"lastEpisodeCheck"`
	MaxEpisodesToKeep        int                 `json:"maxEpisodesToKeep"`
	MaxNewEpisodesToDownload int                 `json:"maxNewEpisodesToDownload"`
	AutoDeletePlayed         bool                `json:"autoDeletePlayed"`
	SkipIntroDuration        int                 `json:"skipIntroDuration"`
	SkipOutroDuration        int                 `json:"skipOutroDuration"`
	Size                     int64               `json:"size"`
	Episodes                 []interface{}       `json:"episodes,omitempty"`
}

type PodcastMetadataMin struct {
	Title             string   `json:"title"`
	TitleIgnorePrefix string   `json:"titleIgnorePrefix"`
	Author            *string  `json:"author"`
	Description       *string  `json:"description"`
	ReleaseDate       *string  `json:"releaseDate"`
	Genres            []string `json:"genres"`
	FeedURL           *string  `json:"feedUrl"`
	ImageURL          *string  `json:"imageUrl"`
	ItunesPageURL     *string  `json:"itunesPageUrl"`
	ItunesID          *string  `json:"itunesId"`
	ItunesArtistID    *string  `json:"itunesArtistId"`
	Explicit          bool     `json:"explicit"`
	Language          *string  `json:"language"`
	Type              *string  `json:"type"`
	LockedFields      []string `json:"lockedFields"`
}
