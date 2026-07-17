package handlers

// AuthorExpandedJSON represents the expanded author object with book count
type AuthorExpandedJSON struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	LastFirst   string `json:"lastFirst"`
	Asin        string `json:"asin"`
	Description string `json:"description"`
	ImagePath   string `json:"imagePath"`
	AddedAt     int64  `json:"addedAt"`
	UpdatedAt   int64  `json:"updatedAt"`
	NumBooks    int    `json:"numBooks"`
}

// BookUpdate represents the information needed to update a book and its metadata on disk and broadcast updates.
type BookUpdate struct {
	bid          string
	itemID       string
	authorNames  []string
	metadataPath string
}
