package downloads

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ryoku-dev/ryotunes/internal/model"
)

func TestBuildYTDLPArgsFormats(t *testing.T) {
	tests := []struct {
		name   string
		format model.DownloadFormat
		want   []string
	}{
		{name: "original", format: model.DownloadOriginal, want: []string{"--audio-format", "best"}},
		{name: "mp3", format: model.DownloadMP3, want: []string{"--audio-format", "mp3", "--audio-quality", "320K"}},
		{name: "opus", format: model.DownloadOpus, want: []string{"--audio-format", "opus", "--audio-quality", "160K"}},
		{name: "flac", format: model.DownloadFLAC, want: []string{"--audio-format", "flac", "--postprocessor-args", "ExtractAudio:-sample_fmt s16"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			args := buildYTDLPArgs("/tmp/stage", test.format, true, "target")
			if !containsSequence(args, test.want) {
				t.Fatalf("argv %q does not contain %q", args, test.want)
			}
			common := []string{"--ignore-config", "--no-plugin-dirs", "--no-playlist", "--no-exec", "--no-continue", "--newline", "--no-color", "--progress", "--progress-delta", "0.4"}
			if !reflect.DeepEqual(args[:len(common)], common) {
				t.Fatalf("common argv = %q", args[:len(common)])
			}
			if !containsSequence(args, []string{"--embed-metadata", "--", "target"}) {
				t.Fatalf("terminal argv = %q", args)
			}
		})
	}
}

func TestParseProgress(t *testing.T) {
	tests := []struct {
		line string
		want int
		ok   bool
	}{
		{"[download]  12.6% of 3MiB", 13, true},
		{"[download] 100.0% of 3MiB", 99, true},
		{"[download] Destination: song", 0, false},
		{"[ExtractAudio] 50.0%", 0, false},
	}
	for _, test := range tests {
		got, ok := parseProgress(test.line)
		if got != test.want || ok != test.ok {
			t.Errorf("parseProgress(%q) = %d, %v; want %d, %v", test.line, got, ok, test.want, test.ok)
		}
	}
}

func TestSanitizeAndCollisionSuffix(t *testing.T) {
	if got := sanitizeComponent(" ../A/B:*?  Song\t. "); got != "_A_B___ Song" {
		t.Fatalf("sanitizeComponent = %q", got)
	}
	long := strings.Repeat("界", 50)
	if got := sanitizeComponent(long); len(got) > 120 || !strings.HasSuffix(got, "界") {
		t.Fatalf("UTF-8 bound failed: %q (%d bytes)", got, len(got))
	}
	base := filepath.Join("music", "Artist - Song.mp3")
	if got := collisionPath(base, 0); got != base {
		t.Fatalf("zero suffix = %q", got)
	}
	if got := collisionPath(base, 7); got != filepath.Join("music", "Artist - Song (7).mp3") {
		t.Fatalf("collision suffix = %q", got)
	}
}

func TestAdmissionCountsReservedCancelledAttempt(t *testing.T) {
	manager := &Manager{
		jobs: []model.DownloadEntry{
			{ID: "queued", Status: model.DownloadStatusQueued},
			{ID: "cancelled", Status: model.DownloadStatusCancelled},
			{ID: "done", Status: model.DownloadStatusCompleted},
		},
		running: map[string]*attempt{"cancelled": {cancel: make(chan struct{})}},
	}
	if got := manager.admissionLocked(); got != 2 {
		t.Fatalf("admission = %d, want queued + reserved = 2", got)
	}
}

func TestLoadedSettingsFallback(t *testing.T) {
	settings := normalizeLoadedSettings(model.DownloadSettings{Path: "/tmp/music", Format: "future", Workers: 12})
	if settings.Format != model.DownloadOriginal {
		t.Fatalf("format = %q", settings.Format)
	}
	if settings.Workers != 4 {
		t.Fatalf("workers = %d", settings.Workers)
	}
}

func TestMetadataBlockPicture(t *testing.T) {
	var pngBytes bytes.Buffer
	picture := image.NewNRGBA(image.Rect(0, 0, 3, 2))
	picture.Set(0, 0, color.NRGBA{R: 255, A: 255})
	if err := png.Encode(&pngBytes, picture); err != nil {
		t.Fatal(err)
	}
	encoded, err := encodeMetadataBlockPicture(pngBytes.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	block, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatal(err)
	}
	cursor := 0
	readUint32 := func() uint32 {
		value := binary.BigEndian.Uint32(block[cursor : cursor+4])
		cursor += 4
		return value
	}
	if got := readUint32(); got != 3 {
		t.Fatalf("picture type = %d", got)
	}
	mimeLength := int(readUint32())
	mime := string(block[cursor : cursor+mimeLength])
	cursor += mimeLength
	if mime != "image/png" || readUint32() != 0 {
		t.Fatalf("mime/description = %q", mime)
	}
	width, height, depth, colours := readUint32(), readUint32(), readUint32(), readUint32()
	if width != 3 || height != 2 || depth != 32 || colours != 0 {
		t.Fatalf("geometry = %dx%d depth=%d colours=%d", width, height, depth, colours)
	}
	dataLength := int(readUint32())
	if dataLength != len(pngBytes.Bytes()) || !bytes.Equal(block[cursor:], pngBytes.Bytes()) {
		t.Fatal("picture payload differs")
	}
}
func TestCompletedDownloadResolvesToExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Saved track.opus")
	if err := os.WriteFile(path, []byte("audio"), 0o600); err != nil {
		t.Fatal(err)
	}
	manager := &Manager{jobs: []model.DownloadEntry{{
		VideoID: "video-id", Status: model.DownloadStatusCompleted, FilePath: &path,
	}}}
	manager.rebuildFileIndexLocked()
	stream, ok := manager.Stream("video-id")
	if !ok {
		t.Fatal("completed download did not resolve")
	}
	parsed, err := url.Parse(stream.URL)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Scheme != "file" || parsed.Path != path {
		t.Fatalf("stream URL = %q", stream.URL)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, ok := manager.Stream("video-id"); ok {
		t.Fatal("missing download must fall back to provider resolution")
	}
}

func TestDownloadedStreamLookupDoesNotWaitForManagerLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Saved track.opus")
	if err := os.WriteFile(path, []byte("audio"), 0o600); err != nil {
		t.Fatal(err)
	}
	manager := &Manager{jobs: []model.DownloadEntry{{
		VideoID: "video-id", Status: model.DownloadStatusCompleted, FilePath: &path,
	}}}
	manager.rebuildFileIndexLocked()
	manager.mu.Lock()
	result := make(chan bool, 1)
	go func() {
		_, ok := manager.Stream("video-id")
		result <- ok
	}()
	select {
	case ok := <-result:
		if !ok {
			t.Fatal("completed download did not resolve")
		}
	case <-time.After(time.Second):
		t.Fatal("stream lookup waited for the manager lock")
	}
	manager.mu.Unlock()
}

func containsSequence(values, sequence []string) bool {
	for index := range values {
		if index+len(sequence) <= len(values) && reflect.DeepEqual(values[index:index+len(sequence)], sequence) {
			return true
		}
	}
	return false
}
