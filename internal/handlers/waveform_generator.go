package handlers

import (
	log "audiobookshelf/internal/logger"
	"fmt"
)

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
