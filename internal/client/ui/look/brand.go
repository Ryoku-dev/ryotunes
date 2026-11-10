package look

import (
	"image/color"
	"math"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/palette"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/tokens"
	"github.com/ryoku-dev/ryotunes/internal/model"
)

var (
	RyokuOrange    = color.NRGBA{R: 0xce, G: 0x5e, B: 0x32, A: 0xff}
	youtubeFill    = color.NRGBA{R: 0xff, G: 0x2d, B: 0x2d, A: 0xff}
	spotifyFill    = color.NRGBA{R: 0x1d, G: 0xb9, B: 0x54, A: 0xff}
	soundCloudFill = color.NRGBA{R: 0xff, G: 0x55, B: 0x00, A: 0xff}
	tidalFill      = color.NRGBA{R: 0x4f, G: 0xc3, B: 0xdc, A: 0xff}
)

// BrandFill returns a provider's identity fill.
func BrandFill(provider model.Provider, roles palette.Roles) color.NRGBA {
	switch provider {
	case model.ProviderYouTube:
		return youtubeFill
	case model.ProviderSpotify:
		return spotifyFill
	case model.ProviderSoundCloud:
		return soundCloudFill
	case model.ProviderTidal:
		return tidalFill
	default:
		return roles.Lapis
	}
}

// BrandInk returns the provider identity re-toned for readable use on stone2.
func BrandInk(provider model.Provider, roles palette.Roles) color.NRGBA {
	ink := BrandFill(provider, roles)
	if palette.ContrastRatio(ink, roles.Stone2) >= float64(tokens.ContrastMinimum) {
		return ink
	}
	hue, saturation, lightness := brandRGBToHSL(ink)
	toward := 1.0
	if !roles.Dark {
		toward = 0
	}
	best := ink
	for step := 1; step <= 100; step++ {
		fraction := float64(step) / 100
		candidate := brandHSLToRGB(hue, saturation, lightness+(toward-lightness)*fraction, ink.A)
		best = candidate
		if palette.ContrastRatio(candidate, roles.Stone2) >= float64(tokens.ContrastMinimum) {
			return candidate
		}
	}
	return best
}

func brandRGBToHSL(c color.NRGBA) (float64, float64, float64) {
	r, g, b := float64(c.R)/255, float64(c.G)/255, float64(c.B)/255
	maximum, minimum := max(r, g, b), min(r, g, b)
	lightness := (maximum + minimum) / 2
	if maximum == minimum {
		return 0, 0, lightness
	}
	delta := maximum - minimum
	saturation := delta / (1 - math.Abs(2*lightness-1))
	var hue float64
	switch maximum {
	case r:
		hue = math.Mod((g-b)/delta, 6)
	case g:
		hue = (b-r)/delta + 2
	default:
		hue = (r-g)/delta + 4
	}
	hue *= 60
	if hue < 0 {
		hue += 360
	}
	return hue, saturation, lightness
}

func brandHSLToRGB(hue, saturation, lightness float64, alpha uint8) color.NRGBA {
	chroma := (1 - math.Abs(2*lightness-1)) * saturation
	x := chroma * (1 - math.Abs(math.Mod(hue/60, 2)-1))
	var r, g, b float64
	switch int(hue/60) % 6 {
	case 0:
		r, g = chroma, x
	case 1:
		r, g = x, chroma
	case 2:
		g, b = chroma, x
	case 3:
		g, b = x, chroma
	case 4:
		r, b = x, chroma
	default:
		r, b = chroma, x
	}
	m := lightness - chroma/2
	channel := func(value float64) uint8 { return uint8(math.Round(max(0, min(1, value+m)) * 255)) }
	return color.NRGBA{R: channel(r), G: channel(g), B: channel(b), A: alpha}
}
