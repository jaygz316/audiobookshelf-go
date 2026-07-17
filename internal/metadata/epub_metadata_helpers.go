package metadata

import (
	"strconv"
	"strings"
)

func mapOpfToEbookMetadata(opf *opfPackage) *EbookMetadata {
	meta := &EbookMetadata{}

	if len(opf.Metadata.Title) > 0 {
		meta.Title = opf.Metadata.Title[0].Value
	}

	if len(opf.Metadata.Publisher) > 0 {
		meta.Publisher = opf.Metadata.Publisher[0]
	}

	if len(opf.Metadata.Description) > 0 {
		meta.Description = stripAllTags(opf.Metadata.Description[0])
	}

	if len(opf.Metadata.Language) > 0 {
		meta.Language = opf.Metadata.Language[0]
	}

	if len(opf.Metadata.Date) > 0 {
		meta.PublishedYear = parsePublishedYear(opf.Metadata.Date[0])
	}

	authors := fetchAuthors(opf)
	meta.Author = strings.Join(authors, ", ")

	meta.ISBN = fetchISBN(opf)

	return meta
}

func fetchAuthors(opf *opfPackage) []string {
	refines := make(map[string]map[string]string)
	for _, m := range opf.Metadata.Meta {
		if m.Refines != "" && m.Property != "" {
			refID := strings.TrimPrefix(m.Refines, "#")
			if refines[refID] == nil {
				refines[refID] = make(map[string]string)
			}
			refines[refID][m.Property] = m.Value
		}
	}

	var authors []string
	for _, c := range opf.Metadata.Creator {
		role := c.Role
		if c.ID != "" {
			if refined, ok := refines[c.ID]; ok {
				if rVal, ok := refined["role"]; ok {
					role = rVal
				}
			}
		}

		if role == "aut" || role == "" {
			name := strings.TrimSpace(c.Value)
			if name != "" {
				authors = append(authors, name)
			}
		}
	}

	return authors
}

func fetchISBN(opf *opfPackage) string {
	for _, id := range opf.Metadata.Identifier {
		if strings.EqualFold(id.Scheme, "ISBN") {
			return strings.TrimSpace(id.Value)
		}
	}
	return ""
}

func parsePublishedYear(dateStr string) string {
	if dateStr == "" {
		return ""
	}
	parts := strings.Split(dateStr, "-")
	if len(parts) > 0 && len(parts[0]) == 4 {
		if _, err := strconv.Atoi(parts[0]); err == nil {
			return parts[0]
		}
	}
	if len(dateStr) >= 4 {
		if _, err := strconv.Atoi(dateStr[:4]); err == nil {
			return dateStr[:4]
		}
	}
	return ""
}
