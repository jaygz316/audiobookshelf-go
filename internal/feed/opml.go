package feed

import (
	"context"
	"database/sql"
	"encoding/xml"
	"fmt"
)

// OPML structure
type opml struct {
	XMLName xml.Name `xml:"opml"`
	Version string   `xml:"version,attr"`
	Head    opmlHead `xml:"head"`
	Body    opmlBody `xml:"body"`
}

type opmlHead struct {
	Title string `xml:"title"`
}

type opmlBody struct {
	Outlines []opmlOutline `xml:"outline"`
}

type opmlOutline struct {
	Type        string `xml:"type,attr"`
	Text        string `xml:"text,attr"`
	Title       string `xml:"title,attr"`
	XMLURL      string `xml:"xmlUrl,attr"`
	Description string `xml:"description,attr,omitempty"`
	HTMLURL     string `xml:"htmlUrl,attr,omitempty"`
	Language    string `xml:"language,attr,omitempty"`
}

// GenerateOPML generates an OPML XML payload mapping all podcasts inside a user's library.
func (m *FeedManager) GenerateOPML(ctx context.Context, userID, libraryID string) (string, error) {
	// 1. Check user access to the library
	ok, err := m.checkUserAccess(ctx, userID, libraryID)
	if err != nil {
		return "", fmt.Errorf("check user access: %w", err)
	}
	if !ok {
		return "", fmt.Errorf("user does not have access to library %s", libraryID)
	}

	// 2. Query all podcasts in this library
	rows, err := m.db.QueryContext(ctx, `
		SELECT p.title, p.feedURL, p.description, p.itunesPageURL, p.language
		FROM libraryItems li
		JOIN podcasts p ON li.mediaId = p.id AND li.mediaType = 'podcast'
		WHERE li.libraryId = ?
	`, libraryID)
	if err != nil {
		return "", fmt.Errorf("query podcasts in library: %w", err)
	}
	defer rows.Close()

	var outlines []opmlOutline
	for rows.Next() {
		var title sql.NullString
		var feedURL sql.NullString
		var description sql.NullString
		var htmlURL sql.NullString
		var language sql.NullString

		if err := rows.Scan(&title, &feedURL, &description, &htmlURL, &language); err != nil {
			return "", fmt.Errorf("scan podcast row: %w", err)
		}

		if !feedURL.Valid || feedURL.String == "" {
			continue
		}

		outlines = append(outlines, opmlOutline{
			Type:        "rss",
			Text:        title.String,
			Title:       title.String,
			XMLURL:      feedURL.String,
			Description: description.String,
			HTMLURL:     htmlURL.String,
			Language:    language.String,
		})
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("podcast rows error: %w", err)
	}

	opmlData := opml{
		Version: "1.0",
		Head: opmlHead{
			Title: "Audiobookshelf Podcast Subscriptions",
		},
		Body: opmlBody{
			Outlines: outlines,
		},
	}

	xmlBytes, err := xml.MarshalIndent(opmlData, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal OPML: %w", err)
	}

	return xml.Header + string(xmlBytes), nil
}
