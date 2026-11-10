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
	"sync/atomic"
	"time"

	"gioui.org/op/paint"
	"github.com/ryoku-dev/ryotunes/internal/model"
	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

const (
	defaultMaxBytes    = 64 << 20
	defaultMaxSamples  = 256
	defaultWorkers     = 12
	defaultDiskWorkers = 4
	maxSourceBytes     = 32 << 20
	sampleLoadSize     = 64
	failureTTL         = 30 * time.Second
	// staleRequests is how many newer requests a queued job may fall behind
	// before a worker drops it. Rows on screen are requested every frame, so
	// only rows that scrolled away fall this far behind.
	staleRequests = 512
)

// requestSizes is the one ladder of cover sizes. Rounding every request onto
// it bounds the number of decoded copies and disk entries per cover.
var requestSizes = [...]int{32, 48, 64, 96, 128, 192, 256, 384, 512, 768, 1024, 1536, 2048}

// RequestSize rounds a cover size in pixels up to the cache's size ladder.
func RequestSize(size int) int {
	for _, candidate := range requestSizes {
		if size <= candidate {
			return candidate
		}
	}
	return requestSizes[len(requestSizes)-1]
}

// Options configures a bounded artwork cache.
type Options struct {
	MaxBytes     int
	MaxSamples   int
	Workers      int
	DiskWorkers  int
	DiskMaxBytes int64
	CacheDir     string
	Timeout      time.Duration
	Invalidate   func()
	HTTPClient   *http.Client
}

// Cache loads artwork on bounded disk and network worker pools.
type Cache struct {
	ctx          context.Context
	cancel       context.CancelFunc
	client       *http.Client
	maxBytes     int
	maxSamples   int
	diskDir      string
	diskMaxBytes int64
	invalidate   func()

	mu             sync.Mutex
	entries        map[cacheKey]*cacheEntry
	pending        map[cacheKey]*loadJob
	pendingURL     map[string]*loadJob
	samplePending  map[string]*loadJob
	refreshing     map[cacheKey]*loadJob
	failed         map[cacheKey]time.Time
	failedURL      map[string]time.Time
	lru            list.List
	bytes          int
	samples        map[string]*sampleCacheEntry
	sampleLRU      list.List
	nextGeneration uint64
	nextPriority   uint64
	diskJobs       *jobQueue
	networkJobs    *jobQueue
	ownsClient     bool
	prune          chan struct{}
	pruneMu        sync.Mutex
	diskScanned    bool
	diskBytes      atomic.Int64
	clearDisk      atomic.Bool
	diskEpoch      atomic.Uint64
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

// New returns an artwork cache with safe defaults for zero-valued options.
func New(options Options) *Cache {
	networkWorkers := options.Workers
	if networkWorkers <= 0 {
		networkWorkers = defaultWorkers
	}
	diskWorkers := options.DiskWorkers
	if diskWorkers <= 0 {
		diskWorkers = defaultDiskWorkers
	}
	maxBytes := options.MaxBytes
	if maxBytes <= 0 {
		maxBytes = defaultMaxBytes
	}
	maxSamples := options.MaxSamples
	if maxSamples <= 0 {
		maxSamples = defaultMaxSamples
	}
	diskMaxBytes := options.DiskMaxBytes
	if diskMaxBytes <= 0 {
		diskMaxBytes = defaultDiskMaxBytes
	}
	client := options.HTTPClient
	ownsClient := client == nil
	if ownsClient {
		timeout := options.Timeout
		if timeout <= 0 {
			timeout = 10 * time.Second
		}
		// The default of two idle connections per host makes most parallel
		// fetches to one cover host reconnect.
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.MaxIdleConnsPerHost = networkWorkers
		client = &http.Client{Timeout: timeout, Transport: transport}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cache := &Cache{
		ctx: ctx, cancel: cancel, client: client, ownsClient: ownsClient, maxBytes: maxBytes, maxSamples: maxSamples,
		diskDir: options.CacheDir, diskMaxBytes: diskMaxBytes, invalidate: options.Invalidate,
		entries: make(map[cacheKey]*cacheEntry), samples: make(map[string]*sampleCacheEntry),
		pending: make(map[cacheKey]*loadJob), pendingURL: make(map[string]*loadJob),
		samplePending: make(map[string]*loadJob), refreshing: make(map[cacheKey]*loadJob),
		failed:    make(map[cacheKey]time.Time),
		failedURL: make(map[string]time.Time), diskJobs: newJobQueue(), networkJobs: newJobQueue(),
		prune: make(chan struct{}, 1), placeholder: Placeholder(),
	}
	cache.workers.Add(diskWorkers + networkWorkers)
	for range diskWorkers {
		go cache.diskWorker()
	}
	for range networkWorkers {
		go cache.networkWorker()
	}
	if cache.diskDir != "" {
		cache.workers.Add(1)
		go cache.pruneWorker()
	}
	return cache
}

// Get returns cached artwork immediately or a placeholder while a worker loads
// it. size is rounded with RequestSize.
func (c *Cache) Get(rawURL string, size int) (paint.ImageOp, bool) {
	key := cacheKey{url: rawURL, size: RequestSize(size)}
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
	c.enqueueArtworkLocked(key)
	c.mu.Unlock()
	return c.placeholder, false
}

// Palette returns the cached cover palette and whether sampling has completed.
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
	priority := c.nextPriorityLocked()
	if job := c.samplePending[rawURL]; job != nil {
		c.promoteJobLocked(job, priority)
		return Palette{}, false
	}
	// The cover load in flight samples the palette and invalidates when done.
	if job := c.pendingURL[rawURL]; job != nil {
		c.promoteJobLocked(job, priority)
		return Palette{}, false
	}
	if c.diskDir != "" && persistArtwork(rawURL) {
		job := c.newJobLocked(paletteJob, key, priority)
		c.samplePending[rawURL] = job
		if !c.diskJobs.push(job) {
			delete(c.samplePending, rawURL)
		}
		return Palette{}, false
	}
	c.enqueueArtworkLocked(key)
	return Palette{}, false
}

// Failed reports whether rawURL at size cannot load now.
func (c *Cache) Failed(rawURL string, size int) bool {
	key := cacheKey{url: rawURL, size: RequestSize(size)}
	c.mu.Lock()
	defer c.mu.Unlock()
	return rawURL == "" || c.ctx.Err() != nil || c.failureActiveLocked(key)
}

// Forget removes every cached, failed, and in-flight size and sample for one
// URL, on disk too, so the next request fetches it again.
func (c *Cache) Forget(rawURL string) {
	c.diskEpoch.Add(1)
	if c.diskDir != "" {
		for _, size := range requestSizes {
			c.removeDiskArtwork(cacheKey{url: rawURL, size: size})
		}
		c.removeDiskPalette(rawURL)
	}
	c.mu.Lock()
	for key, entry := range c.entries {
		if key.url == rawURL {
			c.removeLocked(entry)
		}
	}
	for key, job := range c.pending {
		if key.url == rawURL {
			delete(c.pending, key)
			if c.pendingURL[rawURL] == job {
				delete(c.pendingURL, rawURL)
			}
		}
	}
	delete(c.samplePending, rawURL)
	for key := range c.refreshing {
		if key.url == rawURL {
			delete(c.refreshing, key)
		}
	}
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

// Clear drops every cover and palette from memory and from the disk cache, so
// each one is fetched again. The disk entries are removed in the background
// and are not read in the meantime.
func (c *Cache) Clear() {
	c.diskEpoch.Add(1)
	if c.diskDir != "" {
		c.clearDisk.Store(true)
	}
	c.mu.Lock()
	clear(c.entries)
	c.lru.Init()
	c.bytes = 0
	clear(c.samples)
	c.sampleLRU.Init()
	clear(c.pending)
	clear(c.pendingURL)
	clear(c.samplePending)
	clear(c.refreshing)
	clear(c.failed)
	clear(c.failedURL)
	invalidate := c.invalidate
	c.mu.Unlock()
	c.requestPrune()
	if invalidate != nil {
		invalidate()
	}
}

// Close stops workers and discards future arrivals.
func (c *Cache) Close() {
	c.cancel()
	c.diskJobs.close()
	c.networkJobs.close()
	c.workers.Wait()
	if c.ownsClient {
		c.client.CloseIdleConnections()
	}
}

// Placeholder returns the static transparent artwork placeholder operation.
func Placeholder() paint.ImageOp {
	pixel := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	pixel.SetNRGBA(0, 0, color.NRGBA{})
	return paint.NewImageOp(pixel)
}

func (c *Cache) nextPriorityLocked() uint64 {
	c.nextPriority++
	return c.nextPriority
}

func (c *Cache) newJobLocked(kind jobKind, key cacheKey, priority uint64) *loadJob {
	c.nextGeneration++
	job := &loadJob{kind: kind, key: key, generation: c.nextGeneration, priority: priority}
	job.requested.Store(priority)
	return job
}

func (c *Cache) promoteJobLocked(job *loadJob, priority uint64) {
	job.requested.Store(priority)
	c.diskJobs.promote(job, priority)
	c.networkJobs.promote(job, priority)
}

func (c *Cache) enqueueArtworkLocked(key cacheKey) bool {
	priority := c.nextPriorityLocked()
	if job := c.pending[key]; job != nil {
		c.promoteJobLocked(job, priority)
		return false
	}
	if job := c.pendingURL[key.url]; job != nil {
		c.promoteJobLocked(job, priority)
		return false
	}
	job := c.newJobLocked(artworkJob, key, priority)
	c.pending[key] = job
	c.pendingURL[key.url] = job
	if c.diskJobs.push(job) {
		return true
	}
	delete(c.pending, key)
	delete(c.pendingURL, key.url)
	return false
}

// dropIfStale reports whether a popped job should not run: it was forgotten or
// cleared, or its row has not been asked for in staleRequests requests. A
// dropped artwork job is queued again by the next Get.
func (c *Cache) dropIfStale(job *loadJob) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	switch job.kind {
	case paletteJob:
		if c.samplePending[job.key.url] != job {
			return true
		}
	case refreshJob:
		if c.refreshing[job.key] != job {
			return true
		}
	default:
		if c.pending[job.key] != job {
			return true
		}
	}
	if c.nextPriority-job.requested.Load() <= staleRequests {
		return false
	}
	switch job.kind {
	case paletteJob:
		delete(c.samplePending, job.key.url)
	case refreshJob:
		delete(c.refreshing, job.key)
	default:
		delete(c.pending, job.key)
		if c.pendingURL[job.key.url] == job {
			delete(c.pendingURL, job.key.url)
		}
	}
	return true
}

// refresh queues a background refetch of a cover just served from an old disk
// entry. The old cover stays on screen until the new one arrives.
func (c *Cache) refresh(job *loadJob) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ctx.Err() != nil || c.refreshing[job.key] != nil || c.entries[job.key] == nil {
		return
	}
	refresh := c.newJobLocked(refreshJob, job.key, job.requested.Load())
	c.refreshing[job.key] = refresh
	if !c.networkJobs.push(refresh) {
		delete(c.refreshing, job.key)
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

func (c *Cache) diskWorker() {
	defer c.workers.Done()
	for {
		job, ok := c.diskJobs.pop(c.ctx)
		if !ok {
			return
		}
		if c.dropIfStale(job) {
			continue
		}
		if job.kind == paletteJob {
			c.loadPaletteFromDisk(job)
			continue
		}
		if isLocalArtwork(job.key.url) {
			img, sample, err := c.load(c.ctx, job.key.url, job.key.size, c.needsSample(job.key.url))
			c.finish(job, img, sample, err)
			continue
		}
		if img, sample, hit, old := c.readDiskArtwork(job.key); hit {
			c.finish(job, img, sample, nil)
			if old {
				c.refresh(job)
			}
			continue
		}
		if !c.networkJobs.push(job) {
			c.finish(job, nil, artworkSample{}, c.ctx.Err())
		}
	}
}

func (c *Cache) networkWorker() {
	defer c.workers.Done()
	for {
		job, ok := c.networkJobs.pop(c.ctx)
		if !ok {
			return
		}
		if c.dropIfStale(job) {
			continue
		}
		// A refresh always resamples: the cover may have changed.
		needSample := job.kind == refreshJob || c.needsSample(job.key.url)
		img, sample, err := c.load(c.ctx, job.key.url, job.key.size, needSample)
		epoch := c.diskEpoch.Load()
		var shown bool
		if job.kind == refreshJob {
			shown = c.finishRefresh(job, img, sample, err)
		} else {
			shown = c.finish(job, img, sample, err)
		}
		// written after it is shown: encoding a large cover takes long enough to see
		if shown {
			if diskErr := c.writeDiskArtwork(job.key, img, sample); diskErr != nil {
				c.removeDiskArtwork(job.key)
			}
			// a Clear or Forget that came after the cover was shown must not find it written back
			if c.diskEpoch.Load() != epoch {
				c.removeDiskArtwork(job.key)
				c.removeDiskPalette(job.key.url)
			}
		}
	}
}

func (c *Cache) loadPaletteFromDisk(job *loadJob) {
	sample, hit := c.readDiskPalette(job.key.url)
	c.mu.Lock()
	if c.samplePending[job.key.url] != job {
		c.mu.Unlock()
		return
	}
	delete(c.samplePending, job.key.url)
	if c.ctx.Err() != nil {
		c.mu.Unlock()
		return
	}
	if hit {
		c.putSampleLocked(job.key.url, sample)
		invalidate := c.invalidate
		c.mu.Unlock()
		if invalidate != nil {
			invalidate()
		}
		return
	}
	if c.pendingURL[job.key.url] == nil {
		c.enqueueArtworkLocked(job.key)
	}
	c.mu.Unlock()
}

// finish takes a loaded cover in and reports whether it was, so a cover forgotten meanwhile is not kept.
func (c *Cache) finish(job *loadJob, img image.Image, sample artworkSample, loadErr error) bool {
	c.mu.Lock()
	if c.pending[job.key] != job {
		c.mu.Unlock()
		return false
	}
	delete(c.pending, job.key)
	if c.pendingURL[job.key.url] == job {
		delete(c.pendingURL, job.key.url)
	}
	if c.ctx.Err() != nil {
		c.mu.Unlock()
		return false
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
		return false
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
	c.storeLocked(job.key, img)
	c.mu.Unlock()
	if invalidate != nil {
		invalidate()
	}
	return true
}

// finishRefresh swaps a refreshed cover in and reports whether it did. A failed refresh keeps the old one.
func (c *Cache) finishRefresh(job *loadJob, img image.Image, sample artworkSample, loadErr error) bool {
	c.mu.Lock()
	if c.refreshing[job.key] != job {
		c.mu.Unlock()
		return false
	}
	delete(c.refreshing, job.key)
	if c.ctx.Err() != nil || loadErr != nil || img == nil || c.entries[job.key] == nil {
		c.mu.Unlock()
		return false
	}
	if sample.ready {
		c.putSampleLocked(job.key.url, sample)
	}
	c.storeLocked(job.key, img)
	invalidate := c.invalidate
	c.mu.Unlock()
	if invalidate != nil {
		invalidate()
	}
	return true
}

func (c *Cache) storeLocked(key cacheKey, img image.Image) {
	bounds := img.Bounds()
	byteSize := bounds.Dx() * bounds.Dy() * 4
	if entry := c.entries[key]; entry != nil {
		c.bytes += byteSize - entry.bytes
		entry.op, entry.bytes = paint.NewImageOp(img), byteSize
		c.lru.MoveToFront(entry.lru)
	} else {
		entry := &cacheEntry{key: key, op: paint.NewImageOp(img), bytes: byteSize}
		entry.lru = c.lru.PushFront(entry)
		c.entries[key] = entry
		c.bytes += byteSize
	}
	for c.bytes > c.maxBytes && c.lru.Len() > 1 {
		oldest := c.lru.Back().Value.(*cacheEntry)
		c.removeLocked(oldest)
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
	path, local, err := localArtworkPath(rawURL)
	if err != nil {
		return nil, err
	}
	if local {
		file, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		reader = file
	} else {
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
	}
	defer reader.Close()
	img, _, err := image.Decode(io.LimitReader(reader, maxSourceBytes+1))
	if err != nil {
		return nil, err
	}
	return img, nil
}

// isLocalArtwork reports a cover made only from local files. It loads on a
// disk worker and skips the disk cache: the files are already on disk, and
// caching would keep showing a cover file the user has since replaced.
func isLocalArtwork(rawURL string) bool {
	if urls, collage := ParseCollage(rawURL); collage {
		for _, source := range urls {
			if !isLocalSource(source) {
				return false
			}
		}
		return true
	}
	return isLocalSource(rawURL)
}

func isLocalSource(rawURL string) bool {
	_, local, err := localArtworkPath(rawURL)
	return err == nil && local
}

func localArtworkPath(rawURL string) (string, bool, error) {
	if filepath.IsAbs(rawURL) {
		return rawURL, true, nil
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", false, err
	}
	switch parsed.Scheme {
	case "file":
		if parsed.Host != "" && parsed.Host != "localhost" {
			return "", false, errors.New("file artwork host is not local")
		}
		return parsed.Path, true, nil
	case "http", "https":
		return "", false, nil
	default:
		return "", false, errors.New("unsupported artwork URL")
	}
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
