package art

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	defaultDiskMaxBytes int64 = 256 << 20
	// diskRefreshAge is how old a disk entry may get before a hit also refetches
	// it in the background. Most cover URLs never change content, but radio
	// favicons, video thumbnails and some playlist covers do. A month bounds how
	// long such a cover stays wrong at about one request per cover per month.
	diskRefreshAge = 30 * 24 * time.Hour
	// staleTempAge is when a temp file is taken as left by a killed client
	// rather than a write in progress.
	staleTempAge = time.Hour
	// initialScanDelay keeps the first full directory scan out of the way of
	// the first frames' disk reads.
	initialScanDelay = 5 * time.Second
	clearedSuffix    = ".cleared-"
)

type diskEntry struct {
	path   string
	size   int64
	access time.Time
}

func diskArtworkName(key cacheKey) string {
	var size [8]byte
	binary.BigEndian.PutUint64(size[:], uint64(key.size))
	name := make([]byte, 0, len(key.url)+1+len(size))
	name = append(append(append(name, key.url...), 0), size[:]...)
	sum := sha256.Sum256(name)
	return hex.EncodeToString(sum[:])
}

func diskPaletteName(rawURL string) string {
	sum := sha256.Sum256([]byte(rawURL))
	return hex.EncodeToString(sum[:]) + ".palette.json"
}

// errorReader records read failures, so an I/O error is not taken for a
// corrupt entry.
type errorReader struct {
	reader io.Reader
	err    error
}

func (r *errorReader) Read(buffer []byte) (int, error) {
	n, err := r.reader.Read(buffer)
	if err != nil && !errors.Is(err, io.EOF) {
		r.err = err
	}
	return n, err
}

// readDiskArtwork returns a disk hit and whether the entry is old enough to
// refetch. Only an entry that fails to decode is deleted; a file that cannot
// be opened or read now is a miss and kept.
func (c *Cache) readDiskArtwork(key cacheKey) (image.Image, artworkSample, bool, bool) {
	if c.diskDir == "" || c.clearDisk.Load() || !persistArtwork(key.url) {
		return nil, artworkSample{}, false, false
	}
	base := filepath.Join(c.diskDir, diskArtworkName(key))
	for _, extension := range [...]string{".jpg", ".png"} {
		path := base + extension
		file, err := os.Open(path)
		if err != nil {
			continue
		}
		info, statErr := file.Stat()
		reader := &errorReader{reader: file}
		img, _, decodeErr := image.Decode(reader)
		closeErr := file.Close()
		if statErr != nil || reader.err != nil || closeErr != nil {
			continue
		}
		if decodeErr != nil || img == nil || img.Bounds().Empty() {
			c.removeDiskFile(path)
			continue
		}
		now := time.Now()
		// mtime stays the fetch time; atime orders pruning. A cache directory
		// that cannot take the touch still serves the hit.
		_ = os.Chtimes(path, now, time.Time{})
		sample, ok := c.readDiskPalette(key.url)
		if !ok {
			sample = sampleArtwork(img)
			if sample.ready {
				if err := c.writeDiskPalette(key.url, sample); err != nil {
					c.removeDiskFile(filepath.Join(c.diskDir, diskPaletteName(key.url)))
				}
			}
		}
		return img, sample, true, now.Sub(info.ModTime()) > diskRefreshAge
	}
	return nil, artworkSample{}, false, false
}

// diskSample is a cover's palette file: its two colours and the pixels of its
// blurred backdrop.
type diskSample struct {
	Palette
	Backdrop []byte `json:",omitempty"`
}

func (c *Cache) readDiskPalette(rawURL string) (artworkSample, bool) {
	if c.diskDir == "" || c.clearDisk.Load() {
		return artworkSample{}, false
	}
	path := filepath.Join(c.diskDir, diskPaletteName(rawURL))
	content, err := os.ReadFile(path)
	if err != nil {
		return artworkSample{}, false
	}
	var stored diskSample
	if json.Unmarshal(content, &stored) != nil || (stored.Dominant.A == 0 && stored.Vibrant.A == 0) {
		c.removeDiskFile(path)
		return artworkSample{}, false
	}
	// a file without a backdrop of this size is sampled again and rewritten
	if len(stored.Backdrop) != backdropSize*backdropSize*4 {
		return artworkSample{}, false
	}
	// See readDiskArtwork: atime only.
	_ = os.Chtimes(path, time.Now(), time.Time{})
	backdrop := &image.RGBA{Pix: stored.Backdrop, Stride: backdropSize * 4, Rect: image.Rect(0, 0, backdropSize, backdropSize)}
	return artworkSample{palette: stored.Palette, backdrop: backdrop, ready: true}, true
}

func (c *Cache) writeDiskArtwork(key cacheKey, img image.Image, sample artworkSample) error {
	if c.diskDir == "" || !persistArtwork(key.url) {
		return nil
	}
	base := filepath.Join(c.diskDir, diskArtworkName(key))
	extension, alternate := ".jpg", ".png"
	encode := func(writer io.Writer) error { return jpeg.Encode(writer, img, &jpeg.Options{Quality: 88}) }
	if hasTransparency(img) {
		extension, alternate = ".png", ".jpg"
		encode = func(writer io.Writer) error {
			encoder := png.Encoder{CompressionLevel: png.BestSpeed}
			return encoder.Encode(writer, img)
		}
	}
	if err := c.writeDiskFile(base+extension, ".tmp-art-", encode); err != nil {
		return err
	}
	// A leftover entry in the other format would be read first and shadow
	// this one.
	if err := c.removeDiskFile(base + alternate); err != nil {
		return err
	}
	if sample.ready {
		return c.writeDiskPalette(key.url, sample)
	}
	return nil
}

func (c *Cache) writeDiskPalette(rawURL string, sample artworkSample) error {
	if c.diskDir == "" || !sample.ready || !persistArtwork(rawURL) {
		return nil
	}
	stored := diskSample{Palette: sample.palette}
	if sample.backdrop != nil {
		stored.Backdrop = sample.backdrop.Pix
	}
	content, err := json.Marshal(stored)
	if err != nil {
		return err
	}
	return c.writeDiskFile(filepath.Join(c.diskDir, diskPaletteName(rawURL)), ".tmp-palette-", func(writer io.Writer) error {
		_, err := writer.Write(content)
		return err
	})
}

// writeDiskFile writes through a temp file and a rename, and adds the size
// change to the running total.
func (c *Cache) writeDiskFile(path, tempPrefix string, encode func(io.Writer) error) (err error) {
	if err := os.MkdirAll(c.diskDir, 0o700); err != nil {
		return err
	}
	file, err := os.CreateTemp(c.diskDir, tempPrefix)
	if err != nil {
		return err
	}
	tempPath := file.Name()
	renamed := false
	defer func() {
		if !renamed {
			err = errors.Join(err, os.Remove(tempPath))
		}
	}()
	if err := encode(file); err != nil {
		return errors.Join(err, file.Close())
	}
	info, err := file.Stat()
	if err != nil {
		return errors.Join(err, file.Close())
	}
	if err := file.Close(); err != nil {
		return err
	}
	var previous int64
	if old, err := os.Lstat(path); err == nil {
		previous = old.Size()
	}
	if err := os.Rename(tempPath, path); err != nil {
		return err
	}
	renamed = true
	c.addDiskBytes(info.Size() - previous)
	return nil
}

// removeDiskFile deletes one cache file and takes it off the running total.
// A file that is already gone is not an error.
func (c *Cache) removeDiskFile(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	c.diskBytes.Add(-info.Size())
	return nil
}

func (c *Cache) addDiskBytes(delta int64) {
	if c.diskBytes.Add(delta) > c.diskMaxBytes {
		c.requestPrune()
	}
}

// hasTransparency asks the decoded image first: JPEG sources and the cache's
// own NRGBA images answer without boxing a colour per pixel.
func hasTransparency(img image.Image) bool {
	if opaque, ok := img.(interface{ Opaque() bool }); ok {
		return !opaque.Opaque()
	}
	bounds := img.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			_, _, _, alpha := img.At(x, y).RGBA()
			if alpha != 0xffff {
				return true
			}
		}
	}
	return false
}

// persistArtwork reports a cover worth keeping on disk: one with no local
// source. See isLocalArtwork.
func persistArtwork(rawURL string) bool {
	if urls, collage := ParseCollage(rawURL); collage {
		for _, source := range urls {
			if isLocalSource(source) {
				return false
			}
		}
		return true
	}
	return !isLocalSource(rawURL)
}

// removeDiskArtwork is best effort: an entry it cannot delete is replaced by
// the next write or pruned.
func (c *Cache) removeDiskArtwork(key cacheKey) {
	if c.diskDir == "" {
		return
	}
	base := filepath.Join(c.diskDir, diskArtworkName(key))
	c.removeDiskFile(base + ".jpg")
	c.removeDiskFile(base + ".png")
}

func (c *Cache) removeDiskPalette(rawURL string) {
	if c.diskDir == "" {
		return
	}
	c.removeDiskFile(filepath.Join(c.diskDir, diskPaletteName(rawURL)))
}

func (c *Cache) requestPrune() {
	if c.diskDir == "" {
		return
	}
	select {
	case c.prune <- struct{}{}:
	default:
	}
}

func (c *Cache) pruneWorker() {
	defer c.workers.Done()
	scan := time.NewTimer(initialScanDelay)
	defer scan.Stop()
	for {
		select {
		case <-c.ctx.Done():
			return
		case <-scan.C:
			c.pruneDiskCache()
		case <-c.prune:
			if c.clearDisk.Load() {
				c.clearDiskCache()
			}
			if c.diskBytes.Load() > c.diskMaxBytes {
				c.pruneDiskCache()
			}
		}
	}
}

// pruneDiskCache scans the directory once to correct the running total,
// sweeps temp files a killed client left, and when over the bound deletes the
// least recently read entries down to a low-water mark, so the next full scan
// is many writes away.
func (c *Cache) pruneDiskCache() {
	if c.diskDir == "" || c.diskMaxBytes <= 0 {
		return
	}
	c.pruneMu.Lock()
	defer c.pruneMu.Unlock()
	if !c.diskScanned {
		c.diskScanned = true
		c.removeClearedDirs()
	}
	start := c.diskBytes.Load()
	items, err := os.ReadDir(c.diskDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			c.diskBytes.Add(-start)
		}
		return
	}
	now := time.Now()
	entries := make([]diskEntry, 0, len(items))
	var total int64
	for _, item := range items {
		if item.IsDir() {
			continue
		}
		info, err := item.Info()
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		path := filepath.Join(c.diskDir, item.Name())
		if strings.HasPrefix(item.Name(), ".tmp-") {
			if now.Sub(info.ModTime()) > staleTempAge && os.Remove(path) == nil {
				continue
			}
			total += info.Size()
			continue
		}
		entries = append(entries, diskEntry{path: path, size: info.Size(), access: accessTime(info)})
		total += info.Size()
	}
	if total > c.diskMaxBytes {
		lowWater := c.diskMaxBytes - c.diskMaxBytes/10
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].access.Before(entries[j].access)
		})
		for _, entry := range entries {
			if total <= lowWater {
				break
			}
			if os.Remove(entry.path) == nil {
				total -= entry.size
			}
		}
	}
	// Writes during the scan stay counted; at worst a few are counted twice,
	// which only brings the next scan forward.
	c.diskBytes.Add(total - start)
}

// clearDiskCache moves the whole directory aside in one rename, so reads and
// writes see an empty cache at once, then deletes it.
func (c *Cache) clearDiskCache() {
	c.pruneMu.Lock()
	aside, err := os.MkdirTemp(filepath.Dir(c.diskDir), filepath.Base(c.diskDir)+clearedSuffix)
	if err == nil {
		if renameErr := os.Rename(c.diskDir, filepath.Join(aside, "art")); renameErr != nil && !errors.Is(renameErr, os.ErrNotExist) {
			c.removeEntriesInPlace()
		}
	} else {
		c.removeEntriesInPlace()
	}
	c.diskBytes.Store(0)
	c.clearDisk.Store(false)
	c.pruneMu.Unlock()
	if err == nil {
		// Whatever is left is swept by the next start's scan.
		os.RemoveAll(aside)
	}
}

// removeEntriesInPlace is the fallback when the directory cannot be moved;
// best effort like removeDiskArtwork.
func (c *Cache) removeEntriesInPlace() {
	items, err := os.ReadDir(c.diskDir)
	if err != nil {
		return
	}
	for _, item := range items {
		if item.Type().IsRegular() && !strings.HasPrefix(item.Name(), ".tmp-") {
			os.Remove(filepath.Join(c.diskDir, item.Name()))
		}
	}
}

// removeClearedDirs deletes directories a Clear moved aside but could not
// finish deleting before the client exited. Best effort, retried each start.
func (c *Cache) removeClearedDirs() {
	parent := filepath.Dir(c.diskDir)
	prefix := filepath.Base(c.diskDir) + clearedSuffix
	items, err := os.ReadDir(parent)
	if err != nil {
		return
	}
	for _, item := range items {
		if item.IsDir() && strings.HasPrefix(item.Name(), prefix) {
			os.RemoveAll(filepath.Join(parent, item.Name()))
		}
	}
}
