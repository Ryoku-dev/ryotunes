package search

import (
	"image"
	"strings"

	"gioui.org/layout"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/act"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/widgets"
	"github.com/ryoku-dev/ryotunes/internal/model"
)

// contentWidgets returns the selected tab's rows, and whether its only row is
// a page state that owns its spacing.
func (p *page) contentWidgets(e *env.Env, width int) ([]layout.Widget, bool) {
	if p.query == "" {
		return []layout.Widget{widgets.EmptyState(e, look.Icon.Search, "Search Ryotunes", "Use the search field above for a song, album, artist, or playlist.")}, true
	}
	switch p.selected {
	case 0:
		switch {
		case p.mixed.loading && !p.mixed.loaded:
			return []layout.Widget{loadingSearch(e, widgets.LoadingHero)}, false
		case p.mixed.error != "" && !p.mixed.loaded:
			return []layout.Widget{widgets.Failed(e, p.mixed.error, &p.retry)}, true
		case mixedCount(p.mixed.result) == 0:
			return []layout.Widget{noResults(e, p.query)}, true
		}
		return []layout.Widget{p.mixedLayout(e)}, false
	case 1:
		return p.trackWidgets(e)
	default:
		return p.cardWidgets(e, p.selected-2, width)
	}
}

func (p *page) mixedLayout(e *env.Env) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		children := make([]layout.FlexChild, 0, 8)
		if len(p.mixed.result.Top) != 0 {
			top := p.mixed.result.Top[0]
			songs := mixedSongs(p.mixed.result.Songs, 4)
			children = append(children,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { return p.topAndSongs(gtx, e, top, songs) }),
				layout.Rigid(layout.Spacer{Height: unit.Dp(34)}.Layout),
			)
		}
		children = appendShelf(children, p, e, "Artists", limitCards(p.mixed.result.Artists, 8))
		children = appendShelf(children, p, e, "Albums", limitCards(p.mixed.result.Albums, 8))
		children = appendShelf(children, p, e, "Playlists", limitCards(p.mixed.result.Playlists, 8))
		return layout.Flex{Axis: layout.Vertical, Alignment: layout.Start}.Layout(gtx, children...)
	}
}

func (p *page) topAndSongs(gtx layout.Context, e *env.Env, top model.BrowseItem, songs []model.Track) layout.Dimensions {
	tile := func(gtx layout.Context) layout.Dimensions { return p.topResult(gtx, e, top) }
	table := func(gtx layout.Context) layout.Dimensions { return p.songPreview(gtx, e, songs) }
	if gtx.Metric.PxToDp(gtx.Constraints.Max.X) >= 1000 {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Start}.Layout(gtx,
			layout.Flexed(.4, tile),
			layout.Rigid(layout.Spacer{Width: unit.Dp(34)}.Layout),
			layout.Flexed(.6, table),
		)
	}
	return layout.Flex{Axis: layout.Vertical, Alignment: layout.Start}.Layout(gtx,
		layout.Rigid(tile),
		layout.Rigid(layout.Spacer{Height: unit.Dp(34)}.Layout),
		layout.Rigid(table),
	)
}

func (p *page) topResult(gtx layout.Context, e *env.Env, item model.BrowseItem) layout.Dimensions {
	p.topPlay.Kind = kit.PlaqueGold
	p.topPlay.Icon = look.Icon.Play
	p.topPlay.Label = "Play"
	return kit.SlabContent(gtx, e.Th.Theme, func(gtx layout.Context) layout.Dimensions {
		artSize := gtx.Dp(120)
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return p.topArt.Layout(gtx, e.Th, e.Art, thumbnail(item), image.Pt(artSize, artSize), look.Shape{Cut: 7, Octagon: isArtist(item)}, e.Th.Motion.Navigate)
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(21)}.Layout),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical, Alignment: layout.Start}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return kit.Label{Style: e.Th.T.Section, Color: e.Th.R.Fg, MaxLines: 2}.Layout(gtx, e.Th.Theme, item.Title)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return kit.Label{Style: e.Th.T.Caption, Color: e.Th.R.Fg3, MaxLines: 1}.Layout(gtx, e.Th.Theme, resultKind(item))
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(13)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions { return p.topPlay.Layout(gtx, e.Th.Theme) }),
				)
			}),
		)
	})
}

func (p *page) songPreview(gtx layout.Context, e *env.Env, songs []model.Track) layout.Dimensions {
	if len(songs) == 0 {
		return layout.Dimensions{}
	}
	opts := widgets.TrackTableOpts{Columns: widgets.ColArt | widgets.ColArtist | widgets.ColAlbum, Context: "search", DisplayItems: songs}
	children := []layout.FlexChild{layout.Rigid(func(gtx layout.Context) layout.Dimensions { return p.topTable.Header(gtx, e, opts) })}
	for index := range songs {
		index := index
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return p.topTable.Row(gtx, e, songs, index, opts)
		}))
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

func (p *page) trackWidgets(e *env.Env) ([]layout.Widget, bool) {
	switch {
	case p.tracks.loading && !p.tracks.loaded:
		return []layout.Widget{loadingSearch(e, widgets.LoadingTable)}, false
	case p.tracks.error != "" && !p.tracks.loaded:
		return []layout.Widget{widgets.Failed(e, p.tracks.error, &p.retry)}, true
	case len(p.tracks.items) == 0:
		return []layout.Widget{noResults(e, p.query)}, true
	}
	items := p.tracks.items
	opts := widgets.TrackTableOpts{Columns: widgets.ColArt | widgets.ColArtist | widgets.ColAlbum | widgets.ColPlays, Context: "search", DisplayItems: items}
	children := []layout.Widget{func(gtx layout.Context) layout.Dimensions { return p.tracksTable.Header(gtx, e, opts) }}
	for index := range items {
		index := index
		children = append(children, func(gtx layout.Context) layout.Dimensions {
			return p.tracksTable.Row(gtx, e, items, index, opts)
		})
	}
	if p.tracks.paging {
		children = append(children, kit.Text(e.Th.Theme, e.Th.T.Proof, e.Th.R.Fg2, "Loading more songs..."))
	}
	return children, false
}

func (p *page) cardWidgets(e *env.Env, index, width int) ([]layout.Widget, bool) {
	if index < 0 || index >= len(p.cards) {
		return nil, false
	}
	state := &p.cards[index]
	switch {
	case state.loading && !state.loaded:
		return []layout.Widget{loadingSearch(e, widgets.LoadingGrid)}, false
	case state.error != "" && !state.loaded:
		return []layout.Widget{widgets.Failed(e, state.error, &p.retry)}, true
	case len(state.items) == 0:
		return []layout.Widget{noResults(e, p.query)}, true
	}
	rows := state.grid.Rows(width, len(state.items))
	children := make([]layout.Widget, 0, rows+1)
	for row := range rows {
		row := row
		children = append(children, func(gtx layout.Context) layout.Dimensions {
			return state.grid.Row(gtx, e, state.items, row)
		})
	}
	if state.paging {
		children = append(children, kit.Text(e.Th.Theme, e.Th.T.Proof, e.Th.R.Fg2, "Loading more..."))
	}
	return children, false
}

// loadingSearch is laid out as a result row, so it takes the rows' insets.
func loadingSearch(e *env.Env, shape widgets.LoadingShape) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical, Alignment: layout.Start}.Layout(gtx,
			layout.Rigid(widgets.Skeleton(e, shape)),
			layout.Rigid(layout.Spacer{Height: unit.Dp(13)}.Layout),
			layout.Rigid(kit.Text(e.Th.Theme, e.Th.T.Proof, e.Th.R.Fg2, "Resolving songs, artists, albums and playlists...")),
		)
	}
}

func noResults(e *env.Env, query string) layout.Widget {
	return widgets.EmptyState(e, look.Icon.Search, "No results for “"+query+"”.", "Try another spelling or a broader search.")
}

func appendShelf(children []layout.FlexChild, p *page, e *env.Env, title string, items []model.BrowseItem) []layout.FlexChild {
	if len(items) == 0 {
		return children
	}
	view := p.shelves[title]
	if view == nil {
		view = new(widgets.Shelf)
		p.shelves[title] = view
	}
	return append(children,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return view.Layout(gtx, e, title, items, nil) }),
		layout.Rigid(layout.Spacer{Height: unit.Dp(34)}.Layout),
	)
}

func mixedSongs(items []model.BrowseItem, limit int) []model.Track {
	tracks := make([]model.Track, 0, min(limit, len(items)))
	for _, item := range items {
		if item.IsVideo || strings.ToLower(item.Kind) != "song" {
			continue
		}
		track := act.TrackFromCard(item)
		tracks = append(tracks, track)
		if len(tracks) == limit {
			break
		}
	}
	return tracks
}

func limitCards(items []model.BrowseItem, limit int) []model.BrowseItem {
	if len(items) > limit {
		return items[:limit]
	}
	return items
}

func mixedCount(result model.SearchAll) int {
	seen := make(map[string]struct{})
	for _, items := range [][]model.BrowseItem{result.Top, result.Songs, result.Albums, result.Artists, result.Playlists} {
		for _, item := range items {
			if item.ID != "" {
				seen[item.ID] = struct{}{}
			}
		}
	}
	return len(seen)
}

func countSongs(items []model.BrowseItem) int {
	count := 0
	for _, item := range items {
		if strings.ToLower(item.Kind) == "song" && !item.IsVideo {
			count++
		}
	}
	return count
}

func resultKind(item model.BrowseItem) string {
	kind := strings.ToLower(strings.TrimSpace(item.Kind))
	switch kind {
	case "song", "track", "video":
		return "Song"
	case "album", "single", "ep":
		return "Album"
	case "artist", "channel", "user":
		return "Artist"
	case "playlist", "mix":
		return "Playlist"
	default:
		if kind == "" {
			return "Result"
		}
		return strings.ToUpper(kind[:1]) + kind[1:]
	}
}

func isArtist(item model.BrowseItem) bool {
	kind := strings.ToLower(strings.TrimSpace(item.Kind))
	return kind == "artist" || kind == "channel" || kind == "user"
}

func thumbnail(item model.BrowseItem) string {
	if item.Thumbnail == nil {
		return ""
	}
	return *item.Thumbnail
}
