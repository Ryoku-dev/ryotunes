package look

import (
	"image/color"
	"testing"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/palette"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/tokens"
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
	if desktop.Fill != roles.Gold || desktop.On != roles.OnGold || desktop.Ink != roles.GoldInk {
		t.Fatalf("desktop accent = %#v, want loaded gold roles", desktop)
	}
}
