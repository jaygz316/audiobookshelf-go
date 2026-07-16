package metadata

// EbookMetadata represents the parsed metadata and chapters of an e-book or comic.
type EbookMetadata struct {
	Title         string    `json:"title"`
	Author        string    `json:"author"`
	Publisher     string    `json:"publisher"`
	PublishedYear string    `json:"publishedYear"`
	Description   string    `json:"description"`
	Language      string    `json:"language"`
	ISBN          string    `json:"isbn"`
	Chapters      []Chapter `json:"chapters"`
}

// Chapter represents an individual section or chapter in an e-book's table of contents.
type Chapter struct {
	ID          int     `json:"id"`
	Title       string  `json:"title"`
	StartOffset float64 `json:"startOffset"` // in seconds
	EndOffset   float64 `json:"endOffset"`   // in seconds
}
