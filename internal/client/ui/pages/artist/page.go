// Package artist implements the retained artist detail page.
package artist

import (
	"image"
	"net/url"
	"sort"
	"strings"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/act"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/widgets"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

const (
	artistMoreShuffle = iota
	artistMoreFollow
	artistMoreRadio
	artistMoreCopy
)

// Page is a retained artist detail surface.
type Page struct {
	route       env.Route
	scroll      look.Scroller
	hero        widgets.Hero
	heroActions widgets.HeroActions
	tracks      widgets.TrackTable
	shelves     map[string]*widgets.Shelf
	play        kit.Plaque
	shuffle     kit.IconPlaque
	follow      kit.Plaque
	radio       kit.IconPlaque
	more        kit.IconPlaque
	showAll     kit.Plaque
	retry       kit.Plaque

	moreMenu     kit.Menu
	moreTag      int
	moreItems    []kit.MenuItem
	moreCommands []int

	page          model.ArtistPage
	sections      []model.ArtistSection
	showAllSongs  bool
	following     bool
	followPending bool
	loaded        bool
	loading       bool
	errText       string
	request       uint64
	libraryRev    uint64
	meta          []kit.Span
}

// New constructs an artist detail page for r.ID.
func New(r env.Route) env.Page {
	return &Page{route: r, shelves: make(map[string]*widgets.Shelf)}
}

// Title returns the artist name when loaded.
func (p *Page) Title() string {
	if name := value(p.page.Name); name != "" {
		return name
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
		p.following = page.Subscribed
		p.meta = artistMeta(page)
		p.sections = append(p.sections[:0], page.Sections...)
		sort.SliceStable(p.sections, func(i, j int) bool {
			return sectionRank(p.sections[i].Title) < sectionRank(p.sections[j].Title)
		})
		filtered := p.sections[:0]
		for _, section := range p.sections {
			if len(section.Items) > 0 && sectionRank(section.Title) < 5 {
				filtered = append(filtered, section)
			}
		}
		p.sections = filtered
		for _, section := range p.sections {
			if p.shelves[sectionKey(section)] == nil {
				p.shelves[sectionKey(section)] = new(widgets.Shelf)
			}
		}
	})
}

// Layout renders the artist hero, popular tracks, and release shelves.
func (p *Page) Layout(gtx layout.Context, e *env.Env) layout.Dimensions {
	state := e.Store.Snapshot()
	if !p.loaded || p.libraryRev != state.LibraryRevision && !p.loading {
		p.Refresh(e)
	}
	p.handleActions(gtx, e)
	if p.loading && p.page.Name == nil {
		return widgets.Loading(e, widgets.LoadingHero)(gtx)
	}
	if p.errText != "" && p.page.Name == nil {
		return widgets.Failed(e, p.errText, &p.retry)(gtx)
	}

	popular := min(5, len(p.page.TopSongs))
	if p.showAllSongs && p.page.TopSongsID == nil {
		popular = len(p.page.TopSongs)
	}
	sections := p.sections
	base := 1
	if popular > 0 {
		base += popular + 2
	}
	count := base + len(sections)
	return p.scroll.Layout(gtx, e.Th, count, func(gtx layout.Context, index int) layout.Dimensions {
		if index == 0 {
			return p.layoutHero(gtx, e)
		}
		index--
		if popular > 0 {
			if index == 0 {
				return p.layoutPopularHeader(gtx, e)
			}
			if index == 1 {
				opts := p.popularOpts(p.page.TopSongs[:popular])
				return p.inset(e, 0, 0, func(gtx layout.Context) layout.Dimensions { return p.tracks.Header(gtx, e, opts) })(gtx)
			}
			if index < popular+2 {
				row := index - 2
				opts := p.popularOpts(p.page.TopSongs[:popular])
				return p.inset(e, 0, 0, func(gtx layout.Context) layout.Dimensions {
					return p.tracks.Row(gtx, e, p.page.TopSongs[:popular], row, opts)
				})(gtx)
			}
			index -= popular + 2
		}
		if index < 0 || index >= len(sections) {
			return layout.Dimensions{}
		}
		section := sections[index]
		return p.layoutShelf(gtx, e, section, index+1 == len(sections))
	})
}

func (p *Page) inset(e *env.Env, top, bottom unit.Dp, body layout.Widget) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		inset := look.ContentInset(e.Th.Class)
		return layout.Inset{Left: inset, Right: inset, Top: top, Bottom: bottom}.Layout(gtx, body)
	}
}

func (p *Page) layoutHero(gtx layout.Context, e *env.Env) layout.Dimensions {
	return p.hero.Layout(gtx, e, widgets.HeroContent{
		Art:     value(p.page.Thumbnail),
		Octagon: true,
		Kind:    "Artist",
		Title:   p.Title(),
		Meta:    p.meta,
		Acts:    p.layoutActs(e),
	})
}

func (p *Page) popularOpts(items []model.Track) widgets.TrackTableOpts {
	return widgets.TrackTableOpts{
		Columns:      widgets.ColArt | widgets.ColAlbum | widgets.ColPlays,
		Context:      "artist",
		SourceID:     value(p.page.TopSongsID),
		SourceName:   p.Title(),
		SourceItems:  p.page.TopSongs,
		DisplayItems: items,
	}
}

func (p *Page) layoutPopularHeader(gtx layout.Context, e *env.Env) layout.Dimensions {
	p.showAll.Kind, p.showAll.Label = kit.PlaqueQuiet, "Show all"
	if p.showAllSongs && p.page.TopSongsID == nil {
		p.showAll.Label = "Show fewer"
	}
	return p.inset(e, 34, 13, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Flexed(1, kit.Text(e.Th.Theme, e.Th.T.Section, e.Th.R.Fg, "Popular")),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if p.page.TopSongsID == nil && len(p.page.TopSongs) <= 5 {
					return layout.Dimensions{}
				}
				return p.showAll.Layout(gtx, e.Th.Theme)
			}),
		)
	})(gtx)
}

func (p *Page) layoutShelf(gtx layout.Context, e *env.Env, section model.ArtistSection, last bool) layout.Dimensions {
	var more *env.Route
	if section.MoreBrowseID != nil && *section.MoreBrowseID != "" {
		more = &env.Route{Kind: env.RouteList, ID: *section.MoreBrowseID, Title: sectionTitle(section.Title)}
		if section.MoreParams != nil {
			more.Params = *section.MoreParams
		}
	}
	bottom := unit.Dp(0)
	if last {
		bottom = 34
	}
	return p.inset(e, 34, bottom, func(gtx layout.Context) layout.Dimensions {
		return p.shelves[sectionKey(section)].Layout(gtx, e, sectionTitle(section.Title), section.Items, more)
	})(gtx)
}

func (p *Page) handleActions(gtx layout.Context, e *env.Env) {
	if p.retry.Clicked(gtx) {
		p.Refresh(e)
	}
	if p.play.Clicked(gtx) && len(p.page.TopSongs) > 0 {
		act.PlayCollection(e, p.page.TopSongs, 0, value(p.page.TopSongsID), p.Title(), false, nil)
	}
	if p.shuffle.Clicked(gtx) && len(p.page.TopSongs) > 0 {
		act.PlayCollection(e, p.page.TopSongs, 0, value(p.page.TopSongsID), p.Title(), true, nil)
	}
	if p.follow.Clicked(gtx) && !p.followPending {
		p.toggleFollow(e)
	}
	if p.radio.Clicked(gtx) {
		act.StartRadio(e, "artist", p.route.ID, p.Title())
	}
	if p.more.Clicked(gtx) && e.Overlays != nil {
		anchor := e.Overlays.AnchorOf(&p.more.Clickable, image.Pt(gtx.Dp(40), gtx.Dp(40)))
		e.Overlays.Open(&p.moreTag, anchor, look.Placement{Prefer: look.Below, Align: look.End}, p.layoutMore(e))
	}
	if p.showAll.Clicked(gtx) {
		if p.page.TopSongsID != nil {
			e.Nav.Push(env.Route{Kind: env.RoutePlaylist, ID: *p.page.TopSongsID, Title: "Top songs by " + p.Title()})
		} else {
			p.showAllSongs = !p.showAllSongs
		}
	}
}

func (p *Page) toggleFollow(e *env.Env) {
	if !e.Store.Snapshot().Auth.SignedIn {
		e.Toast("Sign in to subscribe", env.ToastInfo)
		return
	}
	id := strings.TrimSpace(p.page.ChannelID)
	if id == "" {
		id = p.route.ID
	}
	previous := p.following
	p.following = !previous
	p.followPending = true
	act.Call[protocol.SetArtistSubscribedResult](e, protocol.MethodSetArtistSubscribed, protocol.SetArtistSubscribedParams{ID: id, Subscribed: p.following}, func(result protocol.SetArtistSubscribedResult, err error) {
		p.followPending = false
		if err != nil {
			p.following = previous
			if strings.Contains(strings.ToLower(err.Error()), "sign in") {
				e.Toast("Sign in to subscribe", env.ToastInfo)
			} else {
				e.Toast("Could not subscribe", env.ToastError)
			}
			return
		}
		p.following = result.Subscribed
		p.page.Subscribed = result.Subscribed
		if result.Subscribed {
			e.Toast("Subscribed to "+p.Title(), env.ToastSuccess)
		} else {
			e.Toast("Unsubscribed", env.ToastSuccess)
		}
	})
}

func (p *Page) layoutActs(e *env.Env) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		p.play.Kind, p.play.Icon, p.play.Label = kit.PlaqueGold, look.Icon.Play, "Play"
		p.shuffle.Kind, p.shuffle.Icon, p.shuffle.Description = kit.PlaqueQuiet, look.Icon.Shuffle, "Shuffle artist"
		p.follow.Kind, p.follow.Label = kit.PlaqueLine, "Subscribe"
		if p.following {
			p.follow.Label = "Subscribed"
		}
		p.follow.Busy = p.followPending
		p.radio.Kind, p.radio.Icon, p.radio.Description = kit.PlaqueQuiet, look.Icon.Radio, "Start artist radio"
		p.more.Kind, p.more.Icon, p.more.Description = kit.PlaqueQuiet, look.Icon.More, "More artist actions"
		play := func(gtx layout.Context) layout.Dimensions {
			if len(p.page.TopSongs) == 0 {
				gtx = gtx.Disabled()
			}
			return p.play.Layout(gtx, e.Th.Theme)
		}
		shuffle := func(gtx layout.Context) layout.Dimensions { return p.shuffle.Layout(gtx, e.Th.Theme) }
		follow := func(gtx layout.Context) layout.Dimensions { return p.follow.Layout(gtx, e.Th.Theme) }
		radio := func(gtx layout.Context) layout.Dimensions { return p.radio.Layout(gtx, e.Th.Theme) }
		more := func(gtx layout.Context) layout.Dimensions { return p.more.Layout(gtx, e.Th.Theme) }
		var actions [5]layout.Widget
		actions[0], actions[1], actions[3], actions[4] = play, shuffle, radio, more
		if model.ProviderOf(p.route.ID) == model.ProviderYouTube {
			actions[2] = follow
		}
		return p.heroActions.Layout(gtx, actions[:])
	}
}

func (p *Page) layoutMore(e *env.Env) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		p.moreItems = p.moreItems[:0]
		p.moreCommands = p.moreCommands[:0]
		add := func(hidden bool, command int, item kit.MenuItem) {
			if hidden {
				p.moreItems = append(p.moreItems, item)
				p.moreCommands = append(p.moreCommands, command)
			}
		}
		add(p.heroActions.Hidden(1), artistMoreShuffle, kit.MenuItem{Icon: look.Icon.Shuffle, Label: "Shuffle artist"})
		followLabel := "Subscribe"
		if p.following {
			followLabel = "Unsubscribe"
		}
		add(p.heroActions.Hidden(2), artistMoreFollow, kit.MenuItem{Icon: look.Icon.HeartOutline, Label: followLabel})
		add(p.heroActions.Hidden(3), artistMoreRadio, kit.MenuItem{Icon: look.Icon.Radio, Label: "Start artist radio"})
		add(true, artistMoreCopy, kit.MenuItem{Icon: look.Icon.Share, Label: "Copy artist link"})
		dims := p.moreMenu.Layout(gtx, e.Th.Theme, p.moreItems)
		if index, ok := p.moreMenu.Clicked(gtx); ok && index < len(p.moreCommands) {
			switch p.moreCommands[index] {
			case artistMoreShuffle:
				if len(p.page.TopSongs) > 0 {
					act.PlayCollection(e, p.page.TopSongs, 0, value(p.page.TopSongsID), p.Title(), true, nil)
				}
			case artistMoreFollow:
				if !p.followPending {
					p.toggleFollow(e)
				}
			case artistMoreRadio:
				act.StartRadio(e, "artist", p.route.ID, p.Title())
			case artistMoreCopy:
				id := strings.TrimSpace(p.page.ChannelID)
				if id == "" {
					id = p.route.ID
				}
				act.CopyText(e, "https://music.youtube.com/channel/"+url.PathEscape(id), "Link copied")
			}
			e.Overlays.Close(&p.moreTag)
			gtx.Execute(op.InvalidateCmd{})
		}
		return dims
	}
}

func artistMeta(page model.ArtistPage) []kit.Span {
	meta := make([]kit.Span, 0, 3)
	if subscribers := value(page.Subscribers); subscribers != "" {
		if !strings.Contains(strings.ToLower(subscribers), "subscriber") && !strings.Contains(strings.ToLower(subscribers), "follower") {
			subscribers += " followers"
		}
		meta = append(meta, kit.Span{Text: subscribers, Mono: true})
	}
	if listeners := value(page.MonthlyListeners); listeners != "" {
		if !strings.Contains(strings.ToLower(listeners), "listener") {
			listeners += " monthly listeners"
		}
		if len(meta) > 0 {
			meta = append(meta, kit.Span{Text: " · "})
		}
		meta = append(meta, kit.Span{Text: listeners, Mono: true})
	}
	return meta
}

func sectionRank(title string) int {
	text := strings.ToLower(title)
	switch {
	case strings.Contains(text, "single") || strings.Contains(text, "ep"):
		return 1
	case strings.Contains(text, "video"):
		return 2
	case strings.Contains(text, "featured") || strings.Contains(text, "playlist") || strings.Contains(text, "set"):
		return 3
	case strings.Contains(text, "fan") || strings.Contains(text, "related") || strings.Contains(text, "similar") || strings.Contains(text, "like"):
		return 4
	case strings.Contains(text, "album"):
		return 0
	default:
		return 5
	}
}

func sectionTitle(title string) string {
	switch sectionRank(title) {
	case 0:
		return "Albums"
	case 1:
		return "Singles and EPs"
	case 2:
		return "Videos"
	case 3:
		return "Featured on"
	case 4:
		return "Fans also like"
	default:
		return title
	}
}

func sectionKey(section model.ArtistSection) string {
	return section.Title + "\x00" + value(section.MoreBrowseID)
}

func value(v *string) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(*v)
}
