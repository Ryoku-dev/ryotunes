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
)

func (p *page) mixedLayout(e *env.Env) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		switch {
		case p.mixed.loading && !p.mixed.loaded:
			return loadingSearch(gtx, e)
		case p.mixed.error != "" && !p.mixed.loaded:
			return widgets.Failed(e, p.mixed.error, &p.retry)(gtx)
		case mixedCount(p.mixed.result) == 0:
			return noResults(e, p.query)(gtx)
		}

		children := make([]layout.FlexChild, 0, 12)
		if len(p.mixed.result.Top) != 0 {
			top := p.mixed.result.Top[0]
			children = append(children,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					label, kind := "Open", lithic.Line
					switch strings.ToLower(top.Kind) {
					case "song", "track", "video", "album", "single", "ep", "playlist", "mix":
						label, kind = "Play", lithic.Gold
					}
					proof := subtitle(top)
					return p.topHero.Layout(gtx, e, widgets.HeroContent{
						Art: thumbnail(top), Octagon: top.Kind == "artist", Kicker: "Top result",
						Name: top.Title, Proof: proof, Tone: lithic.Idle,
						Acts: func(gtx layout.Context) layout.Dimensions {
							return p.topOpen.Layout(gtx, e.Th, kind, nil, label)
						},
					})
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(34)}.Layout),
			)
		}
		if top := limitCards(p.mixed.result.Top, 4); len(top) > 1 {
			children = appendShelf(children, p, e, "Top results", top[1:])
		}
		if songs := mixedSongs(p.mixed.result.Songs, 6); len(songs) != 0 {
			children = append(children,
				layout.Rigid(sectionTitle(e, "Songs")),
				layout.Rigid(layout.Spacer{Height: unit.Dp(13)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.Y = 0
					gtx.Constraints.Max.Y = min(gtx.Constraints.Max.Y, gtx.Dp(unit.Dp(60*len(songs))))
					return p.allSongs.Layout(gtx, e, songs, widgets.TrackListOpts{
						ShowArt: true, ShowAlbum: true, Context: "search",
						OnPlay: func(index int) { act.Play(e, songs[index]) },
					})
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(34)}.Layout),
			)
		}
		children = appendShelf(children, p, e, "Albums", limitCards(p.mixed.result.Albums, 5))
		children = appendShelf(children, p, e, "Artists", limitCards(p.mixed.result.Artists, 3))
		children = appendShelf(children, p, e, "Playlists", limitCards(p.mixed.result.Playlists, 5))
		return layout.Flex{Axis: layout.Vertical, Alignment: layout.Start}.Layout(gtx, children...)
	}
}

func (p *page) tracksLayout(e *env.Env) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		switch {
		case p.tracks.loading && !p.tracks.loaded:
			return loadingSearch(gtx, e)
		case p.tracks.error != "" && !p.tracks.loaded:
			return widgets.Failed(e, p.tracks.error, &p.retry)(gtx)
		case len(p.tracks.items) == 0:
			return noResults(e, p.query)(gtx)
		}
		items := p.tracks.items
		var footer layout.Widget
		if p.tracks.paging {
			footer = lithic.Text(e.Th, e.Th.T.Body, e.Th.R.Fg2, "Loading more songs…")
		}
		return p.tracks.list.Layout(gtx, e, items, widgets.TrackListOpts{
			ShowArt: true, ShowAlbum: true, Context: "search", Footer: footer,
			OnPlay: func(index int) { act.Play(e, items[index]) },
			OnEnd:  func() { p.loadTracksMore(e) },
		})
	}
}

func (p *page) cardsLayout(e *env.Env, index int) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		if index < 0 || index >= len(p.cards) {
			return layout.Dimensions{}
		}
		state := &p.cards[index]
		switch {
		case state.loading && !state.loaded:
			return loadingSearch(gtx, e)
		case state.error != "" && !state.loaded:
			return widgets.Failed(e, state.error, &p.retry)(gtx)
		case len(state.items) == 0:
			return noResults(e, p.query)(gtx)
		}
		children := []layout.FlexChild{layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return state.grid.Layout(gtx, e, state.items)
		})}
		if state.continuation != nil && *state.continuation != "" {
			children = append(children,
				layout.Rigid(layout.Spacer{Height: unit.Dp(21)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					label := "Load more"
					if state.paging {
						label = "Loading…"
					}
					return state.more.Layout(gtx, e.Th, lithic.Quiet, nil, label)
				}),
			)
		}
		return layout.Flex{Axis: layout.Vertical, Alignment: layout.Start}.Layout(gtx, children...)
	}
}

func loadingSearch(gtx layout.Context, e *env.Env) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(widgets.Loading(e)),
		layout.Rigid(layout.Spacer{Height: unit.Dp(13)}.Layout),
		layout.Rigid(lithic.Text(e.Th, e.Th.T.Body, e.Th.R.Fg2, "Resolving songs, artists, albums and playlists…")),
	)
}

func noResults(e *env.Env, query string) layout.Widget {
	return widgets.EmptyState(e, "search", "No results for “"+query+"”.", "Try another spelling or a broader search.")
}

func appendShelf(children []layout.FlexChild, p *page, e *env.Env, title string, items []model.BrowseItem) []layout.FlexChild {
	if len(items) == 0 {
		return children
	}
	view := p.shelves[title]
	if view == nil {
		view = &widgets.Shelf{}
		p.shelves[title] = view
	}
	shelf := model.Shelf{Title: title, Items: items}
	return append(children,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return view.Layout(gtx, e, shelf) }),
		layout.Rigid(layout.Spacer{Height: unit.Dp(34)}.Layout),
	)
}

func sectionTitle(e *env.Env, title string) layout.Widget {
	return lithic.Text(e.Th, e.Th.T.Section, e.Th.R.Fg, title)
}

func mixedSongs(items []model.BrowseItem, limit int) []model.Track {
	tracks := make([]model.Track, 0, min(limit, len(items)))
	for _, item := range items {
		if item.IsVideo || item.Kind != "song" {
			continue
		}
		track := model.Track{
			VideoID: item.ID, Title: item.Title, Thumbnail: item.Thumbnail, Duration: item.Duration,
			ArtistRuns: item.ArtistRuns, IsUpload: item.IsUpload, Explicit: item.Explicit,
		}
		if item.Subtitle != nil {
			track.Artists = strings.TrimSpace(*item.Subtitle)
		}
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
		if item.Kind == "song" && !item.IsVideo {
			count++
		}
	}
	return count
}

func subtitle(item model.BrowseItem) string {
	if item.Subtitle == nil {
		return ""
	}
	return *item.Subtitle
}

func thumbnail(item model.BrowseItem) string {
	if item.Thumbnail == nil {
		return ""
	}
	return *item.Thumbnail
}
