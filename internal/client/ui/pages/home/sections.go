package home

import (
	"fmt"
	"strings"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryotunes/internal/client/state"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/widgets"
	"github.com/ryoku-dev/ryotunes/internal/model"
)

type homeContentKey struct {
	contentRevision  uint64
	personalRevision uint64
	insightsRevision uint64
	session          homeSessionKey
	account          string
	hour             int
}

type homeProjectionKey struct {
	feedRevision     uint64
	personalRevision uint64
}

func (p *page) content(e *env.Env, snapshot state.State) []layout.Widget {
	key := homeContentKey{
		contentRevision:  p.contentRevision,
		personalRevision: snapshot.PersonalRevision,
		insightsRevision: p.insights.revision,
		session:          homeSession(snapshot),
		account:          stringValue(accountName(snapshot)),
		hour:             e.Now.Hour(),
	}
	if key == p.contentKey && p.children != nil {
		return p.children
	}
	p.contentKey = key
	p.stateIndex = -1

	children := make([]layout.Widget, 0, len(p.feed.Sections)+6)
	children = append(children, p.headerLayout(e, snapshot))
	if p.loading && !p.loaded {
		p.children = append(children, widgets.Skeleton(e, widgets.LoadingGrid))
		return p.children
	}
	if p.errorText != "" && !p.loaded {
		p.stateIndex = len(children)
		children = append(children, func(gtx layout.Context) layout.Dimensions {
			dimensions := widgets.Failed(e, p.errorText, &p.retry)(gtx)
			if p.retry.Clicked(gtx) {
				p.fetchAll(e)
				gtx.Execute(op.InvalidateCmd{})
			}
			return dimensions
		})
		p.children = children
		return p.children
	}

	if len(p.chips) != 0 {
		children = append(children, p.feedFiltersLayout(e))
	}
	if p.loading && p.pendingChip > 0 {
		p.children = append(children, widgets.Skeleton(e, widgets.LoadingShelf))
		return p.children
	}
	if p.activeChip > 0 {
		shelves := p.feedShelves(e)
		children = append(children, shelves...)
		if len(shelves) == 0 {
			p.stateIndex = len(children)
			children = append(children, widgets.EmptyState(e, look.Icon.Sparkle, "Nothing here yet", ""))
		}
		p.children = children
		return p.children
	}

	p.prepareProjections(e, snapshot)
	if p.quickWidget != nil {
		children = append(children, p.quickWidget)
	}
	if p.recentWidget != nil {
		children = append(children, p.recentWidget)
	}
	if p.pickedWidget != nil {
		children = append(children, p.pickedWidget)
	}
	children = append(children, p.feedShelves(e)...)
	if facts := p.insights.layout(e); facts != nil {
		children = append(children, facts)
	}
	p.children = children
	return p.children
}

func (p *page) feedShelves(e *env.Env) []layout.Widget {
	if p.shelfRevision == p.feedRevision && p.shelfChildren != nil {
		return p.shelfChildren
	}
	children := make([]layout.Widget, 0, len(p.feed.Sections))
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
	p.shelfRevision = p.feedRevision
	p.shelfChildren = children
	return p.shelfChildren
}

func (p *page) prepareProjections(e *env.Env, snapshot state.State) {
	key := homeProjectionKey{feedRevision: p.feedRevision, personalRevision: snapshot.PersonalRevision}
	if key == p.projectionKey && p.projectionReady {
		return
	}
	p.projectionKey = key
	p.projectionReady = true
	p.quickWidget = nil
	p.recentWidget = nil
	p.pickedWidget = nil
	if quick := p.quickItems(snapshot.Personal); len(quick) != 0 {
		p.quickWidget = p.quickGrid(e, quick)
	}
	if recent := recentItems(snapshot.Personal); len(recent) != 0 {
		p.recentWidget = func(gtx layout.Context) layout.Dimensions {
			return p.recent.Layout(gtx, e, "Jump back in", recent, nil)
		}
	}
	if picked := state.PickedForYou(p.feed, snapshot.Personal); len(picked) != 0 {
		p.pickedWidget = func(gtx layout.Context) layout.Dimensions {
			return p.picked.Layout(gtx, e, "Picked for you", picked, nil)
		}
	}
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func (p *page) quickGrid(e *env.Env, items []model.BrowseItem) layout.Widget {
	layouts := make(map[int][]layout.FlexChild, 3)
	for _, columns := range []int{2, 3, 4} {
		rows := (len(items) + columns - 1) / columns
		rowWidgets := make([]layout.FlexChild, 0, rows*2)
		for row := range rows {
			start := row * columns
			end := min(len(items), start+columns)
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
			rowChildren := children
			rowWidgets = append(rowWidgets, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, rowChildren...)
			}))
			if row+1 < rows {
				rowWidgets = append(rowWidgets, layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout))
			}
		}
		layouts[columns] = rowWidgets
	}
	return func(gtx layout.Context) layout.Dimensions {
		width := int(gtx.Metric.PxToDp(gtx.Constraints.Max.X))
		columns := 2
		if width >= 1100 {
			columns = 4
		} else if width >= 820 {
			columns = 3
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, layouts[columns]...)
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
