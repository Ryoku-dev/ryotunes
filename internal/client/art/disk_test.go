package art

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ryoku-dev/ryotunes/internal/model"
)

func TestCacheLoadsArtworkAndPaletteFromDiskAfterRestart(t *testing.T) {
	encoded := testCoverPNG(t, false)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		_, _ = response.Write(encoded)
	}))
	defer server.Close()
	cacheDir := t.TempDir()

	first := New(Options{CacheDir: cacheDir, HTTPClient: server.Client()})
	waitForArtwork(t, first, server.URL, 128)
	first.Close()
	if got := requests.Load(); got != 1 {
		t.Fatalf("first cache requests = %d, want 1", got)
	}

	second := New(Options{CacheDir: cacheDir, HTTPClient: server.Client()})
	defer second.Close()
	waitForArtwork(t, second, server.URL, 128)
	waitForPalette(t, second, server.URL)
	if got := requests.Load(); got != 1 {
		t.Fatalf("requests after restart = %d, want disk hit with 1 total", got)
	}
}

func TestDiskCachePrunesOldestAccessFirst(t *testing.T) {
	directory := t.TempDir()
	oldest := filepath.Join(directory, "oldest.jpg")
	middle := filepath.Join(directory, "middle.jpg")
	newest := filepath.Join(directory, "newest.jpg")
	for _, path := range []string{oldest, middle, newest} {
		if err := os.WriteFile(path, bytes.Repeat([]byte(path), 16), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now()
	for index, path := range []string{oldest, middle, newest} {
		// Fetched in the opposite order to reads: pruning must follow reads.
		read := now.Add(time.Duration(index-3) * time.Hour)
		fetched := now.Add(time.Duration(-index) * time.Hour)
		if err := os.Chtimes(path, read, fetched); err != nil {
			t.Fatal(err)
		}
	}
	newestInfo, err := os.Stat(newest)
	if err != nil {
		t.Fatal(err)
	}
	limit := newestInfo.Size() * 6 / 5
	cache := New(Options{CacheDir: directory, DiskMaxBytes: limit})
	cache.pruneDiskCache()
	cache.Close()
	if _, err := os.Stat(newest); err != nil {
		t.Fatalf("most recently read cache entry was pruned: %v", err)
	}
	if _, err := os.Stat(oldest); !os.IsNotExist(err) {
		t.Fatalf("least recently read cache entry still exists: %v", err)
	}
	if got := directorySize(t, directory); got > limit-limit/10 {
		t.Fatalf("disk cache size = %d, want at most the low-water mark %d", got, limit-limit/10)
	}
}

func TestDiskWritesKeepRunningTotalAndPruneToLowWater(t *testing.T) {
	cacheDir := filepath.Join(t.TempDir(), "art")
	const limit = 60 << 10
	cache := New(Options{CacheDir: cacheDir, DiskMaxBytes: limit})
	defer cache.Close()
	img := decodeTestCover(t, 96)
	var written []cacheKey
	for crossed := false; !crossed; {
		if len(written) > 500 {
			t.Fatal("cache never crossed its bound")
		}
		before := len(written)
		sizeBefore := directorySizeIfAny(t, cacheDir)
		key := cacheKey{url: fmt.Sprintf("https://covers.invalid/%d.jpg", before), size: 96}
		if err := cache.writeDiskArtwork(key, img, artworkSample{}); err != nil {
			t.Fatal(err)
		}
		written = append(written, key)
		info, err := os.Stat(filepath.Join(cacheDir, diskArtworkName(key)+".jpg"))
		if err != nil {
			t.Fatal(err)
		}
		// judged by the sizes going in: by now the prune may already have answered a crossing
		if crossed = sizeBefore+info.Size() > limit; !crossed && countArtworkFiles(t, cacheDir) != before+1 {
			t.Fatalf("a write under the bound deleted entries: %d files after %d writes", countArtworkFiles(t, cacheDir), before+1)
		}
	}
	deadline := time.Now().Add(4 * time.Second)
	for directorySize(t, cacheDir) > limit-limit/10 {
		if time.Now().After(deadline) {
			t.Fatalf("disk cache size = %d, want pruned to %d", directorySize(t, cacheDir), limit-limit/10)
		}
		time.Sleep(time.Millisecond)
	}
	cache.pruneMu.Lock()
	counted := cache.diskBytes.Load()
	cache.pruneMu.Unlock()
	if actual := directorySize(t, cacheDir); counted != actual {
		t.Fatalf("running total = %d, directory holds %d", counted, actual)
	}
	last := written[len(written)-1]
	if _, err := os.Stat(filepath.Join(cacheDir, diskArtworkName(last)+".jpg")); err != nil {
		t.Fatalf("newest write was pruned: %v", err)
	}
}

func TestStaleTempFilesAreSweptByTheScan(t *testing.T) {
	directory := t.TempDir()
	stale := filepath.Join(directory, ".tmp-art-stale")
	live := filepath.Join(directory, ".tmp-art-live")
	for _, path := range []string{stale, live} {
		if err := os.WriteFile(path, []byte("partial"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(stale, old, old); err != nil {
		t.Fatal(err)
	}
	cache := New(Options{CacheDir: directory})
	cache.pruneDiskCache()
	cache.Close()
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("temp file left by a killed client survived: %v", err)
	}
	if _, err := os.Stat(live); err != nil {
		t.Fatalf("temp file of a write in progress was removed: %v", err)
	}
}

func TestCorruptDiskEntryIsDeletedAndRefetched(t *testing.T) {
	encoded := testCoverPNG(t, false)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		_, _ = response.Write(encoded)
	}))
	defer server.Close()
	cacheDir := t.TempDir()

	first := New(Options{CacheDir: cacheDir, HTTPClient: server.Client()})
	waitForArtwork(t, first, server.URL, 96)
	first.Close()
	artworkPath := findArtworkFile(t, cacheDir)
	if err := os.WriteFile(artworkPath, []byte("not an image"), 0o600); err != nil {
		t.Fatal(err)
	}

	second := New(Options{CacheDir: cacheDir, HTTPClient: server.Client()})
	defer second.Close()
	waitForArtwork(t, second, server.URL, 96)
	if got := requests.Load(); got != 2 {
		t.Fatalf("requests after corrupt entry = %d, want refetch", got)
	}
	file, err := os.Open(findArtworkFile(t, cacheDir))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if _, _, err := image.Decode(file); err != nil {
		t.Fatalf("replacement disk entry is unreadable: %v", err)
	}
}

func TestCurrentArtworkRequestMovesAheadOfStaleQueue(t *testing.T) {
	encoded := testCoverPNG(t, false)
	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	var mu sync.Mutex
	var order []string
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		mu.Lock()
		order = append(order, request.URL.Path)
		mu.Unlock()
		if request.URL.Path == "/block" {
			once.Do(func() { close(started) })
			<-release
		}
		_, _ = response.Write(encoded)
	}))
	defer server.Close()
	cache := New(Options{CacheDir: t.TempDir(), Workers: 1, DiskWorkers: 1, HTTPClient: server.Client()})
	defer cache.Close()

	cache.Get(server.URL+"/block", 96)
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("blocking request did not start")
	}
	current := server.URL + "/current"
	stale := server.URL + "/stale"
	cache.Get(current, 96)
	waitForNetworkQueue(t, cache, 1)
	cache.Get(stale, 96)
	waitForNetworkQueue(t, cache, 2)
	cache.Get(current, 96)
	close(release)
	waitForArtwork(t, cache, current, 96)
	waitForArtwork(t, cache, stale, 96)

	mu.Lock()
	got := append([]string(nil), order...)
	mu.Unlock()
	if !slices.Equal(got, []string{"/block", "/current", "/stale"}) {
		t.Fatalf("request order = %v", got)
	}
}

func TestLocalArtworkSkipsNetworkAndDiskCache(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cover.png")
	if err := os.WriteFile(path, testCoverPNG(t, true), 0o600); err != nil {
		t.Fatal(err)
	}
	var requests atomic.Int32
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		requests.Add(1)
		return nil, os.ErrInvalid
	})}
	cacheDir := t.TempDir()
	cache := New(Options{CacheDir: cacheDir, HTTPClient: client})
	defer cache.Close()
	waitForArtwork(t, cache, path, 96)
	if got := requests.Load(); got != 0 {
		t.Fatalf("network requests for local cover = %d", got)
	}
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".jpg") || strings.HasSuffix(entry.Name(), ".png") || strings.HasSuffix(entry.Name(), ".json") {
			t.Fatalf("local artwork wrote disk cache entry %q", entry.Name())
		}
	}
}

func TestScrolledPastRequestsAreDroppedBeforeFetch(t *testing.T) {
	var requests atomic.Int32
	release := make(chan struct{})
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		requests.Add(1)
		<-release
		return nil, os.ErrDeadlineExceeded
	})}
	cache := New(Options{CacheDir: t.TempDir(), HTTPClient: client})
	defer cache.Close()
	const rows = 3000
	for index := range rows {
		cache.Get(fmt.Sprintf("https://covers.invalid/%d.jpg", index), 96)
	}
	deadline := time.Now().Add(4 * time.Second)
	for requests.Load() < defaultWorkers {
		if time.Now().After(deadline) {
			t.Fatal("network workers did not start")
		}
		time.Sleep(time.Millisecond)
	}
	close(release)
	waitForNoPending(t, cache)
	fetched := requests.Load()
	if fetched > staleRequests+2*defaultWorkers {
		t.Fatalf("a fling past %d rows fetched %d covers, want at most about %d", rows, fetched, staleRequests)
	}
	again := ""
	for index := range rows {
		rawURL := fmt.Sprintf("https://covers.invalid/%d.jpg", index)
		if !cache.Failed(rawURL, 96) {
			again = rawURL
			break
		}
	}
	if again == "" {
		t.Fatal("every row was fetched")
	}
	cache.Get(again, 96)
	waitForNoPending(t, cache)
	if requests.Load() != fetched+1 {
		t.Fatalf("a dropped row requested again was not fetched: %d requests, want %d", requests.Load(), fetched+1)
	}
}

func TestPaletteWhileCoverInFlightQueuesNoJobs(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		once.Do(func() { close(started) })
		<-release
		return nil, os.ErrDeadlineExceeded
	})}
	cache := New(Options{CacheDir: t.TempDir(), HTTPClient: client})
	defer cache.Close()
	defer close(release)
	rawURL := "https://covers.invalid/now-playing.jpg"
	cache.Get(rawURL, 96)
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("cover fetch did not start")
	}
	for range 60 {
		if _, ready := cache.Palette(rawURL); ready {
			t.Fatal("palette ready before its cover")
		}
		time.Sleep(time.Millisecond)
	}
	cache.mu.Lock()
	jobs := cache.nextGeneration
	cache.mu.Unlock()
	if jobs != 1 {
		t.Fatalf("60 palette frames during one cover fetch made %d jobs, want only the cover's", jobs)
	}
}

func TestOldDiskCoverIsShownThenRefreshedInBackground(t *testing.T) {
	orange := solidCoverPNG(t, color.NRGBA{R: 235, G: 120, B: 20, A: 255})
	blue := solidCoverPNG(t, color.NRGBA{R: 20, G: 80, B: 235, A: 255})
	var served atomic.Pointer[[]byte]
	served.Store(&orange)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		body := served.Load()
		if body == &blue {
			// Slow enough to see the old cover from disk first.
			time.Sleep(200 * time.Millisecond)
		}
		_, _ = response.Write(*body)
	}))
	defer server.Close()
	cacheDir := t.TempDir()
	first := New(Options{CacheDir: cacheDir, HTTPClient: server.Client()})
	waitForArtwork(t, first, server.URL, 96)
	waitForPalette(t, first, server.URL)
	first.Close()
	served.Store(&blue)
	artwork := findArtworkFile(t, cacheDir)

	young := time.Now().Add(-10 * 24 * time.Hour)
	if err := os.Chtimes(artwork, young, young); err != nil {
		t.Fatal(err)
	}
	second := New(Options{CacheDir: cacheDir, HTTPClient: server.Client()})
	waitForArtwork(t, second, server.URL, 96)
	time.Sleep(100 * time.Millisecond)
	second.Close()
	if got := requests.Load(); got != 1 {
		t.Fatalf("a 10 day old cover made %d requests, want none after the first", got-1)
	}
	if info, err := os.Stat(artwork); err != nil || info.ModTime().Sub(young).Abs() > time.Second {
		t.Fatalf("a disk hit moved the fetch time: %v", err)
	}

	old := time.Now().Add(-40 * 24 * time.Hour)
	if err := os.Chtimes(artwork, old, old); err != nil {
		t.Fatal(err)
	}
	third := New(Options{CacheDir: cacheDir, HTTPClient: server.Client()})
	waitForArtwork(t, third, server.URL, 96)
	if palette := waitForPalette(t, third, server.URL); palette.Vibrant.R < palette.Vibrant.B {
		t.Fatal("old cover was not shown from disk first")
	}
	deadline := time.Now().Add(4 * time.Second)
	for {
		palette, _ := third.Palette(server.URL)
		if palette.Vibrant.B > palette.Vibrant.R {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("old cover was not refreshed: %d requests", requests.Load())
		}
		time.Sleep(time.Millisecond)
	}
	third.Close()
	if got := requests.Load(); got != 2 {
		t.Fatalf("requests = %d, want one refresh", got)
	}

	fourth := New(Options{CacheDir: cacheDir, HTTPClient: server.Client()})
	defer fourth.Close()
	waitForArtwork(t, fourth, server.URL, 96)
	if palette := waitForPalette(t, fourth, server.URL); palette.Vibrant.B < palette.Vibrant.R {
		t.Fatal("refreshed cover was not written to disk")
	}
	if got := requests.Load(); got != 2 {
		t.Fatalf("a refreshed cover was fetched again: %d requests", got)
	}
}

func TestClearAndForgetRemoveDiskCovers(t *testing.T) {
	encoded := testCoverPNG(t, false)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		_, _ = response.Write(encoded)
	}))
	defer server.Close()
	cacheDir := filepath.Join(t.TempDir(), "art")
	cache := New(Options{CacheDir: cacheDir, HTTPClient: server.Client()})
	waitForArtwork(t, cache, server.URL, 96)
	waitForPalette(t, cache, server.URL)
	waitForArtworkFile(t, cacheDir)

	cache.Clear()
	cache.mu.Lock()
	inMemory := len(cache.entries) + len(cache.samples)
	cache.mu.Unlock()
	if inMemory != 0 {
		t.Fatalf("%d covers and palettes still in memory after Clear", inMemory)
	}
	deadline := time.Now().Add(4 * time.Second)
	for {
		parent, err := os.ReadDir(filepath.Dir(cacheDir))
		if err != nil {
			t.Fatal(err)
		}
		if !cache.clearDisk.Load() && len(parent) == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("disk clear did not finish: %d entries remain", len(parent))
		}
		time.Sleep(time.Millisecond)
	}
	waitForArtwork(t, cache, server.URL, 96)
	if got := requests.Load(); got != 2 {
		t.Fatalf("requests after Clear = %d, want a refetch", got)
	}

	waitForArtworkFile(t, cacheDir)
	cache.Forget(server.URL)
	cache.Close()
	if count := countArtworkFiles(t, cacheDir); count != 0 {
		t.Fatalf("Forget left %d disk entries", count)
	}
	if _, err := os.Stat(filepath.Join(cacheDir, diskPaletteName(server.URL))); !os.IsNotExist(err) {
		t.Fatalf("Forget left the palette file: %v", err)
	}
	next := New(Options{CacheDir: cacheDir, HTTPClient: server.Client()})
	defer next.Close()
	waitForArtwork(t, next, server.URL, 96)
	if got := requests.Load(); got != 3 {
		t.Fatalf("requests after Forget = %d, want a refetch", got)
	}
}

func TestClearOrForgetDuringACoverWriteLeavesNothingOnDisk(t *testing.T) {
	// a large noisy cover keeps the JPEG encode busy after it is shown
	img := image.NewNRGBA(image.Rect(0, 0, 2048, 2048))
	for i := range img.Pix {
		img.Pix[i] = uint8(i*7919 + i/2048*104729)
	}
	for i := 3; i < len(img.Pix); i += 4 {
		img.Pix[i] = 255
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, img); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		_, _ = response.Write(encoded.Bytes())
	}))
	defer server.Close()
	for _, test := range []struct {
		name string
		drop func(cache *Cache, url string)
	}{
		{"clear", func(cache *Cache, _ string) { cache.Clear() }},
		{"forget", func(cache *Cache, url string) { cache.Forget(url) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			cacheDir := filepath.Join(t.TempDir(), "art")
			cache := New(Options{CacheDir: cacheDir, HTTPClient: server.Client()})
			waitForArtwork(t, cache, server.URL, 2048)
			test.drop(cache, server.URL)
			cache.Close()
			if count := countArtworkFiles(t, cacheDir); count != 0 {
				t.Fatalf("a cover shown before %s was written back: %d files", test.name, count)
			}
			if _, err := os.Stat(filepath.Join(cacheDir, diskPaletteName(server.URL))); !os.IsNotExist(err) {
				t.Fatalf("its palette was written back after %s: %v", test.name, err)
			}
		})
	}
}

func TestUnreadableDiskEntryIsKept(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads mode 000 files")
	}
	encoded := testCoverPNG(t, false)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		_, _ = response.Write(encoded)
	}))
	cacheDir := t.TempDir()
	first := New(Options{CacheDir: cacheDir, HTTPClient: server.Client()})
	waitForArtwork(t, first, server.URL, 96)
	first.Close()
	server.Close()
	artwork := findArtworkFile(t, cacheDir)
	if err := os.Chmod(artwork, 0); err != nil {
		t.Fatal(err)
	}
	offline := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, os.ErrDeadlineExceeded
	})}
	changed := make(chan struct{}, 1)
	second := New(Options{CacheDir: cacheDir, HTTPClient: offline, Invalidate: func() {
		select {
		case changed <- struct{}{}:
		default:
		}
	}})
	second.Get(server.URL, 96)
	select {
	case <-changed:
	case <-time.After(2 * time.Second):
		t.Fatal("unreadable entry did not finish loading")
	}
	second.Close()
	if err := os.Chmod(artwork, 0o600); err != nil {
		t.Fatalf("unreadable cache entry was deleted: %v", err)
	}
	third := New(Options{CacheDir: cacheDir, HTTPClient: offline})
	defer third.Close()
	waitForArtwork(t, third, server.URL, 96)
}

func TestLocalCollageSkipsDiskCache(t *testing.T) {
	dir := t.TempDir()
	sources := make([]string, 4)
	for index := range sources {
		sources[index] = filepath.Join(dir, fmt.Sprintf("cover-%d.png", index))
		if err := os.WriteFile(sources[index], testCoverPNG(t, false), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	marker, err := json.Marshal(sources)
	if err != nil {
		t.Fatal(err)
	}
	collage := model.CollageThumbnailPrefix + string(marker)
	var requests atomic.Int32
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		requests.Add(1)
		return nil, os.ErrInvalid
	})}
	cacheDir := t.TempDir()
	cache := New(Options{CacheDir: cacheDir, HTTPClient: client})
	defer cache.Close()
	waitForArtwork(t, cache, collage, 96)
	waitForPalette(t, cache, collage)
	if count := countArtworkFiles(t, cacheDir); count != 0 || requests.Load() != 0 {
		t.Fatalf("local collage wrote %d disk entries and made %d requests", count, requests.Load())
	}
	if _, err := os.Stat(filepath.Join(cacheDir, diskPaletteName(collage))); !os.IsNotExist(err) {
		t.Fatalf("local collage wrote a palette file: %v", err)
	}
}

func TestHasTransparencyDoesNotAllocatePerPixel(t *testing.T) {
	opaque := image.NewNRGBA(image.Rect(0, 0, 512, 512))
	for index := 3; index < len(opaque.Pix); index += 4 {
		opaque.Pix[index] = 0xff
	}
	photo := image.NewYCbCr(image.Rect(0, 0, 512, 512), image.YCbCrSubsampleRatio420)
	if allocations := testing.AllocsPerRun(5, func() {
		if hasTransparency(opaque) || hasTransparency(photo) {
			t.Fatal("opaque cover reported transparent")
		}
	}); allocations != 0 {
		t.Fatalf("hasTransparency allocations = %v, want 0", allocations)
	}
	opaque.Pix[len(opaque.Pix)-1] = 0x80
	if !hasTransparency(opaque) {
		t.Fatal("translucent pixel was missed")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func testCoverPNG(t *testing.T, transparent bool) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 320, 320))
	for y := range 320 {
		for x := range 320 {
			alpha := uint8(255)
			if transparent && (x+y)%13 == 0 {
				alpha = 80
			}
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x*17 + y*3), G: uint8(x*5 + y*11), B: uint8(x*7 + y*19), A: alpha})
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, img); err != nil {
		t.Fatal(err)
	}
	return encoded.Bytes()
}

func waitForArtwork(t *testing.T, cache *Cache, rawURL string, size int) {
	t.Helper()
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		if _, ready := cache.Get(rawURL, size); ready {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("artwork %q did not become ready", rawURL)
}

func waitForPalette(t *testing.T, cache *Cache, rawURL string) Palette {
	t.Helper()
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		if palette, ready := cache.Palette(rawURL); ready {
			return palette
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("palette %q did not become ready", rawURL)
	return Palette{}
}

func waitForNetworkQueue(t *testing.T, cache *Cache, count int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		cache.networkJobs.mu.Lock()
		length := len(cache.networkJobs.jobs)
		cache.networkJobs.mu.Unlock()
		if length >= count {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("network queue did not reach %d jobs", count)
}

// findArtworkFile waits for the cover a worker writes once it is shown.
func findArtworkFile(t *testing.T, directory string) string {
	t.Helper()
	waitForArtworkFile(t, directory)
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".jpg") || strings.HasSuffix(entry.Name(), ".png") {
			return filepath.Join(directory, entry.Name())
		}
	}
	t.Fatal("disk artwork entry not found")
	return ""
}

func directorySize(t *testing.T, directory string) int64 {
	t.Helper()
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	var size int64
	for _, entry := range entries {
		info, err := entry.Info()
		if os.IsNotExist(err) {
			// the prune worker removed it between the listing and the stat
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().IsRegular() {
			size += info.Size()
		}
	}
	return size
}

func solidCoverPNG(t *testing.T, fill color.NRGBA) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 160, 160))
	for y := range 160 {
		for x := range 160 {
			img.SetNRGBA(x, y, fill)
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, img); err != nil {
		t.Fatal(err)
	}
	return encoded.Bytes()
}

func decodeTestCover(t *testing.T, size int) image.Image {
	t.Helper()
	img, _, err := image.Decode(bytes.NewReader(testCoverPNG(t, false)))
	if err != nil {
		t.Fatal(err)
	}
	return downscale(img, size)
}

func waitForNoPending(t *testing.T, cache *Cache) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		cache.mu.Lock()
		pending := len(cache.pending)
		cache.mu.Unlock()
		if pending == 0 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("artwork jobs did not drain")
}

func waitForArtworkFile(t *testing.T, directory string) {
	t.Helper()
	deadline := time.Now().Add(4 * time.Second)
	for countArtworkFiles(t, directory) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("artwork was not written to disk")
		}
		time.Sleep(time.Millisecond)
	}
}

func countArtworkFiles(t *testing.T, directory string) int {
	t.Helper()
	entries, err := os.ReadDir(directory)
	if os.IsNotExist(err) {
		return 0
	}
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".jpg") || strings.HasSuffix(entry.Name(), ".png") {
			count++
		}
	}
	return count
}

func directorySizeIfAny(t *testing.T, directory string) int64 {
	t.Helper()
	if _, err := os.Stat(directory); os.IsNotExist(err) {
		return 0
	}
	return directorySize(t, directory)
}
