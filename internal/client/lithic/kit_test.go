// These tests lock dark/light role mapping and contrast to a real Ryoku wallpaper palette.
package lithic

import (
	"image"
	"image/color"
	"testing"

	"gioui.org/f32"
)

func fixtureColor(r, g, b uint8) color.NRGBA {
	return color.NRGBA{R: r, G: g, B: b, A: 255}
}

func fixturePalette() map[string]color.NRGBA {
	return map[string]color.NRGBA{
		"error":                   fixtureColor(255, 180, 171),
		"onPrimary":               fixtureColor(0, 53, 69),
		"onSecondaryContainer":    fixtureColor(207, 230, 241),
		"onSurface":               fixtureColor(222, 227, 230),
		"onSurfaceVariant":        fixtureColor(192, 200, 204),
		"outline":                 fixtureColor(138, 146, 150),
		"outlineVariant":          fixtureColor(64, 72, 76),
		"primary":                 fixtureColor(138, 208, 238),
		"scrim":                   fixtureColor(0, 0, 0),
		"secondaryContainer":      fixtureColor(53, 74, 83),
		"surface":                 fixtureColor(15, 20, 23),
		"surfaceContainer":        fixtureColor(27, 32, 35),
		"surfaceContainerHigh":    fixtureColor(37, 43, 45),
		"surfaceContainerHighest": fixtureColor(48, 54, 56),
		"surfaceContainerLow":     fixtureColor(23, 28, 31),
		"surfaceContainerLowest":  fixtureColor(10, 15, 17),
	}
}

func TestFromPaletteDarkMapping(t *testing.T) {
	palette := Palette{Roles: fixturePalette(), Dark: true, Source: "wallpaper"}
	roles := FromPalette(palette)
	want := map[string]color.NRGBA{
		"ground": palette.Roles["surfaceContainerLowest"],
		"stone1": palette.Roles["surface"],
		"stone2": palette.Roles["surfaceContainerLow"],
		"stone3": palette.Roles["surfaceContainer"],
		"stone4": palette.Roles["surfaceContainerHigh"],
		"vein":   palette.Roles["surfaceContainerHighest"],
	}
	got := map[string]color.NRGBA{
		"ground": roles.Ground, "stone1": roles.Stone1, "stone2": roles.Stone2,
		"stone3": roles.Stone3, "stone4": roles.Stone4, "vein": roles.Vein,
	}
	for name, expected := range want {
		if got[name] != expected {
			t.Errorf("%s = %#v, want %#v", name, got[name], expected)
		}
	}
	if roles.Scrim.A != 158 {
		t.Errorf("dark scrim alpha = %d, want 158", roles.Scrim.A)
	}
	if ratio := contrastRatio(roles.GoldInk, roles.Stone2); ratio < 4.5 {
		t.Errorf("goldInk contrast = %.3f, want at least 4.5", ratio)
	}
}

func TestFromPaletteLightMapping(t *testing.T) {
	palette := Palette{Roles: fixturePalette(), Dark: false, Source: "wallpaper"}
	roles := FromPalette(palette)
	want := map[string]color.NRGBA{
		"ground": palette.Roles["surfaceContainerHighest"],
		"stone1": palette.Roles["surfaceContainerHigh"],
		"stone2": palette.Roles["surfaceContainerLowest"],
		"stone3": palette.Roles["surfaceContainer"],
		"stone4": palette.Roles["surfaceContainerLowest"],
		"vein":   palette.Roles["outlineVariant"],
	}
	got := map[string]color.NRGBA{
		"ground": roles.Ground, "stone1": roles.Stone1, "stone2": roles.Stone2,
		"stone3": roles.Stone3, "stone4": roles.Stone4, "vein": roles.Vein,
	}
	for name, expected := range want {
		if got[name] != expected {
			t.Errorf("%s = %#v, want %#v", name, got[name], expected)
		}
	}
	if roles.Scrim.A != 114 {
		t.Errorf("light scrim alpha = %d, want 114", roles.Scrim.A)
	}
	if ratio := contrastRatio(roles.GoldInk, roles.Stone2); ratio < 4.5 {
		t.Errorf("goldInk contrast = %.3f, want at least 4.5", ratio)
	}
}

func TestFromPaletteLightJasperInkContrast(t *testing.T) {
	palette := Palette{
		Roles: map[string]color.NRGBA{
			"error":                   fixtureColor(255, 180, 171),
			"onError":                 fixtureColor(105, 0, 5),
			"surfaceContainerLowest":  fixtureColor(250, 248, 244),
			"surfaceContainerHigh":    fixtureColor(235, 229, 222),
			"surfaceContainerHighest": fixtureColor(224, 217, 208),
			"surfaceContainer":        fixtureColor(242, 237, 231),
			"outlineVariant":          fixtureColor(203, 196, 184),
		},
		Dark:   false,
		Source: "wallpaper",
	}
	roles := FromPalette(palette)
	if roles.Jasper != palette.Roles["error"] {
		t.Fatalf("jasper fill = %#v, want palette error %#v", roles.Jasper, palette.Roles["error"])
	}
	if roles.OnJasper != palette.Roles["onError"] {
		t.Fatalf("on-jasper ink = %#v, want palette onError %#v", roles.OnJasper, palette.Roles["onError"])
	}
	if ratio := contrastRatio(roles.JasperInk, roles.Stone2); ratio < 4.5 {
		t.Errorf("jasperInk contrast = %.3f, want at least 4.5", ratio)
	}
}

func TestForcedModeKeepsSignatureNeutrals(t *testing.T) {
	palette := Palette{Roles: fixturePalette(), Dark: true, Forced: true, Source: "wallpaper"}
	roles := FromPalette(palette)
	signature := Signature(true)
	if roles.Stone2 != signature.Stone2 || roles.Ground != signature.Ground {
		t.Fatalf("forced mode replaced signature neutrals")
	}
	if roles.Gold != palette.Roles["primary"] || roles.Sel != palette.Roles["secondaryContainer"] {
		t.Fatalf("forced mode did not preserve palette accents")
	}
}

func TestCutPointsGeometry(t *testing.T) {
	points := cutPoints(image.Rect(10, 20, 110, 70), 7, 6, 5, 4)
	want := [8]f32.Point{
		f32.Pt(17, 20), f32.Pt(104, 20), f32.Pt(110, 26), f32.Pt(110, 65),
		f32.Pt(105, 70), f32.Pt(14, 70), f32.Pt(10, 66), f32.Pt(10, 27),
	}
	if points != want {
		t.Fatalf("cut points = %#v, want %#v", points, want)
	}
}

func TestEaseChiselEndpoints(t *testing.T) {
	if got := EaseChisel(0); got != 0 {
		t.Fatalf("EaseChisel(0) = %v, want 0", got)
	}
	if got := EaseChisel(1); got != 1 {
		t.Fatalf("EaseChisel(1) = %v, want 1", got)
	}
}
