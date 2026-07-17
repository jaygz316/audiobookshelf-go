package metadata

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// TestExtractEpubMetadata_Concurrency runs ExtractEpubMetadata in parallel.
func TestExtractEpubMetadata_Concurrency(t *testing.T) {
	// Create a dummy EPUB file.
	containerXML := `<?xml version="1.0"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>`

	opfXML := `<?xml version="1.0" encoding="utf-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="2.0">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:title>Concurrent EPUB Title</dc:title>
    <dc:creator id="creator0" role="aut">Author Name</dc:creator>
    <dc:publisher>Publisher Name</dc:publisher>
    <dc:date>2026-07-16</dc:date>
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
  </navMap>
</ncx>`

	zipBytes := buildZip(t, map[string][]byte{
		"META-INF/container.xml": []byte(containerXML),
		"OEBPS/content.opf":      []byte(opfXML),
		"OEBPS/toc.ncx":          []byte(ncxXML),
	})

	tmpDir, err := os.MkdirTemp("", "epub-concurrency-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	epubPath := filepath.Join(tmpDir, "test.epub")
	if err := os.WriteFile(epubPath, zipBytes, 0644); err != nil {
		t.Fatal(err)
	}

	const numGoroutines = 20
	const numIterations = 10
	var wg sync.WaitGroup

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			// Introduce slight jitter
			time.Sleep(time.Duration(rand.Intn(10)) * time.Millisecond)

			for j := 0; j < numIterations; j++ {
				ctx := context.Background()
				meta, err := ExtractEpubMetadata(ctx, epubPath)
				if err != nil {
					t.Errorf("Goroutine %d iteration %d: unexpected error: %v", id, j, err)
					return
				}
				if meta.Title != "Concurrent EPUB Title" {
					t.Errorf("Goroutine %d iteration %d: expected Title 'Concurrent EPUB Title', got %q", id, j, meta.Title)
					return
				}
			}
		}(i)
	}

	wg.Wait()
}

// TestExtractEpubCover_Concurrency runs ExtractEpubCover in parallel.
func TestExtractEpubCover_Concurrency(t *testing.T) {
	containerXML := `<?xml version="1.0"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="content.opf" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>`

	opfXML := `<?xml version="1.0" encoding="utf-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0">
  <metadata></metadata>
  <manifest>
    <item id="cover-id" href="cover.png" media-type="image/png" properties="cover-image"/>
  </manifest>
  <spine></spine>
</package>`

	zipBytes := buildZip(t, map[string][]byte{
		"META-INF/container.xml": []byte(containerXML),
		"content.opf":            []byte(opfXML),
		"cover.png":              []byte("dummy_png_bytes_for_concurrency"),
	})

	tmpDir, err := os.MkdirTemp("", "epub-cover-concurrency-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	epubPath := filepath.Join(tmpDir, "test.epub")
	if err := os.WriteFile(epubPath, zipBytes, 0644); err != nil {
		t.Fatal(err)
	}

	const numGoroutines = 20
	var wg sync.WaitGroup

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			destPath := filepath.Join(tmpDir, fmt.Sprintf("cover_dest_%d.jpg", id))

			ctx := context.Background()
			err := ExtractEpubCover(ctx, epubPath, destPath)
			if err != nil {
				t.Errorf("Goroutine %d: unexpected error: %v", id, err)
				return
			}

			data, err := os.ReadFile(destPath)
			if err != nil {
				t.Errorf("Goroutine %d: failed to read destination cover file: %v", id, err)
				return
			}

			if string(data) != "dummy_png_bytes_for_concurrency" {
				t.Errorf("Goroutine %d: expected cover content 'dummy_png_bytes_for_concurrency', got %q", id, string(data))
			}
		}(i)
	}

	wg.Wait()
}

// TestExtractEpub_InvalidFiles checks how the functions handle invalid/malformed zip/epub files.
func TestExtractEpub_InvalidFiles(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "epub-invalid-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Case 1: Non-existent file
	nonExistentPath := filepath.Join(tmpDir, "non_existent.epub")
	ctx := context.Background()
	_, err = ExtractEpubMetadata(ctx, nonExistentPath)
	if err == nil {
		t.Error("expected error for non-existent file, got nil")
	}

	err = ExtractEpubCover(ctx, nonExistentPath, filepath.Join(tmpDir, "cover.jpg"))
	if err == nil {
		t.Error("expected error for non-existent file cover extraction, got nil")
	}

	// Case 2: Empty file
	emptyPath := filepath.Join(tmpDir, "empty.epub")
	if err := os.WriteFile(emptyPath, []byte{}, 0644); err != nil {
		t.Fatal(err)
	}
	_, err = ExtractEpubMetadata(ctx, emptyPath)
	if err == nil {
		t.Error("expected error for empty file, got nil")
	}

	err = ExtractEpubCover(ctx, emptyPath, filepath.Join(tmpDir, "cover.jpg"))
	if err == nil {
		t.Error("expected error for empty file cover extraction, got nil")
	}

	// Case 3: Malformed ZIP file (contains non-zip garbage)
	corruptPath := filepath.Join(tmpDir, "corrupt.epub")
	if err := os.WriteFile(corruptPath, []byte("NOT_A_ZIP_ARCHIVE_DATA"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err = ExtractEpubMetadata(ctx, corruptPath)
	if err == nil {
		t.Error("expected error for corrupt zip, got nil")
	}

	err = ExtractEpubCover(ctx, corruptPath, filepath.Join(tmpDir, "cover.jpg"))
	if err == nil {
		t.Error("expected error for corrupt zip cover extraction, got nil")
	}
}

// TestExtractEpubMetadata_PathTraversal checks that path clean handles any directory traversal attempts inside container.xml.
func TestExtractEpubMetadata_PathTraversal(t *testing.T) {
	containerXML := `<?xml version="1.0"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="../../../../etc/passwd" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>`

	zipBytes := buildZip(t, map[string][]byte{
		"META-INF/container.xml": []byte(containerXML),
	})

	tmpDir, err := os.MkdirTemp("", "epub-traversal-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	epubPath := filepath.Join(tmpDir, "test.epub")
	if err := os.WriteFile(epubPath, zipBytes, 0644); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	_, err = ExtractEpubMetadata(ctx, epubPath)
	if err == nil {
		t.Error("expected error for path traversal attempting to read non-existent zip entry, got nil")
	}
}
