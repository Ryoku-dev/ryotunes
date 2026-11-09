package list

import (
	"strings"

	"gioui.org/layout"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryotunes/internal/client/lithic"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/act"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/widgets"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

type page struct {
	route env.Route
	list  layout.List
	grid  widgets.CardGrid
	retry lithic.Button

	items   []model.BrowseItem
	started bool
	loading bool
	loaded  bool
	error   string
	request uint64
}

// New returns a retained generic browse-grid page.
func New(r env.Route) env.Page {
	return &page{route: r, list: layout.List{Axis: layout.Vertical}}
}

func (p *page) Title() string {
	if strings.TrimSpace(p.route.Title) != "" {
		return strings.TrimSpace(p.route.Title)
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
	children := []layout.Widget{widgets.PageHeader(e, p.Title(), "", nil)}
	switch {
	case p.loading && !p.loaded:
		children = append(children, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(widgets.Loading(e)),
				layout.Rigid(layout.Spacer{Height: unit.Dp(13)}.Layout),
				layout.Rigid(lithic.Text(e.Th, e.Th.T.Body, e.Th.R.Fg2, "Loading…")),
			)
		})
	case p.error != "" && !p.loaded:
		children = append(children, widgets.Failed(e, p.error, &p.retry))
	case len(p.items) == 0:
		children = append(children, widgets.EmptyState(e, "library", "Nothing here.", "This collection has no items."))
	default:
		children = append(children, func(gtx layout.Context) layout.Dimensions {
			return p.grid.Layout(gtx, e, p.items)
		})
	}
	return layout.Inset{Top: unit.Dp(34), Right: unit.Dp(34), Bottom: unit.Dp(55), Left: unit.Dp(34)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return p.list.Layout(gtx, len(children), func(gtx layout.Context, index int) layout.Dimensions {
			return layout.Inset{Bottom: unit.Dp(34)}.Layout(gtx, children[index])
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
