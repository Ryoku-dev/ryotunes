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
		Field: accentField(fill, roles),
	}
}

// AccentFromDesktop is the desktop's own accent, the one the Hub wears
// (Tone.accent is Lithic.sel): sel as the face, onSel on it, and an ink of
// sel that reads on every pane. On Ryoku's colours that is the signature
// orange; on the wallpaper's it is the palette's primary, never its tertiary
// (which the palette maps to gold).
func AccentFromDesktop(roles palette.Roles) Accent {
	fill := roles.Sel
	fill.A = 0xff
	ink := roles.GoldInk
	if roles.Gold != roles.Sel {
		// gold's ink is tuned for gold; sel needs its own on the panes
		ink = contrastAccent(fill, leastContrastPane(fill, roles), roles.Dark)
	}
	return Accent{
		Fill:  fill,
		On:    roles.OnSel,
		Ink:   ink,
		Field: accentField(fill, roles),
	}
}

// leastContrastPane is the plane a window lays out (ground, the stones, the
// slab) that ink reads worst on, so an ink cleared against it reads on all.
func leastContrastPane(ink color.NRGBA, roles palette.Roles) color.NRGBA {
	worst, ratio := roles.Ground, palette.ContrastRatio(ink, roles.Ground)
	for _, plane := range [...]color.NRGBA{roles.Stone1, roles.Stone2, roles.Stone3, roles.Slab} {
		if r := palette.ContrastRatio(ink, plane); r < ratio {
			worst, ratio = plane, r
		}
	}
	return worst
}

func accentFromFill(fill color.NRGBA, roles palette.Roles) Accent {
	fill.A = 0xff
	return Accent{
		Fill:  fill,
		On:    palette.InkOn(fill),
		Ink:   contrastAccent(fill, roles.Stone2, roles.Dark),
		Field: accentField(fill, roles),
	}
}

// accentFieldChroma is the field's colour at most: a mineral field's chroma
// (tokens.DyeMineral), so the accent's field sits among jade's and amber's.
var accentFieldChroma = tokens.DyeMineral["amber"]

// accentField is the accent's mineral field, as palette.Roles.MineralField
// builds jade's or amber's: the ladder's mineral-field lightness, a mineral's
// chroma, and the accent's hue. A colourless accent gives a grey field.
func accentField(fill color.NRGBA, roles palette.Roles) color.NRGBA {
	field := colorToOKLCH(roles.MineralField(palette.Attention))
	accent := colorToOKLCH(fill)
	field.H = accent.H
	field.C = min(accentFieldChroma, accent.C)
	field.A = 0xff
	return oklchToColor(field)
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
