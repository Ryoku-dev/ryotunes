package art

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"
)

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
