package scanner

import (
	"regexp"
	"strings"
)

// FilenameMetadata holds metadata extracted from directory/file names.
type FilenameMetadata struct {
	Title          string
	Subtitle       string
	ASIN           string
	Authors        []string
	Narrators      []string
	SeriesName     string
	SeriesSequence string
	PublishedYear  string
}

// GetBookDataFromDir extracts metadata from a relative directory path.
func GetBookDataFromDir(relPath string) *FilenameMetadata {
	parts := splitPath(relPath)
	if len(parts) == 0 {
		return &FilenameMetadata{}
	}

	folder := parts[len(parts)-1]

	var series string
	if len(parts) > 2 {
		series = parts[len(parts)-2]
	}

	var author string
	if len(parts) > 3 {
		author = parts[len(parts)-3]
	} else if len(parts) == 2 {
		author = parts[0]
	}

	folder, asin := getASIN(folder)
	folder, narratorsVal := getNarrator(folder)
	var sequence string
	if series != "" {
		folder, sequence = getSequence(folder)
	}
	folder, publishedYear := getPublishedYear(folder)

	title, subtitle := getSubtitle(folder)

	var authors []string
	if author != "" {
		authors = parseNameString(author)
	}

	var narrators []string
	if narratorsVal != "" {
		narrators = parseNameString(narratorsVal)
	}

	return &FilenameMetadata{
		Title:          title,
		Subtitle:       subtitle,
		ASIN:           asin,
		Authors:        authors,
		Narrators:      narrators,
		SeriesName:     series,
		SeriesSequence: sequence,
		PublishedYear:  publishedYear,
	}
}

var asinRegex = regexp.MustCompile(`(?: |^)\[([A-Z0-9]{10})](?: |$)`)

func getASIN(folder string) (string, string) {
	match := asinRegex.FindStringSubmatch(folder)
	if len(match) > 1 {
		asin := match[1]
		folder = strings.Replace(folder, match[0], "", 1)
		return strings.TrimSpace(folder), asin
	}
	return folder, ""
}

var narratorRegex = regexp.MustCompile(`^(.*) \{(.*)\}$`)

func getNarrator(folder string) (string, string) {
	match := narratorRegex.FindStringSubmatch(folder)
	if len(match) > 2 {
		return strings.TrimSpace(match[1]), strings.TrimSpace(match[2])
	}
	return folder, ""
}

var sequenceRegex = regexp.MustCompile(`(?i)^(vol\.? |volume |book )?(\d{0,3}(?:\.\d{1,2})?)(\.?)(?: (.*))?$`)

func getSequence(folder string) (string, string) {
	parts := strings.Split(folder, " - ")
	var seq string
	for i, part := range parts {
		match := sequenceRegex.FindStringSubmatch(part)
		if len(match) > 0 {
			volLabel := match[1]
			sequence := match[2]
			trailingDot := match[3]
			suffix := match[4]

			if suffix != "" && volLabel == "" && trailingDot == "" {
				continue
			}
			if sequence != "" {
				seq = sequence
				if suffix != "" {
					parts[i] = suffix
				} else {
					parts = append(parts[:i], parts[i+1:]...)
				}
				break
			}
		}
	}
	return strings.Join(parts, " - "), seq
}

var yearRegex = regexp.MustCompile(`^ *\(?([0-9]{4})\)? * - *(.+)`)

func getPublishedYear(folder string) (string, string) {
	match := yearRegex.FindStringSubmatch(folder)
	if len(match) > 2 {
		return strings.TrimSpace(match[2]), match[1]
	}
	return folder, ""
}

func getSubtitle(folder string) (string, string) {
	parts := strings.Split(folder, " - ")
	if len(parts) > 1 {
		return parts[0], strings.Join(parts[1:], " - ")
	}
	return folder, ""
}

func parseNameString(s string) []string {
	s = strings.ReplaceAll(s, " & ", ", ")
	s = strings.ReplaceAll(s, " and ", ", ")
	var names []string
	for _, part := range strings.Split(s, ",") {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			names = append(names, trimmed)
		}
	}
	return names
}
