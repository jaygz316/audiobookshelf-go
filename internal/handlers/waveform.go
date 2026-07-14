package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"audiobookshelf/internal/core"
	"audiobookshelf/internal/utils"
)

type AudioFileInfo struct {
	Path     string
	Duration float64
}

func handleGetWaveform(db *sql.DB, cfg *core.Config, itemID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userVal := r.Context().Value(core.UserContextKey)
		if userVal == nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error": "Unauthorized"}`))
			return
		}

		// Validate itemID to prevent path traversal
		if strings.Contains(itemID, "..") || strings.Contains(itemID, "/") || strings.Contains(itemID, "\\") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error": "Invalid item ID"}`))
			return
		}

		itemDir := filepath.Join(cfg.MetadataPath, "items", itemID)
		waveformPath := filepath.Join(itemDir, "waveform.json")

		if data, err := os.ReadFile(waveformPath); err == nil {
			w.Header().Set("Content-Type", "application/json")
			w.Write(data)
			return
		}

		infos, err := getAudioFilesInfo(db, itemID)
		if err != nil {
			log.Errorf("[Waveform] Failed to resolve audio files for %s: %v", itemID, err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(fmt.Sprintf(`{"error": "%v"}`, err)))
			return
		}

		// Verify safety of all audio file paths
		for _, info := range infos {
			if !utils.IsSafeFilePath(db, cfg.MetadataPath, info.Path) {
				log.Warnf("[Waveform] Unsafe audio file path traversal blocked: %s", info.Path)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				w.Write([]byte(`{"error": "Forbidden"}`))
				return
			}
		}

		// Target 200 points for the player waveform
		peaks, err := GenerateWaveform(infos, 200)
		if err != nil {
			log.Errorf("[Waveform] Failed to generate waveform for %s: %v", itemID, err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(fmt.Sprintf(`{"error": "Failed to generate waveform: %v"}`, err)))
			return
		}

		respData := map[string]interface{}{
			"itemId": itemID,
			"peaks":  peaks,
		}

		jsonData, err := json.Marshal(respData)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error": "Failed to marshal response"}`))
			return
		}

		_ = os.MkdirAll(itemDir, 0755)
		_ = os.WriteFile(waveformPath, jsonData, 0644)

		w.Header().Set("Content-Type", "application/json")
		w.Write(jsonData)
	}
}


func getAudioFilesInfo(db *sql.DB, id string) ([]AudioFileInfo, error) {
	// 1. Try to find the ID in libraryItems
	var mediaId, mediaType string
	err := db.QueryRow("SELECT mediaId, mediaType FROM libraryItems WHERE id = ?", id).Scan(&mediaId, &mediaType)
	if err == nil {
		if mediaType == "book" {
			return getBookAudioInfo(db, mediaId)
		} else if mediaType == "podcast" {
			// Find first episode's audio path for the podcast
			var firstEpId string
			err = db.QueryRow("SELECT id FROM podcastEpisodes WHERE podcastId = ? LIMIT 1", mediaId).Scan(&firstEpId)
			if err == nil {
				return getPodcastEpisodeAudioInfo(db, firstEpId)
			}
		}
	}

	// 2. Try to find the ID in books (directly)
	var bookExists int
	errBook := db.QueryRow("SELECT 1 FROM books WHERE id = ?", id).Scan(&bookExists)
	if errBook == nil && bookExists == 1 {
		return getBookAudioInfo(db, id)
	}

	// 3. Try to find the ID in podcastEpisodes (directly)
	var epExists int
	errEp := db.QueryRow("SELECT 1 FROM podcastEpisodes WHERE id = ?", id).Scan(&epExists)
	if errEp == nil && epExists == 1 {
		return getPodcastEpisodeAudioInfo(db, id)
	}

	return nil, fmt.Errorf("item not found or has no audio files")
}

func getBookAudioInfo(db *sql.DB, bookID string) ([]AudioFileInfo, error) {
	var audioFilesJSONStr string
	err := db.QueryRow("SELECT audioFiles FROM books WHERE id = ?", bookID).Scan(&audioFilesJSONStr)
	if err != nil {
		return nil, err
	}
	type AudioFileJSON struct {
		Exclude  bool    `json:"exclude"`
		Duration float64 `json:"duration"`
		Metadata struct {
			Path string `json:"path"`
		} `json:"metadata"`
	}
	var audioFiles []AudioFileJSON
	if err := json.Unmarshal([]byte(audioFilesJSONStr), &audioFiles); err != nil {
		return nil, err
	}
	var infos []AudioFileInfo
	for _, af := range audioFiles {
		if !af.Exclude && af.Metadata.Path != "" {
			infos = append(infos, AudioFileInfo{
				Path:     af.Metadata.Path,
				Duration: af.Duration,
			})
		}
	}
	return infos, nil
}

func getPodcastEpisodeAudioInfo(db *sql.DB, epID string) ([]AudioFileInfo, error) {
	var audioFileJSONStr string
	err := db.QueryRow("SELECT audioFile FROM podcastEpisodes WHERE id = ?", epID).Scan(&audioFileJSONStr)
	if err != nil {
		return nil, err
	}
	type AudioFileStruct struct {
		Duration float64 `json:"duration"`
		Metadata struct {
			Path string `json:"path"`
		} `json:"metadata"`
	}
	var audioFile AudioFileStruct
	if err := json.Unmarshal([]byte(audioFileJSONStr), &audioFile); err != nil {
		return nil, err
	}
	if audioFile.Metadata.Path != "" {
		return []AudioFileInfo{{
			Path:     audioFile.Metadata.Path,
			Duration: audioFile.Duration,
		}}, nil
	}
	return nil, fmt.Errorf("no audio path found for podcast episode")
}

func GenerateWaveform(infos []AudioFileInfo, targetPoints int) ([]int, error) {
	if len(infos) == 0 {
		return nil, fmt.Errorf("no audio files to process")
	}

	var totalDuration float64
	for _, info := range infos {
		totalDuration += info.Duration
	}
	if totalDuration <= 0 {
		for i := range infos {
			infos[i].Duration = 1.0
		}
		totalDuration = float64(len(infos))
	}

	var combinedPeaks []int
	pointsAssigned := 0

	for i, info := range infos {
		filePoints := int((info.Duration / totalDuration) * float64(targetPoints))
		if filePoints == 0 && info.Duration > 0 {
			filePoints = 1
		}
		if i == len(infos)-1 {
			filePoints = targetPoints - pointsAssigned
		}
		if filePoints <= 0 {
			continue
		}

		peaks, err := GenerateWaveformForFile(info.Path, filePoints)
		if err != nil {
			log.Errorf("[Waveform] Failed to generate for file %s: %v", info.Path, err)
			peaks = make([]int, filePoints)
		}
		combinedPeaks = append(combinedPeaks, peaks...)
		pointsAssigned += filePoints
	}

	if len(combinedPeaks) < targetPoints {
		diff := targetPoints - len(combinedPeaks)
		for i := 0; i < diff; i++ {
			combinedPeaks = append(combinedPeaks, 0)
		}
	} else if len(combinedPeaks) > targetPoints {
		combinedPeaks = combinedPeaks[:targetPoints]
	}

	maxVal := 0
	for _, v := range combinedPeaks {
		if v > maxVal {
			maxVal = v
		}
	}
	if maxVal > 0 {
		for i := range combinedPeaks {
			combinedPeaks[i] = (combinedPeaks[i] * 255) / maxVal
		}
	}

	return combinedPeaks, nil
}

func GenerateWaveformForFile(path string, targetPoints int) ([]int, error) {
	cmd := exec.Command("ffmpeg", "-i", path, "-f", "s16le", "-ac", "1", "-ar", "100", "-")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	var rawSamples []int16
	buf := make([]byte, 4096)
	var leftover byte
	hasLeftover := false

	for {
		n, err := stdout.Read(buf)
		if n > 0 {
			startIdx := 0
			if hasLeftover {
				val := int16(leftover) | (int16(buf[0]) << 8)
				rawSamples = append(rawSamples, val)
				startIdx = 1
				hasLeftover = false
			}

			for i := startIdx; i < n; i += 2 {
				if i+1 < n {
					val := int16(buf[i]) | (int16(buf[i+1]) << 8)
					rawSamples = append(rawSamples, val)
				} else {
					leftover = buf[i]
					hasLeftover = true
				}
			}
		}
		if err != nil {
			break
		}
	}
	_ = cmd.Wait()

	if len(rawSamples) == 0 {
		return nil, fmt.Errorf("no samples decoded")
	}

	peaks := make([]int, targetPoints)
	maxVal := 0
	for i := 0; i < targetPoints; i++ {
		start := (i * len(rawSamples)) / targetPoints
		end := ((i + 1) * len(rawSamples)) / targetPoints
		if start >= len(rawSamples) {
			break
		}
		if end > len(rawSamples) {
			end = len(rawSamples)
		}
		if start == end {
			end = start + 1
		}

		localMax := 0
		for j := start; j < end; j++ {
			absVal := int(rawSamples[j])
			if absVal < 0 {
				absVal = -absVal
			}
			if absVal > localMax {
				localMax = absVal
			}
		}
		peaks[i] = localMax
		if localMax > maxVal {
			maxVal = localMax
		}
	}

	if maxVal > 0 {
		for i := 0; i < targetPoints; i++ {
			peaks[i] = (peaks[i] * 255) / maxVal
		}
	}

	return peaks, nil
}
