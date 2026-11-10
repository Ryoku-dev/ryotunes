package art

import (
	"bytes"
	"image"
	"image/color"
	"math"

	xdraw "golang.org/x/image/draw"
)

const accentSampleSize = 24

// A pixel counts as vibrant only when its channels spread this far apart out
// of 255. Spread over the maximum, the old measure, calls a blue-tinted black
// vivid, and a dark cover picked a bright blue accent from its shadows.
const vibrantMinimumChroma = 40

// Palette contains the cover colours used by the client. A zero-alpha colour
// means that the corresponding pick was not available.
type Palette struct {
	Vibrant  color.NRGBA
	Dominant color.NRGBA
}

type artworkSample struct {
	palette  Palette
	backdrop *image.RGBA
	ready    bool
}

func sameSample(a, b artworkSample) bool {
	if a.palette != b.palette || (a.backdrop == nil) != (b.backdrop == nil) {
		return false
	}
	return a.backdrop == nil || bytes.Equal(a.backdrop.Pix, b.backdrop.Pix)
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

	var dominantBuckets, vibrantBuckets [512]sampleBucket
	dominantIndex, vibrantIndex := -1, -1
	for y := range accentSampleSize {
		for x := range accentSampleSize {
			pixel := pixels.NRGBAAt(x, y)
			if pixel.A < 128 {
				continue
			}
			key := int(pixel.R>>5)<<6 | int(pixel.G>>5)<<3 | int(pixel.B>>5)
			dominant := &dominantBuckets[key]
			dominant.count++
			dominant.red += uint64(pixel.R)
			dominant.green += uint64(pixel.G)
			dominant.blue += uint64(pixel.B)
			if dominantIndex < 0 || dominant.count > dominantBuckets[dominantIndex].count {
				dominantIndex = key
			}

			maximum := max(pixel.R, pixel.G, pixel.B)
			minimum := min(pixel.R, pixel.G, pixel.B)
			if maximum-minimum < vibrantMinimumChroma {
				continue
			}
			chroma := float64(maximum-minimum) / 255
			value := float64(maximum) / 255
			vibrant := &vibrantBuckets[key]
			vibrant.count++
			vibrant.red += uint64(pixel.R)
			vibrant.green += uint64(pixel.G)
			vibrant.blue += uint64(pixel.B)
			vibrant.score += chroma * (1 - math.Abs(value-.65))
			if vibrantIndex < 0 || vibrant.score > vibrantBuckets[vibrantIndex].score {
				vibrantIndex = key
			}
		}
	}

	sample := artworkSample{ready: true, backdrop: backdropOf(source)}
	if dominantIndex >= 0 {
		sample.palette.Dominant = bucketColour(dominantBuckets[dominantIndex])
	}
	if vibrantIndex >= 0 {
		sample.palette.Vibrant = bucketColour(vibrantBuckets[vibrantIndex])
	} else {
		greyscale(sample.backdrop)
	}
	return sample
}

func bucketColour(bucket sampleBucket) color.NRGBA {
	if bucket.count == 0 {
		return color.NRGBA{}
	}
	count := uint64(bucket.count)
	return color.NRGBA{
		R: uint8(bucket.red / count),
		G: uint8(bucket.green / count),
		B: uint8(bucket.blue / count),
		A: 0xff,
	}
}
