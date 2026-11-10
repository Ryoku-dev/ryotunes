// Package shell owns Ryotunes windows, routing, chrome, overlays, and input.
package shell

import (
	"image"
	"strings"
	"sync"

	"github.com/ryoku-dev/ryotunes/internal/client/ui/demo"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
)

type navigator struct {
	mu           sync.RWMutex
	stack        []env.Route
	forward      []env.Route
	panel        env.Panel
	nowPlaying   bool
	mini         bool
	focusSearch  bool
	collapsed    bool
	sidebarClass look.Class
	sidebarSheet bool
	invalidate   func()
	onChange     func()
	onMini       func(bool)
	onSound      func(image.Rectangle)
}

func newNavigator(initial env.Route, panel env.Panel, mini bool, invalidate func()) *navigator {
	if initial.Kind == "" {
		initial = env.Route{Kind: env.RouteHome}
	}
	return &navigator{stack: []env.Route{initial}, panel: panel, mini: mini, invalidate: invalidate}
}

func (n *navigator) changed() {
	if n.onChange != nil {
		n.onChange()
	}
	if n.invalidate != nil {
		n.invalidate()
	}
}

func (n *navigator) redraw() {
	if n.invalidate != nil {
		n.invalidate()
	}
}

func (n *navigator) Push(route env.Route) {
	if route.Kind == "" {
		return
	}
	n.mu.Lock()
	n.stack = append(n.stack, route)
	n.forward = nil
	n.nowPlaying = false
	n.mu.Unlock()
	n.changed()
}

func (n *navigator) Replace(route env.Route) {
	if route.Kind == "" {
		return
	}
	n.mu.Lock()
	if len(n.stack) == 0 {
		n.stack = []env.Route{route}
	} else {
		n.stack[len(n.stack)-1] = route
	}
	n.nowPlaying = false
	n.mu.Unlock()
	n.changed()
}

func (n *navigator) Back() bool {
	n.mu.Lock()
	if n.nowPlaying {
		n.nowPlaying = false
		n.mu.Unlock()
		n.changed()
		return true
	}
	if len(n.stack) <= 1 {
		n.mu.Unlock()
		return false
	}
	current := n.stack[len(n.stack)-1]
	n.stack = n.stack[:len(n.stack)-1]
	n.forward = append(n.forward, current)
	n.mu.Unlock()
	n.changed()
	return true
}

func (n *navigator) Forward() bool {
	n.mu.Lock()
	if len(n.forward) == 0 {
		n.mu.Unlock()
		return false
	}
	index := len(n.forward) - 1
	n.stack = append(n.stack, n.forward[index])
	n.forward = n.forward[:index]
	n.nowPlaying = false
	n.mu.Unlock()
	n.changed()
	return true
}

func (n *navigator) CanBack() bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.nowPlaying || len(n.stack) > 1
}

func (n *navigator) CanForward() bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return len(n.forward) > 0
}

func (n *navigator) root(route env.Route) {
	if route.Kind == "" {
		return
	}
	n.mu.Lock()
	n.stack = []env.Route{route}
	n.forward = nil
	n.nowPlaying = false
	n.mu.Unlock()
	n.changed()
}

func (n *navigator) Current() env.Route {
	n.mu.RLock()
	defer n.mu.RUnlock()
	if len(n.stack) == 0 {
		return env.Route{Kind: env.RouteHome}
	}
	return n.stack[len(n.stack)-1]
}

func (n *navigator) Stack() []env.Route {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return append([]env.Route(nil), n.stack...)
}

func (n *navigator) OpenPanel(panel env.Panel) {
	n.mu.Lock()
	n.panel = panel
	n.mu.Unlock()
	n.changed()
}

func (n *navigator) TogglePanel(panel env.Panel) {
	n.mu.Lock()
	if n.panel == panel {
		n.panel = env.PanelNone
	} else {
		n.panel = panel
	}
	n.mu.Unlock()
	n.changed()
}

func (n *navigator) Panel() env.Panel {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.panel
}

func (n *navigator) NowPlaying() bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.nowPlaying
}

func (n *navigator) ToggleNowPlaying() {
	n.mu.Lock()
	n.nowPlaying = !n.nowPlaying
	n.mu.Unlock()
	n.changed()
}

func (n *navigator) closeNowPlaying() bool {
	n.mu.Lock()
	wasOpen := n.nowPlaying
	n.nowPlaying = false
	n.mu.Unlock()
	if wasOpen {
		n.changed()
	}
	return wasOpen
}

func (n *navigator) ToggleMini() {
	n.mu.Lock()
	n.mini = !n.mini
	open := n.mini
	n.mu.Unlock()
	n.changed()
	if n.onMini != nil {
		n.onMini(open)
	}
}

func (n *navigator) FocusSearch() {
	n.mu.Lock()
	n.focusSearch = true
	n.mu.Unlock()
	n.redraw()
}

func (n *navigator) takeSearchFocus() bool {
	n.mu.Lock()
	value := n.focusSearch
	n.focusSearch = false
	n.mu.Unlock()
	return value
}

func (n *navigator) OpenSound(anchor image.Rectangle) {
	if n.onSound != nil {
		n.onSound(anchor)
	}
}

func (n *navigator) SidebarCollapsed() bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.collapsed
}

func (n *navigator) setSidebarClass(class look.Class) {
	n.mu.Lock()
	closed := n.sidebarSheet && class != look.Medium
	n.sidebarClass = class
	if class != look.Medium {
		n.sidebarSheet = false
	}
	n.mu.Unlock()
	if closed {
		n.redraw()
	}
}

func (n *navigator) SidebarSheetOpen() bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.sidebarSheet
}

func (n *navigator) CloseSidebarSheet() bool {
	n.mu.Lock()
	wasOpen := n.sidebarSheet
	n.sidebarSheet = false
	n.mu.Unlock()
	if wasOpen {
		n.redraw()
	}
	return wasOpen
}

func (n *navigator) ToggleSidebar() {
	n.mu.Lock()
	class := n.sidebarClass
	switch class {
	case look.Expanded:
		n.collapsed = !n.collapsed
	case look.Medium:
		n.sidebarSheet = !n.sidebarSheet
	default:
		n.mu.Unlock()
		return
	}
	n.mu.Unlock()
	if class == look.Expanded {
		n.changed()
	} else {
		n.redraw()
	}
}

func (n *navigator) miniOpen() bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.mini
}

func routeKey(route env.Route) string {
	parts := []string{string(route.Kind)}
	for _, value := range []string{route.ID, route.Params, route.Query, route.Title} {
		if value != "" {
			parts = append(parts, value)
		}
	}
	return strings.Join(parts, "\x1f")
}

func persistedRoute(route env.Route) string {
	if route.Kind == "" || route.Kind == env.RouteHome {
		return "home"
	}
	if route.ID != "" {
		return string(route.Kind) + ":" + route.ID
	}
	if route.Query != "" {
		return string(route.Kind) + ":" + route.Query
	}
	if route.Params != "" {
		return string(route.Kind) + ":" + route.Params
	}
	return string(route.Kind)
}

func parseRoute(value string) env.Route {
	kind, data, found := strings.Cut(strings.TrimSpace(value), ":")
	route := env.Route{Kind: env.RouteKind(kind)}
	switch route.Kind {
	case env.RouteSearch:
		if found {
			route.Query = data
		}
	case env.RouteArtist, env.RouteAlbum, env.RoutePlaylist, env.RouteList:
		if found {
			route.ID = data
		}
	case env.RouteSettings, env.RouteHome, env.RouteLibrary:
		if found {
			route.Params = data
		}
	case env.RouteRadio, env.RouteDownloads, env.RouteDiagnostics, env.RouteEqualizer:
	default:
		return env.Route{Kind: env.RouteHome}
	}
	return route
}

func initialRoute(stored string, options Options) env.Route {
	route := parseRoute(stored)
	if options.Route != "" {
		route = parseRoute(options.Route)
	}
	if options.Query != "" {
		route = env.Route{Kind: env.RouteSearch, Query: options.Query, Title: "Search"}
	}
	if options.Demo && route.ID == "" {
		switch route.Kind {
		case env.RouteArtist:
			route.ID = demo.ArtistID
		case env.RouteAlbum:
			route.ID = demo.AlbumID
		case env.RoutePlaylist:
			route.ID = demo.PlaylistID
		}
	}
	return route
}

func parsePanel(value string) env.Panel {
	switch value {
	case "queue":
		return env.PanelQueue
	case "lyrics":
		return env.PanelLyrics
	case "listen", "listen-together":
		return env.PanelListen
	default:
		return env.PanelNone
	}
}

func panelOpensNowPlaying(value string) bool {
	return value == "now" || value == "now-playing"
}
