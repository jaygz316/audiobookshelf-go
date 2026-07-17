package scanner

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestCategorizeGroupFiles(t *testing.T) {
	groupFiles := []FileItem{
		{Path: "book/chapter1.mp3", Name: "chapter1.mp3", Extension: ".mp3"},
		{Path: "book/chapter2.m4b", Name: "chapter2.m4b", Extension: ".m4b"},
		{Path: "book/book.epub", Name: "book.epub", Extension: ".epub"},
		{Path: "book/cover.jpg", Name: "cover.jpg", Extension: ".jpg"},
		{Path: "book/metadata.opf", Name: "metadata.opf", Extension: ".opf"},
		{Path: "book/info.nfo", Name: "info.nfo", Extension: ".nfo"},
		{Path: "book/desc.txt", Name: "desc.txt", Extension: ".txt"},
		{Path: "book/reader.txt", Name: "reader.txt", Extension: ".txt"},
	}

	audioFiles, ebookFiles, imageFiles, opfFile, nfoFile, descFile, readerFile := categorizeGroupFiles(groupFiles, "book", false)

	if len(audioFiles) != 2 || audioFiles[0].Name != "chapter1.mp3" || audioFiles[1].Name != "chapter2.m4b" {
		t.Errorf("unexpected audio files: %+v", audioFiles)
	}
	if len(ebookFiles) != 1 || ebookFiles[0].Name != "book.epub" {
		t.Errorf("unexpected ebook files: %+v", ebookFiles)
	}
	if len(imageFiles) != 1 || imageFiles[0].Name != "cover.jpg" {
		t.Errorf("unexpected image files: %+v", imageFiles)
	}
	if opfFile != "book/metadata.opf" {
		t.Errorf("unexpected opf file: %s", opfFile)
	}
	if nfoFile != "book/info.nfo" {
		t.Errorf("unexpected nfo file: %s", nfoFile)
	}
	if descFile != "book/desc.txt" {
		t.Errorf("unexpected desc file: %s", descFile)
	}
	if readerFile != "book/reader.txt" {
		t.Errorf("unexpected reader file: %s", readerFile)
	}
}

func TestFindBestCoverImage(t *testing.T) {
	tests := []struct {
		name       string
		imageFiles []FileItem
		wantPath   string
	}{
		{
			name: "cover in name",
			imageFiles: []FileItem{
				{Path: "book/image.jpg", Name: "image.jpg"},
				{Path: "book/mycover.jpg", Name: "mycover.jpg"},
			},
			wantPath: "book/mycover.jpg",
		},
		{
			name: "folder in name",
			imageFiles: []FileItem{
				{Path: "book/img.png", Name: "img.png"},
				{Path: "book/folder_art.png", Name: "folder_art.png"},
			},
			wantPath: "book/folder_art.png",
		},
		{
			name: "front in name",
			imageFiles: []FileItem{
				{Path: "book/front.webp", Name: "front.webp"},
				{Path: "book/back.webp", Name: "back.webp"},
			},
			wantPath: "book/front.webp",
		},
		{
			name: "default to first",
			imageFiles: []FileItem{
				{Path: "book/art.jpg", Name: "art.jpg"},
				{Path: "book/other.jpg", Name: "other.jpg"},
			},
			wantPath: "book/art.jpg",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := findBestCoverImage(tt.imageFiles)
			if got != tt.wantPath {
				t.Errorf("findBestCoverImage() = %q; want %q", got, tt.wantPath)
			}
		})
	}
}

func TestParseTxtFilesMetadata(t *testing.T) {
	tempDir := t.TempDir()
	descPath := filepath.Join(tempDir, "desc.txt")
	readerPath := filepath.Join(tempDir, "reader.txt")

	err := os.WriteFile(descPath, []byte("   This is a wonderful book description.   "), 0644)
	if err != nil {
		t.Fatalf("failed to write desc: %v", err)
	}
	err = os.WriteFile(readerPath, []byte("Narrator One, Narrator Two\nSome extra info"), 0644)
	if err != nil {
		t.Fatalf("failed to write reader: %v", err)
	}

	meta := &GroupMetadata{}
	parseTxtFilesMetadata(descPath, readerPath, meta, tempDir)

	if meta.Description != "This is a wonderful book description." {
		t.Errorf("expected description %q, got %q", "This is a wonderful book description.", meta.Description)
	}
	expectedNarrators := []string{"Narrator One", "Narrator Two"}
	if !reflect.DeepEqual(meta.Narrators, expectedNarrators) {
		t.Errorf("expected narrators %+v, got %+v", expectedNarrators, meta.Narrators)
	}

	// Test missing file handling: should not crash, should log and skip
	metaEmpty := &GroupMetadata{}
	parseTxtFilesMetadata("nonexistent_desc.txt", "nonexistent_reader.txt", metaEmpty, tempDir)
	if metaEmpty.Description != "" || len(metaEmpty.Narrators) != 0 {
		t.Errorf("expected empty metadata, got %+v", metaEmpty)
	}
}

func TestParseOPFMetadata(t *testing.T) {
	tempDir := t.TempDir()
	opfPath := filepath.Join(tempDir, "metadata.opf")

	opfContent := `<?xml version="1.0" encoding="utf-8"?>
<package xmlns="http://www.idpf.org/2007/opf" unique-identifier="uuid_id" version="2.0">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:opf="http://www.idpf.org/2007/opf">
    <dc:title>The Lord of the Rings</dc:title>
    <dc:creator opf:role="aut">J.R.R. Tolkien</dc:creator>
    <dc:creator opf:role="nrt">Rob Inglis</dc:creator>
    <dc:publisher>George Allen &amp; Unwin</dc:publisher>
    <dc:date>1954-07-29T00:00:00+00:00</dc:date>
    <dc:description>&lt;p&gt;A great &lt;b&gt;adventure&lt;/b&gt;&lt;/p&gt;</dc:description>
    <dc:identifier opf:scheme="ISBN">9780261103251</dc:identifier>
    <dc:identifier opf:scheme="ASIN">B002RI91BY</dc:identifier>
    <dc:language>eng</dc:language>
    <dc:subject>Fantasy</dc:subject>
    <dc:subject>Adventure</dc:subject>
    <meta name="calibre:series" content="LOTR" />
    <meta name="calibre:series_index" content="1.5" />
  </metadata>
</package>`

	err := os.WriteFile(opfPath, []byte(opfContent), 0644)
	if err != nil {
		t.Fatalf("failed to write opf: %v", err)
	}

	meta := &GroupMetadata{}
	parseOPFMetadata(opfPath, meta, tempDir)

	if meta.Title != "The Lord of the Rings" {
		t.Errorf("expected title 'The Lord of the Rings', got %q", meta.Title)
	}
	expectedAuthors := []string{"J.R.R. Tolkien", "Rob Inglis"}
	if !reflect.DeepEqual(meta.Authors, expectedAuthors) {
		t.Errorf("expected creators %+v, got %+v", expectedAuthors, meta.Authors)
	}
	if meta.Publisher != "George Allen & Unwin" {
		t.Errorf("expected publisher 'George Allen & Unwin', got %q", meta.Publisher)
	}
	if meta.PublishedYear != "1954" {
		t.Errorf("expected published year '1954', got %q", meta.PublishedYear)
	}
	if meta.PublishedDate != "1954-07-29T00:00:00+00:00" {
		t.Errorf("expected published date, got %q", meta.PublishedDate)
	}
	if meta.Description != "A great adventure" {
		t.Errorf("expected description 'A great adventure', got %q", meta.Description)
	}
	if meta.Language != "eng" {
		t.Errorf("expected language 'eng', got %q", meta.Language)
	}
	expectedGenres := []string{"Fantasy", "Adventure"}
	if !reflect.DeepEqual(meta.Genres, expectedGenres) {
		t.Errorf("expected genres %+v, got %+v", expectedGenres, meta.Genres)
	}
	if meta.SeriesName != "LOTR" {
		t.Errorf("expected series LOTR, got %q", meta.SeriesName)
	}
	if meta.SeriesSequence != "1.5" {
		t.Errorf("expected sequence 1.5, got %q", meta.SeriesSequence)
	}
	if meta.ISBN != "9780261103251" {
		t.Errorf("expected ISBN, got %q", meta.ISBN)
	}
	if meta.ASIN != "B002RI91BY" {
		t.Errorf("expected ASIN, got %q", meta.ASIN)
	}

	// Test missing file handling
	metaEmpty := &GroupMetadata{}
	parseOPFMetadata("nonexistent.opf", metaEmpty, tempDir)
	if metaEmpty.Title != "" {
		t.Errorf("expected empty metadata on missing file, got %+v", metaEmpty)
	}
}

func TestParseNFOMetadata(t *testing.T) {
	tempDir := t.TempDir()
	nfoPath := filepath.Join(tempDir, "metadata.nfo")

	nfoContent := `Title: The Hobbit: Or There and Back Again
Author: J.R.R. Tolkien, John Doe
Narrator: Rob Inglis
Series Name: Middle-earth
Position in Series: 0
Genre: Fantasy, Classic
Tags: adventure, magic
Date: (1937)
Publisher: George Allen
ASIN: B007978NU6
ISBN: 9780261103343
Language: English

Book Description
================
In a hole in the ground there lived a hobbit.
Not a nasty, dirty, wet hole...
`

	err := os.WriteFile(nfoPath, []byte(nfoContent), 0644)
	if err != nil {
		t.Fatalf("failed to write nfo: %v", err)
	}

	meta := &GroupMetadata{}
	parseNFOMetadata(nfoPath, meta, true, tempDir)

	if meta.Title != "The Hobbit" {
		t.Errorf("expected title 'The Hobbit', got %q", meta.Title)
	}
	if meta.Subtitle != "Or There and Back Again" {
		t.Errorf("expected subtitle 'Or There and Back Again', got %q", meta.Subtitle)
	}
	expectedAuthors := []string{"J.R.R. Tolkien", "John Doe"}
	if !reflect.DeepEqual(meta.Authors, expectedAuthors) {
		t.Errorf("expected authors %+v, got %+v", expectedAuthors, meta.Authors)
	}
	expectedNarrators := []string{"Rob Inglis"}
	if !reflect.DeepEqual(meta.Narrators, expectedNarrators) {
		t.Errorf("expected narrators %+v, got %+v", expectedNarrators, meta.Narrators)
	}
	if meta.SeriesName != "Middle-earth" {
		t.Errorf("expected series 'Middle-earth', got %q", meta.SeriesName)
	}
	if meta.SeriesSequence != "0" {
		t.Errorf("expected series sequence '0', got %q", meta.SeriesSequence)
	}
	expectedGenres := []string{"Fantasy", "Classic"}
	if !reflect.DeepEqual(meta.Genres, expectedGenres) {
		t.Errorf("expected genres %+v, got %+v", expectedGenres, meta.Genres)
	}
	expectedTags := []string{"adventure", "magic"}
	if !reflect.DeepEqual(meta.Tags, expectedTags) {
		t.Errorf("expected tags %+v, got %+v", expectedTags, meta.Tags)
	}
	if meta.PublishedYear != "1937" {
		t.Errorf("expected published year '1937', got %q", meta.PublishedYear)
	}
	if meta.Publisher != "George Allen" {
		t.Errorf("expected publisher, got %q", meta.Publisher)
	}
	if meta.ASIN != "B007978NU6" {
		t.Errorf("expected ASIN, got %q", meta.ASIN)
	}
	if meta.ISBN != "9780261103343" {
		t.Errorf("expected ISBN, got %q", meta.ISBN)
	}
	if meta.Language != "English" {
		t.Errorf("expected language, got %q", meta.Language)
	}
	expectedDesc := "In a hole in the ground there lived a hobbit.\nNot a nasty, dirty, wet hole..."
	if meta.Description != expectedDesc {
		t.Errorf("expected description %q, got %q", expectedDesc, meta.Description)
	}

	// Test missing file
	metaEmpty := &GroupMetadata{}
	parseNFOMetadata("nonexistent.nfo", metaEmpty, true, tempDir)
	if metaEmpty.Title != "" {
		t.Errorf("expected empty metadata on missing NFO, got %+v", metaEmpty)
	}
}

func TestAdversarialFilenameParsing(t *testing.T) {
	tests := []struct {
		relPath       string
		wantTitle     string
		wantSubtitle  string
		wantASIN      string
		wantAuthors   []string
		wantNarrators []string
		wantSeries    string
		wantSequence  string
		wantYear      string
	}{
		{
			relPath:       "J.K. Rowling/Harry Potter/01 - Harry Potter and the Sorcerer's Stone {Stephen Fry}",
			wantTitle:     "Harry Potter and the Sorcerer's Stone",
			wantSubtitle:  "",
			wantASIN:      "",
			wantAuthors:   nil,
			wantNarrators: []string{"Stephen Fry"},
			wantSeries:    "Harry Potter",
			wantSequence:  "01",
			wantYear:      "",
		},
		{
			relPath:       "Stephen King/The Shining [B001111111]",
			wantTitle:     "The Shining",
			wantSubtitle:  "",
			wantASIN:      "B001111111",
			wantAuthors:   []string{"Stephen King"},
			wantNarrators: nil,
			wantSeries:    "",
			wantSequence:  "",
			wantYear:      "",
		},
		{
			relPath:       "Brandon Sanderson/Mistborn/(2006) - Mistborn - The Final Empire - Subtitle",
			wantTitle:     "Mistborn",
			wantSubtitle:  "The Final Empire - Subtitle",
			wantASIN:      "",
			wantAuthors:   nil,
			wantNarrators: nil,
			wantSeries:    "Mistborn",
			wantSequence:  "",
			wantYear:      "2006",
		},
		{
			relPath:       "OnlyTitle",
			wantTitle:     "OnlyTitle",
			wantSubtitle:  "",
			wantASIN:      "",
			wantAuthors:   nil,
			wantNarrators: nil,
			wantSeries:    "",
			wantSequence:  "",
			wantYear:      "",
		},
		{
			relPath:       "Author & Other/Series/vol 2.5 - My Title {Read by Joe and Jill}",
			wantTitle:     "My Title",
			wantSubtitle:  "",
			wantASIN:      "",
			wantAuthors:   nil,
			wantNarrators: []string{"Read by Joe", "Jill"},
			wantSeries:    "Series",
			wantSequence:  "2.5",
			wantYear:      "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.relPath, func(t *testing.T) {
			meta := GetBookDataFromDir(tt.relPath)
			if meta.Title != tt.wantTitle {
				t.Errorf("Title = %q; want %q", meta.Title, tt.wantTitle)
			}
			if meta.Subtitle != tt.wantSubtitle {
				t.Errorf("Subtitle = %q; want %q", meta.Subtitle, tt.wantSubtitle)
			}
			if meta.ASIN != tt.wantASIN {
				t.Errorf("ASIN = %q; want %q", meta.ASIN, tt.wantASIN)
			}
			if !reflect.DeepEqual(meta.Authors, tt.wantAuthors) {
				t.Errorf("Authors = %+v; want %+v", meta.Authors, tt.wantAuthors)
			}
			if !reflect.DeepEqual(meta.Narrators, tt.wantNarrators) {
				t.Errorf("Narrators = %+v; want %+v", meta.Narrators, tt.wantNarrators)
			}
			if meta.SeriesName != tt.wantSeries {
				t.Errorf("SeriesName = %q; want %q", meta.SeriesName, tt.wantSeries)
			}
			if meta.SeriesSequence != tt.wantSequence {
				t.Errorf("SeriesSequence = %q; want %q", meta.SeriesSequence, tt.wantSequence)
			}
			if meta.PublishedYear != tt.wantYear {
				t.Errorf("PublishedYear = %q; want %q", meta.PublishedYear, tt.wantYear)
			}
		})
	}
}

func TestParseAudioFilesEdgeCases(t *testing.T) {
	files := []FileItem{
		{
			Path:      "nonexistent_audio.mp3",
			RelPath:   "nonexistent_audio.mp3",
			Name:      "nonexistent_audio.mp3",
			Extension: ".mp3",
			Size:      1000,
			Ino:       "9999",
		},
	}

	parsed := parseAudioFiles(files, "some/item/path")
	if len(parsed) != 1 {
		t.Fatalf("expected 1 parsed file, got %d", len(parsed))
	}

	pf := parsed[0]
	if pf.duration != 0 {
		t.Errorf("expected 0 duration for nonexistent file, got %f", pf.duration)
	}
	if pf.tagTitle != "" {
		t.Errorf("expected empty tag title, got %q", pf.tagTitle)
	}

	afObj := pf.afObj

	if afObj["duration"] != 0.0 {
		t.Errorf("expected 0.0 duration in afObj, got %v", afObj["duration"])
	}
	if afObj["channels"] != 0 {
		t.Errorf("expected 0 channels, got %v", afObj["channels"])
	}
}

func TestParseEbookMetadataEdgeCases(t *testing.T) {
	tempDir := t.TempDir()
	meta := &GroupMetadata{}
	ebookFiles := []FileItem{
		{
			Path:      filepath.Join(tempDir, "nonexistent.epub"),
			RelPath:   "nonexistent.epub",
			Name:      "nonexistent.epub",
			Extension: ".epub",
			Size:      500,
		},
	}

	// Should not panic, should handle missing file gracefully
	parseEbookMetadata(nil, "item-123", ebookFiles, meta, true, tempDir)
	if meta.Title != "" {
		t.Errorf("expected empty title for nonexistent ebook, got %q", meta.Title)
	}
}

func TestAdversarialMetadataParsing(t *testing.T) {
	tempDir := t.TempDir()

	// 1. Malformed OPF XML
	malformedOPFPath := filepath.Join(tempDir, "malformed.opf")
	err := os.WriteFile(malformedOPFPath, []byte("<package><metadata><dc:title>Unclosed Tag"), 0644)
	if err != nil {
		t.Fatalf("failed to write malformed opf: %v", err)
	}
	metaOPF := &GroupMetadata{}
	// This should log an error and handle it gracefully without crashing
	parseOPFMetadata(malformedOPFPath, metaOPF, tempDir)
	if metaOPF.Title != "" {
		t.Errorf("expected empty Title for malformed OPF, got %q", metaOPF.Title)
	}

	// 2. Malformed NFO file
	malformedNFOPath := filepath.Join(tempDir, "malformed.nfo")
	nfoContent := `:::invalid line
Title: Title with : in value
Author: 
Narrator: John Doe: Special Narrator
date: invalid-year-1234-and-5678
Description: Description starts without Book Description header
`
	err = os.WriteFile(malformedNFOPath, []byte(nfoContent), 0644)
	if err != nil {
		t.Fatalf("failed to write malformed nfo: %v", err)
	}
	metaNFO := &GroupMetadata{}
	parseNFOMetadata(malformedNFOPath, metaNFO, true, tempDir)
	if metaNFO.Title != "Title with" { // strings.Index(value, ": ") finds the colon inside the value
		t.Errorf("expected Title 'Title with', got %q", metaNFO.Title)
	}
	if metaNFO.Subtitle != "in value" {
		t.Errorf("expected Subtitle 'in value', got %q", metaNFO.Subtitle)
	}
	if metaNFO.PublishedYear != "5678" { // Regex finds the last 4-digit number
		t.Errorf("expected PublishedYear '5678', got %q", metaNFO.PublishedYear)
	}

	// 3. Pathological filenames
	pathologicalPaths := []struct {
		path      string
		wantTitle string
	}{
		{"", ""},
		{"/", ""},
		{"///", ""},
		{"   ", "   "},
		{"📁/🌟", "🌟"},
	}
	for _, tc := range pathologicalPaths {
		fnMeta := GetBookDataFromDir(tc.path)
		if fnMeta.Title != tc.wantTitle {
			t.Errorf("for path %q, got title %q, want %q", tc.path, fnMeta.Title, tc.wantTitle)
		}
	}
}
