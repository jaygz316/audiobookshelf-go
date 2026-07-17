package providers

type audnexusAuthorOrNarrator struct {
	Name string `json:"name"`
}

type audnexusBookDetails struct {
	Title         string                     `json:"title"`
	Subtitle      string                     `json:"subtitle"`
	ASIN          string                     `json:"asin"`
	Authors       []audnexusAuthorOrNarrator `json:"authors"`
	Narrators     []audnexusAuthorOrNarrator `json:"narrators"`
	PublisherName string                     `json:"publisherName"`
	Summary       string                     `json:"summary"`
	ReleaseDate   string                     `json:"releaseDate"`
	Image         string                     `json:"image"`
	Language      string                     `json:"language"`
	ISBN          string                     `json:"isbn"`
}

type AudnexusAuthorASIN struct {
	ASIN string `json:"asin"`
	Name string `json:"name"`
}

type AudnexusAuthorDetails struct {
	ASIN        string `json:"asin"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Image       string `json:"image"`
}
