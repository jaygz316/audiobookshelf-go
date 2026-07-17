package scanner

import (
	"strings"
	"sync"
	"time"

	log "audiobookshelf/internal/logger"
)

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

func parseAudioFiles(audioFiles []FileItem, itemPath string) []parsedAudioFile {
	parsedFiles := make([]parsedAudioFile, len(audioFiles))
	var wg sync.WaitGroup

	for i, f := range audioFiles {
		wg.Add(1)
		go func(i int, f FileItem) {
			defer wg.Done()
			parsedFiles[i] = parseSingleAudioFile(i, f, len(audioFiles), itemPath)
		}(i, f)
	}
	wg.Wait()

	return parsedFiles
}

func parseSingleAudioFile(i int, f FileItem, totalFiles int, itemPath string) parsedAudioFile {
	log.Printf("[Scanner] [%s] Probing audio file (%d/%d): %s", itemPath, i+1, totalFiles, f.Path)
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

	return parsedAudioFile{
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
}
