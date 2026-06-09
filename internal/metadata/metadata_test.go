package metadata

import (
	"archive/zip"
	"bytes"
	"context"
	"hash/crc32"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

// Helper to construct a ZIP archive in memory
func buildZip(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("failed to create zip entry: %v", err)
		}
		_, err = w.Write(content)
		if err != nil {
			t.Fatalf("failed to write zip entry content: %v", err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("failed to close zip writer: %v", err)
	}
	return buf.Bytes()
}

// Helper to construct a RAR 4.0 archive block
func buildRarBlock(htype byte, flags uint16, extraData []byte) []byte {
	headerSize := uint16(7 + len(extraData))
	rest := make([]byte, 5+len(extraData))
	rest[0] = htype
	rest[1] = byte(flags)
	rest[2] = byte(flags >> 8)
	rest[3] = byte(headerSize)
	rest[4] = byte(headerSize >> 8)
	copy(rest[5:], extraData)

	h := crc32.NewIEEE()
	h.Write(rest)
	sum := uint16(h.Sum32())

	res := make([]byte, 2+len(rest))
	res[0] = byte(sum)
	res[1] = byte(sum >> 8)
	copy(res[2:], rest)
	return res
}

// Helper to construct a RAR 4.0 file block (Method 0x30 = Store)
func buildRarFileBlock(name string, content []byte) []byte {
	fileCRC := crc32.ChecksumIEEE(content)

	var extra bytes.Buffer
	packedSize := uint32(len(content))
	extra.WriteByte(byte(packedSize))
	extra.WriteByte(byte(packedSize >> 8))
	extra.WriteByte(byte(packedSize >> 16))
	extra.WriteByte(byte(packedSize >> 24))

	unpackedSize := uint32(len(content))
	extra.WriteByte(byte(unpackedSize))
	extra.WriteByte(byte(unpackedSize >> 8))
	extra.WriteByte(byte(unpackedSize >> 16))
	extra.WriteByte(byte(unpackedSize >> 24))

	extra.WriteByte(0) // HostOS = MS-DOS

	extra.WriteByte(byte(fileCRC))
	extra.WriteByte(byte(fileCRC >> 8))
	extra.WriteByte(byte(fileCRC >> 16))
	extra.WriteByte(byte(fileCRC >> 24))

	// FileTime (4 bytes dummy)
	extra.WriteByte(0x00)
	extra.WriteByte(0x00)
	extra.WriteByte(0x00)
	extra.WriteByte(0x00)

	extra.WriteByte(29)   // UnpackVer (2.9)
	extra.WriteByte(0x30) // Method = Store

	nameLen := uint16(len(name))
	extra.WriteByte(byte(nameLen))
	extra.WriteByte(byte(nameLen >> 8))

	// Attributes (4 bytes dummy)
	extra.WriteByte(0)
	extra.WriteByte(0)
	extra.WriteByte(0)
	extra.WriteByte(0)

	extra.WriteString(name)

	blockHeader := buildRarBlock(0x74, 0x8000, extra.Bytes())
	return append(blockHeader, content...)
}

// Helper to construct a full RAR 4.0 archive in memory
func buildRar(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	// 1. Signature
	buf.Write([]byte{0x52, 0x61, 0x72, 0x21, 0x1A, 0x07, 0x00})
	// 2. Archive header block
	buf.Write(buildRarBlock(0x73, 0x0000, nil))
	// 3. File blocks
	for name, content := range files {
		buf.Write(buildRarFileBlock(name, content))
	}
	// 4. End block
	buf.Write(buildRarBlock(0x7b, 0x0000, nil))
	return buf.Bytes()
}

func TestStripAllTags(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Hello World", "Hello World"},
		{"<p>Hello <b>World</b></p>", "Hello World"},
		{"&lt;div&gt;Hello &amp; welcome&lt;/div&gt;", "Hello & welcome"},
		{"<a href=\"http://example.com\">Test Link</a>", "Test Link"},
		{"   Multiple    Spaces   ", "Multiple    Spaces"},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got := stripAllTags(tc.input)
			if got != tc.expected {
				t.Errorf("stripAllTags(%q) = %q; expected %q", tc.input, got, tc.expected)
			}
		})
	}
}

func TestNaturalLess(t *testing.T) {
	tests := []struct {
		a        string
		b        string
		expected bool
	}{
		{"file2.txt", "file10.txt", true},
		{"file10.txt", "file2.txt", false},
		{"file1.txt", "file1.txt", false},
		{"2.jpg", "10.jpg", true},
		{"10.jpg", "2.jpg", false},
		{"100.png", "20.png", false},
		{"abc", "xyz", true},
		{"xyz", "abc", false},
		{"file02.txt", "file2.txt", false},
		{"file2.txt", "file02.txt", true},
	}

	for _, tc := range tests {
		t.Run(tc.a+"_vs_"+tc.b, func(t *testing.T) {
			got := naturalLess(tc.a, tc.b)
			if got != tc.expected {
				t.Errorf("naturalLess(%q, %q) = %t; expected %t", tc.a, tc.b, got, tc.expected)
			}
		})
	}

	// Test list sorting
	files := []string{"10.jpg", "2.jpg", "100.jpg", "1.jpg", "20.jpg"}
	expected := []string{"1.jpg", "2.jpg", "10.jpg", "20.jpg", "100.jpg"}
	sort.Slice(files, func(i, j int) bool {
		return naturalLess(files[i], files[j])
	})
	if !reflect.DeepEqual(files, expected) {
		t.Errorf("sorted files = %v; expected %v", files, expected)
	}
}

func TestExtractEpubMetadata_NCX(t *testing.T) {
	containerXML := `<?xml version="1.0"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>`

	opfXML := `<?xml version="1.0" encoding="utf-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="2.0">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:title>Test EPUB Title</dc:title>
    <dc:creator id="creator0" role="aut">Author Name</dc:creator>
    <dc:publisher>Publisher Name</dc:publisher>
    <dc:description>&lt;p&gt;Test &lt;b&gt;Description&lt;/b&gt;&lt;/p&gt;</dc:description>
    <dc:language>en</dc:language>
    <dc:date>2023-04-12T00:00:00Z</dc:date>
    <dc:identifier scheme="ISBN">9781234567890</dc:identifier>
  </metadata>
  <manifest>
    <item id="ncx-toc" href="toc.ncx" media-type="application/x-dtbncx+xml"/>
  </manifest>
  <spine toc="ncx-toc"></spine>
</package>`

	ncxXML := `<?xml version="1.0" encoding="utf-8"?>
<ncx xmlns="http://www.daisy.org/z3986/2005/ncx/" version="2005-1">
  <navMap>
    <navPoint>
      <navLabel><text>Chapter 1</text></navLabel>
    </navPoint>
    <navPoint>
      <navLabel><text>Chapter 2</text></navLabel>
    </navPoint>
  </navMap>
</ncx>`

	zipBytes := buildZip(t, map[string][]byte{
		"META-INF/container.xml": []byte(containerXML),
		"OEBPS/content.opf":      []byte(opfXML),
		"OEBPS/toc.ncx":          []byte(ncxXML),
	})

	tmpDir, err := os.MkdirTemp("", "epub-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	epubPath := filepath.Join(tmpDir, "test.epub")
	if err := os.WriteFile(epubPath, zipBytes, 0644); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	meta, err := ExtractEpubMetadata(ctx, epubPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedMeta := &EbookMetadata{
		Title:         "Test EPUB Title",
		Author:        "Author Name",
		Publisher:     "Publisher Name",
		PublishedYear: "2023",
		Description:   "Test Description",
		Language:      "en",
		ISBN:          "9781234567890",
		Chapters: []Chapter{
			{ID: 1, Title: "Chapter 1"},
			{ID: 2, Title: "Chapter 2"},
		},
	}

	if !reflect.DeepEqual(meta, expectedMeta) {
		t.Errorf("ExtractEpubMetadata got %+v; expected %+v", meta, expectedMeta)
	}
}

func TestExtractEpubMetadata_Nav(t *testing.T) {
	containerXML := `<?xml version="1.0"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="content.opf" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>`

	opfXML := `<?xml version="1.0" encoding="utf-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:title>EPUB 3 Title</dc:title>
  </metadata>
  <manifest>
    <item id="nav-toc" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>
  </manifest>
  <spine></spine>
</package>`

	navXML := `<?xml version="1.0" encoding="utf-8"?>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops">
  <body>
    <nav epub:type="toc">
      <ol>
        <li><a href="chap1.xhtml">Chapter A</a></li>
        <li><a href="chap2.xhtml">Chapter B</a></li>
      </ol>
    </nav>
  </body>
</html>`

	zipBytes := buildZip(t, map[string][]byte{
		"META-INF/container.xml": []byte(containerXML),
		"content.opf":            []byte(opfXML),
		"nav.xhtml":              []byte(navXML),
	})

	tmpDir, err := os.MkdirTemp("", "epub-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	epubPath := filepath.Join(tmpDir, "test.epub")
	if err := os.WriteFile(epubPath, zipBytes, 0644); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	meta, err := ExtractEpubMetadata(ctx, epubPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if meta.Title != "EPUB 3 Title" {
		t.Errorf("expected Title 'EPUB 3 Title', got %q", meta.Title)
	}

	expectedChapters := []Chapter{
		{ID: 1, Title: "Chapter A"},
		{ID: 2, Title: "Chapter B"},
	}

	if !reflect.DeepEqual(meta.Chapters, expectedChapters) {
		t.Errorf("expected chapters %+v; got %+v", expectedChapters, meta.Chapters)
	}
}

func TestExtractEpubMetadata_FallbackTitle(t *testing.T) {
	// EPUB with minimal content and no title in metadata
	containerXML := `<?xml version="1.0"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="content.opf" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>`

	opfXML := `<?xml version="1.0" encoding="utf-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="2.0">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/"></metadata>
  <manifest></manifest>
  <spine></spine>
</package>`

	zipBytes := buildZip(t, map[string][]byte{
		"META-INF/container.xml": []byte(containerXML),
		"content.opf":            []byte(opfXML),
	})

	tmpDir, err := os.MkdirTemp("", "epub-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	epubPath := filepath.Join(tmpDir, "My_Custom_Book_Name.epub")
	if err := os.WriteFile(epubPath, zipBytes, 0644); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	meta, err := ExtractEpubMetadata(ctx, epubPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if meta.Title != "My_Custom_Book_Name" {
		t.Errorf("expected Title to fallback to file name 'My_Custom_Book_Name', got %q", meta.Title)
	}
}

func TestExtractEpubCover(t *testing.T) {
	containerXML := `<?xml version="1.0"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="content.opf" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>`

	// Case 1: Cover via <meta name="cover">
	opfXML1 := `<?xml version="1.0" encoding="utf-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="2.0">
  <metadata>
    <meta name="cover" content="cov-id"/>
  </metadata>
  <manifest>
    <item id="cov-id" href="assets/cover_image.jpg" media-type="image/jpeg"/>
  </manifest>
  <spine></spine>
</package>`

	// Case 2: Cover via properties="cover-image"
	opfXML2 := `<?xml version="1.0" encoding="utf-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0">
  <metadata></metadata>
  <manifest>
    <item id="cover-id" href="cover.png" media-type="image/png" properties="cover-image"/>
  </manifest>
  <spine></spine>
</package>`

	// Case 3: Fallback cover by filename
	opfXML3 := `<?xml version="1.0" encoding="utf-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="2.0">
  <metadata></metadata>
  <manifest></manifest>
  <spine></spine>
</package>`

	tests := []struct {
		name     string
		opf      string
		extra    map[string][]byte
		expected []byte
	}{
		{
			name: "Cover via metadata",
			opf:  opfXML1,
			extra: map[string][]byte{
				"assets/cover_image.jpg": []byte("image_data_1"),
			},
			expected: []byte("image_data_1"),
		},
		{
			name: "Cover via manifest property",
			opf:  opfXML2,
			extra: map[string][]byte{
				"cover.png": []byte("image_data_2"),
			},
			expected: []byte("image_data_2"),
		},
		{
			name: "Cover via fallback filename",
			opf:  opfXML3,
			extra: map[string][]byte{
				"cover.jpg": []byte("image_data_fallback"),
			},
			expected: []byte("image_data_fallback"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			files := map[string][]byte{
				"META-INF/container.xml": []byte(containerXML),
				"content.opf":            []byte(tc.opf),
			}
			for k, v := range tc.extra {
				files[k] = v
			}

			zipBytes := buildZip(t, files)

			tmpDir, err := os.MkdirTemp("", "epub-cover-test-*")
			if err != nil {
				t.Fatal(err)
			}
			defer os.RemoveAll(tmpDir)

			epubPath := filepath.Join(tmpDir, "test.epub")
			if err := os.WriteFile(epubPath, zipBytes, 0644); err != nil {
				t.Fatal(err)
			}

			destPath := filepath.Join(tmpDir, "cover_dest.jpg")
			ctx := context.Background()
			err = ExtractEpubCover(ctx, epubPath, destPath)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			gotData, err := os.ReadFile(destPath)
			if err != nil {
				t.Fatalf("failed to read dest cover file: %v", err)
			}

			if !bytes.Equal(gotData, tc.expected) {
				t.Errorf("extracted cover data = %q; expected %q", string(gotData), string(tc.expected))
			}
		})
	}
}

func TestExtractComicMetadata_CBZ(t *testing.T) {
	comicInfoXML := `<?xml version="1.0" encoding="utf-8"?>
<ComicInfo xmlns:xsd="http://www.w3.org/2001/XMLSchema" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <Title>Comic Title</Title>
  <Series>Comic Series</Series>
  <Number>Issue 5</Number>
  <Writer>Comic Writer</Writer>
  <Publisher>Comic Publisher</Publisher>
  <Year>2024</Year>
  <Month>3</Month>
  <Day>15</Day>
  <Summary>Comic Summary Description</Summary>
  <LanguageISO>en</LanguageISO>
  <ISBN>9780000000000</ISBN>
</ComicInfo>`

	zipBytes := buildZip(t, map[string][]byte{
		"ComicInfo.xml": []byte(comicInfoXML),
		"001.jpg":       []byte("fake_image_data"),
	})

	tmpDir, err := os.MkdirTemp("", "cbz-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	cbzPath := filepath.Join(tmpDir, "my_comic.cbz")
	if err := os.WriteFile(cbzPath, zipBytes, 0644); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	meta, err := ExtractComicMetadata(ctx, cbzPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedMeta := &EbookMetadata{
		Title:         "Comic Series Issue 5",
		Author:        "Comic Writer",
		Publisher:     "Comic Publisher",
		PublishedYear: "2024",
		Description:   "Comic Summary Description",
		Language:      "en",
		ISBN:          "9780000000000",
	}

	if !reflect.DeepEqual(meta, expectedMeta) {
		t.Errorf("ExtractComicMetadata(CBZ) got %+v; expected %+v", meta, expectedMeta)
	}
}

func TestExtractComicMetadata_CBR(t *testing.T) {
	comicInfoXML := `<?xml version="1.0" encoding="utf-8"?>
<ComicInfo xmlns:xsd="http://www.w3.org/2001/XMLSchema" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <Title>CBR Title Only</Title>
  <Writer>CBR Writer</Writer>
  <Publisher>CBR Publisher</Publisher>
  <Year>2022</Year>
  <Summary>CBR Summary</Summary>
  <LanguageISO>fr</LanguageISO>
</ComicInfo>`

	rarBytes := buildRar(t, map[string][]byte{
		"ComicInfo.xml": []byte(comicInfoXML),
		"001.jpg":       []byte("cbr_first_image_bytes"),
	})

	tmpDir, err := os.MkdirTemp("", "cbr-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	cbrPath := filepath.Join(tmpDir, "my_comic.cbr")
	if err := os.WriteFile(cbrPath, rarBytes, 0644); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	meta, err := ExtractComicMetadata(ctx, cbrPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedMeta := &EbookMetadata{
		Title:         "CBR Title Only",
		Author:        "CBR Writer",
		Publisher:     "CBR Publisher",
		PublishedYear: "2022",
		Description:   "CBR Summary",
		Language:      "fr",
	}

	if !reflect.DeepEqual(meta, expectedMeta) {
		t.Errorf("ExtractComicMetadata(CBR) got %+v; expected %+v", meta, expectedMeta)
	}
}

func TestExtractComicMetadata_PDF(t *testing.T) {
	// Create mock pdfinfo executable
	tmpBinDir, err := os.MkdirTemp("", "mock-bin-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpBinDir)

	pdfinfoPath := filepath.Join(tmpBinDir, "pdfinfo")
	pdfinfoContent := `#!/bin/bash
echo "Title:           Mocked PDF Title"
echo "Author:          Mocked PDF Author"
echo "Publisher:       Mocked PDF Publisher"
echo "CreationDate:    D:20260609120000"
`
	if err := os.WriteFile(pdfinfoPath, []byte(pdfinfoContent), 0755); err != nil {
		t.Fatal(err)
	}

	// Prepend to PATH
	oldPath := os.Getenv("PATH")
	defer os.Setenv("PATH", oldPath)
	os.Setenv("PATH", tmpBinDir+string(filepath.ListSeparator)+oldPath)

	ctx := context.Background()
	meta, err := ExtractComicMetadata(ctx, "test_comic_book.pdf")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedMeta := &EbookMetadata{
		Title:         "Mocked PDF Title",
		Author:        "Mocked PDF Author",
		Publisher:     "Mocked PDF Publisher",
		PublishedYear: "2026",
	}

	if !reflect.DeepEqual(meta, expectedMeta) {
		t.Errorf("ExtractComicMetadata(PDF) got %+v; expected %+v", meta, expectedMeta)
	}
}

func TestExtractComicMetadata_PDF_Failure(t *testing.T) {
	// Create failing mock pdfinfo
	tmpBinDir, err := os.MkdirTemp("", "mock-bin-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpBinDir)

	pdfinfoPath := filepath.Join(tmpBinDir, "pdfinfo")
	pdfinfoContent := `#!/bin/bash
exit 1
`
	if err := os.WriteFile(pdfinfoPath, []byte(pdfinfoContent), 0755); err != nil {
		t.Fatal(err)
	}

	// Prepend to PATH
	oldPath := os.Getenv("PATH")
	defer os.Setenv("PATH", oldPath)
	os.Setenv("PATH", tmpBinDir+string(filepath.ListSeparator)+oldPath)

	ctx := context.Background()
	meta, err := ExtractComicMetadata(ctx, "another_book_name.pdf")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if meta.Title != "another_book_name" {
		t.Errorf("expected Title to fallback to file name 'another_book_name', got %q", meta.Title)
	}
}

func TestExtractComicCover_CBZ(t *testing.T) {
	zipBytes := buildZip(t, map[string][]byte{
		"002.jpg": []byte("image_2"),
		"010.jpg": []byte("image_10"),
		"001.jpg": []byte("image_1_first"),
	})

	tmpDir, err := os.MkdirTemp("", "cbz-cover-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	cbzPath := filepath.Join(tmpDir, "comic.cbz")
	if err := os.WriteFile(cbzPath, zipBytes, 0644); err != nil {
		t.Fatal(err)
	}

	destPath := filepath.Join(tmpDir, "cover.jpg")
	ctx := context.Background()
	err = ExtractComicCover(ctx, cbzPath, destPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	gotBytes, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("failed to read destination cover file: %v", err)
	}

	if string(gotBytes) != "image_1_first" {
		t.Errorf("expected extracted cover content 'image_1_first', got %q", string(gotBytes))
	}
}

func TestExtractComicCover_CBR(t *testing.T) {
	rarBytes := buildRar(t, map[string][]byte{
		"002.jpg": []byte("image_2"),
		"010.jpg": []byte("image_10"),
		"001.jpg": []byte("image_1_first"),
	})

	tmpDir, err := os.MkdirTemp("", "cbr-cover-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	cbrPath := filepath.Join(tmpDir, "comic.cbr")
	if err := os.WriteFile(cbrPath, rarBytes, 0644); err != nil {
		t.Fatal(err)
	}

	destPath := filepath.Join(tmpDir, "cover.jpg")
	ctx := context.Background()
	err = ExtractComicCover(ctx, cbrPath, destPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	gotBytes, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("failed to read destination cover file: %v", err)
	}

	if string(gotBytes) != "image_1_first" {
		t.Errorf("expected extracted cover content 'image_1_first', got %q", string(gotBytes))
	}
}

func TestExtractComicCover_PDF(t *testing.T) {
	tmpBinDir, err := os.MkdirTemp("", "mock-bin-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpBinDir)

	pdftoppmPath := filepath.Join(tmpBinDir, "pdftoppm")
	pdftoppmContent := `#!/bin/bash
# Pre-rendered first page written to outputPrefix.jpg
echo "mocked_pdf_cover_image_bytes" > "${@: -1}.jpg"
`
	if err := os.WriteFile(pdftoppmPath, []byte(pdftoppmContent), 0755); err != nil {
		t.Fatal(err)
	}

	// Prepend to PATH
	oldPath := os.Getenv("PATH")
	defer os.Setenv("PATH", oldPath)
	os.Setenv("PATH", tmpBinDir+string(filepath.ListSeparator)+oldPath)

	tmpDir, err := os.MkdirTemp("", "pdf-cover-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	destPath := filepath.Join(tmpDir, "extracted_pdf_cover.jpg")
	ctx := context.Background()
	err = ExtractComicCover(ctx, "comic.pdf", destPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	gotBytes, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("failed to read destination cover: %v", err)
	}

	if string(gotBytes) != "mocked_pdf_cover_image_bytes\n" {
		t.Errorf("expected cover content %q, got %q", "mocked_pdf_cover_image_bytes\n", string(gotBytes))
	}
}

func TestExtractContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := ExtractEpubMetadata(ctx, "test.epub")
	if err != context.Canceled {
		t.Errorf("expected context.Canceled error, got: %v", err)
	}

	err = ExtractEpubCover(ctx, "test.epub", "dest.jpg")
	if err != context.Canceled {
		t.Errorf("expected context.Canceled error, got: %v", err)
	}

	_, err = ExtractComicMetadata(ctx, "test.cbz")
	if err != context.Canceled {
		t.Errorf("expected context.Canceled error, got: %v", err)
	}

	err = ExtractComicCover(ctx, "test.cbz", "dest.jpg")
	if err != context.Canceled {
		t.Errorf("expected context.Canceled error, got: %v", err)
	}
}
