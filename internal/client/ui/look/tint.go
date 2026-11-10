package look

import (
	"image/color"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/palette"
	"github.com/ryoku-dev/ryotunes/internal/client/art"
)

// CoverTint re-hues the neutral roles toward a cover while preserving each
// role's OKLCH lightness and every state mineral. A cover without colour
// turns them grey.
func CoverTint(base palette.Roles, cover art.Palette) (palette.Roles, bool) {
	hue, strength, ok := coverTintSource(cover)
	if !ok {
		return base, false
	}
	modeStrength := strength
	if !base.Dark {
		modeStrength *= .8
	}
	tint := func(chroma float64, source color.NRGBA) color.NRGBA {
		value := colorToOKLCH(source)
		value.C = chroma * modeStrength
		value.H = hue
		return oklchToColor(value)
	}

	result := base
	result.Ground = tint(.020, base.Ground)
	result.Stone1 = tint(.026, base.Stone1)
	result.Stone2 = tint(.030, base.Stone2)
	result.Stone3 = tint(.034, base.Stone3)
	result.Stone4 = tint(.036, base.Stone4)
	result.Vein = tint(.030, base.Vein)
	result.Fg = tint(.010, base.Fg)
	result.Fg2 = tint(.014, base.Fg2)
	result.Fg3 = tint(.018, base.Fg3)
	result.Sel = tint(.018, base.Sel)
	return result, true
}

func coverTintSource(cover art.Palette) (hue, strength float64, ok bool) {
	if cover.Dominant.A != 0 {
		dominant := colorToOKLCH(cover.Dominant)
		if dominant.C >= .04 {
			return dominant.H, min(1, dominant.C/.12), true
		}
	}
	if cover.Vibrant.A != 0 {
		return colorToOKLCH(cover.Vibrant).H, .35, true
	}
	// a black and white cover shouldn't be framed in the desktop's hue
	if cover.Dominant.A != 0 {
		return 0, 0, true
	}
	return 0, 0, false
}
