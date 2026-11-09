// Package radio implements the demand-driven internet radio directory page.
package radio

import (
	"strings"

	"gioui.org/layout"
	"gioui.org/widget"

	"github.com/ryoku-dev/ryotunes/internal/client/lithic"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/act"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

const pageSize = 36

// Page is the retained internet-radio directory surface.
type Page struct {
	route env.Route

	search lithic.Search
	list   widget.List
	rows   map[string]*stationRow

	stations  []model.RadioStation
	query     string
	error     string
	moreErr   string
	loaded    bool
	loading   bool
	more      bool
	retry     lithic.Button
	moreRetry lithic.Button
	hasMore   bool
	request   uint64
}

// New constructs a retained radio page without fetching the directory.
func New(r env.Route) env.Page {
	p := &Page{
		route: r,
		list:  widget.List{List: layout.List{Axis: layout.Vertical}},
		rows:  make(map[string]*stationRow),
	}
	p.search.SingleLine = true
	p.search.Submit = true
	if q := normalizedQuery(r.Query); q != "" {
		p.search.SetText(q)
	}
	return p
}

// Title returns the page title.
func (p *Page) Title() string { return "Radio" }

// Refresh reloads the first directory page for the current search.
func (p *Page) Refresh(e *env.Env) {
	p.load(e, normalizedQuery(p.search.Text()))
}

func normalizedQuery(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func (p *Page) ensureLoaded(e *env.Env) {
	if !p.loaded && !p.loading {
		p.load(e, normalizedQuery(p.search.Text()))
	}
}

func (p *Page) load(e *env.Env, query string) {
	p.request++
	request := p.request
	p.query = query
	p.search.SetText(query)
	p.error = ""
	p.moreErr = ""
	p.loaded = false
	p.loading = true
	p.more = false
	p.hasMore = false
	p.stations = nil

	offset, limit := 0, pageSize
	params := protocol.RadioStationsParams{Offset: &offset, Limit: &limit}
	if query != "" {
		params.Query = &query
	}
	act.Call[[]model.RadioStation](e, protocol.MethodRadioStations, params, func(stations []model.RadioStation, err error) {
		if request != p.request {
			return
		}
		p.loading = false
		p.loaded = true
		if err != nil {
			p.error = err.Error()
			return
		}
		p.stations = stations
		p.hasMore = len(stations) == pageSize
	})
}

func (p *Page) loadMore(e *env.Env) {
	if p.loading || p.more || !p.hasMore {
		return
	}
	p.more = true
	p.moreErr = ""
	request := p.request
	offset, limit := len(p.stations), pageSize
	params := protocol.RadioStationsParams{Offset: &offset, Limit: &limit}
	if p.query != "" {
		query := p.query
		params.Query = &query
	}
	act.Call[[]model.RadioStation](e, protocol.MethodRadioStations, params, func(stations []model.RadioStation, err error) {
		if request != p.request {
			return
		}
		p.more = false
		if err != nil {
			p.moreErr = err.Error()
			return
		}
		p.stations = append(p.stations, stations...)
		p.hasMore = len(stations) == pageSize
	})
}
