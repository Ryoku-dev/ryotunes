package look

import (
	"image/color"
	"math"
	"testing"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/palette"
	libtheme "github.com/ryoku-dev/ryoku/docs/lithic/library/go/theme"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/tokens"
	"github.com/ryoku-dev/ryotunes/internal/client/art"
)

func TestAccentInkMeetsContrastAcrossHues(t *testing.T) {
	for _, dark := range []bool{false, true} {
		roles := palette.Signature(dark)
		for hue := 0; hue < 360; hue += 15 {
			sample := brandHSLToRGB(float64(hue), .72, .5, 0xff)
			accent := AccentFromSample(sample, roles)
			if ratio := palette.ContrastRatio(accent.Ink, roles.Stone2); ratio < float64(tokens.ContrastMinimum) {
				t.Fatalf("dark=%v hue=%d contrast=%.3f ink=%#v stone2=%#v", dark, hue, ratio, accent.Ink, roles.Stone2)
			}
			if accent.On != palette.InkOn(accent.Fill) {
				t.Fatalf("dark=%v hue=%d on=%#v want InkOn(%#v)", dark, hue, accent.On, accent.Fill)
			}
		}
	}
}

func TestAccentClampsSaturationAndPinsLightness(t *testing.T) {
	for _, test := range []struct {
		dark      bool
		lightness float64
	}{
		{dark: true, lightness: .70},
		{dark: false, lightness: .42},
	} {
		accent := AccentFromSample(color.NRGBA{R: 180, G: 120, B: 120, A: 0xff}, palette.Signature(test.dark))
		_, saturation, lightness := brandRGBToHSL(accent.Fill)
		if saturation < .54 || saturation > .86 {
			t.Fatalf("dark=%v saturation=%f", test.dark, saturation)
		}
		if difference := lightness - test.lightness; difference < -.01 || difference > .01 {
			t.Fatalf("dark=%v lightness=%f want %f", test.dark, lightness, test.lightness)
		}
	}
}

func TestSourceAndDesktopAccentKeepTheirRoleContracts(t *testing.T) {
	roles := palette.Signature(true)
	source := AccentFromSource("youtube", roles)
	if source.Fill != BrandFill("youtube", roles) || source.Ink != BrandInk("youtube", roles) {
		t.Fatalf("source accent = %#v, want provider brand fill and ink", source)
	}
	desktop := AccentFromDesktop(roles)
	if desktop.Fill != roles.Sel || desktop.Fill != RyokuOrange || desktop.On != roles.OnSel || desktop.Ink != roles.GoldInk {
		t.Fatalf("desktop accent on Ryoku's colours = %#v, want the signature orange (sel) with its tuned ink", desktop)
	}
}

// wallpaperPalette is a matugen-like wallpaper palette whose primary,
// secondary container and tertiary differ in hue, as the owner's do.
func wallpaperPalette(dark bool, primary, container, tertiary, surface, onSurface color.NRGBA) palette.Palette {
	return palette.Palette{Dark: dark, Source: "wallpaper", Roles: map[string]color.NRGBA{
		"surface": surface, "onSurface": onSurface, "primary": primary, "onPrimary": {A: 0xff},
		"primaryContainer": container, "secondaryContainer": container, "tertiary": tertiary, "onTertiary": {A: 0xff},
		"surfaceContainerLow": surface, "surfaceContainer": surface, "surfaceContainerHigh": surface,
		"surfaceContainerHighest": surface, "outline": onSurface, "outlineVariant": onSurface,
	}}
}

func TestDesktopAccentIsTheDesktopsSelNotItsTertiary(t *testing.T) {
	black, bone := color.NRGBA{R: 21, G: 19, B: 12, A: 0xff}, color.NRGBA{R: 232, G: 226, B: 212, A: 0xff}
	for name, wallpaper := range map[string]palette.Palette{
		// yellow wallpaper: a yellow primary, a green tertiary
		"yellow": wallpaperPalette(true, color.NRGBA{R: 222, G: 196, B: 108, A: 0xff}, color.NRGBA{R: 82, G: 70, B: 30, A: 0xff}, color.NRGBA{R: 160, G: 208, B: 160, A: 0xff}, black, bone),
		// blue wallpaper: a blue primary, a purple tertiary
		"blue": wallpaperPalette(true, color.NRGBA{R: 155, G: 203, B: 250, A: 0xff}, color.NRGBA{R: 40, G: 72, B: 110, A: 0xff}, color.NRGBA{R: 211, G: 191, B: 230, A: 0xff}, black, bone),
	} {
		for look, roles := range map[string]palette.Roles{
			"lithic":   palette.FromPalette(wallpaper, palette.Look{Tint: palette.TintMedium}),
			"material": palette.Material(wallpaper, palette.Look{}),
		} {
			accent := AccentFromDesktop(roles)
			if accent.Fill != roles.Sel || accent.On != roles.OnSel {
				t.Fatalf("%s %s: accent %v on %v, want sel %v on %v", name, look, accent.Fill, accent.On, roles.Sel, roles.OnSel)
			}
			fill, tertiary := colorToOKLCH(accent.Fill), colorToOKLCH(wallpaper.Roles["tertiary"])
			if hueDistance(fill.H, tertiary.H) < .5 {
				t.Fatalf("%s %s: accent hue %.2f sits on the tertiary's %.2f", name, look, fill.H, tertiary.H)
			}
			for _, plane := range []color.NRGBA{roles.Ground, roles.Stone1, roles.Stone2, roles.Stone3, roles.Slab} {
				if ratio := palette.ContrastRatio(accent.Ink, plane); ratio < float64(tokens.ContrastMinimum)-.01 {
					t.Fatalf("%s %s: accent ink reads at %.2f:1 on a pane", name, look, ratio)
				}
			}
			theme := NewTheme(roles, libtheme.Comfortable)
			if theme.Accent != accent || theme.R.Gold != roles.Sel {
				t.Fatalf("%s %s: the theme's accent is not the desktop's sel", name, look)
			}
			// tint with the cover lays its hue over the planes, never over the accent
			tinted, _ := CoverTint(roles, art.Palette{Dominant: color.NRGBA{R: 200, G: 40, B: 60, A: 0xff}})
			if AccentFromDesktop(tinted).Fill != accent.Fill {
				t.Fatalf("%s %s: the cover tint moved the desktop accent", name, look)
			}
		}
	}
}

// hueDistance is the distance between two OKLCH hues in radians.
func hueDistance(a, b float64) float64 {
	d := math.Mod(math.Abs(a-b), 2*math.Pi)
	return min(d, 2*math.Pi-d)
}
