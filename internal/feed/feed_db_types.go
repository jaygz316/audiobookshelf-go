package feed

// Database helper structs
type audioFile struct {
	Index    int     `json:"index"`
	Duration float64 `json:"duration"`
	Codec    string  `json:"codec"`
	MimeType string  `json:"mimeType"`
	Metadata struct {
		Path     string `json:"path"`
		RelPath  string `json:"relPath"`
		Filename string `json:"filename"`
		Ext      string `json:"ext"`
		Size     int64  `json:"size"`
	} `json:"metadata"`
}

type audiobookTrack struct {
	Index       int     `json:"index"`
	Exclude     bool    `json:"exclude"`
	Duration    float64 `json:"duration"`
	Codec       string  `json:"codec"`
	MimeType    string  `json:"mimeType"`
	StartOffset float64 `json:"startOffset"`
	Metadata    struct {
		Path     string `json:"path"`
		Filename string `json:"filename"`
		Size     int64  `json:"size"`
	} `json:"metadata"`
}

type audiobookChapter struct {
	ID    int     `json:"id"`
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Title string  `json:"title"`
}

type podcastEpData struct {
	ID          string
	Title       string
	AudioFile   string
	PubDate     string
	Description string
	Season      string
	Episode     string
	EpisodeType string
}
