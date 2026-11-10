package art

import (
	"image"
	"image/color"
	"image/png"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestRequestSizeRoundsUpToStableLadder(t *testing.T) {
	cases := map[int]int{
		0: 32, 1: 32, 32: 32, 33: 48, 64: 64, 65: 96,
		191: 192, 193: 256, 512: 512, 513: 768, 2049: 2048,
	}
	for input, want := range cases {
		if got := RequestSize(input); got != want {
			t.Fatalf("RequestSize(%d) = %d, want %d", input, got, want)
		}
	}
}

func TestCacheHitsDoNotAllocate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cover.png")
	if err := os.WriteFile(path, testCoverPNG(t, false), 0o600); err != nil {
		t.Fatal(err)
	}
	cache := New(Options{})
	defer cache.Close()
	waitForArtwork(t, cache, path, 96)
	waitForPalette(t, cache, path)
	allocations := testing.AllocsPerRun(200, func() {
		cache.Get(path, 90)
		cache.Palette(path)
		cache.Failed(path, 90)
	})
	if allocations != 0 {
		t.Fatalf("cache hit allocations per frame = %v, want 0", allocations)
	}
}

func TestSchemeLessArtworkIsNotOpenedFromWorkingDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "cover.png"), testCoverPNG(t, false), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	var requests atomic.Int32
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		requests.Add(1)
		return nil, os.ErrInvalid
	})}
	changed := make(chan struct{}, 1)
	cache := New(Options{HTTPClient: client, Invalidate: func() {
		select {
		case changed <- struct{}{}:
		default:
		}
	}})
	defer cache.Close()
	cache.Get("cover.png", 96)
	select {
	case <-changed:
	case <-time.After(2 * time.Second):
		t.Fatal("scheme-less artwork did not finish")
	}
	if _, ready := cache.Get("cover.png", 96); ready || !cache.Failed("cover.png", 96) {
		t.Fatalf("scheme-less artwork ready=%v, want a failed load", ready)
	}
	if got := requests.Load(); got != 0 {
		t.Fatalf("network requests = %d", got)
	}
}

func TestCacheLoadsLocalCoverFromBarePath(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Songs ? 100%")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "cover.png")
	img := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	for y := range 32 {
		for x := range 32 {
			img.SetNRGBA(x, y, color.NRGBA{R: 230, G: 110, B: 20, A: 0xff})
		}
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(file, img); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	changed := make(chan struct{}, 1)
	cache := New(Options{Workers: 1, Invalidate: func() {
		select {
		case changed <- struct{}{}:
		default:
		}
	}})
	defer cache.Close()
	cache.Palette(path)
	select {
	case <-changed:
	case <-time.After(2 * time.Second):
		t.Fatal("local cover did not load")
	}
	picked, ready := cache.Palette(path)
	if !ready || picked.Vibrant.A == 0 || picked.Vibrant.R < picked.Vibrant.B {
		t.Fatalf("local cover palette = %#v, ready=%v, want the orange cover", picked, ready)
	}
}

func TestCacheFailedTracksPermanentInputsAndCooldowns(t *testing.T) {
	changed := make(chan struct{}, 1)
	cache := New(Options{Workers: 1, Invalidate: func() {
		select {
		case changed <- struct{}{}:
		default:
		}
	}})
	if !cache.Failed("", 32) {
		t.Fatal("empty artwork URL was not reported as failed")
	}

	missing := filepath.Join(t.TempDir(), "missing.png")
	cache.Get(missing, 32)
	select {
	case <-changed:
	case <-time.After(2 * time.Second):
		t.Fatal("missing artwork did not finish loading")
	}
	if !cache.Failed(missing, 32) {
		t.Fatal("failed artwork URL was not reported during its cooldown")
	}

	cache.Close()
	if !cache.Failed("https://example.invalid/cover.png", 32) {
		t.Fatal("closed cache was not reported as failed")
	}
}
