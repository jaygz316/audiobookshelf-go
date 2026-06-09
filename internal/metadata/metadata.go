package metadata

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"html"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/nwaples/rardecode"
)

// EbookMetadata represents the parsed metadata and chapters of an e-book or comic.
type EbookMetadata struct {
	Title         string    `json:"title"`
	Author        string    `json:"author"`
	Publisher     string    `json:"publisher"`
	PublishedYear string    `json:"publishedYear"`
	Description   string    `json:"description"`
	Language      string    `json:"language"`
	ISBN          string    `json:"isbn"`
	Chapters      []Chapter `json:"chapters"`
}

// Chapter represents an individual section or chapter in an e-book's table of contents.
type Chapter struct {
	ID          int     `json:"id"`
	Title       string  `json:"title"`
	StartOffset float64 `json:"startOffset"` // in seconds
	EndOffset   float64 `json:"endOffset"`   // in seconds
}

// container XML structures
type container struct {
	XMLName   xml.Name  `xml:"container"`
	Rootfiles rootfiles `xml:"rootfiles"`
}

type rootfiles struct {
	Rootfile []rootfile `xml:"rootfile"`
}

type rootfile struct {
	FullPath string `xml:"full-path,attr"`
}

// opfPackage XML structures
type opfPackage struct {
	XMLName  xml.Name    `xml:"package"`
	Metadata opfMetadata `xml:"metadata"`
	Manifest opfManifest `xml:"manifest"`
	Spine    opfSpine    `xml:"spine"`
}

type opfMetadata struct {
	Title       []opfString  `xml:"title"`
	Creator     []opfCreator `xml:"creator"`
	Publisher   []string     `xml:"publisher"`
	Date        []string     `xml:"date"`
	Language    []string     `xml:"language"`
	Identifier  []opfID      `xml:"identifier"`
	Description []string     `xml:"description"`
	Meta        []opfMeta    `xml:"meta"`
}

type opfString struct {
	Value string `xml:",chardata"`
}

type opfCreator struct {
	Value  string `xml:",chardata"`
	Role   string `xml:"role,attr"`
	FileAs string `xml:"file-as,attr"`
	ID     string `xml:"id,attr"`
}

type opfID struct {
	Value  string `xml:",chardata"`
	Scheme string `xml:"scheme,attr"`
	ID     string `xml:"id,attr"`
}

type opfMeta struct {
	Name     string `xml:"name,attr"`
	Content  string `xml:"content,attr"`
	Refines  string `xml:"refines,attr"`
	Property string `xml:"property,attr"`
	Value    string `xml:",chardata"`
}

type opfManifest struct {
	Items []opfItem `xml:"item"`
}

type opfItem struct {
	ID         string `xml:"id,attr"`
	Href       string `xml:"href,attr"`
	MediaType  string `xml:"media-type,attr"`
	Properties string `xml:"properties,attr"`
}

type opfSpine struct {
	Toc string `xml:"toc,attr"`
}

// ncx XML structures
type ncx struct {
	XMLName xml.Name  `xml:"ncx"`
	NavMap  ncxNavMap `xml:"navMap"`
}

type ncxNavMap struct {
	NavPoints []ncxNavPoint `xml:"navPoint"`
}

type ncxNavPoint struct {
	NavLabel  ncxNavLabel   `xml:"navLabel"`
	NavPoints []ncxNavPoint `xml:"navPoint"`
}

type ncxNavLabel struct {
	Text string `xml:"text"`
}

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

var htmlTagRegex = regexp.MustCompile(`(?i)<[^>]*>`)
var yearRegex = regexp.MustCompile(`\b(19\d{2}|20\d{2})\b`)
var supportedImages = map[string]bool{
	".jpg":  true,
	".jpeg": true,
	".png":  true,
	".webp": true,
	".gif":  true,
}

// ExtractEpubMetadata parses a local EPUB file and extracts its metadata and table of contents.
func ExtractEpubMetadata(ctx context.Context, filePath string) (*EbookMetadata, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	r, err := zip.OpenReader(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open zip reader: %w", err)
	}
	defer r.Close()

	// Locate container.xml
	containerFile := findZipEntryCaseInsensitive(&r.Reader, "META-INF/container.xml")
	if containerFile == nil {
		return nil, errors.New("META-INF/container.xml not found")
	}

	// Read container.xml
	rc, err := containerFile.Open()
	if err != nil {
		return nil, fmt.Errorf("failed to open container.xml: %w", err)
	}
	defer rc.Close()

	containerData, err := io.ReadAll(rc)
	if err != nil {
		return nil, fmt.Errorf("failed to read container.xml: %w", err)
	}

	var container container
	if err := xml.Unmarshal(containerData, &container); err != nil {
		return nil, fmt.Errorf("failed to unmarshal container.xml: %w", err)
	}

	if len(container.Rootfiles.Rootfile) == 0 {
		return nil, errors.New("no rootfile found in container.xml")
	}

	packageDocPath := container.Rootfiles.Rootfile[0].FullPath
	opfFile := findZipEntryCaseInsensitive(&r.Reader, packageDocPath)
	if opfFile == nil {
		return nil, fmt.Errorf("opf file not found at %s", packageDocPath)
	}

	// Read OPF file
	rcOPF, err := opfFile.Open()
	if err != nil {
		return nil, fmt.Errorf("failed to open opf file: %w", err)
	}
	defer rcOPF.Close()

	opfData, err := io.ReadAll(rcOPF)
	if err != nil {
		return nil, fmt.Errorf("failed to read opf file: %w", err)
	}

	var opf opfPackage
	if err := xml.Unmarshal(opfData, &opf); err != nil {
		return nil, fmt.Errorf("failed to unmarshal opf file: %w", err)
	}

	meta := mapOpfToEbookMetadata(&opf)

	// Resolve chapters
	tocHref, tocType := findTocPath(&opf)
	if tocHref != "" {
		tocZipPath := path.Clean(path.Join(path.Dir(packageDocPath), tocHref))
		tocFile := findZipEntryCaseInsensitive(&r.Reader, tocZipPath)
		if tocFile != nil {
			rcTOC, err := tocFile.Open()
			if err == nil {
				defer rcTOC.Close()
				tocData, err := io.ReadAll(rcTOC)
				if err == nil {
					if tocType == "ncx" {
						var ncx ncx
						if err := xml.Unmarshal(tocData, &ncx); err == nil {
							var chapters []Chapter
							flattenNcxNavPoints(ncx.NavMap.NavPoints, &chapters)
							meta.Chapters = chapters
						}
					} else if tocType == "nav" {
						if chapters, err := parseNavChapters(tocData); err == nil {
							meta.Chapters = chapters
						}
					}
				}
			}
		}
	}

	if meta.Chapters == nil {
		meta.Chapters = []Chapter{}
	}

	if meta.Title == "" {
		meta.Title = strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath))
	}

	return meta, nil
}

// ExtractEpubCover parses an EPUB file, locates the cover page/image, and writes it to the destination cover file.
func ExtractEpubCover(ctx context.Context, filePath, destPath string) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	r, err := zip.OpenReader(filePath)
	if err != nil {
		return fmt.Errorf("failed to open zip reader: %w", err)
	}
	defer r.Close()

	containerFile := findZipEntryCaseInsensitive(&r.Reader, "META-INF/container.xml")
	if containerFile == nil {
		return extractCoverByNameFallback(&r.Reader, destPath)
	}

	rc, err := containerFile.Open()
	if err != nil {
		return fmt.Errorf("failed to open container.xml: %w", err)
	}
	defer rc.Close()

	containerData, err := io.ReadAll(rc)
	if err != nil {
		return fmt.Errorf("failed to read container.xml: %w", err)
	}

	var container container
	if err := xml.Unmarshal(containerData, &container); err != nil {
		return fmt.Errorf("failed to unmarshal container.xml: %w", err)
	}

	if len(container.Rootfiles.Rootfile) == 0 {
		return extractCoverByNameFallback(&r.Reader, destPath)
	}

	packageDocPath := container.Rootfiles.Rootfile[0].FullPath
	opfFile := findZipEntryCaseInsensitive(&r.Reader, packageDocPath)
	if opfFile == nil {
		return extractCoverByNameFallback(&r.Reader, destPath)
	}

	rcOPF, err := opfFile.Open()
	if err != nil {
		return fmt.Errorf("failed to open opf file: %w", err)
	}
	defer rcOPF.Close()

	opfData, err := io.ReadAll(rcOPF)
	if err != nil {
		return fmt.Errorf("failed to read opf file: %w", err)
	}

	var opf opfPackage
	if err := xml.Unmarshal(opfData, &opf); err != nil {
		return fmt.Errorf("failed to unmarshal opf file: %w", err)
	}

	coverHref := findCoverHref(&opf)
	if coverHref == "" {
		return extractCoverByNameFallback(&r.Reader, destPath)
	}

	coverZipPath := path.Clean(path.Join(path.Dir(packageDocPath), coverHref))
	coverFile := findZipEntryCaseInsensitive(&r.Reader, coverZipPath)
	if coverFile == nil {
		return extractCoverByNameFallback(&r.Reader, destPath)
	}

	return extractZipEntry(coverFile, destPath)
}

// ExtractComicMetadata parses Comic metadata (ComicInfo.xml, etc.) and cover image from CBZ, CBR, or PDF.
func ExtractComicMetadata(ctx context.Context, filePath string) (*EbookMetadata, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".cbz":
		return extractCbzMetadata(ctx, filePath)
	case ".cbr":
		return extractCbrMetadata(ctx, filePath)
	case ".pdf":
		return parsePdfInfo(ctx, filePath)
	default:
		return nil, fmt.Errorf("unsupported comic format: %s", ext)
	}
}

// ExtractComicCover extracts the cover page/image from CBZ, CBR, or PDF and writes it to destPath.
func ExtractComicCover(ctx context.Context, filePath, destPath string) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".cbz":
		return extractCbzCover(ctx, filePath, destPath)
	case ".cbr":
		return extractCbrCover(ctx, filePath, destPath)
	case ".pdf":
		return extractPdfCover(ctx, filePath, destPath)
	default:
		return fmt.Errorf("unsupported comic format: %s", ext)
	}
}

// Helper methods

func findZipEntryCaseInsensitive(r *zip.Reader, targetPath string) *zip.File {
	cleaned := path.Clean(targetPath)
	for _, f := range r.File {
		if f.Name == cleaned {
			return f
		}
	}
	for _, f := range r.File {
		if strings.EqualFold(f.Name, cleaned) {
			return f
		}
	}
	return nil
}

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

func stripAllTags(s string) string {
	s = html.UnescapeString(s)
	s = htmlTagRegex.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	return strings.TrimSpace(s)
}

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

func findCoverHref(opf *opfPackage) string {
	var coverID string
	for _, m := range opf.Metadata.Meta {
		if m.Name == "cover" {
			coverID = m.Content
			break
		}
	}

	if coverID != "" {
		for _, item := range opf.Manifest.Items {
			if item.ID == coverID {
				return item.Href
			}
		}
	}

	for _, item := range opf.Manifest.Items {
		properties := strings.Split(item.Properties, " ")
		for _, p := range properties {
			if p == "cover-image" {
				return item.Href
			}
		}
	}

	for _, item := range opf.Manifest.Items {
		if strings.HasPrefix(item.MediaType, "image/") {
			return item.Href
		}
	}

	return ""
}

func extractCoverByNameFallback(r *zip.Reader, destPath string) error {
	var coverCandidates []string
	for _, f := range r.File {
		if !f.FileInfo().IsDir() {
			base := strings.ToLower(filepath.Base(f.Name))
			if base == "cover.jpg" || base == "cover.jpeg" || base == "cover.png" ||
				base == "folder.jpg" || base == "folder.jpeg" || base == "folder.png" {
				coverCandidates = append(coverCandidates, f.Name)
			}
		}
	}

	if len(coverCandidates) == 0 {
		return errors.New("no cover image found in epub zip archive")
	}

	sort.Slice(coverCandidates, func(i, j int) bool {
		iBase := strings.ToLower(filepath.Base(coverCandidates[i]))
		jBase := strings.ToLower(filepath.Base(coverCandidates[j]))
		if strings.HasPrefix(iBase, "cover") && strings.HasPrefix(jBase, "folder") {
			return true
		}
		if strings.HasPrefix(iBase, "folder") && strings.HasPrefix(jBase, "cover") {
			return false
		}
		return coverCandidates[i] < coverCandidates[j]
	})

	targetEntry := findZipEntryCaseInsensitive(r, coverCandidates[0])
	if targetEntry == nil {
		return errors.New("failed to locate fallback cover file inside zip")
	}

	return extractZipEntry(targetEntry, destPath)
}

func extractZipEntry(f *zip.File, destPath string) error {
	rc, err := f.Open()
	if err != nil {
		return fmt.Errorf("failed to open zip entry for extraction: %w", err)
	}
	defer rc.Close()

	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return fmt.Errorf("failed to create destination parent directory: %w", err)
	}

	destFile, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("failed to open destination file for cover extraction: %w", err)
	}
	defer destFile.Close()

	if _, err = io.Copy(destFile, rc); err != nil {
		return fmt.Errorf("failed to write cover image data: %w", err)
	}

	return nil
}

func isImageFile(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	return supportedImages[ext]
}

func naturalLess(a, b string) bool {
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		aChar := a[i]
		bChar := b[j]
		if isDigit(aChar) && isDigit(bChar) {
			aNumStr := ""
			for i < len(a) && isDigit(a[i]) {
				aNumStr += string(a[i])
				i++
			}
			bNumStr := ""
			for j < len(b) && isDigit(b[j]) {
				bNumStr += string(b[j])
				j++
			}
			aNum, _ := strconv.Atoi(aNumStr)
			bNum, _ := strconv.Atoi(bNumStr)
			if aNum != bNum {
				return aNum < bNum
			}
		} else {
			aLower := strings.ToLower(string(aChar))
			bLower := strings.ToLower(string(bChar))
			if aLower != bLower {
				return aLower < bLower
			}
			i++
			j++
		}
	}
	return len(a) < len(b)
}

func isDigit(c byte) bool {
	return c >= '0' && c <= '9'
}

func extractCbzMetadata(ctx context.Context, filePath string) (*EbookMetadata, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	r, err := zip.OpenReader(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open cbz reader: %w", err)
	}
	defer r.Close()

	var comicInfoData []byte
	for _, f := range r.File {
		if !f.FileInfo().IsDir() && strings.ToLower(path.Base(f.Name)) == "comicinfo.xml" {
			rc, err := f.Open()
			if err != nil {
				return nil, fmt.Errorf("failed to open comicinfo.xml inside cbz: %w", err)
			}
			comicInfoData, err = io.ReadAll(rc)
			rc.Close()
			if err != nil {
				return nil, fmt.Errorf("failed to read comicinfo.xml inside cbz: %w", err)
			}
			break
		}
	}

	var meta *EbookMetadata
	if len(comicInfoData) > 0 {
		var info comicInfo
		if err := xml.Unmarshal(comicInfoData, &info); err == nil {
			meta = mapComicInfoToEbookMetadata(&info)
		}
	}

	if meta == nil {
		meta = &EbookMetadata{}
	}

	if meta.Title == "" {
		meta.Title = strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath))
	}

	return meta, nil
}

func extractCbrMetadata(ctx context.Context, filePath string) (*EbookMetadata, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open cbr file: %w", err)
	}
	defer f.Close()

	rr, err := rardecode.NewReader(f, "")
	if err != nil {
		return nil, fmt.Errorf("failed to create rar reader: %w", err)
	}

	var comicInfoData []byte
	for {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		header, err := rr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read next rar entry: %w", err)
		}
		if !header.IsDir && strings.ToLower(path.Base(header.Name)) == "comicinfo.xml" {
			comicInfoData, err = io.ReadAll(rr)
			if err != nil {
				return nil, fmt.Errorf("failed to read comicinfo.xml inside cbr: %w", err)
			}
			break
		}
	}

	var meta *EbookMetadata
	if len(comicInfoData) > 0 {
		var info comicInfo
		if err := xml.Unmarshal(comicInfoData, &info); err == nil {
			meta = mapComicInfoToEbookMetadata(&info)
		}
	}

	if meta == nil {
		meta = &EbookMetadata{}
	}

	if meta.Title == "" {
		meta.Title = strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath))
	}

	return meta, nil
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

func parsePdfInfo(ctx context.Context, filePath string) (*EbookMetadata, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	cmd := exec.CommandContext(ctx, "pdfinfo", filePath)
	out, err := cmd.Output()
	if err != nil {
		return &EbookMetadata{
			Title: strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath)),
		}, nil
	}

	meta := &EbookMetadata{}
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		switch key {
		case "Title":
			meta.Title = val
		case "Author":
			meta.Author = val
		case "Publisher":
			meta.Publisher = val
		case "CreationDate":
			meta.PublishedYear = extractYearFromPdfDate(val)
		}
	}

	if meta.Title == "" {
		meta.Title = strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath))
	}
	return meta, nil
}

func extractYearFromPdfDate(dateStr string) string {
	if match := yearRegex.FindString(dateStr); match != "" {
		return match
	}
	if strings.HasPrefix(dateStr, "D:") && len(dateStr) >= 6 {
		return dateStr[2:6]
	}
	return ""
}

func extractCbzCover(ctx context.Context, filePath, destPath string) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	r, err := zip.OpenReader(filePath)
	if err != nil {
		return fmt.Errorf("failed to open cbz reader: %w", err)
	}
	defer r.Close()

	var imageFiles []string
	for _, f := range r.File {
		if !f.FileInfo().IsDir() && isImageFile(f.Name) {
			if !strings.HasPrefix(filepath.Base(f.Name), ".") && !strings.Contains(f.Name, "__MACOSX") {
				imageFiles = append(imageFiles, f.Name)
			}
		}
	}

	if len(imageFiles) == 0 {
		return errors.New("no images found in cbz archive")
	}

	sort.Slice(imageFiles, func(i, j int) bool {
		return naturalLess(imageFiles[i], imageFiles[j])
	})

	firstImage := imageFiles[0]

	for _, f := range r.File {
		if f.Name == firstImage {
			return extractZipEntry(f, destPath)
		}
	}

	return fmt.Errorf("could not find image file %s in archive", firstImage)
}

func extractCbrCover(ctx context.Context, filePath, destPath string) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open cbr file: %w", err)
	}
	defer f.Close()

	rr, err := rardecode.NewReader(f, "")
	if err != nil {
		return fmt.Errorf("failed to create rar reader: %w", err)
	}

	var imageFiles []string
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		header, err := rr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read next rar entry: %w", err)
		}
		if !header.IsDir && isImageFile(header.Name) {
			if !strings.HasPrefix(filepath.Base(header.Name), ".") && !strings.Contains(header.Name, "__MACOSX") {
				imageFiles = append(imageFiles, header.Name)
			}
		}
	}

	if len(imageFiles) == 0 {
		return errors.New("no images found in cbr archive")
	}

	sort.Slice(imageFiles, func(i, j int) bool {
		return naturalLess(imageFiles[i], imageFiles[j])
	})

	firstImage := imageFiles[0]

	_, err = f.Seek(0, 0)
	if err != nil {
		return fmt.Errorf("failed to seek cbr file: %w", err)
	}
	rr, err = rardecode.NewReader(f, "")
	if err != nil {
		return fmt.Errorf("failed to recreate rar reader: %w", err)
	}

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		header, err := rr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read next rar entry: %w", err)
		}
		if header.Name == firstImage {
			if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
				return fmt.Errorf("failed to create destination parent directory: %w", err)
			}
			destFile, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
			if err != nil {
				return fmt.Errorf("failed to open destination cover file: %w", err)
			}
			_, err = io.Copy(destFile, rr)
			destFile.Close()
			if err != nil {
				return fmt.Errorf("failed to write cover image data: %w", err)
			}
			return nil
		}
	}

	return fmt.Errorf("could not find image file %s in archive", firstImage)
}

func extractPdfCover(ctx context.Context, filePath, destPath string) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	tmpDir, err := os.MkdirTemp("", "pdf-cover-*")
	if err != nil {
		return fmt.Errorf("failed to create temporary directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	outputPrefix := filepath.Join(tmpDir, "cover")
	cmd := exec.CommandContext(ctx, "pdftoppm", "-jpeg", "-f", "1", "-l", "1", "-singlefile", filePath, outputPrefix)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("pdftoppm failed: %w", err)
	}

	resultPath := outputPrefix + ".jpg"
	return copyFile(resultPath, destPath)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return fmt.Errorf("failed to create destination parent directory: %w", err)
	}

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("failed to open destination file: %w", err)
	}
	defer out.Close()

	if _, err = io.Copy(out, in); err != nil {
		return fmt.Errorf("failed to copy file data: %w", err)
	}
	return nil
}
