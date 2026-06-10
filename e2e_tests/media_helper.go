package e2e_tests

import (
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"os/exec"
	"path/filepath"
)

// locateFFmpeg searches for the ffmpeg binary in the system PATH,
// and falls back to relative paths for the precompiled static binary.
func locateFFmpeg() string {
	if path, err := exec.LookPath("ffmpeg"); err == nil {
		return path
	}
	// Try "../ffmpeg" (relative to e2e_tests directory)
	if _, err := os.Stat("../ffmpeg"); err == nil {
		if abs, err := filepath.Abs("../ffmpeg"); err == nil {
			return abs
		}
	}
	// Try "./ffmpeg" (relative to project root)
	if _, err := os.Stat("./ffmpeg"); err == nil {
		if abs, err := filepath.Abs("./ffmpeg"); err == nil {
			return abs
		}
	}
	return "ffmpeg"
}

// GenerateMockAudio runs ffmpeg to generate a 1-second silent MP3 file
// with the specified metadata tags.
func GenerateMockAudio(filePath string, title, artist, album, track, year string) error {
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return err
	}

	ffmpegBin := locateFFmpeg()
	args := []string{
		"-y",
		"-f", "lavfi",
		"-i", "anullsrc=r=44100:cl=mono",
		"-t", "1",
		"-metadata", fmt.Sprintf("title=%s", title),
		"-metadata", fmt.Sprintf("artist=%s", artist),
		"-metadata", fmt.Sprintf("album=%s", album),
		"-metadata", fmt.Sprintf("track=%s", track),
		"-metadata", fmt.Sprintf("date=%s", year),
		"-c:a", "libmp3lame",
		filePath,
	}

	cmd := exec.Command(ffmpegBin, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg failed: %v, output: %s", err, string(output))
	}
	return nil
}

// GenerateMockCover writes a small valid JPEG file to act as cover art.
func GenerateMockCover(filePath string) error {
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return err
	}
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	for y := 0; y < 10; y++ {
		for x := 0; x < 10; x++ {
			img.Set(x, y, color.RGBA{R: 200, G: 50, B: 50, A: 255})
		}
	}
	f, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer f.Close()
	return jpeg.Encode(f, img, nil)
}
