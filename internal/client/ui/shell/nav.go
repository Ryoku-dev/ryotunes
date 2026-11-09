// Package shell owns Ryotunes windows, routing, chrome, overlays, and input.
package shell

import (
	"strings"
	"sync"

	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
)

type navigator struct {
	mu          sync.RWMutex
	stack       []env.Route
	panel       env.Panel
	mini        bool
	palette     bool
	focusSearch bool
	invalidate  func()
	onChange    func()
	onMini      func(bool)
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
	n.mu.Unlock()
	n.changed()
}

func (n *navigator) Back() bool {
	n.mu.Lock()
	if len(n.stack) <= 1 {
		n.mu.Unlock()
		return false
	}
	n.stack = n.stack[:len(n.stack)-1]
	n.mu.Unlock()
	n.changed()
	return true
}

func (n *navigator) root(route env.Route) {
	if route.Kind == "" {
		return
	}
	n.mu.Lock()
	n.stack = []env.Route{route}
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

func (n *navigator) OpenPalette() {
	n.mu.Lock()
	n.palette = true
	n.mu.Unlock()
	n.redraw()
}

func (n *navigator) togglePalette() {
	n.mu.Lock()
	n.palette = !n.palette
	n.mu.Unlock()
	n.redraw()
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

func (n *navigator) paletteOpen() bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.palette
}

func (n *navigator) closePalette() bool {
	n.mu.Lock()
	wasOpen := n.palette
	n.palette = false
	n.mu.Unlock()
	if wasOpen {
		n.redraw()
	}
	return wasOpen
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
	case env.RouteSettings:
		if found {
			route.Params = data
		}
	case env.RouteHome, env.RouteLibrary, env.RouteRadio, env.RouteDownloads, env.RouteDiagnostics:
	default:
		return env.Route{Kind: env.RouteHome}
	}
	return route
}

func parsePanel(value string) env.Panel {
	switch value {
	case "queue":
		return env.PanelQueue
	case "lyrics":
		return env.PanelLyrics
	case "now", "now-playing":
		return env.PanelNowPlaying
	case "listen", "listen-together":
		return env.PanelListen
	default:
		return env.PanelNone
	}
}
