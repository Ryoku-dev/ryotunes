// Package album implements the retained album detail page.
package album

import (
	"fmt"
	"strconv"
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

// Page is a retained album detail surface.
type Page struct {
	route  env.Route
	hero   widgets.Hero
	list   widgets.TrackList
	filter lithic.Search

	play      lithic.SplitButton
	artist    lithic.Button
	addQueue  lithic.Button
	download  lithic.Button
	save      lithic.Button
	more      lithic.Button
	retry     lithic.Button
	moreRetry lithic.Button

	page        model.AlbumPage
	loaded      bool
	loading     bool
	loadingMore bool
	errText     string
	moreError   string
	request     uint64
	seenTokens  map[string]struct{}
	libraryRev  uint64
	saved       bool
	savePending bool
}

// New constructs an album detail page for r.ID.
func New(r env.Route) env.Page {
	return &Page{route: r, seenTokens: make(map[string]struct{})}
}

// Title returns the loaded album title.
func (p *Page) Title() string {
	if p.page.Title != nil && *p.page.Title != "" {
		return *p.page.Title
	}
	if p.route.Title != "" {
		return p.route.Title
	}
	return "Album"
}

// Refresh reloads the album and its saved-library state.
func (p *Page) Refresh(e *env.Env) {
	p.loaded = true
	p.loading = true
	p.errText = ""
	p.moreError = ""
	p.request++
	p.seenTokens = make(map[string]struct{})
	p.libraryRev = e.Store.Snapshot().LibraryRevision
	req := p.request
	act.Call[model.AlbumPage](e, protocol.MethodGetAlbum, protocol.IDParams{ID: p.route.ID}, func(page model.AlbumPage, err error) {
		if req != p.request {
			return
		}
		p.loading = false
		if err != nil {
			p.errText = err.Error()
			return
		}
		p.page = page
	})
	if albumCanSave(p.route.ID) {
		act.Call[[]model.BrowseItem](e, protocol.MethodGetLibraryAlbums, protocol.Null{}, func(items []model.BrowseItem, err error) {
			if req != p.request || err != nil {
				return
			}
			p.saved = false
			for _, item := range items {
				if item.ID == p.route.ID {
					p.saved = true
					break
				}
			}
		})
	}
}

// Layout renders the album hero, filter, and numbered track list.
func (p *Page) Layout(gtx layout.Context, e *env.Env) layout.Dimensions {
	state := e.Store.Snapshot()
	if !p.loaded || (p.libraryRev != state.LibraryRevision && !p.loading && !p.savePending) {
		p.Refresh(e)
	}
	p.handleActions(gtx, e)
	if p.loading && p.page.Title == nil {
		return widgets.Loading(e)(gtx)
	}
	if p.errText != "" && p.page.Title == nil {
		return widgets.Failed(e, p.errText, &p.retry)(gtx)
	}

	items, indices := p.filtered()
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return p.hero.Layout(gtx, e, widgets.HeroContent{
				Art:    value(p.page.Thumbnail),
				Kicker: albumKicker(p.page),
				Name:   p.Title(),
				Proof:  value(p.page.Subtitle),
				Facts:  albumFacts(p.page),
				Tone:   lithic.Idle,
				Acts:   p.layoutActs(e),
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: 34, Right: 34, Top: 21, Bottom: 13}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return p.filter.Layout(gtx, e.Th, "Search this album", "/")
			})
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: 34, Right: 34, Bottom: 34}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				if len(items) == 0 && strings.TrimSpace(p.filter.Text()) != "" {
					return widgets.EmptyState(e, "search_off", "No tracks match “"+strings.TrimSpace(p.filter.Text())+"”.", "Clear the filter to see the whole album.")(gtx)
				}
				return p.list.Layout(gtx, e, items, widgets.TrackListOpts{
					ShowArt:   false,
					ShowAlbum: false,
					Footer:    p.layoutFooter(e),
					OnEnd:     func() { p.loadMore(e) },
					OnPlay: func(i int) {
						if i >= 0 && i < len(indices) {
							act.PlayCollection(e, p.page.Items, indices[i], p.route.ID, p.Title(), false, p.page.Continuation)
						}
					},
				})
			})
		}),
	)
}

func (p *Page) handleActions(gtx layout.Context, e *env.Env) {
	if p.retry.Clicked(gtx) {
		p.Refresh(e)
	}
	if p.moreRetry.Clicked(gtx) {
		p.loadMore(e)
	}
	if p.play.Main.Clicked(gtx) && len(p.page.Items) > 0 {
		act.PlayCollection(e, p.page.Items, 0, p.route.ID, p.Title(), false, p.page.Continuation)
	}
	if p.play.More.Clicked(gtx) && len(p.page.Items) > 0 {
		act.PlayCollection(e, p.page.Items, 0, p.route.ID, p.Title(), true, p.page.Continuation)
	}
	if p.addQueue.Clicked(gtx) {
		act.AddToQueue(e, p.page.Items, p.Title())
	}
	if p.download.Clicked(gtx) {
		p.confirmDownload(e)
	}
	if p.save.Clicked(gtx) && !p.savePending {
		if !albumSaveSignedIn(e, p.route.ID) {
			e.Toast("Sign in to save albums to your library", env.ToastInfo)
		} else {
			p.setSaved(e, !p.saved)
		}
	}
	if p.more.Clicked(gtx) {
		e.Menu(env.MenuTarget{Playlist: &p.page})
	}
	if p.artist.Clicked(gtx) {
		if id, name := albumArtist(p.page); id != "" {
			act.OpenArtist(e, id, name)
		}
	}
}

func (p *Page) layoutActs(e *env.Env) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		provider := model.ProviderOf(p.route.ID)
		artistID, artistName := albumArtist(p.page)
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if len(p.page.Items) == 0 {
							gtx = gtx.Disabled()
						}
						return p.play.Layout(gtx, e.Th, lithic.Gold, lithic.Icon("play_arrow"), "Play", false)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return p.addQueue.Layout(gtx, e.Th, lithic.Line, lithic.Icon("queue_music"), "Add to queue")
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if provider == model.ProviderLocal {
							return layout.Dimensions{}
						}
						return p.download.Layout(gtx, e.Th, lithic.Line, lithic.Icon("download"), "Download album")
					}),
				)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if !albumCanSave(p.route.ID) {
							return layout.Dimensions{}
						}
						if p.savePending {
							gtx = gtx.Disabled()
						}
						label := "Save to library"
						if p.saved {
							label = "Remove from library"
						}
						return p.save.Layout(gtx, e.Th, lithic.Quiet, lithic.Icon("favorite"), label)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if artistID == "" {
							return layout.Dimensions{}
						}
						return p.artist.Layout(gtx, e.Th, lithic.Quiet, lithic.Icon("person"), artistName)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return p.more.Layout(gtx, e.Th, lithic.Quiet, lithic.Icon("more_horiz"), "More")
					}),
				)
			}),
		)
	}
}

func (p *Page) setSaved(e *env.Env, saved bool) {
	p.savePending = true
	act.Call[protocol.Null](e, protocol.MethodSetAlbumSaved, protocol.SetAlbumSavedParams{PlaylistID: p.route.ID, Saved: saved}, func(_ protocol.Null, err error) {
		p.savePending = false
		if err != nil {
			e.Toast(err.Error(), env.ToastError)
			return
		}
		p.saved = saved
		if saved {
			e.Toast("Saved to library", env.ToastSuccess)
		} else {
			e.Toast("Removed from library", env.ToastSuccess)
		}
	})
}

func (p *Page) loadMore(e *env.Env) {
	if p.loadingMore || p.page.Continuation == nil || *p.page.Continuation == "" {
		return
	}
	token := *p.page.Continuation
	if _, repeated := p.seenTokens[token]; repeated {
		p.moreError = "Pagination repeated."
		return
	}
	p.seenTokens[token] = struct{}{}
	p.loadingMore = true
	p.moreError = ""
	act.Call[model.PlaylistTracksPage](e, protocol.MethodGetPlaylistMore, protocol.TokenParams{Token: token}, func(more model.PlaylistTracksPage, err error) {
		p.loadingMore = false
		if err != nil {
			delete(p.seenTokens, token)
			p.moreError = err.Error()
			return
		}
		p.page.Items = append(p.page.Items, more.Items...)
		p.page.Continuation = more.Continuation
	})
}

func (p *Page) layoutFooter(e *env.Env) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		if p.loadingMore {
			return layout.Inset{Top: 13, Bottom: 13}.Layout(gtx, lithic.Text(e.Th, e.Th.T.Body, e.Th.R.Fg2, "Loading more…"))
		}
		if p.moreError != "" {
			return layout.Inset{Top: 13, Bottom: 13}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Flexed(1, lithic.Text(e.Th, e.Th.T.Body, e.Th.R.Jasper, p.moreError)),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return p.moreRetry.Layout(gtx, e.Th, lithic.Quiet, nil, "Try again")
					}),
				)
			})
		}
		return layout.Dimensions{}
	}
}

func (p *Page) filtered() ([]model.Track, []int) {
	query := strings.ToLower(strings.TrimSpace(p.filter.Text()))
	if query == "" {
		indices := make([]int, len(p.page.Items))
		for i := range indices {
			indices[i] = i
		}
		return p.page.Items, indices
	}
	items := make([]model.Track, 0, len(p.page.Items))
	indices := make([]int, 0, len(p.page.Items))
	for i, track := range p.page.Items {
		album := value(track.Album)
		if strings.Contains(strings.ToLower(track.Title), query) || strings.Contains(strings.ToLower(track.Artists), query) || strings.Contains(strings.ToLower(album), query) {
			items = append(items, track)
			indices = append(indices, i)
		}
	}
	return items, indices
}

func albumFacts(page model.AlbumPage) []string {
	facts := make([]string, 0, 3)
	if year := findYear(value(page.Subtitle) + " " + value(page.Description)); year != "" {
		facts = append(facts, year)
	}
	facts = append(facts, fmt.Sprintf("%d tracks", len(page.Items)))
	if seconds := totalDuration(page.Items); seconds > 0 {
		facts = append(facts, widgets.Duration(float64(seconds)))
	}
	return facts
}

func albumKicker(page model.AlbumPage) string {
	text := strings.ToLower(value(page.Subtitle) + " " + value(page.Description))
	if strings.Contains(text, " ep") || strings.HasPrefix(text, "ep") {
		return "EP"
	}
	return "Album"
}

func albumArtist(page model.AlbumPage) (string, string) {
	for _, track := range page.Items {
		if track.ArtistID != nil && *track.ArtistID != "" {
			return *track.ArtistID, track.Artists
		}
		for _, run := range track.ArtistRuns {
			if run.ID != nil && *run.ID != "" {
				return *run.ID, run.Text
			}
		}
	}
	return "", ""
}

func findYear(s string) string {
	for _, field := range strings.FieldsFunc(s, func(r rune) bool { return r < '0' || r > '9' }) {
		if len(field) == 4 {
			if year, err := strconv.Atoi(field); err == nil && year >= 1900 && year <= 2200 {
				return field
			}
		}
	}
	return ""
}

func totalDuration(items []model.Track) int {
	total := 0
	for _, item := range items {
		if item.Duration == nil {
			continue
		}
		parts := strings.Split(*item.Duration, ":")
		seconds := 0
		valid := true
		for _, part := range parts {
			n, err := strconv.Atoi(strings.TrimSpace(part))
			if err != nil {
				valid = false
				break
			}
			seconds = seconds*60 + n
		}
		if valid {
			total += seconds
		}
	}
	return total
}

func value(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func albumCanSave(id string) bool {
	provider := model.ProviderOf(id)
	return provider == model.ProviderYouTube || provider == model.ProviderSpotify
}

func albumSaveSignedIn(e *env.Env, id string) bool {
	state := e.Store.Snapshot()
	if model.ProviderOf(id) == model.ProviderSpotify {
		return state.Spotify.SignedIn
	}
	return state.Auth.SignedIn
}
