package art

import (
	"bytes"
	"encoding/json"
	"image"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestBackdropComesBackFromDiskAndOldPaletteFilesAreSampledAgain(t *testing.T) {
	encoded := testCoverPNG(t, false)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		_, _ = response.Write(encoded)
	}))
	defer server.Close()
	cacheDir := t.TempDir()

	first := New(Options{CacheDir: cacheDir, HTTPClient: server.Client()})
	want := waitForBackdrop(t, first, server.URL)
	first.Close()

	second := New(Options{CacheDir: cacheDir, HTTPClient: server.Client()})
	got := waitForBackdrop(t, second, server.URL)
	second.Close()
	if !bytes.Equal(got.Pix, want.Pix) {
		t.Fatal("backdrop read back from disk differs from the one sampled")
	}

	// A palette file from before backdrops holds only the two colours.
	path := filepath.Join(cacheDir, diskPaletteName(server.URL))
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var stored diskSample
	if err := json.Unmarshal(content, &stored); err != nil {
		t.Fatal(err)
	}
	old, err := json.Marshal(stored.Palette)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, old, 0o600); err != nil {
		t.Fatal(err)
	}
	third := New(Options{CacheDir: cacheDir, HTTPClient: server.Client()})
	defer third.Close()
	waitForBackdrop(t, third, server.URL)
	if got := requests.Load(); got != 1 {
		t.Fatalf("cover requests = %d, want the one fetch with every later sample from disk", got)
	}
	content, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	stored = diskSample{}
	if err := json.Unmarshal(content, &stored); err != nil {
		t.Fatal(err)
	}
	if len(stored.Backdrop) != accentSampleSize*accentSampleSize*4 {
		t.Fatalf("rewritten palette file holds %d backdrop bytes, want %d", len(stored.Backdrop), accentSampleSize*accentSampleSize*4)
	}
}

func TestBackdropIsASoftBlurOfTheCover(t *testing.T) {
	sample := image.NewNRGBA(image.Rect(0, 0, accentSampleSize, accentSampleSize))
	for index := range sample.Pix {
		sample.Pix[index] = 255
	}
	// one black column on white
	for y := range accentSampleSize {
		sample.Pix[y*sample.Stride+12*4], sample.Pix[y*sample.Stride+12*4+1], sample.Pix[y*sample.Stride+12*4+2] = 0, 0, 0
	}
	backdrop := backdropOf(sample)
	row := backdrop.Pix[10*backdrop.Stride : 11*backdrop.Stride]
	centre := row[12*4]
	if centre < 150 || centre > 230 {
		t.Fatalf("hard edge survived the blur: centre %d, want a soft grey", centre)
	}
	for x := range accentSampleSize - 1 {
		if step := int(row[(x+1)*4]) - int(row[x*4]); step > 40 || step < -40 {
			t.Fatalf("step of %d between columns %d and %d, want a smooth ramp", step, x, x+1)
		}
	}
	if row[0] != 255 || row[3] != 255 {
		t.Fatalf("far edge = %v, want untouched opaque white", row[:4])
	}
}

func waitForBackdrop(t *testing.T, cache *Cache, rawURL string) *image.RGBA {
	t.Helper()
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		if backdrop, ready := cache.Backdrop(rawURL); ready {
			return backdrop
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("backdrop %q did not become ready", rawURL)
	return nil
}
