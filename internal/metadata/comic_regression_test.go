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

// TestExtractComicMetadata_Concurrency runs ExtractComicMetadata in parallel for CBZ and CBR.
func TestExtractComicMetadata_Concurrency(t *testing.T) {
	// Create dummy CBZ content
	comicInfoXML := `<?xml version="1.0" encoding="utf-8"?>
<ComicInfo xmlns:xsd="http://www.w3.org/2001/XMLSchema" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <Title>Concurrent Title</Title>
  <Series>Concurrent Series</Series>
  <Number>Issue 9</Number>
  <Writer>Concurrent Writer</Writer>
  <Publisher>Concurrent Publisher</Publisher>
  <Year>2026</Year>
</ComicInfo>`

	zipBytes := buildZip(t, map[string][]byte{
		"ComicInfo.xml": []byte(comicInfoXML),
		"001.jpg":       []byte("fake_image_data_concurrency"),
	})

	// Create dummy CBR content
	rarBytes := buildRar(t, map[string][]byte{
		"ComicInfo.xml": []byte(comicInfoXML),
		"001.jpg":       []byte("fake_image_data_concurrency"),
	})

	tmpDir, err := os.MkdirTemp("", "comic-concurrency-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	cbzPath := filepath.Join(tmpDir, "test.cbz")
	if err := os.WriteFile(cbzPath, zipBytes, 0644); err != nil {
		t.Fatal(err)
	}

	cbrPath := filepath.Join(tmpDir, "test.cbr")
	if err := os.WriteFile(cbrPath, rarBytes, 0644); err != nil {
		t.Fatal(err)
	}

	const numGoroutines = 20
	const numIterations = 10
	var wg sync.WaitGroup

	// Concurrently read CBZ and CBR metadata
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			time.Sleep(time.Duration(rand.Intn(5)) * time.Millisecond)

			for j := 0; j < numIterations; j++ {
				ctx := context.Background()
				// Test CBZ
				meta, err := ExtractComicMetadata(ctx, cbzPath)
				if err != nil {
					t.Errorf("Goroutine %d iteration %d CBZ: unexpected error: %v", id, j, err)
					return
				}
				if meta.Title != "Concurrent Series Issue 9" {
					t.Errorf("Goroutine %d iteration %d CBZ: expected Title 'Concurrent Series Issue 9', got %q", id, j, meta.Title)
					return
				}

				// Test CBR
				meta, err = ExtractComicMetadata(ctx, cbrPath)
				if err != nil {
					t.Errorf("Goroutine %d iteration %d CBR: unexpected error: %v", id, j, err)
					return
				}
				if meta.Title != "Concurrent Series Issue 9" {
					t.Errorf("Goroutine %d iteration %d CBR: expected Title 'Concurrent Series Issue 9', got %q", id, j, meta.Title)
					return
				}
			}
		}(i)
	}

	wg.Wait()
}

// TestExtractComicCover_Concurrency runs ExtractComicCover in parallel.
func TestExtractComicCover_Concurrency(t *testing.T) {
	zipBytes := buildZip(t, map[string][]byte{
		"002.jpg": []byte("image_2"),
		"001.jpg": []byte("image_1_first"),
	})

	rarBytes := buildRar(t, map[string][]byte{
		"002.jpg": []byte("image_2"),
		"001.jpg": []byte("image_1_first"),
	})

	tmpDir, err := os.MkdirTemp("", "comic-cover-concurrency-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	cbzPath := filepath.Join(tmpDir, "test.cbz")
	if err := os.WriteFile(cbzPath, zipBytes, 0644); err != nil {
		t.Fatal(err)
	}

	cbrPath := filepath.Join(tmpDir, "test.cbr")
	if err := os.WriteFile(cbrPath, rarBytes, 0644); err != nil {
		t.Fatal(err)
	}

	const numGoroutines = 20
	var wg sync.WaitGroup

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			destPathCbz := filepath.Join(tmpDir, fmt.Sprintf("cbz_cover_dest_%d.jpg", id))
			destPathCbr := filepath.Join(tmpDir, fmt.Sprintf("cbr_cover_dest_%d.jpg", id))

			ctx := context.Background()

			// Extract from CBZ
			err := ExtractComicCover(ctx, cbzPath, destPathCbz)
			if err != nil {
				t.Errorf("Goroutine %d CBZ: unexpected error: %v", id, err)
				return
			}
			data, err := os.ReadFile(destPathCbz)
			if err != nil {
				t.Errorf("Goroutine %d CBZ: failed to read destination cover file: %v", id, err)
				return
			}
			if string(data) != "image_1_first" {
				t.Errorf("Goroutine %d CBZ: expected cover content 'image_1_first', got %q", id, string(data))
			}

			// Extract from CBR
			err = ExtractComicCover(ctx, cbrPath, destPathCbr)
			if err != nil {
				t.Errorf("Goroutine %d CBR: unexpected error: %v", id, err)
				return
			}
			data, err = os.ReadFile(destPathCbr)
			if err != nil {
				t.Errorf("Goroutine %d CBR: failed to read destination cover file: %v", id, err)
				return
			}
			if string(data) != "image_1_first" {
				t.Errorf("Goroutine %d CBR: expected cover content 'image_1_first', got %q", id, string(data))
			}
		}(i)
	}

	wg.Wait()
}

// TestExtractComic_InvalidFiles checks that CBZ and CBR parsers handle errors gracefully.
func TestExtractComic_InvalidFiles(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "comic-invalid-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	ctx := context.Background()

	// 1. Non-existent file
	nonExistentPath := filepath.Join(tmpDir, "non_existent.cbz")
	_, err = ExtractComicMetadata(ctx, nonExistentPath)
	if err == nil {
		t.Error("expected error for non-existent file, got nil")
	}
	err = ExtractComicCover(ctx, nonExistentPath, filepath.Join(tmpDir, "cover.jpg"))
	if err == nil {
		t.Error("expected error for non-existent file cover extraction, got nil")
	}

	// 2. Empty file
	emptyCbzPath := filepath.Join(tmpDir, "empty.cbz")
	if err := os.WriteFile(emptyCbzPath, []byte{}, 0644); err != nil {
		t.Fatal(err)
	}
	_, err = ExtractComicMetadata(ctx, emptyCbzPath)
	if err == nil {
		t.Error("expected error for empty file, got nil")
	}
	err = ExtractComicCover(ctx, emptyCbzPath, filepath.Join(tmpDir, "cover.jpg"))
	if err == nil {
		t.Error("expected error for empty file cover extraction, got nil")
	}

	// 3. Corrupt files (garbage content)
	corruptCbzPath := filepath.Join(tmpDir, "corrupt.cbz")
	if err := os.WriteFile(corruptCbzPath, []byte("NOT_A_ZIP_OR_RAR_ARCHIVE"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err = ExtractComicMetadata(ctx, corruptCbzPath)
	if err == nil {
		t.Error("expected error for corrupt archive, got nil")
	}
	err = ExtractComicCover(ctx, corruptCbzPath, filepath.Join(tmpDir, "cover.jpg"))
	if err == nil {
		t.Error("expected error for corrupt archive cover extraction, got nil")
	}

	// 4. Archive with no images (for cover extraction)
	noImagesZipBytes := buildZip(t, map[string][]byte{
		"ComicInfo.xml": []byte("<ComicInfo></ComicInfo>"),
		"readme.txt":    []byte("some text"),
	})
	noImagesCbzPath := filepath.Join(tmpDir, "no_images.cbz")
	if err := os.WriteFile(noImagesCbzPath, noImagesZipBytes, 0644); err != nil {
		t.Fatal(err)
	}
	err = ExtractComicCover(ctx, noImagesCbzPath, filepath.Join(tmpDir, "cover.jpg"))
	if err == nil {
		t.Error("expected error extracting cover from archive with no images, got nil")
	}

	noImagesRarBytes := buildRar(t, map[string][]byte{
		"ComicInfo.xml": []byte("<ComicInfo></ComicInfo>"),
		"readme.txt":    []byte("some text"),
	})
	noImagesCbrPath := filepath.Join(tmpDir, "no_images.cbr")
	if err := os.WriteFile(noImagesCbrPath, noImagesRarBytes, 0644); err != nil {
		t.Fatal(err)
	}
	err = ExtractComicCover(ctx, noImagesCbrPath, filepath.Join(tmpDir, "cover.jpg"))
	if err == nil {
		t.Error("expected error extracting cover from rar with no images, got nil")
	}
}
