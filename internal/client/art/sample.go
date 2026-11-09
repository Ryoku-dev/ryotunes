package art

import (
	"image"
	"image/color"
	"math"

	xdraw "golang.org/x/image/draw"
)

const accentSampleSize = 24

type artworkSample struct {
	color color.NRGBA
	ready bool
	ok    bool
}

type sampleBucket struct {
	count int
	red   uint64
	green uint64
	blue  uint64
	score float64
}

func sampleArtwork(source image.Image) artworkSample {
	if source == nil || source.Bounds().Empty() {
		return artworkSample{}
	}
	pixels := image.NewNRGBA(image.Rect(0, 0, accentSampleSize, accentSampleSize))
	xdraw.CatmullRom.Scale(pixels, pixels.Bounds(), source, source.Bounds(), xdraw.Src, nil)
	buckets := make(map[uint16]*sampleBucket, 64)
	var best *sampleBucket
	for y := range accentSampleSize {
		for x := range accentSampleSize {
			pixel := pixels.NRGBAAt(x, y)
			if pixel.A < 128 {
				continue
			}
			maximum := max(pixel.R, pixel.G, pixel.B)
			minimum := min(pixel.R, pixel.G, pixel.B)
			if maximum == 0 {
				continue
			}
			saturation := float64(maximum-minimum) / float64(maximum)
			if saturation < .12 {
				continue
			}
			value := float64(maximum) / 255
			score := saturation * (1 - math.Abs(value-.65))
			key := uint16(pixel.R>>5)<<6 | uint16(pixel.G>>5)<<3 | uint16(pixel.B>>5)
			bucket := buckets[key]
			if bucket == nil {
				bucket = &sampleBucket{}
				buckets[key] = bucket
			}
			bucket.count++
			bucket.red += uint64(pixel.R)
			bucket.green += uint64(pixel.G)
			bucket.blue += uint64(pixel.B)
			bucket.score += score
			if best == nil || bucket.score > best.score {
				best = bucket
			}
		}
	}
	if best == nil || best.count == 0 {
		return artworkSample{ready: true}
	}
	count := uint64(best.count)
	return artworkSample{color: color.NRGBA{
		R: uint8(best.red / count),
		G: uint8(best.green / count),
		B: uint8(best.blue / count),
		A: 0xff,
	}, ready: true, ok: true}
}
