package look

import (
	"image/color"
	"testing"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/palette"
	libtheme "github.com/ryoku-dev/ryoku/docs/lithic/library/go/theme"
	"github.com/ryoku-dev/ryotunes/internal/model"
)

func TestThemeFirstProviderAssignmentSnaps(t *testing.T) {
	roles := palette.Signature(true)
	theme := NewTheme(roles, libtheme.Comfortable)
	var operations op.Ops
	gtx := layout.Context{Ops: &operations, Now: time.Unix(1000, 0)}

	theme.SetProvider(gtx, model.ProviderYouTube)
	if want := sourceFor(model.ProviderYouTube, roles); theme.Src != want {
		t.Fatalf("first provider source = %#v, want %#v", theme.Src, want)
	}

	before := theme.Src
	theme.SetProvider(gtx, model.ProviderSpotify)
	if theme.Src.Fill != before.Fill || theme.Src.Ink != before.Ink || theme.Src.Field != before.Field {
		t.Fatalf("real provider switch snapped colors to %#v, want tween start %#v", theme.Src, before)
	}
	if want := sourceFor(model.ProviderSpotify, roles); theme.srcTo != want {
		t.Fatalf("provider switch target = %#v, want %#v", theme.srcTo, want)
	}
}

func TestThemeKnownProviderStartsAtProviderSource(t *testing.T) {
	roles := palette.Signature(true)
	theme := NewTheme(roles, libtheme.Comfortable, model.ProviderTidal)
	if want := sourceFor(model.ProviderTidal, roles); theme.Src != want {
		t.Fatalf("initial provider source = %#v, want %#v", theme.Src, want)
	}
}

func TestThemeAccentSnapsThenTweens(t *testing.T) {
	theme := NewTheme(palette.Signature(true), libtheme.Comfortable)
	var operations op.Ops
	start := time.Unix(1000, 0)
	gtx := layout.Context{Ops: &operations, Now: start}
	first := Accent{
		Fill:  color.NRGBA{R: 20, G: 40, B: 60, A: 0xff},
		On:    color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff},
		Ink:   color.NRGBA{R: 70, G: 90, B: 110, A: 0xff},
		Field: color.NRGBA{R: 12, G: 22, B: 32, A: 0xff},
	}
	second := Accent{
		Fill:  color.NRGBA{R: 180, G: 120, B: 40, A: 0xff},
		On:    color.NRGBA{A: 0xff},
		Ink:   color.NRGBA{R: 210, G: 170, B: 90, A: 0xff},
		Field: color.NRGBA{R: 50, G: 40, B: 30, A: 0xff},
	}

	theme.SetAccent(gtx, first)
	if theme.Accent != first || theme.R.Gold != first.Fill || theme.R.OnGold != first.On || theme.R.GoldInk != first.Ink {
		t.Fatalf("first accent did not snap and reapply roles: %#v %#v", theme.Accent, theme.R)
	}
	theme.SetAccent(gtx, second)
	if theme.Accent != first {
		t.Fatalf("accent change snapped to %#v, want tween start %#v", theme.Accent, first)
	}
	gtx.Now = start.Add(theme.Motion.Field / 2)
	theme.Animate(gtx)
	if theme.Accent == first || theme.Accent == second {
		t.Fatalf("halfway accent = %#v, want an intermediate colour", theme.Accent)
	}
	gtx.Now = start.Add(theme.Motion.Field)
	theme.Animate(gtx)
	if theme.Accent != second || theme.R.Gold != second.Fill {
		t.Fatalf("finished accent = %#v roles gold=%#v, want %#v", theme.Accent, theme.R.Gold, second)
	}
}

func TestThemeCornerSnapsThenTweens(t *testing.T) {
	theme := NewTheme(palette.Signature(true), libtheme.Comfortable)
	var operations op.Ops
	start := time.Unix(2000, 0)
	gtx := layout.Context{Ops: &operations, Now: start}

	theme.SetCorner(gtx, unit.Dp(8))
	if theme.Cut.Large != 8 {
		t.Fatalf("first corner = %v, want 8", theme.Cut.Large)
	}
	theme.SetCorner(gtx, unit.Dp(28))
	if theme.Cut.Large != 8 {
		t.Fatalf("corner change snapped to %v, want tween start 8", theme.Cut.Large)
	}
	gtx.Now = start.Add(theme.Motion.Select / 2)
	theme.SetCorner(gtx, unit.Dp(28))
	if theme.Cut.Large <= 8 || theme.Cut.Large >= 28 {
		t.Fatalf("halfway corner = %v, want between 8 and 28", theme.Cut.Large)
	}
	gtx.Now = start.Add(theme.Motion.Select)
	theme.SetCorner(gtx, unit.Dp(28))
	if theme.Cut.Large != 28 {
		t.Fatalf("finished corner = %v, want 28", theme.Cut.Large)
	}
}

func TestThemeRolesReapplyCurrentAccentAndSnapNextTarget(t *testing.T) {
	theme := NewTheme(palette.Signature(true), libtheme.Comfortable)
	var operations op.Ops
	gtx := layout.Context{Ops: &operations, Now: time.Unix(3000, 0)}
	current := AccentFromSample(color.NRGBA{R: 220, G: 40, B: 80, A: 0xff}, theme.LoadedRoles())
	theme.SetAccent(gtx, current)

	light := palette.Signature(false)
	theme.SetRoles(light)
	if theme.R.Gold != current.Fill {
		t.Fatalf("SetRoles lost current accent: gold=%#v want %#v", theme.R.Gold, current.Fill)
	}
	target := AccentFromSample(color.NRGBA{R: 20, G: 120, B: 220, A: 0xff}, light)
	theme.SetAccent(gtx, target)
	if theme.Accent != target {
		t.Fatalf("post-palette accent = %#v, want snapped %#v", theme.Accent, target)
	}
}
