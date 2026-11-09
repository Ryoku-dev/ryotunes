// Package art loads and bounds artwork away from Gio's frame goroutine.
package art

import (
	"container/list"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/draw"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gioui.org/op/paint"
	"github.com/ryoku-dev/ryotunes/internal/model"
	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

const (
	defaultMaxBytes   = 64 << 20
	defaultMaxSamples = 256
	defaultWorkers    = 4
	maxSourceBytes    = 32 << 20
	sampleLoadSize    = 64
	failureTTL        = 30 * time.Second
)

// Options configures a bounded artwork cache.
type Options struct {
	MaxBytes   int
	MaxSamples int
	Workers    int
	Timeout    time.Duration
	Invalidate func()
	HTTPClient *http.Client
}

// Cache loads artwork on a bounded worker pool and evicts least-recently-used pixels.
type Cache struct {
	ctx        context.Context
	cancel     context.CancelFunc
	client     *http.Client
	maxBytes   int
	maxSamples int
	invalidate func()

	mu             sync.Mutex
	entries        map[cacheKey]*cacheEntry
	pending        map[cacheKey]uint64
	pendingURL     map[string]int
	failed         map[cacheKey]time.Time
	failedURL      map[string]time.Time
	lru            list.List
	bytes          int
	samples        map[string]*sampleCacheEntry
	sampleLRU      list.List
	nextGeneration uint64
	jobs           chan loadJob
	workers        sync.WaitGroup
	placeholder    paint.ImageOp
}

type cacheKey struct {
	url  string
	size int
}

type cacheEntry struct {
	key   cacheKey
	op    paint.ImageOp
	bytes int
	lru   *list.Element
}

type sampleCacheEntry struct {
	url    string
	sample artworkSample
	lru    *list.Element
}

type loadJob struct {
	key        cacheKey
	generation uint64
}

// New returns an artwork cache with safe defaults for zero-valued options.
func New(options Options) *Cache {
	workers := options.Workers
	if workers <= 0 {
		workers = defaultWorkers
	}
	maxBytes := options.MaxBytes
	if maxBytes <= 0 {
		maxBytes = defaultMaxBytes
	}
	maxSamples := options.MaxSamples
	if maxSamples <= 0 {
		maxSamples = defaultMaxSamples
	}
	client := options.HTTPClient
	if client == nil {
		timeout := options.Timeout
		if timeout <= 0 {
			timeout = 10 * time.Second
		}
		client = &http.Client{Timeout: timeout}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cache := &Cache{
		ctx: ctx, cancel: cancel, client: client, maxBytes: maxBytes, maxSamples: maxSamples,
		invalidate: options.Invalidate, entries: make(map[cacheKey]*cacheEntry),
		samples: make(map[string]*sampleCacheEntry), pending: make(map[cacheKey]uint64),
		pendingURL: make(map[string]int), failed: make(map[cacheKey]time.Time),
		failedURL: make(map[string]time.Time), jobs: make(chan loadJob, workers*4),
		placeholder: Placeholder(),
	}
	cache.workers.Add(workers)
	for range workers {
		go cache.worker()
	}
	return cache
}

// Get returns cached artwork immediately or a placeholder while a worker loads it.
func (c *Cache) Get(rawURL string, size int) (paint.ImageOp, bool) {
	if size <= 0 {
		size = 1
	}
	key := cacheKey{url: rawURL, size: size}
	c.mu.Lock()
	if entry := c.entries[key]; entry != nil {
		c.lru.MoveToFront(entry.lru)
		op := entry.op
		c.mu.Unlock()
		return op, true
	}
	if rawURL == "" || c.ctx.Err() != nil || c.failureActiveLocked(key) {
		c.mu.Unlock()
		return c.placeholder, false
	}
	c.enqueueLocked(key)
	c.mu.Unlock()
	return c.placeholder, false
}

// Palette returns the cached cover palette and whether sampling has completed.
// On a miss it queues one small load, independent of whether any surface draws the cover.
func (c *Cache) Palette(rawURL string) (palette Palette, ready bool) {
	if rawURL == "" {
		return Palette{}, true
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if entry := c.samples[rawURL]; entry != nil {
		c.sampleLRU.MoveToFront(entry.lru)
		return entry.sample.palette, true
	}
	key := cacheKey{url: rawURL, size: sampleLoadSize}
	if c.ctx.Err() != nil || c.failureActiveLocked(key) {
		return Palette{}, true
	}
	c.enqueueLocked(key)
	return Palette{}, false
}

// Failed reports whether rawURL at size cannot load now.
func (c *Cache) Failed(rawURL string, size int) bool {
	if size <= 0 {
		size = 1
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return rawURL == "" || c.ctx.Err() != nil || c.failureActiveLocked(cacheKey{url: rawURL, size: size})
}

// Forget removes every cached, failed, and in-flight size and sample for one URL.
func (c *Cache) Forget(rawURL string) {
	c.mu.Lock()
	for key, entry := range c.entries {
		if key.url == rawURL {
			c.removeLocked(entry)
		}
	}
	for key := range c.pending {
		if key.url == rawURL {
			delete(c.pending, key)
		}
	}
	delete(c.pendingURL, rawURL)
	for key := range c.failed {
		if key.url == rawURL {
			delete(c.failed, key)
		}
	}
	delete(c.failedURL, rawURL)
	if entry := c.samples[rawURL]; entry != nil {
		delete(c.samples, rawURL)
		c.sampleLRU.Remove(entry.lru)
	}
	c.mu.Unlock()
}

// Close stops workers and discards future arrivals.
func (c *Cache) Close() {
	c.cancel()
	c.workers.Wait()
}

// Placeholder returns the static transparent artwork placeholder operation.
func Placeholder() paint.ImageOp {
	pixel := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	pixel.SetNRGBA(0, 0, color.NRGBA{})
	return paint.NewImageOp(pixel)
}

func (c *Cache) enqueueLocked(key cacheKey) bool {
	if key.url == "" || c.pendingURL[key.url] > 0 {
		return false
	}
	if _, loading := c.pending[key]; loading {
		return false
	}
	c.nextGeneration++
	generation := c.nextGeneration
	c.pending[key] = generation
	c.pendingURL[key.url]++
	select {
	case c.jobs <- loadJob{key: key, generation: generation}:
		return true
	default:
		delete(c.pending, key)
		delete(c.pendingURL, key.url)
		return false
	}
}

func (c *Cache) failureActiveLocked(key cacheKey) bool {
	if len(c.failed) == 0 && len(c.failedURL) == 0 {
		return false
	}
	now := time.Now()
	if until, failed := c.failedURL[key.url]; failed {
		if now.Before(until) {
			return true
		}
		delete(c.failedURL, key.url)
	}
	if until, failed := c.failed[key]; failed {
		if now.Before(until) {
			return true
		}
		delete(c.failed, key)
	}
	return false
}

func (c *Cache) needsSample(rawURL string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.samples[rawURL] == nil
}

// ParseCollage parses a ryotunes-collage marker into exactly four nonempty URLs.
func ParseCollage(rawURL string) ([4]string, bool) {
	var result [4]string
	if !strings.HasPrefix(rawURL, model.CollageThumbnailPrefix) {
		return result, false
	}
	var urls []string
	if json.Unmarshal([]byte(strings.TrimPrefix(rawURL, model.CollageThumbnailPrefix)), &urls) != nil || len(urls) != 4 {
		return result, false
	}
	for i, item := range urls {
		if item == "" {
			return [4]string{}, false
		}
		result[i] = item
	}
	return result, true
}

func (c *Cache) worker() {
	defer c.workers.Done()
	for {
		select {
		case <-c.ctx.Done():
			return
		case job := <-c.jobs:
			img, sample, err := c.load(c.ctx, job.key.url, job.key.size, c.needsSample(job.key.url))
			c.finish(job, img, sample, err)
		}
	}
}

func (c *Cache) finish(job loadJob, img image.Image, sample artworkSample, loadErr error) {
	c.mu.Lock()
	generation, current := c.pending[job.key]
	if !current || generation != job.generation {
		c.mu.Unlock()
		return
	}
	delete(c.pending, job.key)
	delete(c.pendingURL, job.key.url)
	if c.ctx.Err() != nil {
		c.mu.Unlock()
		return
	}
	invalidate := c.invalidate
	if loadErr != nil || img == nil {
		until := time.Now().Add(failureTTL)
		c.failed[job.key] = until
		c.failedURL[job.key.url] = until
		c.mu.Unlock()
		if invalidate != nil {
			invalidate()
		}
		return
	}
	delete(c.failedURL, job.key.url)
	for key := range c.failed {
		if key.url == job.key.url {
			delete(c.failed, key)
		}
	}
	if sample.ready {
		c.putSampleLocked(job.key.url, sample)
	}
	bounds := img.Bounds()
	byteSize := bounds.Dx() * bounds.Dy() * 4
	entry := &cacheEntry{key: job.key, op: paint.NewImageOp(img), bytes: byteSize}
	entry.lru = c.lru.PushFront(entry)
	c.entries[job.key] = entry
	c.bytes += byteSize
	for c.bytes > c.maxBytes && c.lru.Len() > 1 {
		oldest := c.lru.Back().Value.(*cacheEntry)
		c.removeLocked(oldest)
	}
	c.mu.Unlock()
	if invalidate != nil {
		invalidate()
	}
}

func (c *Cache) removeLocked(entry *cacheEntry) {
	delete(c.entries, entry.key)
	c.lru.Remove(entry.lru)
	c.bytes -= entry.bytes
}

func (c *Cache) putSampleLocked(rawURL string, sample artworkSample) {
	if rawURL == "" {
		return
	}
	if entry := c.samples[rawURL]; entry != nil {
		entry.sample = sample
		c.sampleLRU.MoveToFront(entry.lru)
		return
	}
	entry := &sampleCacheEntry{url: rawURL, sample: sample}
	entry.lru = c.sampleLRU.PushFront(entry)
	c.samples[rawURL] = entry
	for c.sampleLRU.Len() > c.maxSamples {
		oldest := c.sampleLRU.Back().Value.(*sampleCacheEntry)
		delete(c.samples, oldest.url)
		c.sampleLRU.Remove(oldest.lru)
	}
}

func (c *Cache) load(ctx context.Context, rawURL string, size int, needSample bool) (image.Image, artworkSample, error) {
	if urls, collage := ParseCollage(rawURL); collage {
		return c.loadCollage(ctx, urls, size, needSample)
	}
	if strings.HasPrefix(rawURL, model.CollageThumbnailPrefix) {
		return nil, artworkSample{}, errors.New("malformed artwork collage")
	}
	img, err := c.loadSource(ctx, rawURL)
	if err != nil {
		return nil, artworkSample{}, err
	}
	var sample artworkSample
	if needSample {
		sample = sampleArtwork(img)
	}
	return downscale(img, size), sample, nil
}

func (c *Cache) loadCollage(ctx context.Context, urls [4]string, size int, needSample bool) (image.Image, artworkSample, error) {
	if size < 2 {
		size = 2
	}
	canvas := image.NewNRGBA(image.Rect(0, 0, size, size))
	left := size / 2
	right := size - left
	top := size / 2
	bottom := size - top
	widths := [4]int{left, right, left, right}
	heights := [4]int{top, top, bottom, bottom}
	offsets := [4]image.Point{{}, {X: left}, {Y: top}, {X: left, Y: top}}
	for i, source := range urls {
		if strings.HasPrefix(source, model.CollageThumbnailPrefix) {
			return nil, artworkSample{}, errors.New("nested artwork collage")
		}
		img, err := c.loadSource(ctx, source)
		if err != nil {
			return nil, artworkSample{}, err
		}
		cell := cover(img, widths[i], heights[i])
		destination := image.Rectangle{Min: offsets[i], Max: offsets[i].Add(image.Pt(widths[i], heights[i]))}
		draw.Draw(canvas, destination, cell, cell.Bounds().Min, draw.Src)
	}
	if needSample {
		return canvas, sampleArtwork(canvas), nil
	}
	return canvas, artworkSample{}, nil
}

func (c *Cache) loadSource(ctx context.Context, rawURL string) (image.Image, error) {
	var reader io.ReadCloser
	if filepath.IsAbs(rawURL) {
		// the local provider hands out the cover path itself, which url.Parse would
		// mangle at the first '?' or '%'
		file, err := os.Open(rawURL)
		if err != nil {
			return nil, err
		}
		reader = file
	} else {
		parsed, err := url.Parse(rawURL)
		if err != nil {
			return nil, err
		}
		switch parsed.Scheme {
		case "file":
			if parsed.Host != "" && parsed.Host != "localhost" {
				return nil, errors.New("file artwork host is not local")
			}
			file, err := os.Open(parsed.Path)
			if err != nil {
				return nil, err
			}
			reader = file
		case "http", "https":
			request, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
			if err != nil {
				return nil, err
			}
			response, err := c.client.Do(request)
			if err != nil {
				return nil, err
			}
			if response.StatusCode < 200 || response.StatusCode >= 300 {
				_ = response.Body.Close()
				return nil, errors.New("artwork request failed: " + response.Status)
			}
			reader = response.Body
		default:
			return nil, errors.New("unsupported artwork URL")
		}
	}
	defer reader.Close()
	img, _, err := image.Decode(io.LimitReader(reader, maxSourceBytes+1))
	if err != nil {
		return nil, err
	}
	return img, nil
}

func downscale(source image.Image, size int) image.Image {
	bounds := source.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width <= size && height <= size {
		return source
	}
	scale := mathMin(float64(size)/float64(width), float64(size)/float64(height))
	newWidth := max(1, int(float64(width)*scale+0.5))
	newHeight := max(1, int(float64(height)*scale+0.5))
	destination := image.NewNRGBA(image.Rect(0, 0, newWidth, newHeight))
	xdraw.CatmullRom.Scale(destination, destination.Bounds(), source, bounds, xdraw.Src, nil)
	return destination
}

func cover(source image.Image, width, height int) image.Image {
	bounds := source.Bounds()
	scale := mathMax(float64(width)/float64(bounds.Dx()), float64(height)/float64(bounds.Dy()))
	scaledWidth := max(1, int(float64(bounds.Dx())*scale+0.5))
	scaledHeight := max(1, int(float64(bounds.Dy())*scale+0.5))
	scaled := image.NewNRGBA(image.Rect(0, 0, scaledWidth, scaledHeight))
	xdraw.CatmullRom.Scale(scaled, scaled.Bounds(), source, bounds, xdraw.Src, nil)
	left := max(0, (scaledWidth-width)/2)
	top := max(0, (scaledHeight-height)/2)
	cropped := image.NewNRGBA(image.Rect(0, 0, width, height))
	draw.Draw(cropped, cropped.Bounds(), scaled, image.Pt(left, top), draw.Src)
	return cropped
}

func mathMin(left, right float64) float64 {
	if left < right {
		return left
	}
	return right
}

func mathMax(left, right float64) float64 {
	if left > right {
		return left
	}
	return right
}
