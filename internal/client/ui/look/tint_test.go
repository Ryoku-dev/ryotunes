package look

import (
	"image/color"
	"math"
	"testing"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/palette"
	libtheme "github.com/ryoku-dev/ryoku/docs/lithic/library/go/theme"
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

func TestGreyCoverTurnsTheWindowAndAccentNeutral(t *testing.T) {
	// a desktop palette with a navy cast, as a blue wallpaper gives
	navy, ok := CoverTint(palette.Signature(true), art.Palette{Dominant: color.NRGBA{R: 30, G: 50, B: 110, A: 0xff}})
	if !ok {
		t.Fatal("navy cover did not tint")
	}
	grey := art.Palette{Dominant: color.NRGBA{R: 40, G: 40, B: 42, A: 0xff}}
	got, ok := CoverTint(navy, grey)
	if !ok {
		t.Fatal("a black and white cover left the desktop's hue on the window")
	}
	pairs := map[string][2]color.NRGBA{
		"ground": {navy.Ground, got.Ground}, "stone1": {navy.Stone1, got.Stone1}, "stone2": {navy.Stone2, got.Stone2},
		"stone3": {navy.Stone3, got.Stone3}, "stone4": {navy.Stone4, got.Stone4}, "vein": {navy.Vein, got.Vein},
		"fg": {navy.Fg, got.Fg}, "fg2": {navy.Fg2, got.Fg2}, "fg3": {navy.Fg3, got.Fg3}, "sel": {navy.Sel, got.Sel},
	}
	for name, pair := range pairs {
		before, after := colorToOKLCH(pair[0]), colorToOKLCH(pair[1])
		if after.C > .005 {
			t.Errorf("%s chroma %.3f, want grey", name, after.C)
		}
		if math.Abs(after.L-before.L) > .01 {
			t.Errorf("%s lightness moved from %.3f to %.3f", name, before.L, after.L)
		}
	}

	theme := NewTheme(palette.Signature(true), libtheme.Comfortable)
	theme.SetRoles(navy)
	roles, accent := theme.ArtScheme(grey, true)
	if chroma := colorToOKLCH(accent.Fill).C; chroma > .005 {
		t.Fatalf("accent fill %#v has chroma %.3f, want grey", accent.Fill, chroma)
	}
	if ratio := palette.ContrastRatio(accent.Ink, roles.Stone2); ratio < 4.5 {
		t.Fatalf("accent ink on stone2 reads at %.2f:1, want 4.5:1", ratio)
	}
	// sung words wear the ink over the rest of the line in fg2
	if sung, rest := colorToOKLCH(accent.Ink).L, colorToOKLCH(roles.Fg2).L; sung < rest+.05 {
		t.Fatalf("sung words at lightness %.3f against the line's %.3f, want them to stand out", sung, rest)
	}
	if hero := theme.ArtAccent(grey); hero.Fill != accent.Fill {
		t.Fatalf("hero accent %#v for a grey cover, want the grey %#v", hero.Fill, accent.Fill)
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
