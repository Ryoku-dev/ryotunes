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
	cache.Sample(path)
	select {
	case <-changed:
	case <-time.After(2 * time.Second):
		t.Fatal("local cover did not load")
	}
	sample, ready := cache.Sample(path)
	if !ready || sample.A == 0 || sample.R < sample.B {
		t.Fatalf("local cover sample = %#v, ready=%v, want the orange cover", sample, ready)
	}
}
