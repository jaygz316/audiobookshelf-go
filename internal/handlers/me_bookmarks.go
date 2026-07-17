package handlers

// Bookmark represents a user's bookmark inside a library item.
type Bookmark struct {
	LibraryItemID string  `json:"libraryItemId"`
	Time          float64 `json:"time"`
	Title         string  `json:"title"`
	Note          string  `json:"note"`
	Color         string  `json:"color"`
	Cfi           string  `json:"cfi,omitempty"`
	CreatedAt     int64   `json:"createdAt"`
}
