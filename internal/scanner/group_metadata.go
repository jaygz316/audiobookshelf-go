package scanner

import (
	"database/sql"
	"encoding/json"
	"strings"
)

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

	scannerParseSubtitles, scannerFindCovers := getScannerSettings(dbConn)

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

	audioFiles, ebookFiles, imageFiles, opfFile, nfoFile, descFile, readerFile := categorizeGroupFiles(groupFiles, mediaType, audiobooksOnly)

	if scannerFindCovers && len(imageFiles) > 0 {
		meta.CoverPath = findBestCoverImage(imageFiles)
	}

	if len(audioFiles) > 0 {
		parsedFiles := parseAudioFiles(audioFiles, itemPath)
		var totalDuration float64
		var audioFilesData []interface{}
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
	}

	if len(ebookFiles) > 0 {
		parseEbookMetadata(dbConn, itemID, ebookFiles, meta, scannerFindCovers, itemPath)
	}

	if opfFile != "" {
		parseOPFMetadata(opfFile, meta, itemPath)
	}

	if nfoFile != "" {
		parseNFOMetadata(nfoFile, meta, scannerParseSubtitles, itemPath)
	}

	if descFile != "" || readerFile != "" {
		parseTxtFilesMetadata(descFile, readerFile, meta, itemPath)
	}

	return meta
}

func getScannerSettings(dbConn *sql.DB) (scannerParseSubtitles bool, scannerFindCovers bool) {
	scannerParseSubtitles = true
	scannerFindCovers = true
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
	return
}
