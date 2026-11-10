package search

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

var searchTabs = []struct {
	label string
	kind  model.SearchKind
}{
	{"All", model.SearchKindAll},
	{"Songs", model.SearchKindSongs},
	{"Albums", model.SearchKindAlbums},
	{"Artists", model.SearchKindArtists},
	{"Playlists", model.SearchKindPlaylists},
}

type mixedState struct {
	result  model.SearchAll
	loaded  bool
	loading bool
	error   string
	request uint64
}

type trackState struct {
	items        []model.Track
	continuation *string
	loaded       bool
	loading      bool
	paging       bool
	error        string
	request      uint64
}

type cardState struct {
	items        []model.BrowseItem
	continuation *string
	loaded       bool
	loading      bool
	paging       bool
	error        string
	request      uint64
	grid         widgets.Grid
}

type page struct {
	query          string
	scroll         look.Scroller
	header         widgets.Header
	provider       model.Provider
	spotifyBlocked bool
	selected       int
	filters        kit.Filters
	retry          kit.Plaque
	topPlay        kit.Plaque
	topArt         look.ArtView
	topTable       widgets.TrackTable
	tracksTable    widgets.TrackTable
	shelves        map[string]*widgets.Shelf
	mixed          mixedState
	tracks         trackState
	cards          [3]cardState
}

// New returns a retained Search page for the route query.
func New(r env.Route) env.Page {
	return &page{
		query:   strings.Join(strings.Fields(r.Query), " "),
		shelves: make(map[string]*widgets.Shelf),
	}
}

func (p *page) Title() string { return "Search" }

func (p *page) Refresh(e *env.Env) {
	p.invalidateResults()
	snapshot := e.Store.Snapshot()
	if snapshot.Provider != model.ProviderSpotify || snapshot.Spotify.SignedIn {
		p.loadSelected(e)
	}
}

func (p *page) invalidateResults() {
	p.mixed.request++
	p.mixed = mixedState{request: p.mixed.request}
	p.tracks.request++
	p.tracks = trackState{request: p.tracks.request}
	for index := range p.cards {
		p.cards[index].request++
		request, grid := p.cards[index].request, p.cards[index].grid
		p.cards[index] = cardState{request: request, grid: grid}
	}
}

func (p *page) Layout(gtx layout.Context, e *env.Env) layout.Dimensions {
	snapshot := e.Store.Snapshot()
	if p.provider != snapshot.Provider {
		p.provider = snapshot.Provider
		p.invalidateResults()
	}
	blocked := snapshot.Provider == model.ProviderSpotify && !snapshot.Spotify.SignedIn
	if p.spotifyBlocked != blocked {
		p.spotifyBlocked = blocked
		p.invalidateResults()
	}
	p.handleControls(gtx, e, blocked)
	if !blocked && p.query != "" && !p.selectedLoaded() && !p.selectedLoading() {
		p.loadSelected(e)
	}
	inset := look.ContentInset(e.Th.Class)
	width := int(gtx.Metric.PxToDp(gtx.Constraints.Max.X - 2*gtx.Dp(inset)))
	children := []layout.Widget{
		func(gtx layout.Context) layout.Dimensions {
			return p.header.Layout(gtx, e, searchTitle(p.query), "", nil)
		},
		p.filtersLayout(e),
	}
	stateIndex := -1
	if blocked {
		stateIndex = len(children)
		children = append(children, widgets.EmptyState(e, look.Icon.Person, "Sign in to Spotify", "Sign in from Settings to search Spotify."))
	} else {
		content, isState := p.contentWidgets(e, max(1, width))
		if isState {
			stateIndex = len(children)
		}
		children = append(children, content...)
	}
	return p.scroll.Layout(gtx, e.Th, len(children), func(gtx layout.Context, index int) layout.Dimensions {
		if index >= len(children)-2 {
			p.loadSelectedMore(e)
		}
		if index == stateIndex {
			return children[index](gtx)
		}
		bottom := unit.Dp(34)
		if index == len(children)-1 {
			bottom = 21
		} else if index+1 == stateIndex {
			bottom = 0
		}
		if index == 0 {
			return layout.Inset{Bottom: bottom}.Layout(gtx, children[index])
		}
		return layout.Inset{Right: inset, Bottom: bottom, Left: inset}.Layout(gtx, children[index])
	})
}

func (p *page) handleControls(gtx layout.Context, e *env.Env, blocked bool) {
	if selected, changed := p.filters.Changed(); changed && selected >= 0 && selected < len(searchTabs) {
		p.selected = selected
		p.scroll.ScrollTo(0)
		if !blocked {
			p.loadSelected(e)
		}
	}
	if p.retry.Clicked(gtx) && !blocked {
		p.Refresh(e)
	}
	if p.topPlay.Clicked(gtx) && len(p.mixed.result.Top) != 0 {
		p.activateTop(e, p.mixed.result.Top[0])
	}
}

func (p *page) activateTop(e *env.Env, item model.BrowseItem) {
	switch strings.ToLower(item.Kind) {
	case "album", "single", "ep":
		act.Call[model.AlbumPage](e, protocol.MethodGetAlbum, protocol.IDParams{ID: item.ID}, func(result model.AlbumPage, err error) {
			if err != nil {
				e.Toast(err.Error(), env.ToastError)
				return
			}
			act.PlayCollection(e, result.Items, 0, item.ID, item.Title, false, result.Continuation)
		})
	case "playlist", "mix":
		act.Call[model.PlaylistPage](e, protocol.MethodGetPlaylist, protocol.PlaylistParams{ID: item.ID}, func(result model.PlaylistPage, err error) {
			if err != nil {
				e.Toast(err.Error(), env.ToastError)
				return
			}
			act.PlayCollection(e, result.Items, 0, item.ID, item.Title, false, result.Continuation)
		})
	default:
		act.OpenCard(e, item)
	}
}

func (p *page) filtersLayout(e *env.Env) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		labels := make([]string, len(searchTabs))
		for index := range searchTabs {
			labels[index] = searchTabs[index].label
		}
		p.filters.Labels = labels
		p.filters.Counts = p.resultCounts()
		p.filters.Selected = p.selected
		return p.filters.Layout(gtx, e.Th.Theme)
	}
}

func (p *page) resultCounts() []int {
	if !p.mixed.loaded {
		return nil
	}
	result := p.mixed.result
	counts := []int{mixedCount(result), countSongs(result.Songs), len(result.Albums), len(result.Artists), len(result.Playlists)}
	if p.tracks.loaded {
		counts[1] = len(p.tracks.items)
	}
	for index := range p.cards {
		if p.cards[index].loaded {
			counts[index+2] = len(p.cards[index].items)
		}
	}
	return counts
}

func (p *page) selectedLoaded() bool {
	switch p.selected {
	case 0:
		return p.mixed.loaded
	case 1:
		return p.tracks.loaded
	default:
		return p.cards[p.selected-2].loaded
	}
}

func (p *page) selectedLoading() bool {
	switch p.selected {
	case 0:
		return p.mixed.loading
	case 1:
		return p.tracks.loading
	default:
		return p.cards[p.selected-2].loading
	}
}

func (p *page) loadSelectedMore(e *env.Env) {
	switch p.selected {
	case 1:
		p.loadTracksMore(e)
	case 2, 3, 4:
		p.loadCardsMore(e, p.selected-2)
	}
}

func searchTitle(query string) string {
	if query == "" {
		return "Search results"
	}
	return "Results for “" + query + "”"
}
