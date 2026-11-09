package list

import (
	"strings"

	"gioui.org/layout"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/act"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/widgets"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

type page struct {
	route  env.Route
	scroll look.Scroller
	grid   widgets.Grid
	retry  kit.Plaque

	items   []model.BrowseItem
	started bool
	loading bool
	loaded  bool
	error   string
	request uint64
}

// New returns a retained generic browse-grid page.
func New(r env.Route) env.Page { return &page{route: r} }

func (p *page) Title() string {
	if title := strings.TrimSpace(p.route.Title); title != "" {
		return title
	}
	return "Browse"
}

func (p *page) Refresh(e *env.Env) { p.fetch(e) }

func (p *page) Layout(gtx layout.Context, e *env.Env) layout.Dimensions {
	if p.retry.Clicked(gtx) {
		p.fetch(e)
	}
	if !p.started {
		p.fetch(e)
	}
	inset := look.ContentInset(e.Th.Class)
	width := int(gtx.Metric.PxToDp(gtx.Constraints.Max.X - 2*gtx.Dp(inset)))
	children := []layout.Widget{widgets.PageHeader(e, p.Title(), "", nil)}
	switch {
	case p.loading && !p.loaded:
		children = append(children, widgets.Loading(e, widgets.LoadingGrid))
	case p.error != "" && !p.loaded:
		children = append(children, widgets.Failed(e, p.error, &p.retry))
	case len(p.items) == 0:
		children = append(children, widgets.EmptyState(e, look.Icon.Library, "Nothing here.", "This collection has no items."))
	default:
		rows := p.grid.Rows(max(1, width), len(p.items))
		for row := range rows {
			row := row
			children = append(children, func(gtx layout.Context) layout.Dimensions {
				return p.grid.Row(gtx, e, p.items, row)
			})
		}
	}
	return layout.Inset{Bottom: unit.Dp(21)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return p.scroll.Layout(gtx, e.Th, len(children), func(gtx layout.Context, index int) layout.Dimensions {
			bottom := unit.Dp(21)
			if index == len(children)-1 {
				bottom = 0
			}
			if index == 0 {
				return layout.Inset{Bottom: bottom}.Layout(gtx, children[index])
			}
			return layout.Inset{Right: inset, Bottom: bottom, Left: inset}.Layout(gtx, children[index])
		})
	})
}

func (p *page) fetch(e *env.Env) {
	p.started = true
	p.loading = true
	p.error = ""
	p.request++
	request := p.request
	params := optionalString(p.route.Params)
	act.Call[[]model.BrowseItem](e, protocol.MethodGetBrowseGrid, protocol.BrowseGridParams{ID: p.route.ID, Params: params}, func(items []model.BrowseItem, err error) {
		if request != p.request {
			return
		}
		p.loading = false
		if err != nil {
			p.error = err.Error()
			return
		}
		p.items = dedupe(items)
		p.loaded = true
	})
}

func optionalString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func dedupe(items []model.BrowseItem) []model.BrowseItem {
	seen := make(map[string]struct{}, len(items))
	out := make([]model.BrowseItem, 0, len(items))
	for _, item := range items {
		if item.ID == "" {
			continue
		}
		if _, duplicate := seen[item.ID]; duplicate {
			continue
		}
		seen[item.ID] = struct{}{}
		out = append(out, item)
	}
	return out
}
