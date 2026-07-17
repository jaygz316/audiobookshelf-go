package scanner

import (
	"bufio"
	"os"
	"regexp"
	"strings"

	log "audiobookshelf/internal/logger"
)

// NFOMetadata holds parsed NFO file metadata.
type NFOMetadata struct {
	Title         string
	Subtitle      string
	Authors       []string
	Narrators     []string
	Series        string
	Sequence      string
	Genres        []string
	Tags          []string
	PublishedYear string
	Abridged      bool
	Publisher     string
	ASIN          string
	ISBN          string
	Language      string
	Description   string
}

func parseNFOFile(filePath string) (*NFOMetadata, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	meta := &NFOMetadata{}
	sc := bufio.NewScanner(f)

	insideDescription := false
	for sc.Scan() {
		line := sc.Text()

		if strings.EqualFold(strings.TrimSpace(line), "book description") {
			insideDescription = true
			continue
		}

		if insideDescription {
			if strings.HasPrefix(strings.TrimSpace(line), "===") {
				continue
			}
			meta.Description += line + "\n"
			continue
		}

		idx := strings.Index(line, ":")
		if idx == -1 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(line[:idx]))
		value := strings.TrimSpace(line[idx+1:])
		if value == "" {
			continue
		}

		switch key {
		case "title":
			if sIdx := strings.Index(value, ": "); sIdx != -1 {
				meta.Title = strings.TrimSpace(value[:sIdx])
				meta.Subtitle = strings.TrimSpace(value[sIdx+2:])
			} else {
				meta.Title = value
			}
		case "author":
			for _, a := range strings.Split(value, ",") {
				if strings.TrimSpace(a) != "" {
					meta.Authors = append(meta.Authors, strings.TrimSpace(a))
				}
			}
		case "narrator", "read by":
			for _, n := range strings.Split(value, ",") {
				if strings.TrimSpace(n) != "" {
					meta.Narrators = append(meta.Narrators, strings.TrimSpace(n))
				}
			}
		case "series name":
			meta.Series = value
		case "genre":
			for _, g := range strings.Split(value, ",") {
				if strings.TrimSpace(g) != "" {
					meta.Genres = append(meta.Genres, strings.TrimSpace(g))
				}
			}
		case "tags":
			for _, t := range strings.Split(value, ",") {
				if strings.TrimSpace(t) != "" {
					meta.Tags = append(meta.Tags, strings.TrimSpace(t))
				}
			}
		case "copyright", "audible.com release", "audiobook copyright", "book copyright", "recording copyright", "release date", "date":
			re := regexp.MustCompile(`\d{4}`)
			years := re.FindAllString(value, -1)
			if len(years) > 0 {
				meta.PublishedYear = years[len(years)-1]
			}
		case "position in series":
			meta.Sequence = value
		case "unabridged":
			meta.Abridged = !strings.EqualFold(value, "yes")
		case "abridged":
			meta.Abridged = !strings.EqualFold(value, "no")
		case "publisher":
			meta.Publisher = value
		case "asin":
			meta.ASIN = value
		case "isbn", "isbn-10", "isbn-13":
			meta.ISBN = value
		case "language", "lang":
			meta.Language = value
		}
	}

	meta.Description = strings.TrimSpace(meta.Description)
	return meta, nil
}

func parseNFOMetadata(nfoFile string, meta *GroupMetadata, scannerParseSubtitles bool, itemPath string) {
	log.Printf("[Scanner] [%s] Parsing NFO file: %s", itemPath, nfoFile)
	if nfo, err := parseNFOFile(nfoFile); err == nil {
		if nfo.Title != "" {
			meta.Title = nfo.Title
		}
		if scannerParseSubtitles && nfo.Subtitle != "" {
			meta.Subtitle = nfo.Subtitle
		}
		if len(nfo.Authors) > 0 {
			meta.Authors = nfo.Authors
		}
		if len(nfo.Narrators) > 0 {
			meta.Narrators = nfo.Narrators
		}
		if nfo.Series != "" {
			meta.SeriesName = nfo.Series
		}
		if nfo.Sequence != "" {
			meta.SeriesSequence = nfo.Sequence
		}
		if len(nfo.Genres) > 0 {
			meta.Genres = nfo.Genres
		}
		if len(nfo.Tags) > 0 {
			meta.Tags = nfo.Tags
		}
		if nfo.PublishedYear != "" {
			meta.PublishedYear = nfo.PublishedYear
		}
		if nfo.Publisher != "" {
			meta.Publisher = nfo.Publisher
		}
		if nfo.ASIN != "" {
			meta.ASIN = nfo.ASIN
		}
		if nfo.ISBN != "" {
			meta.ISBN = nfo.ISBN
		}
		if nfo.Language != "" {
			meta.Language = nfo.Language
		}
		if nfo.Description != "" {
			meta.Description = nfo.Description
		}
	} else {
		log.Printf("[Scanner] [%s] Parsing NFO file failed: %v", itemPath, err)
	}
}
