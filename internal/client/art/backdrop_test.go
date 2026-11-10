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
	if len(stored.Backdrop) != backdropSize*backdropSize*4 {
		t.Fatalf("rewritten palette file holds %d backdrop bytes, want %d", len(stored.Backdrop), backdropSize*backdropSize*4)
	}
}

func TestBackdropIsASoftBlurOfTheCover(t *testing.T) {
	cover := image.NewNRGBA(image.Rect(0, 0, 2*backdropSize, 2*backdropSize))
	for index := range cover.Pix {
		cover.Pix[index] = 255
	}
	// one thin black line down the middle of a white cover
	for y := range 2 * backdropSize {
		for x := backdropSize; x < backdropSize+2; x++ {
			at := y*cover.Stride + x*4
			cover.Pix[at], cover.Pix[at+1], cover.Pix[at+2] = 0, 0, 0
		}
	}
	backdrop := backdropOf(cover)
	if size := backdrop.Bounds().Size(); size != image.Pt(backdropSize, backdropSize) {
		t.Fatalf("backdrop is %v, want %d px square", size, backdropSize)
	}
	row := backdrop.Pix[(backdropSize/2)*backdrop.Stride : (backdropSize/2+1)*backdrop.Stride]
	centre := row[(backdropSize/2)*4]
	if centre < 100 || centre > 240 {
		t.Fatalf("the line reads %d after the blur, want a soft grey", centre)
	}
	for x := range backdropSize - 1 {
		if step := int(row[(x+1)*4]) - int(row[x*4]); step > 40 || step < -40 {
			t.Fatalf("step of %d between columns %d and %d, want a smooth ramp", step, x, x+1)
		}
	}
	if row[0] != 255 || row[3] != 255 {
		t.Fatalf("far edge = %v, want untouched opaque white", row[:4])
	}
}

func TestLoadingTheCoverKeepsTheBackdropItAlreadyShows(t *testing.T) {
	encoded := testCoverPNG(t, false)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		_, _ = response.Write(encoded)
	}))
	defer server.Close()
	cacheDir := t.TempDir()

	first := New(Options{CacheDir: cacheDir, HTTPClient: server.Client()})
	waitForArtwork(t, first, server.URL, 128)
	palettePath := filepath.Join(cacheDir, diskPaletteName(server.URL))
	deadline := time.Now().Add(4 * time.Second)
	for _, err := os.Stat(palettePath); err != nil; _, err = os.Stat(palettePath) {
		if time.Now().After(deadline) {
			t.Fatal("palette was not written to disk")
		}
		time.Sleep(time.Millisecond)
	}
	first.Close()

	// After a restart the backdrop comes from the palette file first, then the
	// cover itself loads from disk and samples the same picture again.
	second := New(Options{CacheDir: cacheDir, HTTPClient: server.Client()})
	defer second.Close()
	shown := waitForBackdrop(t, second, server.URL)
	waitForArtwork(t, second, server.URL, 128)
	if got, _ := second.Backdrop(server.URL); got != shown {
		t.Fatal("loading the cover swapped in a copy of its backdrop, which fades in again for nothing")
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
