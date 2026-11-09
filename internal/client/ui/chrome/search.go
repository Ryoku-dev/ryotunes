package chrome

import (
	"image"
	"strings"
	"time"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/act"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

const topSearchDebounce = 200 * time.Millisecond

type topSearch struct {
	field kit.Search
	menu  kit.Menu
	tag   struct{}

	recent      []string
	suggestions []string
	lastText    string
	routeQuery  string
	lastRoute   env.Route
	routeReady  bool
	provider    model.Provider
	due         time.Time
	request     uint64
	highlight   int
	focusNext   bool
	highlighted bool
	active      bool
	suppressed  bool
	wasFocused  bool
}

func (search *topSearch) focus(gtx layout.Context) {
	search.focusNext = true
	search.suppressed = false
	search.field.Focus(gtx)
}

func (search *topSearch) focused(gtx layout.Context) bool {
	return search.focusNext || gtx.Focused(&search.field.Editor)
}

func topSearchQuery(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func (search *topSearch) syncRoute(gtx layout.Context, route env.Route) bool {
	changed := search.routeReady && route != search.lastRoute
	search.lastRoute, search.routeReady = route, true
	if route.Kind == env.RouteSearch {
		if route.Query != search.routeQuery {
			search.routeQuery = route.Query
			search.field.SetText(route.Query)
			search.lastText = route.Query
		}
		return changed
	}
	if changed || search.routeQuery != "" {
		search.routeQuery = ""
		search.field.SetText("")
		search.lastText = ""
		search.suggestions = nil
		search.highlighted = false
		search.request++
		if search.focused(gtx) {
			gtx.Execute(key.FocusCmd{Tag: nil})
		}
	}
	return changed
}

func (search *topSearch) remember(query string) {
	query = topSearchQuery(query)
	if query == "" {
		return
	}
	recent := []string{query}
	for _, existing := range search.recent {
		if !strings.EqualFold(existing, query) {
			recent = append(recent, existing)
		}
		if len(recent) == 8 {
			break
		}
	}
	search.recent = recent
}

func (search *topSearch) submit(e *env.Env, query string) {
	query = topSearchQuery(query)
	if query == "" || e == nil || e.Nav == nil {
		return
	}
	search.remember(query)
	search.routeQuery = query
	search.field.SetText(query)
	search.lastText = query
	search.suggestions = nil
	search.highlighted = false
	search.suppressed = true
	search.request++
	if e.Overlays != nil {
		e.Overlays.Close(&search.tag)
	}
	e.Nav.Push(env.Route{Kind: env.RouteSearch, Query: query, Title: "Search"})
}

func topSearchResultTitles(result model.SearchAll) []string {
	groups := [][]model.BrowseItem{result.Top, result.Songs, result.Albums, result.Artists, result.Playlists}
	seen := make(map[string]struct{}, 6)
	out := make([]string, 0, 6)
	for _, group := range groups {
		for _, item := range group {
			title := topSearchQuery(item.Title)
			key := strings.ToLower(title)
			if title == "" {
				continue
			}
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, title)
			if len(out) == 6 {
				return out
			}
		}
	}
	return out
}

func (search *topSearch) beginSuggestions(e *env.Env, query string, provider model.Provider) {
	search.request++
	request := search.request
	act.Call[model.SearchAll](e, protocol.MethodSearchAll, protocol.QueryParams{Query: query}, func(result model.SearchAll, err error) {
		if request != search.request || err != nil || provider != search.provider || query != topSearchQuery(search.field.Text()) {
			return
		}
		search.suggestions = topSearchResultTitles(result)
		search.highlighted = false
	})
}

func (search *topSearch) update(gtx layout.Context, e *env.Env) (escaped, blurred bool) {
	entries := search.entries()
	for {
		raw, ok := gtx.Event(
			key.Filter{Focus: &search.field.Editor, Name: key.NameEscape},
			key.Filter{Focus: &search.field.Editor, Name: key.NameUpArrow},
			key.Filter{Focus: &search.field.Editor, Name: key.NameDownArrow},
		)
		if !ok {
			break
		}
		pressed, ok := raw.(key.Event)
		if !ok || pressed.State != key.Press {
			continue
		}
		switch pressed.Name {
		case key.NameEscape:
			escaped = true
			search.suggestions = nil
			search.highlighted = false
			search.request++
			if e != nil && e.Overlays != nil {
				e.Overlays.Close(&search.tag)
			}
			if search.field.Text() != "" || !search.suppressed {
				search.field.SetText("")
				search.lastText = ""
				search.due = time.Time{}
				search.suppressed = true
			} else {
				blurred = true
				gtx.Execute(key.FocusCmd{Tag: nil})
			}
			gtx.Execute(op.InvalidateCmd{})
		case key.NameUpArrow, key.NameDownArrow:
			if len(entries) == 0 {
				continue
			}
			if !search.highlighted {
				search.highlight = 0
				if pressed.Name == key.NameUpArrow {
					search.highlight = len(entries) - 1
				}
			} else if pressed.Name == key.NameUpArrow {
				search.highlight = (search.highlight - 1 + len(entries)) % len(entries)
			} else {
				search.highlight = (search.highlight + 1) % len(entries)
			}
			search.highlighted = true
			gtx.Execute(op.InvalidateCmd{})
		}
	}
	for {
		raw, ok := search.field.Update(gtx)
		if !ok {
			break
		}
		if submitted, ok := raw.(widget.SubmitEvent); ok {
			entries = search.entries()
			if search.highlighted && search.highlight >= 0 && search.highlight < len(entries) {
				search.submit(e, entries[search.highlight])
			} else {
				search.submit(e, submitted.Text)
			}
		}
	}
	return escaped, blurred
}

func (search *topSearch) schedule(gtx layout.Context, e *env.Env, provider model.Provider) {
	text := topSearchQuery(search.field.Text())
	if provider != search.provider {
		search.provider = provider
		search.suggestions = nil
		search.highlighted = false
		search.request++
		if text != "" {
			search.due = gtx.Now.Add(topSearchDebounce)
		}
	}
	if text != search.lastText {
		search.lastText = text
		search.suggestions = nil
		search.highlighted = false
		search.suppressed = false
		search.request++
		if text == "" {
			search.due = time.Time{}
		} else {
			search.due = gtx.Now.Add(topSearchDebounce)
		}
	}
	if search.due.IsZero() || text == "" {
		return
	}
	if gtx.Now.Before(search.due) {
		gtx.Execute(op.InvalidateCmd{At: search.due})
		return
	}
	search.due = time.Time{}
	search.beginSuggestions(e, text, provider)
}

func (search *topSearch) entries() []string {
	if topSearchQuery(search.field.Text()) == "" {
		return search.recent
	}
	return search.suggestions
}

func (search *topSearch) openSuggestions(gtx layout.Context, e *env.Env, anchor image.Rectangle) {
	if e == nil || e.Overlays == nil {
		return
	}
	entries := search.entries()
	if search.suppressed || !search.focused(gtx) || len(entries) == 0 {
		e.Overlays.Close(&search.tag)
		return
	}
	items := make([]kit.MenuItem, len(entries))
	for index, entry := range entries {
		items[index] = kit.MenuItem{Icon: look.Icon.Search, Label: entry}
		if search.highlighted && index == search.highlight {
			items[index].Preview = kit.StateFocus
		}
	}
	content := func(gtx layout.Context) layout.Dimensions {
		dimensions := search.menu.Layout(gtx, e.Th.Theme, items)
		if index, ok := search.menu.Clicked(gtx); ok && index >= 0 && index < len(entries) {
			search.submit(e, entries[index])
			gtx.Execute(op.InvalidateCmd{})
		}
		return dimensions
	}
	width := unit.Dp(float32(anchor.Dx()) / gtx.Metric.PxPerDp)
	e.Overlays.Open(&search.tag, anchor, look.Placement{Prefer: look.Below, Align: look.Start, Gap: 8, Width: width, Passive: true}, content)
}

func (search *topSearch) layout(gtx layout.Context, e *env.Env, anchor image.Rectangle) layout.Dimensions {
	if e == nil || e.Th == nil {
		return layout.Dimensions{}
	}
	routeChanged := search.syncRoute(gtx, e.Nav.Current())
	if routeChanged {
		search.suppressed = true
		search.highlighted = false
		e.Overlays.Close(&search.tag)
	}
	search.field.Submit = true
	escaped, blurred := search.update(gtx, e)
	snapshot := e.Store.Snapshot()
	search.schedule(gtx, e, snapshot.Provider)
	search.field.Placeholder = "Search tracks, albums, artists…"
	search.field.Hint = "/"
	requested := search.focusNext
	dimensions := search.field.Layout(gtx, e.Th.Theme)
	if requested {
		search.field.Focus(gtx)
		search.focusNext = false
	}
	focused := requested || gtx.Focused(&search.field.Editor)
	if blurred {
		focused = false
	}
	if focused && !search.wasFocused && !escaped && !routeChanged {
		search.suppressed = false
	}
	search.wasFocused = focused
	search.active = focused
	if routeChanged || escaped || !focused {
		e.Overlays.Close(&search.tag)
	} else {
		search.openSuggestions(gtx, e, anchor)
	}
	return dimensions
}
