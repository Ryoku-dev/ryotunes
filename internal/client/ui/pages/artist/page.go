// Package artist implements the retained artist detail page.
package artist

import (
	"sort"
	"strings"

	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"

	"github.com/ryoku-dev/ryotunes/internal/client/lithic"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/act"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/widgets"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

// Page is a retained artist detail surface.
type Page struct {
	route   env.Route
	list    widget.List
	hero    widgets.Hero
	tracks  widgets.TrackList
	shelves map[string]*widgets.Shelf

	play    lithic.Button
	radio   lithic.Button
	showAll lithic.Button
	retry   lithic.Button

	page         model.ArtistPage
	sections     []model.ArtistSection
	showAllSongs bool
	loaded       bool
	loading      bool
	errText      string
	request      uint64
	libraryRev   uint64
}

// New constructs an artist detail page for r.ID.
func New(r env.Route) env.Page {
	return &Page{
		route:   r,
		list:    widget.List{List: layout.List{Axis: layout.Vertical}},
		shelves: make(map[string]*widgets.Shelf),
	}
}

// Title returns the artist name when loaded.
func (p *Page) Title() string {
	if p.page.Name != nil && *p.page.Name != "" {
		return *p.page.Name
	}
	if p.route.Title != "" {
		return p.route.Title
	}
	return "Artist"
}

// Refresh reloads the artist and its ordered sections.
func (p *Page) Refresh(e *env.Env) {
	p.loaded = true
	p.loading = true
	p.errText = ""
	p.request++
	p.libraryRev = e.Store.Snapshot().LibraryRevision
	req := p.request
	method := protocol.MethodGetArtist
	if strings.HasPrefix(p.route.ID, "sc:user:") {
		method = protocol.MethodGetSCUser
	}
	act.Call[model.ArtistPage](e, method, protocol.IDParams{ID: p.route.ID}, func(page model.ArtistPage, err error) {
		if req != p.request {
			return
		}
		p.loading = false
		if err != nil {
			p.errText = err.Error()
			return
		}
		p.page = page
		p.sections = append(p.sections[:0], page.Sections...)
		sort.SliceStable(p.sections, func(i, j int) bool {
			return sectionRank(p.sections[i].Title) < sectionRank(p.sections[j].Title)
		})
		for _, section := range p.sections {
			if p.shelves[sectionKey(section)] == nil {
				p.shelves[sectionKey(section)] = new(widgets.Shelf)
			}
		}
	})
}

// Layout renders the artist hero, popular tracks, and ordered shelves.
func (p *Page) Layout(gtx layout.Context, e *env.Env) layout.Dimensions {
	state := e.Store.Snapshot()
	if !p.loaded || (p.libraryRev != state.LibraryRevision && !p.loading) {
		p.Refresh(e)
	}
	if p.retry.Clicked(gtx) {
		p.Refresh(e)
	}
	if p.play.Clicked(gtx) && len(p.page.TopSongs) > 0 {
		act.PlayCollection(e, p.page.TopSongs, 0, value(p.page.TopSongsID), p.Title(), true, nil)
	}
	if p.radio.Clicked(gtx) {
		act.StartRadio(e, "artist", p.route.ID, p.Title())
	}
	if p.showAll.Clicked(gtx) {
		if p.page.TopSongsID != nil {
			e.Nav.Push(env.Route{Kind: env.RoutePlaylist, ID: *p.page.TopSongsID, Title: "Top songs by " + p.Title()})
		} else {
			p.showAllSongs = !p.showAllSongs
		}
	}

	if p.loading && p.page.Name == nil {
		return widgets.Loading(e)(gtx)
	}
	if p.errText != "" && p.page.Name == nil {
		return widgets.Failed(e, p.errText, &p.retry)(gtx)
	}
	return p.list.Layout(gtx, 1, func(gtx layout.Context, _ int) layout.Dimensions {
		children := []layout.FlexChild{
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return p.hero.Layout(gtx, e, widgets.HeroContent{
					Art:     value(p.page.Thumbnail),
					Octagon: true,
					Kicker:  "Artist",
					Name:    p.Title(),
					Proof:   artistProof(p.page),
					Tone:    lithic.Idle,
					Acts:    p.layoutActs(e),
				})
			}),
		}
		if len(p.page.TopSongs) > 0 {
			children = append(children,
				layout.Rigid(layout.Spacer{Height: unit.Dp(34)}.Layout),
				layout.Rigid(p.layoutPopular(e)),
			)
		}
		for _, section := range p.sections {
			section := section
			if len(section.Items) == 0 {
				continue
			}
			children = append(children,
				layout.Rigid(layout.Spacer{Height: unit.Dp(34)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					shelf := model.Shelf{
						Title:         section.Title,
						TitleIsArtist: strings.Contains(strings.ToLower(section.Title), "related"),
						Items:         section.Items,
						MoreBrowseID:  section.MoreBrowseID,
						MoreParams:    section.MoreParams,
					}
					return p.shelves[sectionKey(section)].Layout(gtx, e, shelf)
				}),
			)
		}
		return layout.Inset{Bottom: 55}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
		})
	})
}

func (p *Page) layoutActs(e *env.Env) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if len(p.page.TopSongs) == 0 {
					gtx = gtx.Disabled()
				}
				return p.play.Layout(gtx, e.Th, lithic.Gold, lithic.Icon("play_arrow"), "Play")
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return p.radio.Layout(gtx, e.Th, lithic.Line, lithic.Icon("radio"), "Start radio")
			}),
		)
	}
}

func (p *Page) layoutPopular(e *env.Env) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		limit := min(5, len(p.page.TopSongs))
		if p.showAllSongs {
			limit = len(p.page.TopSongs)
		}
		items := p.page.TopSongs[:limit]
		return layout.Inset{Left: 34, Right: 34}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Flexed(1, lithic.Text(e.Th, e.Th.T.Section, e.Th.R.Fg, "Top songs")),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							if p.page.TopSongsID == nil && len(p.page.TopSongs) <= 5 {
								return layout.Dimensions{}
							}
							label := "Show all"
							if p.showAllSongs {
								label = "Show fewer"
							}
							return p.showAll.Layout(gtx, e.Th, lithic.Quiet, nil, label)
						}),
					)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(13)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return p.tracks.Layout(gtx, e, items, widgets.TrackListOpts{
						ShowArt: true,
						OnPlay: func(i int) {
							act.PlayCollection(e, p.page.TopSongs, i, value(p.page.TopSongsID), p.Title(), false, nil)
						},
					})
				}),
			)
		})
	}
}

func artistProof(page model.ArtistPage) string {
	parts := make([]string, 0, 2)
	if page.Subscribers != nil && *page.Subscribers != "" {
		proof := *page.Subscribers
		lower := strings.ToLower(proof)
		if !strings.Contains(lower, "subscriber") && !strings.Contains(lower, "follower") {
			proof += " subscribers"
		}
		parts = append(parts, proof)
	}
	if page.MonthlyListeners != nil && *page.MonthlyListeners != "" {
		proof := *page.MonthlyListeners
		if !strings.Contains(strings.ToLower(proof), "listener") {
			proof += " monthly listeners"
		}
		parts = append(parts, proof)
	}
	return strings.Join(parts, " · ")
}

func sectionRank(title string) int {
	t := strings.ToLower(title)
	switch {
	case strings.Contains(t, "album"):
		return 0
	case strings.Contains(t, "single") || strings.Contains(t, "ep"):
		return 1
	case strings.Contains(t, "video"):
		return 2
	case strings.Contains(t, "playlist") || strings.Contains(t, "set"):
		return 3
	case strings.Contains(t, "related") || strings.Contains(t, "similar") || strings.Contains(t, "like"):
		return 4
	default:
		return 5
	}
}

func sectionKey(section model.ArtistSection) string {
	return section.Title + "\x00" + value(section.MoreBrowseID)
}

func value(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
