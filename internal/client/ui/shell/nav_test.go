package shell

import (
	"testing"

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

func TestPanelDockingBoundary(t *testing.T) {
	if panelDockEligible(955, look.Medium) {
		t.Fatal("955 dp medium frame docked below the content minimum")
	}
	if !panelDockEligible(956, look.Medium) {
		t.Fatal("956 dp medium frame did not dock at the exact content minimum")
	}
	if panelDockEligible(1280, look.Compact) {
		t.Fatal("compact frame allowed a docked panel")
	}
	if !panelDockEligible(1280, look.Expanded) {
		t.Fatal("expanded default frame did not dock the panel")
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
