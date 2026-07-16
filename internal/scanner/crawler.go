package scanner

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	log "audiobookshelf/internal/logger"
)

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
