package art

import (
	"image"
	"strings"

	xdraw "golang.org/x/image/draw"

	"github.com/ryoku-dev/ryotunes/internal/model"
)

// The backdrop is the cover at 48 px blurred by three box passes of radius 2,
// close to a gaussian: drawn across a pane its big shapes and colours survive
// and its detail doesn't.
const (
	backdropSize   = 48
	backdropRadius = 2
)

// Backdrop returns a heavily blurred copy of the cover, made with its palette
// sample, for drawing large behind a surface, and whether it is ready. The
// image is shared and never changes.
func (c *Cache) Backdrop(rawURL string) (*image.RGBA, bool) {
	// a collage is never sampled, so asking would only queue loads
	if rawURL == "" || strings.HasPrefix(rawURL, model.CollageThumbnailPrefix) {
		return nil, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if entry := c.samples[rawURL]; entry != nil {
		c.sampleLRU.MoveToFront(entry.lru)
		return entry.sample.backdrop, entry.sample.backdrop != nil
	}
	c.requestSampleLocked(rawURL)
	return nil, false
}

func backdropOf(source image.Image) *image.RGBA {
	blurred := image.NewRGBA(image.Rect(0, 0, backdropSize, backdropSize))
	xdraw.CatmullRom.Scale(blurred, blurred.Bounds(), source, source.Bounds(), xdraw.Src, nil)
	scratch := make([]uint8, len(blurred.Pix))
	for range 3 {
		blurPass(scratch, blurred.Pix, backdropSize, backdropSize, blurred.Stride, false)
		blurPass(blurred.Pix, scratch, backdropSize, backdropSize, blurred.Stride, true)
	}
	return blurred
}

// greyscale drops a backdrop's colour for a cover the sampler found none in:
// its shadows can still lean blue, and the window around it is grey.
func greyscale(backdrop *image.RGBA) {
	for at := 0; at+3 < len(backdrop.Pix); at += 4 {
		luma := (2126*uint32(backdrop.Pix[at]) + 7152*uint32(backdrop.Pix[at+1]) + 722*uint32(backdrop.Pix[at+2]) + 5000) / 10000
		backdrop.Pix[at], backdrop.Pix[at+1], backdrop.Pix[at+2] = uint8(luma), uint8(luma), uint8(luma)
	}
}

// blurPass averages every pixel with its neighbours along one axis, holding
// the edge pixels, in premultiplied RGBA.
func blurPass(dst, src []uint8, width, height, stride int, vertical bool) {
	length, lines, step, lineStep := width, height, 4, stride
	if vertical {
		length, lines, step, lineStep = height, width, stride, 4
	}
	window := 2*backdropRadius + 1
	for line := range lines {
		base := line * lineStep
		for index := range length {
			var sum [4]int
			for offset := index - backdropRadius; offset <= index+backdropRadius; offset++ {
				at := base + min(max(offset, 0), length-1)*step
				for channel := range 4 {
					sum[channel] += int(src[at+channel])
				}
			}
			at := base + index*step
			for channel := range 4 {
				dst[at+channel] = uint8((sum[channel] + window/2) / window)
			}
		}
	}
}
