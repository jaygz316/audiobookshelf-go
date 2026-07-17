package handlers

import "database/sql"

type MergeChapter struct {
	ID    int     `json:"id"`
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Title string  `json:"title"`
}

type MergeAudioFile struct {
	Index       int     `json:"index"`
	Exclude     bool    `json:"exclude"`
	Duration    float64 `json:"duration"`
	Codec       string  `json:"codec"`
	MimeType    string  `json:"mimeType"`
	StartOffset float64 `json:"startOffset"`
	Title       string  `json:"title"`
	Metadata    struct {
		Path     string `json:"path"`
		Filename string `json:"filename"`
		Size     int64  `json:"size"`
	} `json:"metadata"`
}

// MergeContext encapsulates all the data validated and loaded for the merge operation.
type MergeContext struct {
	ItemID         string
	MediaID        string
	Title          string
	AuthorName     sql.NullString
	ActiveFiles    []MergeAudioFile
	OutputPath     string
	OutputFilename string
	UseCopy        bool
}
