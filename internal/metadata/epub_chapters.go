package metadata

import (
	"bytes"
	"encoding/xml"
	"io"
	"strings"
)

func findTocPath(opf *opfPackage) (string, string) {
	for _, item := range opf.Manifest.Items {
		properties := strings.Split(item.Properties, " ")
		for _, p := range properties {
			if p == "nav" {
				return item.Href, "nav"
			}
		}
	}

	if opf.Spine.Toc != "" {
		for _, item := range opf.Manifest.Items {
			if item.ID == opf.Spine.Toc {
				return item.Href, "ncx"
			}
		}
	}

	for _, item := range opf.Manifest.Items {
		if item.MediaType == "application/x-dtbncx+xml" {
			return item.Href, "ncx"
		}
	}

	return "", ""
}

func flattenNcxNavPoints(points []ncxNavPoint, chapters *[]Chapter) {
	for _, p := range points {
		title := strings.TrimSpace(p.NavLabel.Text)
		if title != "" {
			*chapters = append(*chapters, Chapter{
				ID:          len(*chapters) + 1,
				Title:       title,
				StartOffset: 0,
				EndOffset:   0,
			})
		}
		if len(p.NavPoints) > 0 {
			flattenNcxNavPoints(p.NavPoints, chapters)
		}
	}
}

func parseNavChapters(xmlData []byte) ([]Chapter, error) {
	decoder := xml.NewDecoder(bytes.NewReader(xmlData))
	decoder.Entity = xml.HTMLEntity
	decoder.Strict = false

	var chapters []Chapter
	inNav := false
	var currentAnchorText string
	var inAnchor bool

	for {
		t, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}

		switch se := t.(type) {
		case xml.StartElement:
			localName := strings.ToLower(se.Name.Local)
			if localName == "nav" {
				inNav = true
			} else if inNav && localName == "a" {
				inAnchor = true
				currentAnchorText = ""
			}
		case xml.EndElement:
			localName := strings.ToLower(se.Name.Local)
			if localName == "nav" {
				inNav = false
			} else if inNav && localName == "a" {
				inAnchor = false
				title := strings.TrimSpace(currentAnchorText)
				if title != "" {
					chapters = append(chapters, Chapter{
						ID:          len(chapters) + 1,
						Title:       title,
						StartOffset: 0,
						EndOffset:   0,
					})
				}
			}
		case xml.CharData:
			if inAnchor {
				currentAnchorText += string(se)
			}
		}
	}
	return chapters, nil
}
