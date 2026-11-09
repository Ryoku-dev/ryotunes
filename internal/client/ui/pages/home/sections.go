package home

import (
	"fmt"
	"strings"

	"gioui.org/layout"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryotunes/internal/client/state"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/widgets"
	"github.com/ryoku-dev/ryotunes/internal/model"
)

func (p *page) content(e *env.Env, snapshot state.State) []layout.Widget {
	children := []layout.Widget{p.header(e, snapshot)}
	if p.loading && !p.loaded {
		return append(children, widgets.Loading(e, widgets.LoadingGrid))
	}
	if p.errorText != "" && !p.loaded {
		return append(children, widgets.Failed(e, p.errorText, &p.retry))
	}

	if len(p.feed.Chips) > 1 {
		children = append(children, p.feedFiltersLayout(e))
	}
	if quick := p.quickItems(snapshot.Personal); len(quick) != 0 {
		children = append(children, p.quickGrid(e, quick))
	}

	recent := recentItems(snapshot.Personal)
	if len(recent) != 0 {
		children = append(children, func(gtx layout.Context) layout.Dimensions {
			return p.recent.Layout(gtx, e, "Jump back in", recent, nil)
		})
	}
	picked := state.PickedForYou(p.feed, snapshot.Personal)
	if len(picked) != 0 {
		children = append(children, func(gtx layout.Context) layout.Dimensions {
			return p.picked.Layout(gtx, e, "Picked for you", picked, nil)
		})
	}

	if snapshot.Provider == model.ProviderTidal && !snapshot.Tidal.SignedIn {
		children = append(children, widgets.EmptyState(e, look.Icon.Person, "Sign in to TIDAL", "Sign in from Settings to see your TIDAL home feed."))
	} else {
		for index := range p.feed.Sections {
			shelf := p.feed.Sections[index]
			if len(shelf.Items) == 0 {
				continue
			}
			key := fmt.Sprintf("%d:%s", index, shelf.Title)
			view := p.shelves[key]
			if view == nil {
				view = new(widgets.Shelf)
				p.shelves[key] = view
			}
			var more *env.Route
			if shelf.MoreBrowseID != nil && strings.TrimSpace(*shelf.MoreBrowseID) != "" {
				route := env.Route{Kind: env.RouteList, ID: *shelf.MoreBrowseID, Title: shelf.Title}
				if shelf.MoreParams != nil {
					route.Params = *shelf.MoreParams
				}
				more = &route
			}
			title, items, route := shelf.Title, shelf.Items, more
			children = append(children, func(gtx layout.Context) layout.Dimensions {
				return view.Layout(gtx, e, title, items, route)
			})
		}
	}
	if facts := p.insights.layout(e); facts != nil {
		children = append(children, facts)
	}
	return children
}

func (p *page) quickGrid(e *env.Env, items []model.BrowseItem) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		width := int(gtx.Metric.PxToDp(gtx.Constraints.Max.X))
		columns := 2
		if width >= 1100 {
			columns = 4
		} else if width >= 820 {
			columns = 3
		}
		rows := (len(items) + columns - 1) / columns
		rowWidgets := make([]layout.FlexChild, 0, rows*2)
		for row := range rows {
			start := row * columns
			end := min(len(items), start+columns)
			rowWidgets = append(rowWidgets, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				children := make([]layout.FlexChild, 0, columns*2-1)
				for column := range columns {
					if column > 0 {
						children = append(children, layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout))
					}
					index := start + column
					if index >= end {
						children = append(children, layout.Flexed(1, func(layout.Context) layout.Dimensions { return layout.Dimensions{} }))
						continue
					}
					item := items[index]
					tile := p.tile(item)
					children = append(children, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return tile.Layout(gtx, e, item)
					}))
				}
				return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, children...)
			}))
			if row+1 < rows {
				rowWidgets = append(rowWidgets, layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout))
			}
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, rowWidgets...)
	}
}

func (p *page) tile(item model.BrowseItem) *widgets.QuickTile {
	key := strings.ToLower(item.Kind) + "\x00" + item.ID
	tile := p.tiles[key]
	if tile == nil {
		tile = new(widgets.QuickTile)
		p.tiles[key] = tile
	}
	return tile
}

func (p *page) quickItems(personal state.Personal) []model.BrowseItem {
	lookup := make(map[string]model.BrowseItem)
	for _, pick := range personal.Picks {
		lookup[pick.ID] = pick.BrowseItem
	}
	for _, item := range personal.Saved {
		lookup[item.ID] = item
	}
	for _, recent := range state.RecentItems(personal, 0) {
		lookup[recent.ID] = recent.BrowseItem
	}
	for _, shelf := range p.feed.Sections {
		for _, item := range shelf.Items {
			lookup[item.ID] = item
		}
	}

	out := make([]model.BrowseItem, 0, 8)
	seen := make(map[string]struct{})
	appendItem := func(item model.BrowseItem) {
		if len(out) == 8 || item.ID == "" {
			return
		}
		if _, exists := seen[item.ID]; exists {
			return
		}
		seen[item.ID] = struct{}{}
		out = append(out, item)
	}
	for _, id := range personal.Pins {
		appendItem(lookup[id])
	}
	for _, pick := range personal.Picks {
		appendItem(pick.BrowseItem)
	}
	for _, recent := range state.RecentItems(personal, 0) {
		appendItem(recent.BrowseItem)
	}
	return out
}

func recentItems(personal state.Personal) []model.BrowseItem {
	items := make([]model.BrowseItem, 0, 12)
	for _, recent := range state.RecentItems(personal, 0) {
		if recent.Kind == "station" || recent.ID == "" {
			continue
		}
		items = append(items, recent.BrowseItem)
		if len(items) == 12 {
			break
		}
	}
	return items
}
