// Package scanner provides library scanning functionality for audiobookshelf.
package scanner

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/dhowden/tag"
	"github.com/google/uuid"

	"audiobookshelf/internal/core"
	idb "audiobookshelf/internal/db"
	log "audiobookshelf/internal/logger"
	"audiobookshelf/internal/metadata"
	inotification "audiobookshelf/internal/notification"
	isocket "audiobookshelf/internal/socket"
)

var MetadataPath string
var probeSemaphore chan struct{}

func init() {
	concurrency := runtime.NumCPU()
	if concurrency < 4 {
		concurrency = 4
	}
	if concurrency > 8 {
		concurrency = 8
	}
	probeSemaphore = make(chan struct{}, concurrency)
}

// FileItem represents a file found during library scanning.
type FileItem struct {
	Path        string
	RelPath     string
	RelDirPath  string
	Name        string
	Extension   string
	Size        int64
	MtimeMs     int64
	CtimeMs     int64
	BirthtimeMs int64
	Ino         string
}

var cdRegex = regexp.MustCompile(`(?i)^(cd|dis[ck])\s*\d{1,3}$`)

// IsMediaFile returns true if the file extension is a supported media file for the given media type.
func IsMediaFile(mediaType, ext string, audiobooksOnly bool) bool {
	ext = strings.ToLower(strings.TrimPrefix(ext, "."))
	supportedAudio := map[string]bool{
		"mp3": true, "m4a": true, "m4b": true, "flac": true, "opus": true,
		"ogg": true, "wma": true, "wav": true, "mp4": true, "aac": true, "webm": true,
	}
	supportedEbook := map[string]bool{
		"epub": true, "pdf": true, "mobi": true, "azw3": true, "cbz": true, "cbr": true,
	}
	if mediaType == "podcast" {
		return supportedAudio[ext]
	}
	if audiobooksOnly {
		return supportedAudio[ext]
	}
	return supportedAudio[ext] || supportedEbook[ext]
}

// GroupFileItemsIntoLibraryItemDirs groups files into library item directory buckets.
func GroupFileItemsIntoLibraryItemDirs(mediaType string, files []FileItem, audiobooksOnly bool) map[string][]FileItem {
	var itemsFiltered []FileItem
	for _, item := range files {
		isRoot := item.RelDirPath == "" || item.RelDirPath == "."
		if !isRoot || (mediaType == "book" && IsMediaFile(mediaType, item.Extension, audiobooksOnly)) {
			itemsFiltered = append(itemsFiltered, item)
		}
	}

	var mediaFileItems []FileItem
	var otherFileItems []FileItem
	for _, item := range itemsFiltered {
		if IsMediaFile(mediaType, item.Extension, audiobooksOnly) {
			mediaFileItems = append(mediaFileItems, item)
		} else {
			otherFileItems = append(otherFileItems, item)
		}
	}

	libraryItemGroup := make(map[string][]FileItem)
	for _, item := range mediaFileItems {
		isRoot := item.RelDirPath == "" || item.RelDirPath == "."
		if isRoot {
			libraryItemGroup[item.Name] = []FileItem{item}
		} else {
			dirparts := splitPath(item.RelDirPath)
			numparts := len(dirparts)
			var currentPath string
			for i := 0; i < numparts; i++ {
				dirpart := dirparts[i]
				currentPath = filepath.ToSlash(filepath.Join(currentPath, dirpart))

				if _, ok := libraryItemGroup[currentPath]; ok {
					libraryItemGroup[currentPath] = append(libraryItemGroup[currentPath], item)
					break
				}

				if i == numparts-1 {
					libraryItemGroup[currentPath] = []FileItem{item}
					break
				}

				if i == numparts-2 && cdRegex.MatchString(dirparts[i+1]) {
					libraryItemGroup[currentPath] = []FileItem{item}
					break
				}
			}
		}
	}

	for _, item := range otherFileItems {
		dirparts := splitPath(item.RelDirPath)
		numparts := len(dirparts)
		var currentPath string
		for i := 0; i < numparts; i++ {
			dirpart := dirparts[i]
			currentPath = filepath.ToSlash(filepath.Join(currentPath, dirpart))
			if _, ok := libraryItemGroup[currentPath]; ok {
				libraryItemGroup[currentPath] = append(libraryItemGroup[currentPath], item)
				break
			}
		}
	}

	return libraryItemGroup
}

func splitPath(path string) []string {
	path = filepath.ToSlash(path)
	var parts []string
	for _, p := range strings.Split(path, "/") {
		if p != "" && p != "." {
			parts = append(parts, p)
		}
	}
	return parts
}

// WalkLibraryFolder walks a folder and returns all FileItems found.
func WalkLibraryFolder(folderPath string) ([]FileItem, error) {
	log.Printf("[Scanner] WalkLibraryFolder: Starting walk of folder: %s", folderPath)
	var items []FileItem
	err := filepath.WalkDir(folderPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		name := d.Name()
		if strings.HasPrefix(name, ".") {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return nil
		}

		relPath, err := filepath.Rel(folderPath, path)
		if err != nil {
			return nil
		}
		relPath = filepath.ToSlash(relPath)
		relDirPath := filepath.ToSlash(filepath.Dir(relPath))
		if relDirPath == "." {
			relDirPath = ""
		}

		ext := filepath.Ext(name)

		var ino string
		var ctimeMs, birthtimeMs int64
		mtimeMs := info.ModTime().UnixNano() / int64(time.Millisecond)

		if stat, ok := info.Sys().(*syscall.Stat_t); ok {
			ino = strconv.FormatUint(stat.Ino, 10)
			ctimeMs = stat.Ctim.Sec*1000 + stat.Ctim.Nsec/1000000
			birthtimeMs = ctimeMs
		} else {
			ino = strconv.FormatInt(mtimeMs, 10)
			ctimeMs = mtimeMs
			birthtimeMs = mtimeMs
		}

		items = append(items, FileItem{
			Path:        filepath.ToSlash(path),
			RelPath:     relPath,
			RelDirPath:  relDirPath,
			Name:        name,
			Extension:   ext,
			Size:        info.Size(),
			MtimeMs:     mtimeMs,
			CtimeMs:     ctimeMs,
			BirthtimeMs: birthtimeMs,
			Ino:         ino,
		})

		return nil
	})
	return items, err
}

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

// NameToLastFirst converts "First Last" to "Last, First".
func NameToLastFirst(name string) string {
	parts := strings.Fields(name)
	if len(parts) > 1 {
		return parts[len(parts)-1] + ", " + strings.Join(parts[:len(parts)-1], " ")
	}
	return name
}

func uuidStr() string {
	return uuid.New().String()
}

// UUIDStr returns a new UUID string.
func UUIDStr() string {
	return uuidStr()
}

// getSortingPrefixes retrieves the sorting prefixes from the database settings.
func getSortingPrefixes(db *sql.DB) []string {
	var prefixes []string
	var valStr string
	_ = db.QueryRow("SELECT value FROM settings WHERE key = 'server-settings'").Scan(&valStr)
	if valStr != "" {
		var s struct {
			SortingPrefixes []string `json:"sortingPrefixes"`
		}
		if json.Unmarshal([]byte(valStr), &s) == nil {
			prefixes = s.SortingPrefixes
		}
	}
	if len(prefixes) == 0 {
		prefixes = []string{"the", "a", "an"}
	}
	return prefixes
}

// GetTitleIgnorePrefix returns the title with sorting prefixes removed.
func GetTitleIgnorePrefix(db *sql.DB, title string) string {
	prefixes := getSortingPrefixes(db)
	return getTitleIgnorePrefixGo(title, prefixes)
}

// getTitleIgnorePrefixGo strips common prefixes from titles for sorting.
func getTitleIgnorePrefixGo(title string, prefixes []string) string {
	lower := strings.ToLower(title)
	for _, prefix := range prefixes {
		p := strings.ToLower(prefix) + " "
		if strings.HasPrefix(lower, p) {
			return title[len(p):]
		}
	}
	return title
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

// ScanLibrary scans a library and updates the database.
// socketAuth may be nil (used for emitting WebSocket events).
func ScanLibrary(db *sql.DB, libraryID string, socketAuth *isocket.Authority) error {
	log.Printf("[Scanner] Starting scan for library ID: %s", libraryID)

	var libName, mediaType, libSettingsStr string
	err := db.QueryRow("SELECT name, mediaType, settings FROM libraries WHERE id = ?", libraryID).Scan(&libName, &mediaType, &libSettingsStr)
	if err != nil {
		return fmt.Errorf("library not found: %w", err)
	}
	log.Printf("[Scanner] Library name: %s, Media type: %s", libName, mediaType)

	var libSettings struct {
		AudiobooksOnly bool `json:"audiobooksOnly"`
	}
	if libSettingsStr != "" {
		_ = json.Unmarshal([]byte(libSettingsStr), &libSettings)
	}

	if socketAuth != nil {
		socketAuth.Emitter("library_scan_started", libraryID, nil)
	}

	defer func() {
		log.Printf("[Scanner] defer library_scan_complete for library ID: %s", libraryID)
		if socketAuth != nil {
			socketAuth.Emitter("library_scan_complete", libraryID, nil)
		}
	}()

	prefixes := getSortingPrefixes(db)
	log.Printf("[Scanner] Loaded %d sorting prefixes", len(prefixes))

	rows, err := db.Query("SELECT id, path FROM libraryFolders WHERE libraryId = ?", libraryID)
	if err != nil {
		return err
	}
	defer rows.Close()

	var folders []struct {
		id   string
		path string
	}
	for rows.Next() {
		var id, path string
		if err := rows.Scan(&id, &path); err != nil {
			return err
		}
		folders = append(folders, struct{ id, path string }{id, path})
	}
	if err := rows.Err(); err != nil {
		return err
	}
	log.Printf("[Scanner] Found %d library folders to scan", len(folders))

	var foundPaths []string

	for _, folder := range folders {
		log.Printf("[Scanner] Walking folder: %s", folder.path)
		files, err := WalkLibraryFolder(folder.path)
		if err != nil {
			log.Printf("[Scanner] Failed to walk folder %s: %v", folder.path, err)
			continue
		}
		log.Printf("[Scanner] Walk complete. Found %d file items. Grouping them...", len(files))

		grouped := GroupFileItemsIntoLibraryItemDirs(mediaType, files, libSettings.AudiobooksOnly)
		log.Printf("[Scanner] Grouped into %d library item directories", len(grouped))

		type itemInfo struct {
			folderID          string
			itemPath          string
			groupFiles        []FileItem
			isFile            bool
			maxMtime          int64
			maxCtime          int64
			totalSize         int64
			ino               string
			itemRelPath       string
			needsScan         bool
			isNew             bool
			existingID        string
			itemID            string
			existingIsMissing int
			meta              *GroupMetadata
		}

		var items []*itemInfo

		// Phase 1: Sequential Database Verification (Read-Only)
		for groupDir, groupFiles := range grouped {
			var itemPath string
			var isFile bool
			if len(groupFiles) == 1 && groupFiles[0].RelDirPath == "" {
				itemPath = groupFiles[0].Path
				isFile = true
			} else {
				itemPath = filepath.ToSlash(filepath.Join(folder.path, groupDir))
				isFile = false
			}

			var maxMtime, maxCtime int64
			var totalSize int64
			for _, f := range groupFiles {
				if f.MtimeMs > maxMtime {
					maxMtime = f.MtimeMs
				}
				if f.CtimeMs > maxCtime {
					maxCtime = f.CtimeMs
				}
				totalSize += f.Size
			}

			var ino string
			if len(groupFiles) > 0 {
				ino = groupFiles[0].Ino
			}

			var itemRelPath string
			if isFile {
				itemRelPath = groupFiles[0].RelPath
			} else {
				itemRelPath = filepath.Dir(groupFiles[0].RelPath)
				if itemRelPath == "." {
					itemRelPath = ""
				}
			}

			var existingID string
			var existingMtimeStr string
			var existingIsMissing int
			err = db.QueryRow("SELECT id, mtime, isMissing FROM libraryItems WHERE path = ? AND libraryId = ?", itemPath, libraryID).Scan(&existingID, &existingMtimeStr, &existingIsMissing)

			item := &itemInfo{
				folderID:          folder.id,
				itemPath:          itemPath,
				groupFiles:        groupFiles,
				isFile:            isFile,
				maxMtime:          maxMtime,
				maxCtime:          maxCtime,
				totalSize:         totalSize,
				ino:               ino,
				itemRelPath:       itemRelPath,
				existingID:        existingID,
				existingIsMissing: existingIsMissing,
			}

			if err == sql.ErrNoRows {
				item.needsScan = true
				item.isNew = true
				item.itemID = uuidStr()
			} else if err == nil {
				existingMtime := parseEpochMillis(existingMtimeStr)
				if maxMtime != existingMtime {
					item.needsScan = true
					item.isNew = false
					item.itemID = existingID
				} else {
					item.needsScan = false
					item.itemID = existingID
				}
			}

			items = append(items, item)
		}

		// Phase 2: Concurrent Metadata Parsing
		var tasks []*itemInfo
		for _, item := range items {
			if item.needsScan {
				tasks = append(tasks, item)
			}
		}

		if len(tasks) > 0 {
			log.Printf("[Scanner] Parsing metadata concurrently for %d items", len(tasks))
			concurrency := runtime.NumCPU()
			if concurrency < 4 {
				concurrency = 4
			}
			if concurrency > 8 {
				concurrency = 8
			}
			if concurrency > len(tasks) {
				concurrency = len(tasks)
			}

			taskChan := make(chan *itemInfo, len(tasks))
			for _, t := range tasks {
				taskChan <- t
			}
			close(taskChan)

			var wg sync.WaitGroup
			for i := 0; i < concurrency; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for item := range taskChan {
						item.meta = parseMetadataForGroup(db, item.itemID, item.groupFiles, mediaType, item.itemPath, item.itemRelPath, libSettings.AudiobooksOnly)
					}
				}()
			}
			wg.Wait()
			log.Printf("[Scanner] Concurrent metadata parsing complete")
		}

		// Phase 3: Sequential Database Writes
		for _, item := range items {
			foundPaths = append(foundPaths, item.itemPath)

			if item.needsScan {
				if item.isNew {
					log.Printf("[Scanner] Scanning new item at: %s", item.itemPath)
					err := scanNewLibraryItem(db, libraryID, item.folderID, item.itemPath, item.groupFiles, mediaType, item.isFile, item.maxMtime, item.maxCtime, item.totalSize, item.ino, libSettings.AudiobooksOnly, prefixes, socketAuth, item.meta)
					if err != nil {
						log.Printf("[Scanner] Error scanning new item at %s: %v", item.itemPath, err)
					}
				} else {
					if item.existingIsMissing != 0 {
						log.Printf("[Scanner] Item %s marked as missing but exists now. Restoring.", item.itemPath)
						_, _ = db.Exec("UPDATE libraryItems SET isMissing = 0 WHERE id = ?", item.existingID)
					}
					log.Printf("[Scanner] Mtime changed for existing item %s (mtime: %d != existing), rescanning", item.itemPath, item.maxMtime)
					err := scanExistingLibraryItem(db, item.existingID, libraryID, item.folderID, item.itemPath, item.groupFiles, mediaType, item.isFile, item.maxMtime, item.maxCtime, item.totalSize, item.ino, libSettings.AudiobooksOnly, prefixes, socketAuth, item.meta)
					if err != nil {
						log.Printf("[Scanner] Error updating existing item at %s: %v", item.itemPath, err)
					}
				}
			} else {
				if item.existingID != "" && item.existingIsMissing != 0 {
					log.Printf("[Scanner] Item %s marked as missing but exists now. Restoring.", item.itemPath)
					_, _ = db.Exec("UPDATE libraryItems SET isMissing = 0 WHERE id = ?", item.existingID)
				}
				log.Printf("[Scanner] Item %s mtime unchanged, skipping rescan", item.itemPath)
			}
		}
	}

	log.Printf("[Scanner] Checking for missing library items...")
	dbItems, err := db.Query("SELECT id, path FROM libraryItems WHERE libraryId = ? AND isMissing = 0", libraryID)
	if err != nil {
		return err
	}
	defer dbItems.Close()
	foundPathsMap := make(map[string]bool)
	for _, p := range foundPaths {
		foundPathsMap[p] = true
	}

	for dbItems.Next() {
		var id, path string
		if err := dbItems.Scan(&id, &path); err != nil {
			return err
		}
		if !foundPathsMap[path] {
			log.Printf("[Scanner] Item %s not found on disk, marking as missing", path)
			_, err = db.Exec("UPDATE libraryItems SET isMissing = 1 WHERE id = ?", id)
			if err != nil {
				return err
			}

			if socketAuth != nil {
				if minItem, err := GetLibraryItemMinifiedByID(db, id); err == nil {
					EmitLibraryItemEvent(socketAuth, "item_updated", minItem)
				}
			}
		}
	}
	if err := dbItems.Err(); err != nil {
		return err
	}

	log.Printf("[Scanner] Scan complete for library ID: %s", libraryID)
	return nil
}

func scanNewLibraryItem(db *sql.DB, libraryID, folderID, itemPath string, groupFiles []FileItem, mediaType string, isFile bool, mtime, ctime, totalSize int64, ino string, audiobooksOnly bool, prefixes []string, socketAuth *isocket.Authority, meta *GroupMetadata) error {
	itemID := uuidStr()
	mediaID := uuidStr()
	nowStr := time.Now().Format("2006-01-02 15:04:05.000")

	log.Printf("[Scanner] [%s] scanNewLibraryItem: Beginning transaction", itemPath)
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var itemRelPath string
	if isFile {
		itemRelPath = groupFiles[0].RelPath
	} else {
		itemRelPath = filepath.Dir(groupFiles[0].RelPath)
		if itemRelPath == "." {
			itemRelPath = ""
		}
	}

	var title, authorNamesFirstLast, authorNamesLastFirst string
	title = meta.Title
	if title == "" {
		title = filepath.Base(itemPath)
	}
	titleIgnorePrefix := getTitleIgnorePrefixGo(title, prefixes)

	if mediaType == "book" {
		authorNamesFirstLast = strings.Join(meta.Authors, ", ")
		var lfs []string
		for _, a := range meta.Authors {
			lfs = append(lfs, NameToLastFirst(a))
		}
		authorNamesLastFirst = strings.Join(lfs, ", ")

		narratorsJSON, _ := json.Marshal(meta.Narrators)
		audioFilesJSON, _ := json.Marshal(meta.AudioFiles)
		ebookFileJSON, _ := json.Marshal(meta.EbookFile)
		chaptersJSON, _ := json.Marshal(meta.Chapters)
		tagsJSON, _ := json.Marshal(meta.Tags)
		genresJSON, _ := json.Marshal(meta.Genres)

		var coverPath interface{}
		if meta.CoverPath != "" {
			coverPath = meta.CoverPath
		}

		cols := getTableColumnsTx(tx, "books")
		var colNames []string
		var placeholders []string
		var args []interface{}

		addCol := func(name string, val interface{}) {
			if cols[name] {
				colNames = append(colNames, name)
				placeholders = append(placeholders, "?")
				args = append(args, val)
			}
		}

		addCol("id", mediaID)
		addCol("title", title)
		addCol("titleIgnorePrefix", titleIgnorePrefix)
		addCol("subtitle", meta.Subtitle)
		addCol("publishedYear", meta.PublishedYear)
		addCol("publishedDate", meta.PublishedDate)
		addCol("publisher", meta.Publisher)
		addCol("description", meta.Description)
		addCol("isbn", meta.ISBN)
		addCol("asin", meta.ASIN)
		addCol("language", meta.Language)
		addCol("explicit", 0)
		addCol("abridged", 0)
		addCol("coverPath", coverPath)
		addCol("duration", meta.Duration)
		addCol("narrators", narratorsJSON)
		addCol("audioFiles", audioFilesJSON)
		addCol("ebookFile", ebookFileJSON)
		addCol("chapters", chaptersJSON)
		addCol("tags", tagsJSON)
		addCol("genres", genresJSON)
		addCol("createdAt", nowStr)
		addCol("updatedAt", nowStr)

		query := fmt.Sprintf("INSERT INTO books (%s) VALUES (%s)", strings.Join(colNames, ", "), strings.Join(placeholders, ", "))
		log.Printf("[Scanner] [%s] scanNewLibraryItem: Inserting into books table", itemPath)
		_, err = tx.Exec(query, args...)
		if err != nil {
			return err
		}

		log.Printf("[Scanner] [%s] scanNewLibraryItem: Inserting authors", itemPath)
		for _, author := range meta.Authors {
			authorID := uuidStr()
			lastFirst := NameToLastFirst(author)
			_ = insertAuthor(tx, authorID, author, lastFirst, libraryID)

			var existingAuthorID string
			_ = tx.QueryRow("SELECT id FROM authors WHERE name = ? AND libraryId = ?", author, libraryID).Scan(&existingAuthorID)
			if existingAuthorID != "" {
				authorID = existingAuthorID
			}
			_ = insertBookAuthor(tx, mediaID, authorID)
		}

		if meta.SeriesName != "" {
			log.Printf("[Scanner] [%s] scanNewLibraryItem: Inserting series", itemPath)
			seriesID := uuidStr()
			_ = insertSeries(tx, seriesID, meta.SeriesName, libraryID)

			var existingSeriesID string
			_ = tx.QueryRow("SELECT id FROM series WHERE name = ? AND libraryId = ?", meta.SeriesName, libraryID).Scan(&existingSeriesID)
			if existingSeriesID != "" {
				seriesID = existingSeriesID
			}
			_ = insertBookSeries(tx, mediaID, seriesID, meta.SeriesSequence)
		}

	} else if mediaType == "podcast" {
		tagsJSON, _ := json.Marshal(meta.Tags)
		genresJSON, _ := json.Marshal(meta.Genres)
		var author string
		if len(meta.Authors) > 0 {
			author = meta.Authors[0]
		}

		cols := getTableColumnsTx(tx, "podcasts")
		var colNames []string
		var placeholders []string
		var args []interface{}

		addCol := func(name string, val interface{}) {
			if cols[name] {
				colNames = append(colNames, name)
				placeholders = append(placeholders, "?")
				args = append(args, val)
			}
		}

		addCol("id", mediaID)
		addCol("title", title)
		addCol("titleIgnorePrefix", titleIgnorePrefix)
		addCol("author", author)
		addCol("releaseDate", meta.PublishedDate)
		addCol("feedURL", "")
		addCol("imageURL", "")
		addCol("description", meta.Description)
		addCol("itunesPageURL", "")
		addCol("itunesId", "")
		addCol("itunesArtistId", "")
		addCol("language", meta.Language)
		addCol("podcastType", "")
		addCol("explicit", 0)
		addCol("autoDownloadEpisodes", 0)
		addCol("autoDownloadSchedule", "")
		addCol("lastEpisodeCheck", "")
		addCol("maxEpisodesToKeep", 0)
		addCol("maxNewEpisodesToDownload", 0)
		addCol("coverPath", meta.CoverPath)
		addCol("tags", tagsJSON)
		addCol("genres", genresJSON)
		addCol("numEpisodes", len(meta.AudioFiles))
		addCol("createdAt", nowStr)
		addCol("updatedAt", nowStr)

		query := fmt.Sprintf("INSERT INTO podcasts (%s) VALUES (%s)", strings.Join(colNames, ", "), strings.Join(placeholders, ", "))
		log.Printf("[Scanner] [%s] scanNewLibraryItem: Inserting into podcasts table", itemPath)
		_, err = tx.Exec(query, args...)
		if err != nil {
			return err
		}

		log.Printf("[Scanner] [%s] scanNewLibraryItem: Inserting podcast episodes", itemPath)
		for _, ep := range meta.PodcastEpisodes {
			audioFileJSON, _ := json.Marshal(ep.AudioFile)

			colsEp := getTableColumnsTx(tx, "podcastEpisodes")
			var colNamesEp []string
			var placeholdersEp []string
			var argsEp []interface{}

			addColEp := func(name string, val interface{}) {
				if colsEp[name] {
					colNamesEp = append(colNamesEp, name)
					placeholdersEp = append(placeholdersEp, "?")
					argsEp = append(argsEp, val)
				}
			}

			addColEp("id", ep.ID)
			addColEp("podcastId", mediaID)
			addColEp("title", ep.Title)
			addColEp("audioFile", string(audioFileJSON))
			addColEp("createdAt", nowStr)
			addColEp("updatedAt", nowStr)

			qEp := fmt.Sprintf("INSERT INTO podcastEpisodes (%s) VALUES (%s)", strings.Join(colNamesEp, ", "), strings.Join(placeholdersEp, ", "))
			_, err = tx.Exec(qEp, argsEp...)
			if err != nil {
				return err
			}
		}
	}

	mtimeStr := formatEpochMillis(mtime)
	ctimeStr := formatEpochMillis(ctime)

	colsLI := getTableColumnsTx(tx, "libraryItems")
	var colNamesLI []string
	var placeholdersLI []string
	var argsLI []interface{}

	addColLI := func(name string, val interface{}) {
		if colsLI[name] {
			colNamesLI = append(colNamesLI, name)
			placeholdersLI = append(placeholdersLI, "?")
			argsLI = append(argsLI, val)
		}
	}

	addColLI("id", itemID)
	addColLI("ino", ino)
	addColLI("libraryId", libraryID)
	addColLI("path", itemPath)
	addColLI("relPath", itemRelPath)
	addColLI("isFile", isFile)
	addColLI("mtime", mtimeStr)
	addColLI("ctime", ctimeStr)
	addColLI("birthtime", ctimeStr)
	addColLI("createdAt", nowStr)
	addColLI("updatedAt", nowStr)
	addColLI("isMissing", 0)
	addColLI("isInvalid", 0)
	addColLI("mediaType", mediaType)
	addColLI("mediaId", mediaID)
	addColLI("size", totalSize)
	addColLI("libraryFolderId", folderID)
	addColLI("authorNamesFirstLast", authorNamesFirstLast)
	addColLI("authorNamesLastFirst", authorNamesLastFirst)
	addColLI("title", title)
	addColLI("titleIgnorePrefix", titleIgnorePrefix)

	queryLI := fmt.Sprintf("INSERT INTO libraryItems (%s) VALUES (%s)", strings.Join(colNamesLI, ", "), strings.Join(placeholdersLI, ", "))
	log.Printf("[Scanner] [%s] scanNewLibraryItem: Inserting into libraryItems table", itemPath)
	_, err = tx.Exec(queryLI, argsLI...)
	if err != nil {
		return err
	}
	log.Printf("[Scanner] [%s] scanNewLibraryItem: Committing transaction", itemPath)
	err = tx.Commit()
	if err != nil {
		return err
	}
	log.Printf("[Scanner] [%s] scanNewLibraryItem: Transaction committed successfully", itemPath)

	if mediaType == "podcast" {
		var libraryName string
		_ = db.QueryRow("SELECT name FROM libraries WHERE id = ?", libraryID).Scan(&libraryName)
		for _, ep := range meta.PodcastEpisodes {
			extraData := map[string]string{
				"podcastTitle": title,
				"episodeTitle": ep.Title,
				"libraryName":  libraryName,
			}
			inotification.TriggerEvent(context.Background(), db, "onPodcastEpisodeDownloaded", &libraryID, "New Episode", fmt.Sprintf("%s - %s", title, ep.Title), extraData)
		}
	} else if mediaType == "book" {
		var libraryName string
		_ = db.QueryRow("SELECT name FROM libraries WHERE id = ?", libraryID).Scan(&libraryName)
		extraData := map[string]string{
			"title":       title,
			"author":      authorNamesFirstLast,
			"libraryName": libraryName,
		}
		inotification.TriggerEvent(context.Background(), db, "onItemAdded", &libraryID, "New Book Added", fmt.Sprintf("%s by %s", title, authorNamesFirstLast), extraData)
	}

	if socketAuth != nil {
		if minItem, err := GetLibraryItemMinifiedByID(db, itemID); err == nil {
			EmitLibraryItemsEvent(socketAuth, "items_added", minItem)
		}
	}

	return nil
}

func scanExistingLibraryItem(db *sql.DB, itemID, libraryID, folderID, itemPath string, groupFiles []FileItem, mediaType string, isFile bool, mtime, ctime, totalSize int64, ino string, audiobooksOnly bool, prefixes []string, socketAuth *isocket.Authority, meta *GroupMetadata) error {
	var mediaID string
	err := db.QueryRow("SELECT mediaId FROM libraryItems WHERE id = ?", itemID).Scan(&mediaID)
	if err != nil {
		return err
	}

	log.Printf("[Scanner] [%s] scanExistingLibraryItem: Beginning transaction", itemPath)
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	nowStr := time.Now().Format("2006-01-02 15:04:05.000")

	var itemRelPath string
	if isFile {
		itemRelPath = groupFiles[0].RelPath
	} else {
		itemRelPath = filepath.Dir(groupFiles[0].RelPath)
		if itemRelPath == "." {
			itemRelPath = ""
		}
	}

	var title, authorNamesFirstLast, authorNamesLastFirst string
	title = meta.Title
	if title == "" {
		title = filepath.Base(itemPath)
	}
	titleIgnorePrefix := getTitleIgnorePrefixGo(title, prefixes)

	if mediaType == "book" {
		var bLockedFields []byte
		var dbTitle, dbSubtitle, dbPublishedYear, dbPublishedDate, dbPublisher, dbDescription, dbIsbn, dbAsin, dbLanguage, dbCoverPath sql.NullString
		var dbNarrators, dbTags, dbGenres []byte

		_ = tx.QueryRow(`
			SELECT title, subtitle, publishedYear, publishedDate, publisher, description, isbn, asin, language, coverPath, narrators, tags, genres, lockedFields
			FROM books WHERE id = ?
		`, mediaID).Scan(
			&dbTitle, &dbSubtitle, &dbPublishedYear, &dbPublishedDate, &dbPublisher, &dbDescription, &dbIsbn, &dbAsin, &dbLanguage, &dbCoverPath, &dbNarrators, &dbTags, &dbGenres, &bLockedFields,
		)

		var lockedFields []string
		if len(bLockedFields) > 0 {
			_ = json.Unmarshal(bLockedFields, &lockedFields)
		}

		isLocked := func(field string) bool {
			for _, f := range lockedFields {
				if f == field {
					return true
				}
			}
			return false
		}

		if isLocked("title") && dbTitle.String != "" {
			title = dbTitle.String
			titleIgnorePrefix = getTitleIgnorePrefixGo(title, prefixes)
		}
		if isLocked("subtitle") && dbSubtitle.Valid {
			meta.Subtitle = dbSubtitle.String
		}
		if isLocked("publishedYear") && dbPublishedYear.Valid {
			meta.PublishedYear = dbPublishedYear.String
		}
		if isLocked("publishedDate") && dbPublishedDate.Valid {
			meta.PublishedDate = dbPublishedDate.String
		}
		if isLocked("publisher") && dbPublisher.Valid {
			meta.Publisher = dbPublisher.String
		}
		if isLocked("description") && dbDescription.Valid {
			meta.Description = dbDescription.String
		}
		if isLocked("isbn") && dbIsbn.Valid {
			meta.ISBN = dbIsbn.String
		}
		if isLocked("asin") && dbAsin.Valid {
			meta.ASIN = dbAsin.String
		}
		if isLocked("language") && dbLanguage.Valid {
			meta.Language = dbLanguage.String
		}
		if (isLocked("cover") || isLocked("coverPath")) && dbCoverPath.Valid {
			meta.CoverPath = dbCoverPath.String
		}
		if (isLocked("narrators") || isLocked("narrator")) && len(dbNarrators) > 0 {
			var narrators []string
			if err := json.Unmarshal(dbNarrators, &narrators); err == nil {
				meta.Narrators = narrators
			}
		}
		if isLocked("tags") && len(dbTags) > 0 {
			var tags []string
			if err := json.Unmarshal(dbTags, &tags); err == nil {
				meta.Tags = tags
			}
		}
		if isLocked("genres") && len(dbGenres) > 0 {
			var genres []string
			if err := json.Unmarshal(dbGenres, &genres); err == nil {
				meta.Genres = genres
			}
		}
		if isLocked("authors") || isLocked("author") {
			rows, err := tx.Query("SELECT name FROM authors WHERE id IN (SELECT authorId FROM bookAuthors WHERE bookId = ?)", mediaID)
			if err == nil {
				defer rows.Close()
				var dbAuthors []string
				for rows.Next() {
					var name string
					if err := rows.Scan(&name); err == nil {
						dbAuthors = append(dbAuthors, name)
					}
				}
				if len(dbAuthors) > 0 {
					meta.Authors = dbAuthors
				}
			}
		}
		if isLocked("series") {
			var dbSeriesName string
			var dbSequence string
			err := tx.QueryRow(`
				SELECT s.name, bs.sequence
				FROM series s
				JOIN bookSeries bs ON s.id = bs.seriesId
				WHERE bs.bookId = ?
			`, mediaID).Scan(&dbSeriesName, &dbSequence)
			if err == nil {
				meta.SeriesName = dbSeriesName
				meta.SeriesSequence = dbSequence
			}
		}

		authorNamesFirstLast = strings.Join(meta.Authors, ", ")
		var lfs []string
		for _, a := range meta.Authors {
			lfs = append(lfs, NameToLastFirst(a))
		}
		authorNamesLastFirst = strings.Join(lfs, ", ")

		narratorsJSON, _ := json.Marshal(meta.Narrators)
		audioFilesJSON, _ := json.Marshal(meta.AudioFiles)
		ebookFileJSON, _ := json.Marshal(meta.EbookFile)
		chaptersJSON, _ := json.Marshal(meta.Chapters)
		tagsJSON, _ := json.Marshal(meta.Tags)
		genresJSON, _ := json.Marshal(meta.Genres)

		var coverPath interface{}
		if meta.CoverPath != "" {
			coverPath = meta.CoverPath
		}

		cols := getTableColumnsTx(tx, "books")
		var setStmts []string
		var args []interface{}

		addCol := func(name string, val interface{}) {
			if cols[name] {
				setStmts = append(setStmts, fmt.Sprintf("%s = ?", name))
				args = append(args, val)
			}
		}

		addCol("title", title)
		addCol("titleIgnorePrefix", titleIgnorePrefix)
		addCol("subtitle", meta.Subtitle)
		addCol("publishedYear", meta.PublishedYear)
		addCol("publishedDate", meta.PublishedDate)
		addCol("publisher", meta.Publisher)
		addCol("description", meta.Description)
		addCol("isbn", meta.ISBN)
		addCol("asin", meta.ASIN)
		addCol("language", meta.Language)
		addCol("coverPath", coverPath)
		addCol("duration", meta.Duration)
		addCol("narrators", narratorsJSON)
		addCol("audioFiles", audioFilesJSON)
		addCol("ebookFile", ebookFileJSON)
		addCol("chapters", chaptersJSON)
		addCol("tags", tagsJSON)
		addCol("genres", genresJSON)
		addCol("updatedAt", nowStr)

		args = append(args, mediaID)
		query := fmt.Sprintf("UPDATE books SET %s WHERE id = ?", strings.Join(setStmts, ", "))
		log.Printf("[Scanner] [%s] scanExistingLibraryItem: Updating books table", itemPath)
		_, err = tx.Exec(query, args...)
		if err != nil {
			return err
		}

		log.Printf("[Scanner] [%s] scanExistingLibraryItem: Updating authors", itemPath)
		if tableExistsTx(tx, "bookAuthors") {
			_, _ = tx.Exec("DELETE FROM bookAuthors WHERE bookId = ?", mediaID)
		}
		for _, author := range meta.Authors {
			authorID := uuidStr()
			lastFirst := NameToLastFirst(author)
			_ = insertAuthor(tx, authorID, author, lastFirst, libraryID)

			var existingAuthorID string
			_ = tx.QueryRow("SELECT id FROM authors WHERE name = ? AND libraryId = ?", author, libraryID).Scan(&existingAuthorID)
			if existingAuthorID != "" {
				authorID = existingAuthorID
			}
			_ = insertBookAuthor(tx, mediaID, authorID)
		}

		log.Printf("[Scanner] [%s] scanExistingLibraryItem: Updating series", itemPath)
		if tableExistsTx(tx, "bookSeries") {
			_, _ = tx.Exec("DELETE FROM bookSeries WHERE bookId = ?", mediaID)
		}
		if meta.SeriesName != "" {
			seriesID := uuidStr()
			_ = insertSeries(tx, seriesID, meta.SeriesName, libraryID)

			var existingSeriesID string
			_ = tx.QueryRow("SELECT id FROM series WHERE name = ? AND libraryId = ?", meta.SeriesName, libraryID).Scan(&existingSeriesID)
			if existingSeriesID != "" {
				seriesID = existingSeriesID
			}
			_ = insertBookSeries(tx, mediaID, seriesID, meta.SeriesSequence)
		}

	} else if mediaType == "podcast" {
		var pLockedFields []byte
		var dbTitle, dbAuthor, dbDescription, dbLanguage, dbCoverPath sql.NullString
		var dbTags, dbGenres []byte

		_ = tx.QueryRow(`
			SELECT title, author, description, language, coverPath, tags, genres, lockedFields
			FROM podcasts WHERE id = ?
		`, mediaID).Scan(
			&dbTitle, &dbAuthor, &dbDescription, &dbLanguage, &dbCoverPath, &dbTags, &dbGenres, &pLockedFields,
		)

		var lockedFields []string
		if len(pLockedFields) > 0 {
			_ = json.Unmarshal(pLockedFields, &lockedFields)
		}

		isLocked := func(field string) bool {
			for _, f := range lockedFields {
				if f == field {
					return true
				}
			}
			return false
		}

		if isLocked("title") && dbTitle.String != "" {
			title = dbTitle.String
			titleIgnorePrefix = getTitleIgnorePrefixGo(title, prefixes)
		}
		var author string
		if len(meta.Authors) > 0 {
			author = meta.Authors[0]
		}
		if (isLocked("author") || isLocked("authors")) && dbAuthor.Valid {
			author = dbAuthor.String
		}
		if isLocked("description") && dbDescription.Valid {
			meta.Description = dbDescription.String
		}
		if isLocked("language") && dbLanguage.Valid {
			meta.Language = dbLanguage.String
		}
		if (isLocked("cover") || isLocked("coverPath")) && dbCoverPath.Valid {
			meta.CoverPath = dbCoverPath.String
		}
		if isLocked("tags") && len(dbTags) > 0 {
			var tags []string
			if err := json.Unmarshal(dbTags, &tags); err == nil {
				meta.Tags = tags
			}
		}
		if isLocked("genres") && len(dbGenres) > 0 {
			var genres []string
			if err := json.Unmarshal(dbGenres, &genres); err == nil {
				meta.Genres = genres
			}
		}

		tagsJSON, _ := json.Marshal(meta.Tags)
		genresJSON, _ := json.Marshal(meta.Genres)

		cols := getTableColumnsTx(tx, "podcasts")
		var setStmts []string
		var args []interface{}

		addCol := func(name string, val interface{}) {
			if cols[name] {
				setStmts = append(setStmts, fmt.Sprintf("%s = ?", name))
				args = append(args, val)
			}
		}

		addCol("title", title)
		addCol("titleIgnorePrefix", titleIgnorePrefix)
		addCol("author", author)
		addCol("releaseDate", meta.PublishedDate)
		addCol("description", meta.Description)
		addCol("language", meta.Language)
		addCol("coverPath", meta.CoverPath)
		addCol("tags", tagsJSON)
		addCol("genres", genresJSON)
		addCol("numEpisodes", len(meta.AudioFiles))
		addCol("updatedAt", nowStr)

		args = append(args, mediaID)
		query := fmt.Sprintf("UPDATE podcasts SET %s WHERE id = ?", strings.Join(setStmts, ", "))
		log.Printf("[Scanner] [%s] scanExistingLibraryItem: Updating podcasts table", itemPath)
		_, err = tx.Exec(query, args...)
		if err != nil {
			return err
		}

		log.Printf("[Scanner] [%s] scanExistingLibraryItem: Updating podcast episodes", itemPath)
		if tableExistsTx(tx, "podcastEpisodes") {
			_, _ = tx.Exec("DELETE FROM podcastEpisodes WHERE podcastId = ?", mediaID)
		}
		for _, ep := range meta.PodcastEpisodes {
			audioFileJSON, _ := json.Marshal(ep.AudioFile)

			colsEp := getTableColumnsTx(tx, "podcastEpisodes")
			var colNamesEp []string
			var placeholdersEp []string
			var argsEp []interface{}

			addColEp := func(name string, val interface{}) {
				if colsEp[name] {
					colNamesEp = append(colNamesEp, name)
					placeholdersEp = append(placeholdersEp, "?")
					argsEp = append(argsEp, val)
				}
			}

			addColEp("id", ep.ID)
			addColEp("podcastId", mediaID)
			addColEp("title", ep.Title)
			addColEp("audioFile", string(audioFileJSON))
			addColEp("createdAt", nowStr)
			addColEp("updatedAt", nowStr)

			qEp := fmt.Sprintf("INSERT INTO podcastEpisodes (%s) VALUES (%s)", strings.Join(colNamesEp, ", "), strings.Join(placeholdersEp, ", "))
			_, err = tx.Exec(qEp, argsEp...)
			if err != nil {
				return err
			}
		}
	}

	mtimeStr := formatEpochMillis(mtime)
	ctimeStr := formatEpochMillis(ctime)

	colsLI := getTableColumnsTx(tx, "libraryItems")
	var setStmtsLI []string
	var argsLI []interface{}

	addColLI := func(name string, val interface{}) {
		if colsLI[name] {
			setStmtsLI = append(setStmtsLI, fmt.Sprintf("%s = ?", name))
			argsLI = append(argsLI, val)
		}
	}

	addColLI("ino", ino)
	addColLI("mtime", mtimeStr)
	addColLI("ctime", ctimeStr)
	addColLI("updatedAt", nowStr)
	addColLI("size", totalSize)
	addColLI("authorNamesFirstLast", authorNamesFirstLast)
	addColLI("authorNamesLastFirst", authorNamesLastFirst)
	addColLI("title", title)
	addColLI("titleIgnorePrefix", titleIgnorePrefix)

	argsLI = append(argsLI, itemID)
	queryLI := fmt.Sprintf("UPDATE libraryItems SET %s WHERE id = ?", strings.Join(setStmtsLI, ", "))
	log.Printf("[Scanner] [%s] scanExistingLibraryItem: Updating libraryItems table", itemPath)
	_, err = tx.Exec(queryLI, argsLI...)
	if err != nil {
		return err
	}

	log.Printf("[Scanner] [%s] scanExistingLibraryItem: Committing transaction", itemPath)
	err = tx.Commit()
	if err != nil {
		return err
	}
	log.Printf("[Scanner] [%s] scanExistingLibraryItem: Transaction committed successfully", itemPath)

	if socketAuth != nil {
		if minItem, err := GetLibraryItemMinifiedByID(db, itemID); err == nil {
			EmitLibraryItemsEvent(socketAuth, "items_updated", minItem)
		}
	}

	return nil
}

// LibraryItemMinifiedJSON is the minified library item structure for API responses.
type LibraryItemMinifiedJSON struct {
	ID               string      `json:"id"`
	Ino              string      `json:"ino"`
	OldLibraryItemID *string     `json:"oldLibraryItemId"`
	LibraryID        string      `json:"libraryId"`
	FolderID         string      `json:"folderId"`
	Path             string      `json:"path"`
	RelPath          string      `json:"relPath"`
	IsFile           bool        `json:"isFile"`
	MtimeMs          int64       `json:"mtimeMs"`
	CtimeMs          int64       `json:"ctimeMs"`
	BirthtimeMs      int64       `json:"birthtimeMs"`
	AddedAt          int64       `json:"addedAt"`
	UpdatedAt        int64       `json:"updatedAt"`
	IsMissing        bool        `json:"isMissing"`
	IsInvalid        bool        `json:"isInvalid"`
	MediaType        string      `json:"mediaType"`
	Media            interface{} `json:"media"`
	NumFiles         int         `json:"numFiles"`
	Size             int64       `json:"size"`
}

// BookMinifiedJSON is the minified book structure.
type BookMinifiedJSON struct {
	ID            string                `json:"id"`
	Metadata      *BookMetadataMinified `json:"metadata"`
	CoverPath     *string               `json:"coverPath"`
	Tags          []string              `json:"tags"`
	NumTracks     int                   `json:"numTracks"`
	NumAudioFiles int                   `json:"numAudioFiles"`
	NumChapters   int                   `json:"numChapters"`
	Duration      float64               `json:"duration"`
	Size          int64                 `json:"size"`
	EbookFormat   *string               `json:"ebookFormat"`
}

type BookSeriesMinifiedJSON struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Sequence string `json:"sequence"`
}

// BookMetadataMinified holds minified book metadata.
type BookMetadataMinified struct {
	Title             string                    `json:"title"`
	TitleIgnorePrefix string                    `json:"titleIgnorePrefix"`
	Subtitle          *string                   `json:"subtitle"`
	AuthorName        string                    `json:"authorName"`
	AuthorNameLF      string                    `json:"authorNameLF"`
	NarratorName      string                    `json:"narratorName"`
	SeriesName        string                    `json:"seriesName"`
	SeriesSequence    *string                   `json:"seriesSequence"`
	Series            []*BookSeriesMinifiedJSON `json:"series"`
	Genres            []string                  `json:"genres"`
	PublishedYear     *string                   `json:"publishedYear"`
	PublishedDate     *string                   `json:"publishedDate"`
	Publisher         *string                   `json:"publisher"`
	Description       *string                   `json:"description"`
	Isbn              *string                   `json:"isbn"`
	Asin              *string                   `json:"asin"`
	Language          *string                   `json:"language"`
	Explicit          bool                      `json:"explicit"`
	Abridged          bool                      `json:"abridged"`
}

// PodcastMinifiedJSON is the minified podcast structure.
type PodcastMinifiedJSON struct {
	ID                       string              `json:"id"`
	Metadata                 *PodcastMetadataMin `json:"metadata"`
	CoverPath                *string             `json:"coverPath"`
	Tags                     []string            `json:"tags"`
	NumEpisodes              int                 `json:"numEpisodes"`
	AutoDownloadEpisodes     bool                `json:"autoDownloadEpisodes"`
	AutoDownloadSchedule     *string             `json:"autoDownloadSchedule"`
	LastEpisodeCheck         *int64              `json:"lastEpisodeCheck"`
	MaxEpisodesToKeep        int                 `json:"maxEpisodesToKeep"`
	MaxNewEpisodesToDownload int                 `json:"maxNewEpisodesToDownload"`
	Size                     int64               `json:"size"`
}

// PodcastMetadataMin holds minified podcast metadata.
type PodcastMetadataMin struct {
	Title             string   `json:"title"`
	TitleIgnorePrefix string   `json:"titleIgnorePrefix"`
	Author            *string  `json:"author"`
	Description       *string  `json:"description"`
	ReleaseDate       *string  `json:"releaseDate"`
	Genres            []string `json:"genres"`
	FeedURL           *string  `json:"feedUrl"`
	ImageURL          *string  `json:"imageUrl"`
	ItunesPageURL     *string  `json:"itunesPageUrl"`
	ItunesID          *string  `json:"itunesId"`
	ItunesArtistID    *string  `json:"itunesArtistId"`
	Explicit          bool     `json:"explicit"`
	Language          *string  `json:"language"`
	Type              *string  `json:"type"`
}

// GetLibraryItemMinifiedByID fetches a minified library item by ID.
func GetLibraryItemMinifiedByID(db *sql.DB, itemID string) (*LibraryItemMinifiedJSON, error) {
	var li LibraryItemMinifiedJSON
	var id, ino, libraryID, folderID, path, relPath, mediaType, mediaID, mtimeStr, ctimeStr, birthtimeStr, createdAtStr, updatedAtStr string
	var isFileVal, isMissingVal, isInvalidVal int
	var size int64

	query := `
		SELECT id, ino, libraryId, libraryFolderId, path, relPath, isFile, mtime, ctime, birthtime, createdAt, updatedAt, isMissing, isInvalid, mediaType, mediaId, size
		FROM libraryItems
		WHERE id = ?
	`
	err := db.QueryRow(query, itemID).Scan(
		&id, &ino, &libraryID, &folderID, &path, &relPath, &isFileVal, &mtimeStr, &ctimeStr, &birthtimeStr, &createdAtStr, &updatedAtStr, &isMissingVal, &isInvalidVal, &mediaType, &mediaID, &size,
	)
	if err != nil {
		return nil, err
	}

	li.ID = id
	li.Ino = ino
	li.LibraryID = libraryID
	li.FolderID = folderID
	li.Path = path
	li.RelPath = relPath
	li.IsFile = isFileVal != 0
	li.MtimeMs = parseEpochMillis(mtimeStr)
	li.CtimeMs = parseEpochMillis(ctimeStr)
	li.BirthtimeMs = parseEpochMillis(birthtimeStr)
	li.AddedAt = parseEpochMillis(createdAtStr)
	li.UpdatedAt = parseEpochMillis(updatedAtStr)
	li.IsMissing = isMissingVal != 0
	li.IsInvalid = isInvalidVal != 0
	li.MediaType = mediaType
	li.Size = size

	if mediaType == "book" {
		var bTitle, bTitleIgnorePrefix, bSubtitle, bPublishedYear, bPublishedDate, bPublisher, bDescription, bIsbn, bAsin, bLanguage, bCoverPath string
		var bDuration float64
		var bNarrators, bAudioFiles, bEbookFile, bChapters, bTags, bGenres []byte
		var bExplicit, bAbridged int

		err = db.QueryRow(`
			SELECT title, titleIgnorePrefix, subtitle, publishedYear, publishedDate, publisher, description, isbn, asin, language, explicit, abridged, coverPath, duration, narrators, audioFiles, ebookFile, chapters, tags, genres
			FROM books WHERE id = ?
		`, mediaID).Scan(
			&bTitle, &bTitleIgnorePrefix, &bSubtitle, &bPublishedYear, &bPublishedDate, &bPublisher, &bDescription, &bIsbn, &bAsin, &bLanguage, &bExplicit, &bAbridged, &bCoverPath, &bDuration, &bNarrators, &bAudioFiles, &bEbookFile, &bChapters, &bTags, &bGenres,
		)
		if err == nil {
			var tags []string
			_ = json.Unmarshal(bTags, &tags)
			var genres []string
			_ = json.Unmarshal(bGenres, &genres)
			var audioFiles []interface{}
			_ = json.Unmarshal(bAudioFiles, &audioFiles)
			var ebook interface{}
			_ = json.Unmarshal(bEbookFile, &ebook)
			var chapters []interface{}
			_ = json.Unmarshal(bChapters, &chapters)

			var authorNames []string
			var seriesNames []string
			var narratorNames []string
			_ = json.Unmarshal(bNarrators, &narratorNames)

			if tableExists(db, "bookAuthors") && tableExists(db, "authors") {
				rows, err := db.Query("SELECT name FROM authors WHERE id IN (SELECT authorId FROM bookAuthors WHERE bookId = ?)", mediaID)
				if err != nil {
					log.Printf("[Scanner] Failed to query authors: %v", err)
				} else {
					defer rows.Close()
					for rows.Next() {
						var name string
						if err := rows.Scan(&name); err != nil {
							log.Printf("[Scanner] Failed to scan author name: %v", err)
							continue
						}
						authorNames = append(authorNames, name)
					}
					if err := rows.Err(); err != nil {
						log.Printf("[Scanner] Authors iteration error: %v", err)
					}
				}
			}
			var seriesList []*BookSeriesMinifiedJSON
			if tableExists(db, "bookSeries") && tableExists(db, "series") {
				rows, err := db.Query("SELECT s.id, s.name, bs.sequence FROM series s JOIN bookSeries bs ON s.id = bs.seriesId WHERE bs.bookId = ?", mediaID)
				if err != nil {
					log.Printf("[Scanner] Failed to query series: %v", err)
				} else {
					defer rows.Close()
					for rows.Next() {
						var sid, name string
						var sequence sql.NullString
						if err := rows.Scan(&sid, &name, &sequence); err != nil {
							log.Printf("[Scanner] Failed to scan series name/sequence: %v", err)
							continue
						}
						var seqVal string
						if sequence.Valid {
							seqVal = sequence.String
						}
						seriesList = append(seriesList, &BookSeriesMinifiedJSON{
							ID:       sid,
							Name:     name,
							Sequence: seqVal,
						})
						if seqVal != "" {
							seriesNames = append(seriesNames, fmt.Sprintf("%s #%s", name, seqVal))
						} else {
							seriesNames = append(seriesNames, name)
						}
					}
					if err := rows.Err(); err != nil {
						log.Printf("[Scanner] Series iteration error: %v", err)
					}
				}
			}

			var firstSeq *string
			if len(seriesList) > 0 && seriesList[0].Sequence != "" {
				firstSeq = &seriesList[0].Sequence
			}

			authorName := strings.Join(authorNames, ", ")
			seriesName := strings.Join(seriesNames, ", ")
			narratorName := strings.Join(narratorNames, ", ")

			var ebookFormat *string
			if len(bEbookFile) > 0 {
				var eb struct {
					EbookFormat string `json:"ebookFormat"`
				}
				if json.Unmarshal(bEbookFile, &eb) == nil && eb.EbookFormat != "" {
					ebookFormat = &eb.EbookFormat
				}
			}

			bookMin := &BookMinifiedJSON{
				ID:            mediaID,
				CoverPath:     nullIfEmpty(bCoverPath),
				Tags:          tags,
				NumTracks:     len(audioFiles),
				NumAudioFiles: len(audioFiles),
				NumChapters:   len(chapters),
				Duration:      bDuration,
				Size:          size,
				EbookFormat:   ebookFormat,
				Metadata: &BookMetadataMinified{
					Title:             bTitle,
					TitleIgnorePrefix: bTitleIgnorePrefix,
					Subtitle:          nullIfEmpty(bSubtitle),
					AuthorName:        authorName,
					AuthorNameLF:      NameToLastFirst(authorName),
					NarratorName:      narratorName,
					SeriesName:        seriesName,
					SeriesSequence:    firstSeq,
					Series:            seriesList,
					Genres:            genres,
					PublishedYear:     nullIfEmpty(bPublishedYear),
					PublishedDate:     nullIfEmpty(bPublishedDate),
					Publisher:         nullIfEmpty(bPublisher),
					Description:       nullIfEmpty(bDescription),
					Isbn:              nullIfEmpty(bIsbn),
					Asin:              nullIfEmpty(bAsin),
					Language:          nullIfEmpty(bLanguage),
					Explicit:          bExplicit != 0,
					Abridged:          bAbridged != 0,
				},
			}
			li.Media = bookMin
		}
	} else if mediaType == "podcast" {
		var pTitle, pTitleIgnorePrefix, pAuthor, pReleaseDate, pFeedURL, pImageURL, pDescription, pItunesPageURL, pItunesID, pItunesArtistID, pLanguage, pPodcastType, pCoverPath string
		var pExplicit, pAutoDownloadEpisodes, pMaxEpisodesToKeep, pMaxNewEpisodesToDownload, pNumEpisodes int
		var pTags, pGenres []byte

		err = db.QueryRow(`
			SELECT title, titleIgnorePrefix, author, releaseDate, feedURL, imageURL, description, itunesPageURL, itunesId, itunesArtistId, language, podcastType, explicit, autoDownloadEpisodes, maxEpisodesToKeep, maxNewEpisodesToDownload, coverPath, tags, genres, numEpisodes
			FROM podcasts WHERE id = ?
		`, mediaID).Scan(
			&pTitle, &pTitleIgnorePrefix, &pAuthor, &pReleaseDate, &pFeedURL, &pImageURL, &pDescription, &pItunesPageURL, &pItunesID, &pItunesArtistID, &pLanguage, &pPodcastType, &pExplicit, &pAutoDownloadEpisodes, &pMaxEpisodesToKeep, &pMaxNewEpisodesToDownload, &pCoverPath, &pTags, &pGenres, &pNumEpisodes,
		)
		if err == nil {
			var tags []string
			_ = json.Unmarshal(pTags, &tags)
			var genres []string
			_ = json.Unmarshal(pGenres, &genres)

			podcastMin := &PodcastMinifiedJSON{
				ID:                       mediaID,
				CoverPath:                nullIfEmpty(pCoverPath),
				Tags:                     tags,
				NumEpisodes:              pNumEpisodes,
				AutoDownloadEpisodes:     pAutoDownloadEpisodes != 0,
				MaxEpisodesToKeep:        pMaxEpisodesToKeep,
				MaxNewEpisodesToDownload: pMaxNewEpisodesToDownload,
				Size:                     size,
				Metadata: &PodcastMetadataMin{
					Title:             pTitle,
					TitleIgnorePrefix: pTitleIgnorePrefix,
					Author:            nullIfEmpty(pAuthor),
					Description:       nullIfEmpty(pDescription),
					ReleaseDate:       nullIfEmpty(pReleaseDate),
					Genres:            genres,
					FeedURL:           nullIfEmpty(pFeedURL),
					ImageURL:          nullIfEmpty(pImageURL),
					ItunesPageURL:     nullIfEmpty(pItunesPageURL),
					ItunesID:          nullIfEmpty(pItunesID),
					ItunesArtistID:    nullIfEmpty(pItunesArtistID),
					Explicit:          pExplicit != 0,
					Language:          nullIfEmpty(pLanguage),
					Type:              nullIfEmpty(pPodcastType),
				},
			}
			li.Media = podcastMin
		}
	}

	return &li, nil
}

// NullIfEmpty returns nil if the string is empty, otherwise returns a pointer to the string.
func NullIfEmpty(s string) *string {
	return nullIfEmpty(s)
}

func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// EmitLibraryItemEvent emits a WebSocket event for a single library item.
func EmitLibraryItemEvent(socketAuth *isocket.Authority, evt string, item *LibraryItemMinifiedJSON) {
	if socketAuth == nil {
		return
	}
	data, err := json.Marshal(item)
	if err != nil {
		return
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err == nil {
		socketAuth.LibraryItemEmitter(evt, m)
	}
}

// EmitLibraryItemsEvent emits a WebSocket event for multiple library items.
func EmitLibraryItemsEvent(socketAuth *isocket.Authority, evt string, item *LibraryItemMinifiedJSON) {
	if socketAuth == nil {
		return
	}
	data, err := json.Marshal(item)
	if err != nil {
		return
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err == nil {
		socketAuth.LibraryItemsEmitter(evt, []map[string]interface{}{m})
	}
}

// HandleScanLibrary returns an HTTP handler for triggering a library scan.
func HandleScanLibrary(db *sql.DB, libraryID string, socketAuth *isocket.Authority) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userSess := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if userSess.Type != "root" && userSess.Type != "admin" {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		var count int
		err := db.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM libraries WHERE id = ?", libraryID).Scan(&count)
		if err != nil {
			log.Printf("[Scanner] Database error: %v", err)
			http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
			return
		}
		if count == 0 {
			http.Error(w, `{"error": "Library not found"}`, http.StatusNotFound)
			return
		}

		go func() {
			if err := ScanLibrary(db, libraryID, socketAuth); err != nil {
				log.Printf("[Scanner] Scan failed: %v", err)
			}
		}()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"success": true}`))
	}
}

// --- internal helpers ---

func getTableColumnsTx(tx *sql.Tx, tableName string) map[string]bool {
	columns := make(map[string]bool)
	rows, err := tx.Query(fmt.Sprintf("PRAGMA table_info(%s)", tableName))
	if err != nil {
		return columns
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dfltVal sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dfltVal, &pk); err != nil {
			log.Printf("[Scanner] Failed to scan table column info: %v", err)
			continue
		}
		columns[name] = true
	}
	if err := rows.Err(); err != nil {
		log.Printf("[Scanner] Table info iteration error for table %s: %v", tableName, err)
	}
	return columns
}

// InsertAuthor inserts an author record into the authors table.
func InsertAuthor(tx *sql.Tx, id, name, lastFirst, libraryID string) error {
	return insertAuthor(tx, id, name, lastFirst, libraryID)
}

func insertAuthor(tx *sql.Tx, id, name, lastFirst, libraryID string) error {
	cols := getTableColumnsTx(tx, "authors")
	if len(cols) == 0 {
		return nil
	}

	var colNames []string
	var placeholders []string
	var args []interface{}

	addCol := func(name string, val interface{}) {
		if cols[name] {
			colNames = append(colNames, name)
			placeholders = append(placeholders, "?")
			args = append(args, val)
		}
	}

	addCol("id", id)
	addCol("name", name)
	addCol("lastFirst", lastFirst)
	addCol("libraryId", libraryID)

	nowStr := time.Now().Format("2006-01-02 15:04:05.000")
	addCol("createdAt", nowStr)
	addCol("updatedAt", nowStr)

	query := fmt.Sprintf("INSERT OR IGNORE INTO authors (%s) VALUES (%s)",
		strings.Join(colNames, ", "),
		strings.Join(placeholders, ", "))
	_, err := tx.Exec(query, args...)
	return err
}

// InsertBookAuthor inserts a bookAuthors association.
func InsertBookAuthor(tx *sql.Tx, bookID, authorID string) error {
	return insertBookAuthor(tx, bookID, authorID)
}

func insertBookAuthor(tx *sql.Tx, bookID, authorID string) error {
	cols := getTableColumnsTx(tx, "bookAuthors")
	if len(cols) == 0 {
		return nil
	}
	var colNames []string
	var placeholders []string
	var args []interface{}

	addCol := func(name string, val interface{}) {
		if cols[name] {
			colNames = append(colNames, name)
			placeholders = append(placeholders, "?")
			args = append(args, val)
		}
	}

	addCol("bookId", bookID)
	addCol("authorId", authorID)
	nowStr := time.Now().Format("2006-01-02 15:04:05.000")
	addCol("createdAt", nowStr)
	addCol("updatedAt", nowStr)

	query := fmt.Sprintf("INSERT OR IGNORE INTO bookAuthors (%s) VALUES (%s)",
		strings.Join(colNames, ", "),
		strings.Join(placeholders, ", "))
	_, err := tx.Exec(query, args...)
	return err
}

// InsertSeries inserts a series record into the series table.
func InsertSeries(tx *sql.Tx, id, name, libraryID string) error {
	return insertSeries(tx, id, name, libraryID)
}

func insertSeries(tx *sql.Tx, id, name, libraryID string) error {
	cols := getTableColumnsTx(tx, "series")
	if len(cols) == 0 {
		return nil
	}
	var colNames []string
	var placeholders []string
	var args []interface{}

	addCol := func(name string, val interface{}) {
		if cols[name] {
			colNames = append(colNames, name)
			placeholders = append(placeholders, "?")
			args = append(args, val)
		}
	}

	addCol("id", id)
	addCol("name", name)
	addCol("libraryId", libraryID)
	nowStr := time.Now().Format("2006-01-02 15:04:05.000")
	addCol("createdAt", nowStr)
	addCol("updatedAt", nowStr)

	query := fmt.Sprintf("INSERT OR IGNORE INTO series (%s) VALUES (%s)",
		strings.Join(colNames, ", "),
		strings.Join(placeholders, ", "))
	_, err := tx.Exec(query, args...)
	return err
}

// InsertBookSeries inserts a bookSeries association.
func InsertBookSeries(tx *sql.Tx, bookID, seriesID, sequence string) error {
	return insertBookSeries(tx, bookID, seriesID, sequence)
}

func insertBookSeries(tx *sql.Tx, bookID, seriesID, sequence string) error {
	cols := getTableColumnsTx(tx, "bookSeries")
	if len(cols) == 0 {
		return nil
	}
	var colNames []string
	var placeholders []string
	var args []interface{}

	addCol := func(name string, val interface{}) {
		if cols[name] {
			colNames = append(colNames, name)
			placeholders = append(placeholders, "?")
			args = append(args, val)
		}
	}

	addCol("bookId", bookID)
	addCol("seriesId", seriesID)
	addCol("sequence", sequence)
	nowStr := time.Now().Format("2006-01-02 15:04:05.000")
	addCol("createdAt", nowStr)
	addCol("updatedAt", nowStr)

	query := fmt.Sprintf("INSERT OR IGNORE INTO bookSeries (%s) VALUES (%s)",
		strings.Join(colNames, ", "),
		strings.Join(placeholders, ", "))
	_, err := tx.Exec(query, args...)
	return err
}

func timeToDBStr(t time.Time) string {
	return t.UTC().Format("2006-01-02 15:04:05.000 +00:00")
}

func formatEpochMillis(epoch int64) string {
	t := time.Unix(epoch/1000, (epoch%1000)*1000000)
	return timeToDBStr(t)
}

func parseEpochMillis(s string) int64 {
	if s == "" {
		return 0
	}
	// Try integer milliseconds first
	if ms, err := strconv.ParseInt(s, 10, 64); err == nil {
		return ms
	}
	// Try as SQLite timestamp
	formats := []string{
		"2006-01-02 15:04:05.999 -07:00",
		"2006-01-02 15:04:05.999 +00:00",
		"2006-01-02T15:04:05.999Z",
		"2006-01-02 15:04:05.999",
		"2006-01-02 15:04:05",
	}
	for _, format := range formats {
		t, err := time.Parse(format, s)
		if err == nil {
			return t.UnixMilli()
		}
	}
	return 0
}

func nullIfZero(val int) interface{} {
	if val == 0 {
		return nil
	}
	return val
}

func extractTrackNumberFromFilename(filename string) interface{} {
	re := regexp.MustCompile(`(?i)(?:^|[-_ ])(?:track|tr|t)?\s*(\d{1,3})(?:[-_ ]|$)`)
	match := re.FindStringSubmatch(filename)
	if len(match) > 1 {
		if val, err := strconv.Atoi(match[1]); err == nil {
			return val
		}
	}
	return nil
}

func tableExists(db *sql.DB, name string) bool {
	var count int
	err := db.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='table' AND name=?", name).Scan(&count)
	return err == nil && count > 0
}

func tableExistsTx(tx *sql.Tx, name string) bool {
	var count int
	err := tx.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='table' AND name=?", name).Scan(&count)
	return err == nil && count > 0
}
