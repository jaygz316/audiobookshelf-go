package scanner

import (
	"strings"
)

func categorizeGroupFiles(groupFiles []FileItem, mediaType string, audiobooksOnly bool) (
	audioFiles []FileItem,
	ebookFiles []FileItem,
	imageFiles []FileItem,
	opfFile string,
	nfoFile string,
	descFile string,
	readerFile string,
) {
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
	return
}

func findBestCoverImage(imageFiles []FileItem) string {
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
	return bestCover
}
