package scanner

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"encoding/xml"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dhowden/tag"

	idb "audiobookshelf/internal/db"
	log "audiobookshelf/internal/logger"
	"audiobookshelf/internal/metadata"
)

// AudioTags holds parsed audio file tag metadata.
type AudioTags struct {
	Title       string
	Artist      string
	Album       string
	AlbumArtist string
	Genre       string
	Year        string
	Track       int
	Disc        int
	Composer    string
	Comment     string
}

func parseAudioTags(path string) (*AudioTags, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	m, err := tag.ReadFrom(f)
	if err != nil {
		return nil, err
	}

	trackNum, _ := m.Track()
	discNum, _ := m.Disc()

	tags := &AudioTags{
		Title:       m.Title(),
		Artist:      m.Artist(),
		Album:       m.Album(),
		AlbumArtist: m.AlbumArtist(),
		Genre:       m.Genre(),
		Composer:    m.Composer(),
		Comment:     m.Comment(),
		Track:       trackNum,
		Disc:        discNum,
	}
	if m.Year() > 0 {
		tags.Year = strconv.Itoa(m.Year())
	}

	return tags, nil
}

// FFProbeOutput holds the parsed output from ffprobe.
type FFProbeOutput struct {
	Format struct {
		Duration string            `json:"duration"`
		BitRate  string            `json:"bit_rate"`
		Size     string            `json:"size"`
		Tags     map[string]string `json:"tags"`
	} `json:"format"`
	Streams []struct {
		CodecName  string            `json:"codec_name"`
		CodecType  string            `json:"codec_type"`
		BitRate    string            `json:"bit_rate"`
		Channels   int               `json:"channels"`
		SampleRate string            `json:"sample_rate"`
		Tags       map[string]string `json:"tags"`
	} `json:"streams"`
	Chapters []struct {
		ID        int               `json:"id"`
		StartTime string            `json:"start_time"`
		EndTime   string            `json:"end_time"`
		Tags      map[string]string `json:"tags"`
	} `json:"chapters"`
}

// Chapter represents an audio chapter.
type Chapter struct {
	ID    int     `json:"id"`
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Title string  `json:"title"`
}

// AudioMetadata holds parsed audio metadata.
type AudioMetadata struct {
	Duration   float64
	BitRate    int64
	Codec      string
	Channels   int
	SampleRate int
	Chapters   []Chapter
}

func probeAudioFile(path string) (*AudioMetadata, error) {
	probeSemaphore <- struct{}{}
	defer func() { <-probeSemaphore }()

	cmd := exec.Command("ffprobe", "-v", "error", "-show_format", "-show_streams", "-show_chapters", "-of", "json", path)
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var probe FFProbeOutput
	if err := json.Unmarshal(out, &probe); err != nil {
		return nil, err
	}

	meta := &AudioMetadata{}

	if d, err := strconv.ParseFloat(probe.Format.Duration, 64); err == nil {
		meta.Duration = d
	}
	if b, err := strconv.ParseInt(probe.Format.BitRate, 10, 64); err == nil {
		meta.BitRate = b
	}

	for _, stream := range probe.Streams {
		if stream.CodecType == "audio" {
			meta.Codec = stream.CodecName
			meta.Channels = stream.Channels
			if sr, err := strconv.Atoi(stream.SampleRate); err == nil {
				meta.SampleRate = sr
			}
			if meta.BitRate == 0 {
				if b, err := strconv.ParseInt(stream.BitRate, 10, 64); err == nil {
					meta.BitRate = b
				}
			}
			break
		}
	}

	for _, ch := range probe.Chapters {
		start, _ := strconv.ParseFloat(ch.StartTime, 64)
		end, _ := strconv.ParseFloat(ch.EndTime, 64)
		title := ch.Tags["title"]
		if title == "" {
			title = "Chapter " + strconv.Itoa(ch.ID)
		}
		meta.Chapters = append(meta.Chapters, Chapter{
			ID:    ch.ID,
			Start: start,
			End:   end,
			Title: title,
		})
	}

	return meta, nil
}

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

// GroupMetadata holds aggregated metadata for a scanned library item group.
type GroupMetadata struct {
	Title           string
	Subtitle        string
	Authors         []string
	Narrators       []string
	SeriesName      string
	SeriesSequence  string
	PublishedYear   string
	PublishedDate   string
	Publisher       string
	Description     string
	ISBN            string
	ASIN            string
	Language        string
	Duration        float64
	CoverPath       string
	Chapters        []Chapter
	Tags            []string
	Genres          []string
	AudioFiles      []interface{}
	EbookFile       interface{}
	PodcastEpisodes []PodcastEpisodeScanData
}

// PodcastEpisodeScanData holds scan data for a single podcast episode.
type PodcastEpisodeScanData struct {
	ID        string
	Title     string
	AudioFile interface{}
}

func parseMetadataForGroup(dbConn *sql.DB, itemID string, groupFiles []FileItem, mediaType, itemPath, itemRelPath string, audiobooksOnly bool) *GroupMetadata {
	meta := &GroupMetadata{}

	scannerParseSubtitles := true
	scannerFindCovers := true
	if dbConn != nil {
		var valStr string
		err := dbConn.QueryRow("SELECT value FROM settings WHERE key = 'server-settings'").Scan(&valStr)
		if err == nil && valStr != "" {
			var s struct {
				ScannerParseSubtitles bool `json:"scannerParseSubtitles"`
				ScannerFindCovers     bool `json:"scannerFindCovers"`
			}
			s.ScannerParseSubtitles = true
			s.ScannerFindCovers = true
			if err := json.Unmarshal([]byte(valStr), &s); err == nil {
				scannerParseSubtitles = s.ScannerParseSubtitles
				scannerFindCovers = s.ScannerFindCovers
			}
		}
	}

	fnMeta := GetBookDataFromDir(itemRelPath)
	meta.Title = fnMeta.Title
	if scannerParseSubtitles {
		meta.Subtitle = fnMeta.Subtitle
	}
	meta.Authors = fnMeta.Authors
	meta.Narrators = fnMeta.Narrators
	meta.SeriesName = fnMeta.SeriesName
	meta.SeriesSequence = fnMeta.SeriesSequence
	meta.PublishedYear = fnMeta.PublishedYear
	meta.ASIN = fnMeta.ASIN

	var audioFiles []FileItem
	var ebookFiles []FileItem
	var imageFiles []FileItem
	var opfFile string
	var nfoFile string
	var descFile string
	var readerFile string

	for _, f := range groupFiles {
		ext := strings.ToLower(strings.TrimPrefix(f.Extension, "."))
		if IsMediaFile(mediaType, f.Extension, audiobooksOnly) {
			if ext == "epub" || ext == "pdf" || ext == "mobi" || ext == "azw3" || ext == "cbz" || ext == "cbr" {
				ebookFiles = append(ebookFiles, f)
			} else {
				audioFiles = append(audioFiles, f)
			}
		} else if ext == "jpg" || ext == "jpeg" || ext == "png" || ext == "webp" {
			imageFiles = append(imageFiles, f)
		} else if ext == "opf" {
			opfFile = f.Path
		} else if ext == "nfo" {
			nfoFile = f.Path
		} else if f.Name == "desc.txt" {
			descFile = f.Path
		} else if f.Name == "reader.txt" {
			readerFile = f.Path
		}
	}

	if scannerFindCovers && len(imageFiles) > 0 {
		var bestCover string
		for _, img := range imageFiles {
			name := strings.ToLower(img.Name)
			if strings.Contains(name, "cover") || strings.Contains(name, "folder") || strings.Contains(name, "front") {
				bestCover = img.Path
				break
			}
		}
		if bestCover == "" {
			bestCover = imageFiles[0].Path
		}
		meta.CoverPath = bestCover
	}

	var totalDuration float64
	var audioFilesData []interface{}

	type parsedAudioFile struct {
		index     int
		afObj     map[string]interface{}
		duration  float64
		tagTitle  string
		tagArtist string
		tagGenre  string
		tagYear   string
		tagAlbum  string
		chapters  []Chapter
	}

	parsedFiles := make([]parsedAudioFile, len(audioFiles))
	var wg sync.WaitGroup

	for i, f := range audioFiles {
		wg.Add(1)
		go func(i int, f FileItem) {
			defer wg.Done()

			log.Printf("[Scanner] [%s] Probing audio file (%d/%d): %s", itemPath, i+1, len(audioFiles), f.Path)
			probe, err := probeAudioFile(f.Path)
			var duration float64
			var bitrate int64
			var codec string
			var channels int
			var sampleRate int
			var chapters []Chapter
			if err == nil {
				duration = probe.Duration
				bitrate = probe.BitRate
				codec = probe.Codec
				channels = probe.Channels
				sampleRate = probe.SampleRate
				chapters = probe.Chapters
			} else {
				log.Printf("[Scanner] [%s] Probing audio file failed: %v", itemPath, err)
			}

			log.Printf("[Scanner] [%s] Parsing audio tags for: %s", itemPath, f.Path)
			tags, tagsErr := parseAudioTags(f.Path)
			var trackNum, discNum int
			var tagTitle, tagArtist, tagAlbum, tagGenre, tagYear string
			if tags != nil {
				tagTitle = tags.Title
				tagArtist = tags.Artist
				tagAlbum = tags.Album
				tagGenre = tags.Genre
				tagYear = tags.Year
				trackNum = tags.Track
				discNum = tags.Disc
			} else if tagsErr != nil {
				log.Printf("[Scanner] [%s] Parsing audio tags failed: %v", itemPath, tagsErr)
			}

			afObj := map[string]interface{}{
				"index":                i,
				"ino":                  f.Ino,
				"addedAt":              time.Now().UnixMilli(),
				"updatedAt":            time.Now().UnixMilli(),
				"trackNumFromMeta":     nullIfZero(trackNum),
				"discNumFromMeta":      nullIfZero(discNum),
				"trackNumFromFilename": extractTrackNumberFromFilename(f.Name),
				"discNumFromFilename":  nil,
				"manuallyVerified":     false,
				"exclude":              false,
				"error":                nil,
				"format":               strings.TrimPrefix(f.Extension, "."),
				"duration":             duration,
				"bitRate":              bitrate,
				"language":             nil,
				"codec":                codec,
				"timeBase":             "1/1000",
				"channels":             channels,
				"sampleRate":           sampleRate,
				"channelLayout":        nil,
				"chapters":             chapters,
				"embeddedCoverArt":     nil,
				"metaTags": map[string]interface{}{
					"tagTitle":       tagTitle,
					"tagArtist":      tagArtist,
					"tagAlbum":       tagAlbum,
					"tagGenre":       tagGenre,
					"tagDate":        tagYear,
					"tagTrack":       nullIfZero(trackNum),
					"tagDisc":        nullIfZero(discNum),
					"tagAlbumArtist": nil,
				},
				"mimeType": "audio/" + strings.TrimPrefix(f.Extension, "."),
				"metadata": map[string]interface{}{
					"path":     f.Path,
					"relPath":  f.RelPath,
					"filename": f.Name,
					"ext":      f.Extension,
					"size":     f.Size,
					"mtime":    f.MtimeMs,
					"ctime":    f.CtimeMs,
				},
			}

			parsedFiles[i] = parsedAudioFile{
				index:     i,
				afObj:     afObj,
				duration:  duration,
				tagTitle:  tagTitle,
				tagArtist: tagArtist,
				tagGenre:  tagGenre,
				tagYear:   tagYear,
				tagAlbum:  tagAlbum,
				chapters:  chapters,
			}
		}(i, f)
	}
	wg.Wait()

	for i, f := range audioFiles {
		pf := parsedFiles[i]
		totalDuration += pf.duration
		audioFilesData = append(audioFilesData, pf.afObj)

		if mediaType == "podcast" {
			epTitle := pf.tagTitle
			if epTitle == "" {
				epTitle = strings.TrimSuffix(f.Name, f.Extension)
			}
			meta.PodcastEpisodes = append(meta.PodcastEpisodes, PodcastEpisodeScanData{
				ID:        uuidStr(),
				Title:     epTitle,
				AudioFile: pf.afObj,
			})
		}

		if i == 0 {
			if pf.tagTitle != "" && meta.Title == "" {
				meta.Title = pf.tagTitle
			}
			if pf.tagArtist != "" && len(meta.Authors) == 0 {
				meta.Authors = []string{pf.tagArtist}
			}
			if pf.tagGenre != "" && len(meta.Genres) == 0 {
				meta.Genres = []string{pf.tagGenre}
			}
			if pf.tagYear != "" && meta.PublishedYear == "" {
				meta.PublishedYear = pf.tagYear
			}
			if pf.tagAlbum != "" && meta.SeriesName == "" {
				meta.SeriesName = pf.tagAlbum
			}
			if len(pf.chapters) > 0 && len(meta.Chapters) == 0 {
				meta.Chapters = pf.chapters
			}
		}
	}

	meta.Duration = totalDuration
	meta.AudioFiles = audioFilesData

	if len(ebookFiles) > 0 {
		var eb FileItem
		for _, e := range ebookFiles {
			if strings.ToLower(e.Extension) == ".epub" {
				eb = e
				break
			}
		}
		if eb.Path == "" {
			eb = ebookFiles[0]
		}

		meta.EbookFile = map[string]interface{}{
			"ebookFormat": strings.ToLower(strings.TrimPrefix(eb.Extension, ".")),
			"metadata": map[string]interface{}{
				"path":     eb.Path,
				"relPath":  eb.RelPath,
				"filename": eb.Name,
				"ext":      eb.Extension,
				"size":     eb.Size,
				"mtime":    eb.MtimeMs,
				"ctime":    eb.CtimeMs,
			},
		}

		// Parse metadata from ebook using internal/metadata
		var parsed *metadata.EbookMetadata
		var err error
		log.Printf("[Scanner] [%s] Extracting ebook metadata from: %s", itemPath, eb.Path)
		if strings.ToLower(eb.Extension) == ".epub" {
			parsed, err = metadata.ExtractEpubMetadata(context.Background(), eb.Path)
		} else if ext := strings.ToLower(eb.Extension); ext == ".cbz" || ext == ".cbr" || ext == ".pdf" {
			parsed, err = metadata.ExtractComicMetadata(context.Background(), eb.Path)
		}
		if err != nil {
			log.Printf("[Scanner] [%s] Extracting ebook metadata failed: %v", itemPath, err)
		}
		if err == nil && parsed != nil {
			if meta.Title == "" && parsed.Title != "" {
				meta.Title = parsed.Title
			}
			if len(meta.Authors) == 0 && parsed.Author != "" {
				meta.Authors = parseNameString(parsed.Author)
			}
			if meta.Publisher == "" && parsed.Publisher != "" {
				meta.Publisher = parsed.Publisher
			}
			if meta.PublishedYear == "" && parsed.PublishedYear != "" {
				meta.PublishedYear = parsed.PublishedYear
			}
			if meta.Description == "" && parsed.Description != "" {
				meta.Description = parsed.Description
			}
			if meta.Language == "" && parsed.Language != "" {
				meta.Language = parsed.Language
			}
			if meta.ISBN == "" && parsed.ISBN != "" {
				meta.ISBN = parsed.ISBN
			}
			if len(meta.Chapters) == 0 && len(parsed.Chapters) > 0 {
				var chs []Chapter
				for _, c := range parsed.Chapters {
					chs = append(chs, Chapter{
						ID:    c.ID,
						Title: c.Title,
					})
				}
				meta.Chapters = chs
			}
		}

		// Extract cover from ebook if no cover image was found in the folder
		if scannerFindCovers && meta.CoverPath == "" {
			metadataCoverWithItem := true
			if dbConn != nil {
				if settings, err := idb.GetServerSettings(dbConn); err == nil && settings != nil {
					metadataCoverWithItem = settings.MetadataCoverWithItem
				}
			}

			var destCover string
			if metadataCoverWithItem {
				destCover = filepath.Join(filepath.Dir(eb.Path), "cover.jpg")
			} else {
				itemDir := filepath.Join(MetadataPath, "items", itemID)
				_ = os.MkdirAll(itemDir, 0755)
				destCover = filepath.Join(itemDir, "cover.jpg")
			}

			var extractErr error
			log.Printf("[Scanner] [%s] Extracting ebook cover from: %s to %s", itemPath, eb.Path, destCover)
			if strings.ToLower(eb.Extension) == ".epub" {
				extractErr = metadata.ExtractEpubCover(context.Background(), eb.Path, destCover)
			} else if ext := strings.ToLower(eb.Extension); ext == ".cbz" || ext == ".cbr" || ext == ".pdf" {
				extractErr = metadata.ExtractComicCover(context.Background(), eb.Path, destCover)
			}
			if extractErr == nil {
				meta.CoverPath = destCover
			} else {
				log.Printf("[Scanner] [%s] Extracting ebook cover failed: %v", itemPath, extractErr)
			}
		}
	}

	if opfFile != "" {
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

	if nfoFile != "" {
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

	if descFile != "" {
		log.Printf("[Scanner] [%s] Reading description file: %s", itemPath, descFile)
		if data, err := os.ReadFile(descFile); err == nil && len(data) > 0 {
			meta.Description = strings.TrimSpace(string(data))
		} else if err != nil {
			log.Printf("[Scanner] [%s] Reading description file failed: %v", itemPath, err)
		}
	}
	if readerFile != "" {
		log.Printf("[Scanner] [%s] Reading reader file: %s", itemPath, readerFile)
		if data, err := os.ReadFile(readerFile); err == nil && len(data) > 0 {
			lines := strings.Split(string(data), "\n")
			if len(lines) > 0 && strings.TrimSpace(lines[0]) != "" {
				meta.Narrators = parseNameString(strings.TrimSpace(lines[0]))
			}
		} else if err != nil {
			log.Printf("[Scanner] [%s] Reading reader file failed: %v", itemPath, err)
		}
	}

	return meta
}
