package look

import (
	"image/color"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gioui.org/layout"
	"gioui.org/op"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/anim"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/design"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/palette"
	libtheme "github.com/ryoku-dev/ryoku/docs/lithic/library/go/theme"
	"github.com/ryoku-dev/ryotunes/internal/client/art"
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

func TestMixRolesKeepsEveryRole(t *testing.T) {
	from := palette.Signature(true)
	to := palette.FromPalette(palette.Palette{Dark: true, Source: "wallpaper", Roles: map[string]color.NRGBA{
		"surface": {R: 20, G: 30, B: 60, A: 0xff}, "onSurface": {R: 220, G: 225, B: 240, A: 0xff},
		"primary": {R: 120, G: 180, B: 250, A: 0xff},
	}}, palette.Look{Contrast: palette.High, Tint: palette.TintMedium})
	if got := mixRoles(from, to, 1); got != to {
		t.Fatalf("landed tween = %#v, want the target roles", got)
	}
	if got := mixRoles(from, to, 0); got.Slab != from.Slab || got.Field != from.Field || got.Edge != from.Edge || got.Container != from.Container {
		t.Fatal("start of tween lost the from roles' slab, field, edge or container")
	}
	half := mixRoles(from, to, .5)
	for name, pair := range map[string][3]color.NRGBA{
		"slab": {from.Slab, to.Slab, half.Slab}, "field": {from.Field, to.Field, half.Field},
		"edge": {from.Edge, to.Edge, half.Edge}, "container": {from.Container, to.Container, half.Container},
	} {
		if pair[0] != pair[1] && (pair[2] == pair[0] || pair[2] == pair[1]) {
			t.Errorf("%s did not mix: from %v to %v half %v", name, pair[0], pair[1], pair[2])
		}
	}
	if half.MineralField(palette.Alive) != to.MineralField(palette.Alive) {
		t.Fatal("mixed roles lost the target ladder's mineral field lightness")
	}
}

func TestSetDesktopSwapsDesignRolesAndMotion(t *testing.T) {
	defer anim.SetReduce(false)
	config := t.TempDir()
	writeDesktopFile(t, filepath.Join(config, "ryoku", "shell.json"), `{"theme":{"design":"fluent","contrast":"high","motion":{"reduce":true}},"showJapaneseGloss":false}`)
	writeDesktopFile(t, filepath.Join(config, "ryoku", "matugen.json"), `{"mode":"light"}`)
	desktop := palette.ReadDesktop(palette.WithEnv(false), palette.WithDirs(config, t.TempDir(), filepath.Join(config, "os-release")))

	theme := NewTheme(palette.Signature(true), libtheme.Compact)
	shaper := theme.Shaper
	theme.SetDesktop(desktop)
	if theme.Design != design.Fluent || theme.Desktop.Gloss || theme.R.Dark {
		t.Fatalf("design %v gloss %v dark %v, want fluent, no gloss, light", theme.Design, theme.Desktop.Gloss, theme.R.Dark)
	}
	if theme.LoadedRoles() != desktop.Roles || theme.R.Stone3 != desktop.Roles.Stone3 {
		t.Fatal("theme does not wear the desktop's roles")
	}
	if !anim.Reduced() {
		t.Fatal("desktop reduced motion did not reach anim")
	}
	if theme.Shaper != shaper || theme.Density != libtheme.Compact {
		t.Fatal("swapping the desktop dropped the shaper or the density")
	}
}

func writeDesktopFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestThemeRolesSnapAndNextSchemeStartsFresh(t *testing.T) {
	theme := NewTheme(palette.Signature(true), libtheme.Comfortable)
	var operations op.Ops
	gtx := layout.Context{Ops: &operations, Now: time.Unix(3000, 0)}
	current := AccentFromSample(color.NRGBA{R: 220, G: 40, B: 80, A: 0xff}, theme.LoadedRoles())
	theme.SetAccent(gtx, current)

	light := palette.Signature(false)
	theme.SetRoles(light)
	if theme.R != light {
		t.Fatalf("SetRoles did not snap to the untouched light palette")
	}
	targetRoles, target := theme.ArtScheme(art.Palette{
		Vibrant:  color.NRGBA{R: 20, G: 120, B: 220, A: 0xff},
		Dominant: color.NRGBA{R: 30, G: 80, B: 150, A: 0xff},
	}, true)
	theme.SetScheme(gtx, targetRoles, target)
	if theme.Accent != target || theme.R.Stone2 != targetRoles.Stone2 {
		t.Fatalf("post-palette scheme did not snap to its first target")
	}
	if theme.LoadedRoles() != light {
		t.Fatal("loaded roles were changed by cover tint")
	}
}

func TestThemeTweensTintAccentAndSourceTogether(t *testing.T) {
	base := palette.Signature(true)
	theme := NewTheme(base, libtheme.Comfortable, model.ProviderYouTube)
	var operations op.Ops
	start := time.Unix(4000, 0)
	gtx := layout.Context{Ops: &operations, Now: start}
	theme.SetAccent(gtx, AccentFromDesktop(base))

	targetRoles, targetAccent := theme.ArtScheme(art.Palette{
		Vibrant:  color.NRGBA{R: 239, G: 93, B: 72, A: 255},
		Dominant: color.NRGBA{R: 90, G: 95, B: 120, A: 255},
	}, true)
	initialSource := sourceFor(model.ProviderYouTube, base)
	targetSource := sourceFor(model.ProviderYouTube, targetRoles)
	theme.SetScheme(gtx, targetRoles, targetAccent)
	if theme.R.Stone2 != base.Stone2 || theme.Accent != AccentFromDesktop(base) || theme.Src != initialSource {
		t.Fatal("scheme change did not begin at the current roles, accent, and source field")
	}
	if theme.srcTo != targetSource {
		t.Fatalf("source target = %#v, want tinted scheme source %#v", theme.srcTo, targetSource)
	}
	gtx.Now = start.Add(theme.Motion.Field / 2)
	theme.Animate(gtx)
	// stone3: the dark signature's lower stones are black, which no tint moves
	if theme.R.Stone3 == base.Stone3 || theme.R.Stone3 == targetRoles.Stone3 {
		t.Fatalf("halfway stone3 = %#v, want an intermediate tint", theme.R.Stone3)
	}
	if theme.Accent == AccentFromDesktop(base) || theme.Accent == targetAccent {
		t.Fatalf("halfway accent = %#v, want an intermediate accent", theme.Accent)
	}
	gtx.Now = start.Add(theme.Motion.Field)
	theme.Animate(gtx)
	if theme.R.Stone3 != targetRoles.Stone3 || theme.Accent != targetAccent || theme.Src != targetSource {
		t.Fatal("tint, accent, and source field did not finish together")
	}
	if ratio := palette.ContrastRatio(theme.Src.Ink, theme.R.Stone2); ratio < 4.5 {
		t.Fatalf("source ink contrast on tinted stone2 = %.3f", ratio)
	}

	theme.SetProvider(gtx, model.ProviderSpotify)
	if want := sourceFor(model.ProviderSpotify, targetRoles); theme.srcTo != want {
		t.Fatalf("provider target = %#v, want source derived from tinted scheme %#v", theme.srcTo, want)
	}
}
