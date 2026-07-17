package playlist

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// SmartCollectionRules defines the matching rules for a Smart Collection.
type SmartCollectionRules struct {
	Genres         []string `json:"genres,omitempty"`
	Tags           []string `json:"tags,omitempty"`
	Authors        []string `json:"authors,omitempty"`
	Series         []string `json:"series,omitempty"`
	Narrators      []string `json:"narrators,omitempty"`
	PublishedYears []string `json:"publishedYears,omitempty"`
	Search         string   `json:"search,omitempty"`
}

// ResolveSmartCollectionItems dynamically queries library items matching smart rules.
func (m *PlaylistManager) ResolveSmartCollectionItems(ctx context.Context, libraryID string, rulesJSON string) ([]string, error) {
	if rulesJSON == "" {
		return []string{}, nil
	}

	var rules SmartCollectionRules
	if err := json.Unmarshal([]byte(rulesJSON), &rules); err != nil {
		return nil, fmt.Errorf("failed to unmarshal rules: %w", err)
	}

	query := `
		SELECT li.mediaId
		FROM libraryItems li
		JOIN books b ON li.mediaId = b.id AND li.mediaType = 'book'
		WHERE li.libraryId = ? AND li.isMissing = 0 AND li.isInvalid = 0
	`
	args := []interface{}{libraryID}

	if len(rules.Genres) > 0 {
		var placeholders []string
		for _, genre := range rules.Genres {
			if g := strings.TrimSpace(genre); g != "" {
				placeholders = append(placeholders, "?")
				args = append(args, g)
			}
		}
		if len(placeholders) > 0 {
			query += fmt.Sprintf(" AND EXISTS (SELECT 1 FROM json_each(b.genres) WHERE json_valid(b.genres) AND json_each.value COLLATE NOCASE IN (%s))", strings.Join(placeholders, ","))
		}
	}

	if len(rules.Tags) > 0 {
		var placeholders []string
		for _, tag := range rules.Tags {
			if t := strings.TrimSpace(tag); t != "" {
				placeholders = append(placeholders, "?")
				args = append(args, t)
			}
		}
		if len(placeholders) > 0 {
			query += fmt.Sprintf(" AND EXISTS (SELECT 1 FROM json_each(b.tags) WHERE json_valid(b.tags) AND json_each.value COLLATE NOCASE IN (%s))", strings.Join(placeholders, ","))
		}
	}

	if len(rules.Authors) > 0 {
		var placeholders []string
		for _, author := range rules.Authors {
			if a := strings.TrimSpace(author); a != "" {
				placeholders = append(placeholders, "?")
				args = append(args, a)
			}
		}
		if len(placeholders) > 0 {
			query += fmt.Sprintf(" AND EXISTS (SELECT 1 FROM bookAuthors ba JOIN authors a ON ba.authorId = a.id WHERE ba.bookId = b.id AND a.name COLLATE NOCASE IN (%s))", strings.Join(placeholders, ","))
		}
	}

	if len(rules.Series) > 0 {
		var placeholders []string
		for _, ser := range rules.Series {
			if s := strings.TrimSpace(ser); s != "" {
				placeholders = append(placeholders, "?")
				args = append(args, s)
			}
		}
		if len(placeholders) > 0 {
			query += fmt.Sprintf(" AND EXISTS (SELECT 1 FROM bookSeries bs JOIN series s ON bs.seriesId = s.id WHERE bs.bookId = b.id AND s.name COLLATE NOCASE IN (%s))", strings.Join(placeholders, ","))
		}
	}

	if len(rules.Narrators) > 0 {
		var placeholders []string
		for _, narrator := range rules.Narrators {
			if n := strings.TrimSpace(narrator); n != "" {
				placeholders = append(placeholders, "?")
				args = append(args, n)
			}
		}
		if len(placeholders) > 0 {
			query += fmt.Sprintf(" AND EXISTS (SELECT 1 FROM json_each(b.narrators) WHERE json_valid(b.narrators) AND json_each.value COLLATE NOCASE IN (%s))", strings.Join(placeholders, ","))
		}
	}

	if len(rules.PublishedYears) > 0 {
		var placeholders []string
		for _, year := range rules.PublishedYears {
			if y := strings.TrimSpace(year); y != "" {
				placeholders = append(placeholders, "?")
				args = append(args, y)
			}
		}
		if len(placeholders) > 0 {
			query += fmt.Sprintf(" AND b.publishedYear COLLATE NOCASE IN (%s)", strings.Join(placeholders, ","))
		}
	}

	if rules.Search != "" {
		searchTerm := "%" + rules.Search + "%"
		query += ` AND (
			b.title LIKE ? OR 
			b.subtitle LIKE ? OR 
			b.description LIKE ? OR 
			EXISTS (SELECT 1 FROM bookAuthors ba JOIN authors a ON ba.authorId = a.id WHERE ba.bookId = b.id AND a.name LIKE ?)
		)`
		args = append(args, searchTerm, searchTerm, searchTerm, searchTerm)
	}

	rows, err := m.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query smart collection books: %w", err)
	}
	defer rows.Close()

	var bookIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("failed to scan book id: %w", err)
		}
		bookIDs = append(bookIDs, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error in rows iteration: %w", err)
	}

	return bookIDs, nil
}
