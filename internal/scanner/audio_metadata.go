package scanner

import (
	"encoding/json"
	"os"
	"os/exec"
	"strconv"

	"github.com/dhowden/tag"
)

// AudioTags holds parsed audio file tag metadata.
type AudioTags struct {
	Title       string
	Artist      string
	Album       string
	AlbumArtist string
	Genre       string
	Year        string
	Track       int
	Disc        int
	Composer    string
	Comment     string
}

func parseAudioTags(path string) (*AudioTags, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	m, err := tag.ReadFrom(f)
	if err != nil {
		return nil, err
	}

	trackNum, _ := m.Track()
	discNum, _ := m.Disc()

	tags := &AudioTags{
		Title:       m.Title(),
		Artist:      m.Artist(),
		Album:       m.Album(),
		AlbumArtist: m.AlbumArtist(),
		Genre:       m.Genre(),
		Composer:    m.Composer(),
		Comment:     m.Comment(),
		Track:       trackNum,
		Disc:        discNum,
	}
	if m.Year() > 0 {
		tags.Year = strconv.Itoa(m.Year())
	}

	return tags, nil
}

// FFProbeOutput holds the parsed output from ffprobe.
type FFProbeOutput struct {
	Format struct {
		Duration string            `json:"duration"`
		BitRate  string            `json:"bit_rate"`
		Size     string            `json:"size"`
		Tags     map[string]string `json:"tags"`
	} `json:"format"`
	Streams []struct {
		CodecName  string            `json:"codec_name"`
		CodecType  string            `json:"codec_type"`
		BitRate    string            `json:"bit_rate"`
		Channels   int               `json:"channels"`
		SampleRate string            `json:"sample_rate"`
		Tags       map[string]string `json:"tags"`
	} `json:"streams"`
	Chapters []struct {
		ID        int               `json:"id"`
		StartTime string            `json:"start_time"`
		EndTime   string            `json:"end_time"`
		Tags      map[string]string `json:"tags"`
	} `json:"chapters"`
}

// Chapter represents an audio chapter.
type Chapter struct {
	ID    int     `json:"id"`
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Title string  `json:"title"`
}

// AudioMetadata holds parsed audio metadata.
type AudioMetadata struct {
	Duration   float64
	BitRate    int64
	Codec      string
	Channels   int
	SampleRate int
	Chapters   []Chapter
}

func probeAudioFile(path string) (*AudioMetadata, error) {
	probeSemaphore <- struct{}{}
	defer func() { <-probeSemaphore }()

	cmd := exec.Command("ffprobe", "-v", "error", "-show_format", "-show_streams", "-show_chapters", "-of", "json", path)
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var probe FFProbeOutput
	if err := json.Unmarshal(out, &probe); err != nil {
		return nil, err
	}

	meta := &AudioMetadata{}

	if d, err := strconv.ParseFloat(probe.Format.Duration, 64); err == nil {
		meta.Duration = d
	}
	if b, err := strconv.ParseInt(probe.Format.BitRate, 10, 64); err == nil {
		meta.BitRate = b
	}

	for _, stream := range probe.Streams {
		if stream.CodecType == "audio" {
			meta.Codec = stream.CodecName
			meta.Channels = stream.Channels
			if sr, err := strconv.Atoi(stream.SampleRate); err == nil {
				meta.SampleRate = sr
			}
			if meta.BitRate == 0 {
				if b, err := strconv.ParseInt(stream.BitRate, 10, 64); err == nil {
					meta.BitRate = b
				}
			}
			break
		}
	}

	for _, ch := range probe.Chapters {
		start, _ := strconv.ParseFloat(ch.StartTime, 64)
		end, _ := strconv.ParseFloat(ch.EndTime, 64)
		title := ch.Tags["title"]
		if title == "" {
			title = "Chapter " + strconv.Itoa(ch.ID)
		}
		meta.Chapters = append(meta.Chapters, Chapter{
			ID:    ch.ID,
			Start: start,
			End:   end,
			Title: title,
		})
	}

	return meta, nil
}
