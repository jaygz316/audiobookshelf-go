package hls

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNeedsAACForce(t *testing.T) {
	tests := []struct {
		name             string
		codec            string
		mimeType         string
		isResettingToAAC bool
		expected         bool
	}{
		{
			name:     "Copy codec for mp3",
			codec:    "mp3",
			mimeType: "audio/mpeg",
			expected: false,
		},
		{
			name:     "Copy codec for m4a/aac",
			codec:    "aac",
			mimeType: "audio/mp4",
			expected: false,
		},
		{
			name:     "Force AAC for flac codec",
			codec:    "flac",
			mimeType: "audio/flac",
			expected: true,
		},
		{
			name:     "Force AAC for opus codec",
			codec:    "opus",
			mimeType: "audio/opus",
			expected: true,
		},
		{
			name:     "Force AAC for alac codec",
			codec:    "alac",
			mimeType: "audio/m4a",
			expected: true,
		},
		{
			name:     "Force AAC for audio/ogg mime",
			codec:    "vorbis",
			mimeType: "audio/ogg",
			expected: true,
		},
		{
			name:             "Force AAC when isResettingToAAC is true",
			codec:            "mp3",
			mimeType:         "audio/mpeg",
			isResettingToAAC: true,
			expected:         true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Stream{
				isResettingToAAC: tt.isResettingToAAC,
				Tracks: []Track{
					{
						Codec:    tt.codec,
						MimeType: tt.mimeType,
					},
				},
			}
			result := s.needsAACForce()
			if result != tt.expected {
				t.Errorf("Expected needsAACForce() = %v, got %v for codec %s, mime %s", tt.expected, result, tt.codec, tt.mimeType)
			}
		})
	}
}

func TestEscapeSingleQuotes(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{
			input:    "normal_path/file.mp3",
			expected: "normal_path/file.mp3",
		},
		{
			input:    "path'with'quotes/file.mp3",
			expected: "path'\\''with'\\''quotes/file.mp3",
		},
		{
			input:    "backslash" + string(filepath.Separator) + "path" + string(filepath.Separator) + "file.mp3",
			expected: "backslash/path/file.mp3",
		},
	}

	for _, tt := range tests {
		result := escapeSingleQuotes(tt.input)
		if result != tt.expected {
			t.Errorf("Expected escapeSingleQuotes(%q) = %q, got %q", tt.input, tt.expected, result)
		}
	}
}

func TestParseSegmentNumber(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{
			input:    "output-0.ts",
			expected: 0,
		},
		{
			input:    "output-123.ts",
			expected: 123,
		},
		{
			input:    "output-abc.ts",
			expected: -1,
		},
		{
			input:    "segment-0.ts",
			expected: -1,
		},
		{
			input:    "output-99.mp4",
			expected: -1,
		},
	}

	for _, tt := range tests {
		result := parseSegmentNumber(tt.input)
		if result != tt.expected {
			t.Errorf("Expected parseSegmentNumber(%q) = %d, got %d", tt.input, tt.expected, result)
		}
	}
}

func TestGetPlaylistStr(t *testing.T) {
	t.Run("ts format", func(t *testing.T) {
		playlist := getPlaylistStr("output", 15.5, 6.0, "mpegts")
		expectedLines := []string{
			"#EXTM3U",
			"#EXT-X-VERSION:3",
			"#EXT-X-ALLOW-CACHE:NO",
			"#EXT-X-TARGETDURATION:6",
			"#EXT-X-MEDIA-SEQUENCE:0",
			"#EXT-X-PLAYLIST-TYPE:VOD",
			"#EXTINF:6,",
			"output-0.ts",
			"#EXTINF:6,",
			"output-1.ts",
			"#EXTINF:3.5,",
			"output-2.ts",
			"#EXT-X-ENDLIST",
		}
		expected := strings.Join(expectedLines, "\n")
		if playlist != expected {
			t.Errorf("Playlist output mismatch.\nExpected:\n%s\n\nGot:\n%s", expected, playlist)
		}
	})

	t.Run("fmp4 format", func(t *testing.T) {
		playlist := getPlaylistStr("output", 12.0, 6.0, "fmp4")
		expectedLines := []string{
			"#EXTM3U",
			"#EXT-X-VERSION:3",
			"#EXT-X-ALLOW-CACHE:NO",
			"#EXT-X-TARGETDURATION:6",
			"#EXT-X-MEDIA-SEQUENCE:0",
			"#EXT-X-PLAYLIST-TYPE:VOD",
			`#EXT-X-MAP:URI="init.mp4"`,
			"#EXTINF:6,",
			"output-0.m4s",
			"#EXTINF:6,",
			"output-1.m4s",
			"#EXT-X-ENDLIST",
		}
		expected := strings.Join(expectedLines, "\n")
		if playlist != expected {
			t.Errorf("Playlist output mismatch.\nExpected:\n%s\n\nGot:\n%s", expected, playlist)
		}
	})
}

func TestWriteConcatFile(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "hls-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	s := &Stream{
		StreamPath:      tempDir,
		ConcatFilesPath: filepath.Join(tempDir, "files.txt"),
	}

	tracks := []Track{
		{Index: 0, Duration: 10.0, Path: "/path/to/track1.mp3"},
		{Index: 1, Duration: 15.5, Path: "/path/to/track2.mp3"},
	}

	// Case 1: Start from beginning
	s.AdjustedStartTime = 0.0
	startOffset, err := s.writeConcatFile(tracks)
	if err != nil {
		t.Fatalf("writeConcatFile failed: %v", err)
	}
	if startOffset != 0.0 {
		t.Errorf("Expected startOffset 0.0, got %f", startOffset)
	}

	contentBytes, err := os.ReadFile(s.ConcatFilesPath)
	if err != nil {
		t.Fatalf("Failed to read concat file: %v", err)
	}

	expectedContent := "file '/path/to/track1.mp3'\nduration 10.000000\n\nfile '/path/to/track2.mp3'\nduration 15.500000"
	if string(contentBytes) != expectedContent {
		t.Errorf("Concat file content mismatch.\nExpected:\n%q\n\nGot:\n%q", expectedContent, string(contentBytes))
	}

	// Case 2: Start from middle of second track (adjusted start time = 12.0)
	s.AdjustedStartTime = 12.0
	startOffset2, err := s.writeConcatFile(tracks)
	if err != nil {
		t.Fatalf("writeConcatFile failed: %v", err)
	}
	if startOffset2 != 10.0 {
		t.Errorf("Expected startOffset 10.0, got %f", startOffset2)
	}

	contentBytes2, err := os.ReadFile(s.ConcatFilesPath)
	if err != nil {
		t.Fatalf("Failed to read concat file: %v", err)
	}

	expectedContent2 := "file '/path/to/track2.mp3'\nduration 15.500000"
	if string(contentBytes2) != expectedContent2 {
		t.Errorf("Concat file content mismatch for offset start.\nExpected:\n%q\n\nGot:\n%q", expectedContent2, string(contentBytes2))
	}
}

func TestStreamManager_Close(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "hls-mgr-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	sm := NewStreamManager()
	s1Path := filepath.Join(tempDir, "stream1")
	s2Path := filepath.Join(tempDir, "stream2")
	_ = os.MkdirAll(s1Path, 0755)
	_ = os.MkdirAll(s2Path, 0755)

	s1 := &Stream{
		ID:         "stream1",
		StreamPath: s1Path,
	}
	s2 := &Stream{
		ID:         "stream2",
		StreamPath: s2Path,
	}

	sm.AddStream(s1)
	sm.AddStream(s2)

	if sm.GetStream("stream1") == nil || sm.GetStream("stream2") == nil {
		t.Errorf("Expected both streams to be in the manager")
	}

	// Close the manager, which should close and clean up both streams
	sm.Close()

	if sm.GetStream("stream1") != nil || sm.GetStream("stream2") != nil {
		t.Errorf("Expected both streams to be removed from the manager after Close()")
	}

	if _, err := os.Stat(s1Path); !os.IsNotExist(err) {
		t.Errorf("Expected stream1 directory to be deleted")
	}
	if _, err := os.Stat(s2Path); !os.IsNotExist(err) {
		t.Errorf("Expected stream2 directory to be deleted")
	}
}
