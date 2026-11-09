package chrome

import (
	"image"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/palette"
	libtheme "github.com/ryoku-dev/ryoku/docs/lithic/library/go/theme"
	"github.com/ryoku-dev/ryotunes/internal/client/state"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

type topBarTestNav struct {
	current   env.Route
	collapsed bool
	toggles   int
}

func (nav *topBarTestNav) Push(route env.Route)    { nav.current = route }
func (nav *topBarTestNav) Replace(route env.Route) { nav.current = route }
func (*topBarTestNav) Back() bool                  { return false }
func (*topBarTestNav) Forward() bool               { return false }
func (*topBarTestNav) CanBack() bool               { return false }
func (*topBarTestNav) CanForward() bool            { return false }
func (nav *topBarTestNav) Current() env.Route      { return nav.current }
func (nav *topBarTestNav) Stack() []env.Route      { return []env.Route{nav.current} }
func (*topBarTestNav) OpenPanel(env.Panel)         {}
func (*topBarTestNav) TogglePanel(env.Panel)       {}
func (*topBarTestNav) Panel() env.Panel            { return env.PanelNone }
func (*topBarTestNav) NowPlaying() bool            { return false }
func (*topBarTestNav) ToggleNowPlaying()           {}
func (*topBarTestNav) ToggleMini()                 {}
func (*topBarTestNav) FocusSearch()                {}
func (*topBarTestNav) OpenSound(image.Rectangle)   {}
func (nav *topBarTestNav) SidebarCollapsed() bool  { return nav.collapsed }
func (nav *topBarTestNav) ToggleSidebar() {
	nav.collapsed = !nav.collapsed
	nav.toggles++
}

func TestTopBarControlsAreNotWindowMoveAreas(t *testing.T) {
	const width, height = 1280, 56
	metric := unit.Metric{PxPerDp: 1, PxPerSp: 1}
	store := state.New(protocol.Snapshot{Provider: model.ProviderYouTube})
	theme := look.NewTheme(palette.Signature(true), libtheme.Comfortable)
	nav := &topBarTestNav{current: env.Route{Kind: env.RouteHome}}
	e := &env.Env{Th: theme, Overlays: new(look.Overlays), Store: store, Nav: nav}
	frame := Frame{Class: look.Expanded, Sidebar: 248, Content: image.Rect(256, 56, width-8, 700), Decorated: false}

	var top TopBar
	var operations op.Ops
	var router input.Router
	now := time.Now()
	layoutFrame := func() {
		operations.Reset()
		gtx := layout.Context{
			Ops:         &operations,
			Now:         now,
			Source:      router.Source(),
			Metric:      metric,
			Constraints: layout.Exact(image.Pt(width, height)),
		}
		top.Layout(gtx, e, frame)
		router.Frame(&operations)
		now = now.Add(time.Millisecond)
	}
	click := func(point f32.Point) {
		router.Queue(
			pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 1, Buttons: pointer.ButtonPrimary, Position: point},
			pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 1, Position: point},
		)
		layoutFrame()
	}

	layoutFrame()
	spotify := f32.Pt(1074, 28)
	discord := f32.Pt(977, 28)
	blank := f32.Pt(930, 28)
	controlPoints := map[string]f32.Point{
		"Sidebar":    f32.Pt(272, 28),
		"Back":       f32.Pt(308, 28),
		"Forward":    f32.Pt(344, 28),
		"Search":     f32.Pt(500, 28),
		"Discord":    discord,
		"YouTube":    f32.Pt(1030, 28),
		"Spotify":    spotify,
		"SoundCloud": f32.Pt(1118, 28),
		"TIDAL":      f32.Pt(1162, 28),
		"Maximize":   f32.Pt(1215, 28),
		"Close":      f32.Pt(1251, 28),
	}
	for name, point := range controlPoints {
		if action, ok := router.ActionAt(point); ok {
			t.Fatalf("%s control is covered by window action %v", name, action)
		}
	}
	if action, ok := router.ActionAt(blank); !ok || action != system.ActionMove {
		t.Fatalf("blank top-bar area action = %v, %v; want ActionMove", action, ok)
	}
	click(controlPoints["Sidebar"])
	if nav.toggles != 1 || !nav.collapsed {
		t.Fatalf("sidebar toggle state = (%d, %v), want (1, true)", nav.toggles, nav.collapsed)
	}

	click(spotify)
	if top.providers.display != model.ProviderSpotify {
		t.Fatalf("provider after Spotify click = %q; want %q", top.providers.display, model.ProviderSpotify)
	}

	click(discord)
	if !discordEnabled(store.Snapshot().Settings) {
		t.Fatal("Discord setting did not turn on after quick-toggle click")
	}

	frame.Class = look.Compact
	frame.Sidebar = 0
	frame.Content = image.Rect(0, 56, width-8, 700)
	layoutFrame()
	compactProvider := f32.Pt(1166, 28)
	for name, point := range map[string]f32.Point{
		"Compact search":   f32.Pt(66, 28),
		"Compact Discord":  f32.Pt(1117, 28),
		"Compact provider": compactProvider,
	} {
		if action, ok := router.ActionAt(point); ok {
			t.Fatalf("%s control is covered by window action %v", name, action)
		}
	}
	click(compactProvider)
	if !e.Overlays.IsOpen(&top.providers.menuTag) {
		t.Fatal("compact provider click did not open its menu")
	}
}
