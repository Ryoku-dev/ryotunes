package art

import (
	"image"
	"image/draw"
	"strings"

	"github.com/ryoku-dev/ryotunes/internal/model"
)

// three box passes of this reach over the 24 px sample come close to a
// gaussian, and drawn large the cover only shows as soft colour
const backdropRadius = 2

// Backdrop returns a heavily blurred copy of the cover, made with its palette
// sample, for drawing large and faint behind a surface, and whether it is
// ready. The image is shared and never changes.
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

func backdropOf(sample *image.NRGBA) *image.RGBA {
	bounds := sample.Bounds()
	blurred := image.NewRGBA(bounds)
	draw.Draw(blurred, bounds, sample, bounds.Min, draw.Src)
	scratch := make([]uint8, len(blurred.Pix))
	for range 3 {
		blurPass(scratch, blurred.Pix, bounds.Dx(), bounds.Dy(), blurred.Stride, false)
		blurPass(blurred.Pix, scratch, bounds.Dx(), bounds.Dy(), blurred.Stride, true)
	}
	return blurred
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
