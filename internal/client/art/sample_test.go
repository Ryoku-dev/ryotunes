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

func TestSampleArtworkRejectsGreyImage(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 48, 48))
	for y := range 48 {
		for x := range 48 {
			img.SetNRGBA(x, y, color.NRGBA{R: 128, G: 128, B: 128, A: 0xff})
		}
	}
	if sample := sampleArtwork(img); sample.ok {
		t.Fatalf("grey artwork sampled %#v", sample.color)
	}
}

func TestSampleArtworkPicksHighestScoredBucket(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 48, 24))
	for y := range 24 {
		for x := range 48 {
			pixel := color.NRGBA{R: 180, G: 155, B: 155, A: 0xff}
			if x >= 24 {
				pixel = color.NRGBA{G: 100, B: 166, A: 0xff}
			}
			img.SetNRGBA(x, y, pixel)
		}
	}
	sample := sampleArtwork(img)
	if !sample.ok {
		t.Fatal("saturated artwork had no sample")
	}
	if sample.color.B <= sample.color.G || sample.color.B <= sample.color.R {
		t.Fatalf("sample = %#v, want the blue bucket", sample.color)
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
	if _, ready := cache.Sample(server.URL); ready {
		t.Fatal("sample was ready before artwork loaded")
	}
	select {
	case <-changed:
	case <-time.After(2 * time.Second):
		t.Fatal("artwork did not finish loading")
	}
	sample, ready := cache.Sample(server.URL)
	if !ready || sample.A == 0 {
		t.Fatalf("sample = %#v, ready=%v", sample, ready)
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
	if _, ready := cache.Sample(server.URL); ready {
		t.Fatal("failed sample was ready before its request")
	}
	cache.Get(server.URL, 96)
	select {
	case <-changed:
	case <-time.After(2 * time.Second):
		t.Fatal("failed artwork request did not finish")
	}
	for range 10_000 {
		cache.Sample(server.URL)
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
