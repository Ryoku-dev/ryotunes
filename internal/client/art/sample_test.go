package art

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestSampleArtworkRejectsGreyVibrantButKeepsDominant(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 48, 48))
	for y := range 48 {
		for x := range 48 {
			img.SetNRGBA(x, y, color.NRGBA{R: 128, G: 128, B: 128, A: 0xff})
		}
	}
	sample := sampleArtwork(img)
	if sample.palette.Vibrant.A != 0 {
		t.Fatalf("grey artwork vibrant = %#v", sample.palette.Vibrant)
	}
	if sample.palette.Dominant.A == 0 {
		t.Fatal("grey artwork did not produce a dominant colour")
	}
}

func TestSampleArtworkPicksVibrantAndDominantBuckets(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 48, 24))
	dominant := color.NRGBA{R: 28, G: 28, B: 30, A: 0xff}
	vibrant := color.NRGBA{R: 238, G: 82, B: 58, A: 0xff}
	for y := range 24 {
		for x := range 48 {
			pixel := dominant
			if x >= 36 {
				pixel = vibrant
			}
			img.SetNRGBA(x, y, pixel)
		}
	}
	sample := sampleArtwork(img)
	if sample.palette.Vibrant.R <= sample.palette.Vibrant.B {
		t.Fatalf("vibrant = %#v, want coral bucket", sample.palette.Vibrant)
	}
	if sample.palette.Dominant.B <= sample.palette.Dominant.R {
		t.Fatalf("dominant = %#v, want navy bucket", sample.palette.Dominant)
	}
}

func TestCacheSamplesDecodedArtworkWithoutAnotherDownload(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 48, 48))
	for y := range 48 {
		for x := range 48 {
			img.SetNRGBA(x, y, color.NRGBA{R: 30, G: 120, B: 210, A: 0xff})
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, img); err != nil {
		t.Fatal(err)
	}
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		response.Header().Set("Content-Type", "image/png")
		_, _ = response.Write(encoded.Bytes())
	}))
	defer server.Close()

	changed := make(chan struct{}, 1)
	cache := New(Options{Workers: 1, Invalidate: func() {
		select {
		case changed <- struct{}{}:
		default:
		}
	}})
	defer cache.Close()
	if _, ready := cache.Palette(server.URL); ready {
		t.Fatal("palette was ready before artwork loaded")
	}
	select {
	case <-changed:
	case <-time.After(2 * time.Second):
		t.Fatal("artwork did not finish loading")
	}
	picked, ready := cache.Palette(server.URL)
	if !ready || picked.Vibrant.A == 0 || picked.Dominant.A == 0 {
		t.Fatalf("palette = %#v, ready=%v", picked, ready)
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("artwork requests = %d, want 1", got)
	}
}

func TestCacheFailureCooldownStopsRequestAndInvalidationStorms(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		http.Error(response, "missing", http.StatusNotFound)
	}))
	defer server.Close()

	var invalidations atomic.Int32
	changed := make(chan struct{}, 1)
	cache := New(Options{Workers: 1, Invalidate: func() {
		invalidations.Add(1)
		select {
		case changed <- struct{}{}:
		default:
		}
	}})
	defer cache.Close()
	if _, ready := cache.Palette(server.URL); ready {
		t.Fatal("failed palette was ready before its request")
	}
	cache.Get(server.URL, 96)
	select {
	case <-changed:
	case <-time.After(2 * time.Second):
		t.Fatal("failed artwork request did not finish")
	}
	for range 10_000 {
		cache.Palette(server.URL)
		cache.Get(server.URL, 64)
		cache.Get(server.URL, 96)
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("artwork requests = %d, want 1 during failure cooldown", got)
	}
	if got := invalidations.Load(); got != 1 {
		t.Fatalf("invalidations = %d, want 1 during failure cooldown", got)
	}
}
