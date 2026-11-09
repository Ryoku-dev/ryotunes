package search

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
	list         widgets.TrackList
}

type cardState struct {
	items        []model.BrowseItem
	continuation *string
	loaded       bool
	loading      bool
	paging       bool
	error        string
	request      uint64
	grid         widgets.CardGrid
	more         lithic.Button
}

type page struct {
	route          env.Route
	query          string
	list           layout.List
	provider       model.Provider
	spotifyBlocked bool
	selected       int
	filters        lithic.Filters
	retry          lithic.Button
	topOpen        lithic.Button
	topHero        widgets.Hero
	allSongs       widgets.TrackList
	shelves        map[string]*widgets.Shelf

	mixed  mixedState
	tracks trackState
	cards  [3]cardState
}

// New returns a retained Search page for the route query.
func New(r env.Route) env.Page {
	return &page{
		route:   r,
		query:   strings.Join(strings.Fields(r.Query), " "),
		list:    layout.List{Axis: layout.Vertical},
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
	p.mixed.result = model.SearchAll{}
	p.mixed.loaded = false
	p.mixed.loading = false
	p.mixed.error = ""
	p.tracks.request++
	p.tracks.items = nil
	p.tracks.continuation = nil
	p.tracks.loaded = false
	p.tracks.loading = false
	p.tracks.paging = false
	p.tracks.error = ""
	for i := range p.cards {
		p.cards[i].request++
		p.cards[i].items = nil
		p.cards[i].continuation = nil
		p.cards[i].loaded = false
		p.cards[i].loading = false
		p.cards[i].paging = false
		p.cards[i].error = ""
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

	children := []layout.Widget{
		widgets.PageHeader(e, "Search", searchSentence(p.query), nil),
		p.filtersLayout(e),
	}
	if blocked {
		children = append(children, widgets.EmptyState(e, "login", "Sign in to Spotify", "Sign in from Settings to search Spotify."))
	} else {
		children = append(children, p.selectedContent(e))
	}
	inset := layout.Inset{Top: unit.Dp(34), Right: unit.Dp(34), Bottom: unit.Dp(55), Left: unit.Dp(34)}
	return inset.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return p.list.Layout(gtx, len(children), func(gtx layout.Context, index int) layout.Dimensions {
			return layout.Inset{Bottom: unit.Dp(34)}.Layout(gtx, children[index])
		})
	})
}

func (p *page) handleControls(gtx layout.Context, e *env.Env, blocked bool) {
	if selected, changed := p.filters.Changed(gtx); changed && selected >= 0 && selected < len(searchTabs) {
		p.selected = selected
		if !blocked {
			p.loadSelected(e)
		}
	}
	if p.retry.Clicked(gtx) && !blocked {
		p.Refresh(e)
	}
	if p.topOpen.Clicked(gtx) && len(p.mixed.result.Top) != 0 {
		p.activateTop(e, p.mixed.result.Top[0])
	}
	for index := range p.cards {
		if p.cards[index].more.Clicked(gtx) && !blocked {
			p.loadCardsMore(e, index)
		}
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
		for i := range searchTabs {
			labels[i] = searchTabs[i].label
		}
		var counts []int
		if p.mixed.loaded {
			result := p.mixed.result
			counts = []int{
				mixedCount(result), countSongs(result.Songs),
				len(result.Albums), len(result.Artists), len(result.Playlists),
			}
			if p.tracks.loaded {
				counts[1] = len(p.tracks.items)
			}
			for i := range p.cards {
				if p.cards[i].loaded {
					counts[i+2] = len(p.cards[i].items)
				}
			}
		}
		return p.filters.Layout(gtx, e.Th, labels, counts, p.selected)
	}
}

func (p *page) selectedContent(e *env.Env) layout.Widget {
	if p.query == "" {
		return widgets.EmptyState(e, "search", "Search Ryotunes", "Search for a song, album, artist, or playlist.")
	}
	switch p.selected {
	case 0:
		return p.mixedLayout(e)
	case 1:
		return p.tracksLayout(e)
	default:
		return p.cardsLayout(e, p.selected-2)
	}
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

func searchSentence(query string) string {
	if query == "" {
		return "Songs, albums, artists and playlists in one search."
	}
	return "Results for “" + query + "”."
}
