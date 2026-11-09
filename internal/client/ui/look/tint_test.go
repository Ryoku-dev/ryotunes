package look

import (
	"image/color"
	"math"
	"testing"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/palette"
	"github.com/ryoku-dev/ryotunes/internal/client/art"
)

func TestCoverTintPreservesContrastAndStateRoles(t *testing.T) {
	cover := art.Palette{
		Dominant: color.NRGBA{R: 18, G: 104, B: 112, A: 0xff},
		Vibrant:  color.NRGBA{R: 244, G: 96, B: 54, A: 0xff},
	}
	for _, dark := range []bool{true, false} {
		base := palette.Signature(dark)
		tinted, ok := CoverTint(base, cover)
		if !ok {
			t.Fatalf("dark=%v cover did not tint", dark)
		}
		baseBackgrounds := []color.NRGBA{base.Ground, base.Stone1, base.Stone2, base.Stone3, base.Stone4}
		tintedBackgrounds := []color.NRGBA{tinted.Ground, tinted.Stone1, tinted.Stone2, tinted.Stone3, tinted.Stone4}
		baseInks := []color.NRGBA{base.Fg, base.Fg2, base.Fg3}
		tintedInks := []color.NRGBA{tinted.Fg, tinted.Fg2, tinted.Fg3}
		for inkIndex := range baseInks {
			for backgroundIndex := range baseBackgrounds {
				assertContrastWithin(t,
					palette.ContrastRatio(baseInks[inkIndex], baseBackgrounds[backgroundIndex]),
					palette.ContrastRatio(tintedInks[inkIndex], tintedBackgrounds[backgroundIndex]),
				)
			}
		}
		assertContrastWithin(t, palette.ContrastRatio(base.OnSel, base.Sel), palette.ContrastRatio(tinted.OnSel, tinted.Sel))
		if ratio := palette.ContrastRatio(tinted.Fg, tinted.Stone2); ratio < 4.5 {
			t.Fatalf("dark=%v fg on stone2 contrast = %.3f", dark, ratio)
		}
		if tinted.Jade != base.Jade || tinted.Lapis != base.Lapis || tinted.Amber != base.Amber ||
			tinted.Jasper != base.Jasper || tinted.Scrim != base.Scrim || tinted.OnSel != base.OnSel || tinted.OnSel2 != base.OnSel2 {
			t.Fatalf("dark=%v state roles changed", dark)
		}
		for _, value := range []color.NRGBA{
			tinted.Ground, tinted.Stone1, tinted.Stone2, tinted.Stone3, tinted.Stone4,
			tinted.Vein, tinted.Fg, tinted.Fg2, tinted.Fg3, tinted.Sel,
		} {
			if _, inGamut := oklchLinearRGB(colorToOKLCH(value)); !inGamut {
				t.Fatalf("dark=%v output %#v is outside sRGB", dark, value)
			}
		}
	}
}

func TestCoverTintRejectsGreyCover(t *testing.T) {
	base := palette.Signature(true)
	grey := art.Palette{Dominant: color.NRGBA{R: 120, G: 120, B: 120, A: 0xff}}
	got, ok := CoverTint(base, grey)
	if ok || got != base {
		t.Fatalf("grey tint = (%#v, %v), want untouched roles", got, ok)
	}
}

func TestOKLCHRoundTripAndGamutClamp(t *testing.T) {
	for _, source := range []color.NRGBA{
		{R: 12, G: 34, B: 56, A: 0xff},
		{R: 244, G: 96, B: 54, A: 0xff},
		{R: 245, G: 245, B: 240, A: 0x80},
	} {
		got := oklchToColor(colorToOKLCH(source))
		if absChannel(got.R, source.R) > 1 || absChannel(got.G, source.G) > 1 || absChannel(got.B, source.B) > 1 || got.A != source.A {
			t.Fatalf("round trip %#v = %#v", source, got)
		}
	}
	clamped := oklchToColor(oklch{L: .62, C: .8, H: 1.2, A: 0xff})
	if _, ok := oklchLinearRGB(colorToOKLCH(clamped)); !ok {
		t.Fatalf("clamped colour %#v is outside sRGB", clamped)
	}
	whiteEdge := oklchToColor(oklch{L: 1, C: .8, H: 2.4, A: 0xff})
	if whiteEdge.R < 254 || whiteEdge.G < 254 || whiteEdge.B < 254 {
		t.Fatalf("high-chroma L=1 clamp = %#v, want the zero-chroma white gamut edge", whiteEdge)
	}
}

func assertContrastWithin(t *testing.T, base, got float64) {
	t.Helper()
	if delta := math.Abs(got-base) / base; delta > .03 {
		t.Fatalf("contrast %.3f changed to %.3f (%.1f%%)", base, got, delta*100)
	}
}

func absChannel(left, right uint8) int {
	if left >= right {
		return int(left - right)
	}
	return int(right - left)
}
