package scanner

import (
	"encoding/xml"
	"os"
	"regexp"
	"strings"

	log "audiobookshelf/internal/logger"
)

// OPFPackage holds parsed OPF ebook metadata.
type OPFPackage struct {
	XMLName  xml.Name `xml:"package"`
	Metadata struct {
		Title   []string `xml:"title"`
		Creator []struct {
			Value string `xml:",chardata"`
			Role  string `xml:"role,attr"`
		} `xml:"creator"`
		Publisher   []string `xml:"publisher"`
		Date        []string `xml:"date"`
		Description []string `xml:"description"`
		Identifier  []struct {
			Value  string `xml:",chardata"`
			Scheme string `xml:"scheme,attr"`
		} `xml:"identifier"`
		Language []string `xml:"language"`
		Subject  []string `xml:"subject"`
		Meta     []struct {
			Name    string `xml:"name,attr"`
			Content string `xml:"content,attr"`
		} `xml:"meta"`
	} `xml:"metadata"`
}

func parseOPFFile(filePath string) (*OPFPackage, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var opf OPFPackage
	dec := xml.NewDecoder(f)
	dec.Entity = xml.HTMLEntity
	if err := dec.Decode(&opf); err != nil {
		return nil, err
	}
	return &opf, nil
}

func stripHTML(html string) string {
	r := regexp.MustCompile("<[^>]*>")
	return strings.TrimSpace(r.ReplaceAllString(html, ""))
}

func parseOPFMetadata(opfFile string, meta *GroupMetadata, itemPath string) {
	log.Printf("[Scanner] [%s] Parsing OPF file: %s", itemPath, opfFile)
	if opf, err := parseOPFFile(opfFile); err == nil {
		if len(opf.Metadata.Title) > 0 {
			meta.Title = opf.Metadata.Title[0]
		}
		if len(opf.Metadata.Creator) > 0 {
			var creators []string
			for _, c := range opf.Metadata.Creator {
				if c.Value != "" {
					creators = append(creators, c.Value)
				}
			}
			if len(creators) > 0 {
				meta.Authors = creators
			}
		}
		if len(opf.Metadata.Publisher) > 0 {
			meta.Publisher = opf.Metadata.Publisher[0]
		}
		if len(opf.Metadata.Date) > 0 && len(opf.Metadata.Date[0]) >= 4 {
			meta.PublishedYear = opf.Metadata.Date[0][:4]
			meta.PublishedDate = opf.Metadata.Date[0]
		}
		if len(opf.Metadata.Description) > 0 {
			meta.Description = stripHTML(opf.Metadata.Description[0])
		}
		if len(opf.Metadata.Language) > 0 {
			meta.Language = opf.Metadata.Language[0]
		}
		if len(opf.Metadata.Subject) > 0 {
			meta.Genres = opf.Metadata.Subject
		}
		for _, m := range opf.Metadata.Meta {
			if m.Name == "calibre:series" {
				meta.SeriesName = m.Content
			}
			if m.Name == "calibre:series_index" {
				meta.SeriesSequence = m.Content
			}
		}
		for _, id := range opf.Metadata.Identifier {
			if strings.EqualFold(id.Scheme, "isbn") {
				meta.ISBN = id.Value
			}
			if strings.EqualFold(id.Scheme, "asin") {
				meta.ASIN = id.Value
			}
		}
	} else {
		log.Printf("[Scanner] [%s] Parsing OPF file failed: %v", itemPath, err)
	}
}
