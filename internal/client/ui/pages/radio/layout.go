package radio

import (
	"sort"
	"strings"

	"gioui.org/layout"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryotunes/internal/client/state"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/widgets"
	"github.com/ryoku-dev/ryotunes/internal/model"
)

type stationGroup struct {
	title    string
	stations []model.RadioStation
}

// Layout renders the directory and starts its first request only once visible.
func (p *Page) Layout(gtx layout.Context, e *env.Env) layout.Dimensions {
	p.ensureLoaded(e)
	if selected, changed := p.filters.Changed(); changed && selected >= 0 && selected < len(radioFilters) {
		p.selected = selected
		p.scroll.ScrollTo(0)
	}
	if p.retry.Clicked(gtx) {
		p.load(e)
	}
	if p.moreTry.Clicked(gtx) {
		p.loadMore(e)
	}

	snapshot := e.Store.Snapshot()
	inset := look.ContentInset(e.Th.Class)
	width := int(gtx.Metric.PxToDp(gtx.Constraints.Max.X - 2*gtx.Dp(inset)))
	children := []layout.Widget{
		func(gtx layout.Context) layout.Dimensions {
			return p.header.Layout(gtx, e, "Radio", "Live stations from around the world.", nil)
		},
		p.controls(e),
	}
	switch {
	case p.loading && !p.loaded:
		children = append(children, widgets.Loading(e, widgets.LoadingGrid))
	case p.error != "" && len(p.stations) == 0:
		children = append(children, widgets.Failed(e, p.error, &p.retry))
	default:
		groups := p.groups(snapshot.Personal, normalizedQuery(p.filter.Text()))
		if stationGroupCount(groups) == 0 {
			children = append(children, p.empty(e))
		} else {
			playing := ""
			if snapshot.Playback.Now != nil {
				playing = strings.TrimPrefix(snapshot.Playback.Now.VideoID, "RYOTUNES_RADIO:")
			}
			children = append(children, p.groupWidgets(e, snapshot.Personal, groups, max(1, width), playing)...)
		}
		if footer := p.footer(e); footer != nil {
			children = append(children, footer)
		}
	}

	return layout.Inset{Bottom: unit.Dp(21)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return p.scroll.Layout(gtx, e.Th, len(children), func(gtx layout.Context, index int) layout.Dimensions {
			if index >= len(children)-2 {
				p.loadMore(e)
			}
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

func (p *Page) controls(e *env.Env) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		p.filters.Labels = radioFilters
		p.filters.Selected = p.selected
		filters := func(gtx layout.Context) layout.Dimensions { return p.filters.Layout(gtx, e.Th.Theme) }
		field := func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = min(gtx.Constraints.Max.X, gtx.Dp(260))
			gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, gtx.Dp(320))
			return p.filter.Layout(gtx, e.Th.Theme)
		}
		if gtx.Metric.PxToDp(gtx.Constraints.Max.X) >= 1000 {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Flexed(1, filters),
				layout.Rigid(layout.Spacer{Width: unit.Dp(21)}.Layout),
				layout.Rigid(field),
			)
		}
		return layout.Flex{Axis: layout.Vertical, Alignment: layout.Start}.Layout(gtx,
			layout.Rigid(filters),
			layout.Rigid(layout.Spacer{Height: unit.Dp(13)}.Layout),
			layout.Rigid(field),
		)
	}
}

func (p *Page) groups(personal state.Personal, query string) []stationGroup {
	stations := p.filteredStations(personal)
	if query != "" {
		needle := strings.ToLower(query)
		filtered := stations[:0]
		for _, station := range stations {
			if strings.Contains(strings.ToLower(station.Name), needle) {
				filtered = append(filtered, station)
			}
		}
		stations = filtered
	}
	if p.selected < 3 {
		return []stationGroup{{stations: stations}}
	}
	by := make(map[string][]model.RadioStation)
	for _, station := range stations {
		key := strings.TrimSpace(station.Country)
		if p.selected == 4 {
			key = firstGenre(station.Tags)
		}
		if key == "" {
			key = "Other"
		}
		by[key] = append(by[key], station)
	}
	titles := make([]string, 0, len(by))
	for title := range by {
		titles = append(titles, title)
	}
	sort.Strings(titles)
	groups := make([]stationGroup, 0, len(titles))
	for _, title := range titles {
		stations := by[title]
		sort.SliceStable(stations, func(i, j int) bool { return strings.ToLower(stations[i].Name) < strings.ToLower(stations[j].Name) })
		groups = append(groups, stationGroup{title: title, stations: stations})
	}
	return groups
}

func (p *Page) filteredStations(personal state.Personal) []model.RadioStation {
	switch p.selected {
	case 1:
		return favouriteStations(personal, p.stations)
	case 2:
		return recentStations(personal, p.stations)
	default:
		return append([]model.RadioStation(nil), p.stations...)
	}
}

func favouriteStations(personal state.Personal, catalogue []model.RadioStation) []model.RadioStation {
	lookup := stationLookup(catalogue)
	out := make([]model.RadioStation, 0)
	for _, item := range personal.Saved {
		if item.Kind != "station" {
			continue
		}
		station := lookup[strings.TrimPrefix(item.ID, "RYOTUNES_RADIO:")]
		if station.StationUUID == "" {
			station = stationFromBrowse(item)
		}
		if station.StationUUID != "" {
			out = append(out, station)
		}
	}
	return out
}

func recentStations(personal state.Personal, catalogue []model.RadioStation) []model.RadioStation {
	lookup := stationLookup(catalogue)
	out := make([]model.RadioStation, 0)
	for _, recent := range state.RecentItems(personal, 0) {
		if recent.Kind != "station" {
			continue
		}
		station := lookup[strings.TrimPrefix(recent.ID, "RYOTUNES_RADIO:")]
		if station.StationUUID == "" {
			station = stationFromBrowse(recent.BrowseItem)
		}
		if station.StationUUID != "" {
			out = append(out, station)
		}
	}
	return out
}

func stationLookup(stations []model.RadioStation) map[string]model.RadioStation {
	lookup := make(map[string]model.RadioStation, len(stations))
	for _, station := range stations {
		lookup[station.StationUUID] = station
	}
	return lookup
}

func firstGenre(tags string) string {
	for _, tag := range strings.Split(tags, ",") {
		if tag = strings.TrimSpace(tag); tag != "" {
			return tag
		}
	}
	return ""
}

func stationGroupCount(groups []stationGroup) int {
	count := 0
	for _, group := range groups {
		count += len(group.stations)
	}
	return count
}

func (p *Page) groupWidgets(e *env.Env, personal state.Personal, groups []stationGroup, width int, playing string) []layout.Widget {
	children := make([]layout.Widget, 0)
	columns := stationColumns(width)
	for _, group := range groups {
		if group.title != "" {
			title := group.title
			children = append(children, kit.Text(e.Th.Theme, e.Th.T.Section, e.Th.R.Fg, title))
		}
		rows := (len(group.stations) + columns - 1) / columns
		for row := range rows {
			start := row * columns
			end := min(len(group.stations), start+columns)
			stations := group.stations[start:end]
			children = append(children, func(gtx layout.Context) layout.Dimensions {
				return p.stationRow(gtx, e, personal, stations, columns, playing)
			})
		}
	}
	return children
}

func stationColumns(width int) int {
	switch {
	case width >= 1100:
		return 4
	case width >= 820:
		return 3
	case width >= 520:
		return 2
	default:
		return 1
	}
}

func (p *Page) stationRow(gtx layout.Context, e *env.Env, personal state.Personal, stations []model.RadioStation, columns int, playing string) layout.Dimensions {
	children := make([]layout.FlexChild, 0, columns*2-1)
	for index, station := range stations {
		station := station
		tile := p.tile(station)
		favourite := p.favourite(personal, station)
		children = append(children, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return tile.layout(gtx, e, p, station, station.StationUUID == playing, favourite)
		}))
		if index+1 < columns {
			children = append(children, layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout))
		}
	}
	for index := len(stations); index < columns; index++ {
		children = append(children, layout.Flexed(1, func(layout.Context) layout.Dimensions { return layout.Dimensions{} }))
		if index+1 < columns {
			children = append(children, layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout))
		}
	}
	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, children...)
}

func (p *Page) tile(station model.RadioStation) *stationTile {
	tile := p.tiles[station.StationUUID]
	if tile == nil {
		tile = new(stationTile)
		p.tiles[station.StationUUID] = tile
	}
	return tile
}

func (p *Page) empty(e *env.Env) layout.Widget {
	switch p.selected {
	case 1:
		return widgets.EmptyState(e, look.Icon.HeartOutline, "No favourite stations", "Hover a station and choose the heart to keep it here.")
	case 2:
		return widgets.EmptyState(e, look.Icon.Radio, "No recent stations", "Stations you play show here.")
	default:
		return widgets.EmptyState(e, look.Icon.Radio, "No stations found.", "Try a shorter station name or another catalogue view.")
	}
}

func (p *Page) footer(e *env.Env) layout.Widget {
	switch {
	case p.more:
		return kit.Text(e.Th.Theme, e.Th.T.Proof, e.Th.R.Fg2, "Finding more stations...")
	case p.moreErr != "":
		p.moreTry.Kind = kit.PlaqueLine
		p.moreTry.Label = "Try again"
		p.moreTry.Icon = look.Icon.Refresh
		return func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical, Alignment: layout.Start}.Layout(gtx,
				layout.Rigid(kit.Text(e.Th.Theme, e.Th.T.Proof, e.Th.R.Jasper, p.moreErr)),
				layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { return p.moreTry.Layout(gtx, e.Th.Theme) }),
			)
		}
	case p.hasMore:
		return kit.Text(e.Th.Theme, e.Th.T.Proof, e.Th.R.Fg3, "More stations load as you reach them.")
	default:
		return nil
	}
}
