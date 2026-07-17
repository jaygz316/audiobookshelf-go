package metadata

import (
	"encoding/xml"
)

// comicInfo XML structures
type comicInfo struct {
	XMLName     xml.Name `xml:"ComicInfo"`
	Title       string   `xml:"Title"`
	Series      string   `xml:"Series"`
	Number      string   `xml:"Number"`
	Writer      string   `xml:"Writer"`
	Publisher   string   `xml:"Publisher"`
	Year        string   `xml:"Year"`
	Month       string   `xml:"Month"`
	Day         string   `xml:"Day"`
	Summary     string   `xml:"Summary"`
	Notes       string   `xml:"Notes"`
	LanguageISO string   `xml:"LanguageISO"`
	ISBN        string   `xml:"ISBN"`
}

func mapComicInfoToEbookMetadata(info *comicInfo) *EbookMetadata {
	meta := &EbookMetadata{
		Author:      info.Writer,
		Publisher:   info.Publisher,
		Description: info.Summary,
		Language:    info.LanguageISO,
		ISBN:        info.ISBN,
	}

	if info.Series != "" {
		meta.Title = info.Series
		if info.Number != "" {
			meta.Title += " " + info.Number
		}
	} else {
		meta.Title = info.Title
	}

	if info.Year != "" {
		meta.PublishedYear = info.Year
	}

	return meta
}
