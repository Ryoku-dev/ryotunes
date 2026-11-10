package look

import (
	"image/color"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/palette"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/tokens"
	"github.com/ryoku-dev/ryotunes/internal/model"
)

// AccentFromSample keeps a cover's hue while giving it a reliable face and ink
// on the current work plane.
func AccentFromSample(sample color.NRGBA, roles palette.Roles) Accent {
	hue, saturation, _ := brandRGBToHSL(sample)
	saturation = min(max(saturation, .55), .85)
	return accentFromFill(brandHSLToRGB(hue, saturation, accentLightness(roles.Dark), 0xff), roles)
}

// NeutralAccent is the grey accent of a cover without colour. Its ink is fg,
// the strongest type, so sung words still stand apart from the rest of the
// line instead of matching fg2.
func NeutralAccent(roles palette.Roles) Accent {
	accent := accentFromFill(brandHSLToRGB(0, 0, accentLightness(roles.Dark), 0xff), roles)
	accent.Ink = roles.Fg
	return accent
}

// accentLightness puts every cover accent at one lightness per mode, so it
// reads on every stone.
func accentLightness(dark bool) float64 {
	if dark {
		return .70
	}
	return .42
}

// AccentFromSource resolves the active provider's fixed identity as a play accent.
func AccentFromSource(provider model.Provider, roles palette.Roles) Accent {
	fill := BrandFill(provider, roles)
	return Accent{
		Fill:  fill,
		On:    palette.InkOn(fill),
		Ink:   BrandInk(provider, roles),
		Field: palette.Mix(roles.Stone2, fill, tokens.AlphaMineral),
	}
}

// AccentFromDesktop preserves the gold roles supplied by the loaded Ryoku palette.
func AccentFromDesktop(roles palette.Roles) Accent {
	return Accent{
		Fill:  roles.Gold,
		On:    roles.OnGold,
		Ink:   roles.GoldInk,
		Field: palette.Mix(roles.Stone2, roles.Gold, tokens.AlphaMineral),
	}
}

func accentFromFill(fill color.NRGBA, roles palette.Roles) Accent {
	fill.A = 0xff
	return Accent{
		Fill:  fill,
		On:    palette.InkOn(fill),
		Ink:   contrastAccent(fill, roles.Stone2, roles.Dark),
		Field: palette.Mix(roles.Stone2, fill, tokens.AlphaMineral),
	}
}

func contrastAccent(ink, background color.NRGBA, lighter bool) color.NRGBA {
	if palette.ContrastRatio(ink, background) >= float64(tokens.ContrastMinimum) {
		return ink
	}
	hue, saturation, lightness := brandRGBToHSL(ink)
	toward := 0.0
	if lighter {
		toward = 1
	}
	for step := 1; step <= 255; step++ {
		at := float64(step) / 255
		candidate := brandHSLToRGB(hue, saturation, lightness+(toward-lightness)*at, ink.A)
		if palette.ContrastRatio(candidate, background) >= float64(tokens.ContrastMinimum) {
			return candidate
		}
	}
	fallback := palette.InkOn(background)
	fallback.A = ink.A
	return fallback
}
