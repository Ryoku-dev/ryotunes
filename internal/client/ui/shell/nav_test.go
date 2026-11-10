package shell

import (
	"image"
	"testing"

	"gioui.org/layout"

	"github.com/ryoku-dev/ryotunes/internal/client/state"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
)

func TestNavigatorBackForwardAndReplace(t *testing.T) {
	nav := newNavigator(env.Route{Kind: env.RouteHome}, env.PanelNone, false, nil)
	library := env.Route{Kind: env.RouteLibrary}
	album := env.Route{Kind: env.RouteAlbum, ID: "album-1"}
	nav.Push(library)
	nav.Push(album)

	if !nav.CanBack() || nav.CanForward() {
		t.Fatalf("history before Back: canBack=%v canForward=%v", nav.CanBack(), nav.CanForward())
	}
	if !nav.Back() || nav.Current() != library {
		t.Fatalf("Back current = %#v, want %#v", nav.Current(), library)
	}
	if !nav.CanForward() || !nav.Forward() || nav.Current() != album {
		t.Fatalf("Forward current = %#v, want %#v", nav.Current(), album)
	}

	replacement := env.Route{Kind: env.RoutePlaylist, ID: "playlist-1"}
	nav.Replace(replacement)
	if nav.Current() != replacement {
		t.Fatalf("Replace current = %#v, want %#v", nav.Current(), replacement)
	}
	if !nav.Back() || nav.Current() != library {
		t.Fatalf("Back after Replace current = %#v, want %#v", nav.Current(), library)
	}
	nav.Push(env.Route{Kind: env.RouteRadio})
	if nav.CanForward() {
		t.Fatal("Push did not clear forward history")
	}
}

func TestParseHomeRoutePreservesChipParams(t *testing.T) {
	route := parseRoute("home:focus")
	if route.Kind != env.RouteHome || route.Params != "focus" {
		t.Fatalf("route = %#v, want Home focus", route)
	}
}

func TestRelaunchReopensLikedSongsTab(t *testing.T) {
	liked := env.Route{Kind: env.RouteLibrary, Params: "songs", Title: "Liked songs"}
	route := initialRoute(persistedRoute(liked), Options{})
	if route.Kind != env.RouteLibrary || route.Params != "songs" {
		t.Fatalf("relaunched route = %#v, want Library songs", route)
	}
}

func TestNavigatorBackClosesNowPlayingFirst(t *testing.T) {
	nav := newNavigator(env.Route{Kind: env.RouteHome}, env.PanelNone, false, nil)
	nav.ToggleNowPlaying()
	if !nav.Back() || nav.NowPlaying() {
		t.Fatal("Back did not close the now playing view")
	}
	if nav.Back() {
		t.Fatal("Back moved past the root route")
	}
}

func TestPanelDockingBoundaryUsesTargetSidebarWidth(t *testing.T) {
	if panelDockEligible(955, look.Medium, false) {
		t.Fatal("955 dp medium frame docked below the content minimum")
	}
	if !panelDockEligible(956, look.Medium, false) {
		t.Fatal("956 dp medium frame did not dock at the exact content minimum")
	}
	if panelDockEligible(1280, look.Compact, false) {
		t.Fatal("compact frame allowed a docked panel")
	}
	if !panelDockEligible(1280, look.Expanded, false) {
		t.Fatal("expanded default frame did not dock the panel")
	}
	if panelDockEligible(1100, look.Expanded, false) {
		t.Fatal("expanded open target docked below the content minimum")
	}
	if !panelDockEligible(1100, look.Expanded, true) {
		t.Fatal("expanded folded target did not use the rail width")
	}
}

func TestNavigatorSidebarToggleFoldsExpandedAndSheetsMedium(t *testing.T) {
	nav := newNavigator(env.Route{Kind: env.RouteHome}, env.PanelNone, false, nil)
	nav.setSidebarClass(look.Expanded)
	nav.ToggleSidebar()
	if !nav.SidebarCollapsed() || nav.SidebarSheetOpen() {
		t.Fatal("expanded toggle did not fold only the persistent sidebar")
	}
	nav.setSidebarClass(look.Medium)
	nav.ToggleSidebar()
	if !nav.SidebarCollapsed() || !nav.SidebarSheetOpen() {
		t.Fatal("medium toggle changed the fold preference or failed to open the sheet")
	}
	nav.ToggleSidebar()
	if nav.SidebarSheetOpen() {
		t.Fatal("second medium toggle did not close the sheet")
	}
}

func TestMiniSidebarShortcutDoesNotChangePersistentFold(t *testing.T) {
	nav := newNavigator(env.Route{Kind: env.RouteHome}, env.PanelNone, false, nil)
	nav.setSidebarClass(look.Expanded)
	shell := &uiShell{nav: nav, currentMini: true}
	shell.toggleSidebar(&env.Env{Nav: nav})
	if nav.SidebarCollapsed() {
		t.Fatal("mini sidebar shortcut changed the main window fold preference")
	}

	shell.currentMini = false
	shell.toggleSidebar(&env.Env{Nav: nav})
	if !nav.SidebarCollapsed() {
		t.Fatal("main window sidebar shortcut did not change the fold preference")
	}
}

func TestFoldFrameGeometryNeverOverlaps(t *testing.T) {
	size := image.Pt(1280, 800)
	const margin, gap, panel = 8, 8, 340
	for _, progress := range []float32{0, .5, 1} {
		sidebar := 240 + int(float32(72-240)*progress+.5)
		rects := resolveFrameRects(size, 56, 696, margin, gap, sidebar, panel)
		if !rects.Sidebar.Intersect(rects.Content).Empty() || !rects.Content.Intersect(rects.Panel).Empty() {
			t.Fatalf("progress %.1f overlaps: sidebar=%v content=%v panel=%v", progress, rects.Sidebar, rects.Content, rects.Panel)
		}
		if rects.Content.Min.X-rects.Sidebar.Max.X != gap || rects.Panel.Min.X-rects.Content.Max.X != gap {
			t.Fatalf("progress %.1f gaps = (%d, %d), want %d", progress, rects.Content.Min.X-rects.Sidebar.Max.X, rects.Panel.Min.X-rects.Content.Max.X, gap)
		}
		if rects.Content.Dx() < int(look.ContentMinWithPanel) {
			t.Fatalf("progress %.1f content width = %d, want at least %d", progress, rects.Content.Dx(), int(look.ContentMinWithPanel))
		}
	}
}

func TestEscapeClosesMediumSidebarSheetBeforeOtherSheets(t *testing.T) {
	nav := newNavigator(env.Route{Kind: env.RouteHome}, env.PanelQueue, false, nil)
	nav.ToggleNowPlaying()
	nav.setSidebarClass(look.Medium)
	nav.ToggleSidebar()
	shell := &uiShell{nav: nav}
	e := &env.Env{Nav: nav, Overlays: new(look.Overlays)}

	shell.escape(layout.Context{}, e)

	if nav.SidebarSheetOpen() {
		t.Fatal("Escape left the medium sidebar sheet open")
	}
	if nav.Panel() != env.PanelQueue || !nav.NowPlaying() {
		t.Fatal("Escape closed content below the medium sidebar sheet")
	}
}

func TestEscapeLeavesTheEqualizerOnlyAfterEverythingAboveIt(t *testing.T) {
	nav := newNavigator(env.Route{Kind: env.RouteHome}, env.PanelNone, false, nil)
	nav.Push(env.Route{Kind: env.RouteEqualizer})
	nav.ToggleNowPlaying()
	shell := &uiShell{nav: nav, pages: map[string]env.Page{}}
	e := &env.Env{Nav: nav, Overlays: new(look.Overlays)}

	shell.escape(layout.Context{}, e)
	if nav.NowPlaying() || nav.Current().Kind != env.RouteEqualizer {
		t.Fatalf("first Escape: now playing %v over %q, want only now playing closed", nav.NowPlaying(), nav.Current().Kind)
	}
	shell.currentMini = true
	shell.escape(layout.Context{}, e)
	if nav.Current().Kind != env.RouteEqualizer {
		t.Fatal("Escape in the mini window left the main window's equalizer")
	}
	shell.currentMini = false
	shell.escape(layout.Context{}, e)
	if nav.Current().Kind != env.RouteHome {
		t.Fatalf("Escape on the equalizer went to %q, want back to where it was opened", nav.Current().Kind)
	}
	shell.escape(layout.Context{}, e)
	if nav.Current().Kind != env.RouteHome {
		t.Fatalf("Escape on home navigated to %q", nav.Current().Kind)
	}
}

func TestLeavingAnEqualizerRestoredAtLaunchOpensSoundSettings(t *testing.T) {
	nav := newNavigator(parseRoute(persistedRoute(env.Route{Kind: env.RouteEqualizer})), env.PanelNone, false, nil)
	shell := &uiShell{nav: nav, pages: map[string]env.Page{}}

	shell.escape(layout.Context{}, &env.Env{Nav: nav, Overlays: new(look.Overlays)})

	if current := nav.Current(); current.Kind != env.RouteSettings || current.Params != "sound" {
		t.Fatalf("leaving the first page's equalizer went to %#v, want Settings at Sound", current)
	}
	if nav.CanBack() {
		t.Fatal("the equalizer stayed in the history behind Settings")
	}
}

func TestEffectiveWidthKeepsDefaultWindowExpandedAtSupportedZoom(t *testing.T) {
	preferences := state.DefaultClientPreferences()
	for _, zoom := range []int{100, 110} {
		preferences.ZoomPercent = zoom
		width := effectiveInitialWidth(1280, preferences)
		if class := look.ClassFor(width); class != look.Expanded {
			t.Fatalf("1280 dp at %d%% zoom has effective width %.2f and class %v, want expanded", zoom, width, class)
		}
	}
}
