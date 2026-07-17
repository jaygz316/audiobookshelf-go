package handlers

import (
	"fmt"
	"os/exec"
)

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
