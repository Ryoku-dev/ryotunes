// Package playlist implements the retained playlist detail page.
package playlist

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

var sortLabels = []string{"Default", "Newest first", "Oldest first", "Title", "Artist", "Album"}
var sortValues = []model.PlaylistSort{model.PlaylistSortDefault, model.PlaylistSortNewest, model.PlaylistSortOldest, model.PlaylistSortTitle, model.PlaylistSortArtist, model.PlaylistSortAlbum}

// Page is a retained playlist detail surface.
type Page struct {
	route      env.Route
	hero       widgets.Hero
	list       widgets.TrackList
	filter     lithic.Search
	sortPicker lithic.Picker

	play      lithic.Button
	shuffle   lithic.Button
	addQueue  lithic.Button
	download  lithic.Button
	edit      lithic.Button
	more      lithic.Button
	retry     lithic.Button
	moreRetry lithic.Button

	morePopover  lithic.Popover
	moreMenu     lithic.Menu
	editDialog   widgets.EditPlaylistDialog
	exportDialog exportDialog

	page        model.PlaylistPage
	loaded      bool
	loading     bool
	loadingMore bool
	sortPending bool
	errText     string
	moreError   string
	request     uint64
	seenTokens  map[string]struct{}
	libraryRev  uint64
}

// New constructs a playlist detail page for r.ID.
func New(r env.Route) env.Page {
	return &Page{route: r, seenTokens: make(map[string]struct{})}
}

// Title returns the loaded playlist title.
func (p *Page) Title() string {
	if p.page.Title != nil && *p.page.Title != "" {
		return *p.page.Title
	}
	if p.route.Title != "" {
		return p.route.Title
	}
	return "Playlist"
}

// Refresh reloads playlist metadata and tracks.
func (p *Page) Refresh(e *env.Env) {
	p.loaded = true
	p.loading = true
	p.errText = ""
	p.moreError = ""
	p.request++
	p.seenTokens = make(map[string]struct{})
	p.libraryRev = e.Store.Snapshot().LibraryRevision
	req := p.request
	params := protocol.PlaylistParams{ID: p.route.ID}
	if selected := p.selectedSort(); selected != model.PlaylistSortDefault {
		params.Sort = &selected
	}
	act.Call[model.PlaylistPage](e, protocol.MethodGetPlaylist, params, func(page model.PlaylistPage, err error) {
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
}

// Layout renders playlist metadata, controls, sorting, filtering, and tracks.
func (p *Page) Layout(gtx layout.Context, e *env.Env) layout.Dimensions {
	state := e.Store.Snapshot()
	if !p.loaded {
		p.Refresh(e)
	} else if p.libraryRev != state.LibraryRevision {
		if state.ChangedLibraryID == "" || state.ChangedLibraryID == p.route.ID {
			if !p.loading {
				p.Refresh(e)
			}
		} else {
			p.libraryRev = state.LibraryRevision
		}
	}
	p.handleActions(gtx, e)
	if selected, ok := p.sortPicker.Changed(gtx); ok && !p.sortPending && selected >= 0 && selected < len(sortValues) {
		p.setSort(e, sortValues[selected])
	}

	if p.loading && p.page.Title == nil {
		return widgets.Loading(e)(gtx)
	}
	if p.errText != "" && p.page.Title == nil {
		return widgets.Failed(e, p.errText, &p.retry)(gtx)
	}
	items, indices := p.filtered()
	dims := layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return p.hero.Layout(gtx, e, widgets.HeroContent{
				Art:     value(p.page.Thumbnail),
				Collage: strings.HasPrefix(value(p.page.Thumbnail), model.CollageThumbnailPrefix),
				Kicker:  playlistKicker(p.route.ID),
				Name:    p.Title(),
				Proof:   value(p.page.Subtitle),
				Facts:   playlistFacts(p.page.Items),
				Tone:    lithic.Idle,
				Acts:    p.layoutActs(e),
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: 34, Right: 34, Top: 21, Bottom: 13}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.End}.Layout(gtx,
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return p.filter.Layout(gtx, e.Th, "Filter", "/")
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if p.page.SortMenu == nil || !p.page.SortMenu.Editable || isDevice(p.route.ID) {
							return layout.Dimensions{}
						}
						return layout.Inset{Left: 13}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							if p.sortPending {
								gtx = gtx.Disabled()
							}
							return p.sortPicker.Layout(gtx, e.Th, "Sort", sortLabels, sortIndex(p.selectedSort()))
						})
					}),
				)
			})
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: 34, Right: 34, Bottom: 34}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				if len(items) == 0 && strings.TrimSpace(p.filter.Text()) != "" {
					return widgets.EmptyState(e, "search_off", "No tracks match “"+strings.TrimSpace(p.filter.Text())+"”.", "Clear the filter to see the whole playlist.")(gtx)
				}
				return p.list.Layout(gtx, e, items, widgets.TrackListOpts{
					ShowArt:   true,
					ShowAlbum: true,
					Context:   p.route.ID,
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
	p.editDialog.Layout(gtx, e)
	p.exportDialog.Layout(gtx, e, p)
	return dims
}

func (p *Page) handleActions(gtx layout.Context, e *env.Env) {
	if p.retry.Clicked(gtx) {
		p.Refresh(e)
	}
	if p.moreRetry.Clicked(gtx) {
		p.loadMore(e)
	}
	if p.play.Clicked(gtx) && len(p.page.Items) > 0 {
		act.PlayCollection(e, p.page.Items, 0, p.route.ID, p.Title(), false, p.page.Continuation)
	}
	if p.shuffle.Clicked(gtx) && len(p.page.Items) > 0 {
		act.PlayCollection(e, p.page.Items, 0, p.route.ID, p.Title(), true, p.page.Continuation)
	}
	if p.addQueue.Clicked(gtx) {
		act.AddToQueue(e, p.page.Items, p.Title())
	}
	if p.download.Clicked(gtx) {
		p.confirmDownload(e)
	}
	if p.edit.Clicked(gtx) {
		p.editDialog.Page = &p.page
		p.editDialog.Open = true
	}
	if p.more.Clicked(gtx) {
		p.morePopover.Open = !p.morePopover.Open
	}
}

func (p *Page) layoutActs(e *env.Env) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		editable := playlistEditable(p.route.ID, p.page.Owned)
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if len(p.page.Items) == 0 {
							gtx = gtx.Disabled()
						}
						return p.play.Layout(gtx, e.Th, lithic.Gold, lithic.Icon("play_arrow"), "Play")
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return p.shuffle.Layout(gtx, e.Th, lithic.Line, lithic.Icon("shuffle"), "Shuffle")
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return p.addQueue.Layout(gtx, e.Th, lithic.Line, lithic.Icon("queue_music"), "Add to queue")
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return p.download.Layout(gtx, e.Th, lithic.Line, lithic.Icon("download"), "Download")
					}),
				)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if !editable {
							return layout.Dimensions{}
						}
						return p.edit.Layout(gtx, e.Th, lithic.Quiet, lithic.Icon("edit"), "Edit")
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if !editable {
							return layout.Dimensions{}
						}
						return layout.Spacer{Width: unit.Dp(8)}.Layout(gtx)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return p.layoutMore(gtx, e, editable)
					}),
				)
			}),
		)
	}
}

func (p *Page) layoutMore(gtx layout.Context, e *env.Env, editable bool) layout.Dimensions {
	return p.morePopover.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			return p.more.Layout(gtx, e.Th, lithic.Quiet, lithic.Icon("more_horiz"), "More")
		},
		func(gtx layout.Context) layout.Dimensions {
			items := []lithic.MenuItem{
				{Icon: lithic.Icon("more_horiz"), Label: "Other playlist actions"},
				{Icon: lithic.Icon("download_for_offline"), Label: "Export to file"},
			}
			if editable {
				items = append(items, lithic.MenuItem{Icon: lithic.Icon("delete"), Label: "Delete playlist", Danger: true})
			}
			dims := p.moreMenu.Layout(gtx, e.Th, items)
			if index, ok := p.moreMenu.Clicked(gtx); ok {
				p.morePopover.Open = false
				switch index {
				case 0:
					if e.Menu != nil {
						e.Menu(env.MenuTarget{Playlist: &p.page, Context: p.route.ID})
					}
				case 1:
					p.exportDialog.Open(p.Title())
				case 2:
					if editable {
						p.confirmDelete(e)
					}
				}
			}
			return dims
		},
	)
}

func (p *Page) confirmDelete(e *env.Env) {
	body := "This removes it from your account. It can't be undone."
	if isDevice(p.route.ID) {
		body = "This removes it from this device. It can't be undone."
	}
	typeToConfirm := p.Title()
	e.Confirm("Delete this playlist?", body, "Delete", typeToConfirm, func() {
		act.Call[protocol.Null](e, protocol.MethodDeletePlaylist, protocol.PlaylistIDParams{PlaylistID: p.route.ID}, func(_ protocol.Null, err error) {
			if err != nil {
				e.Toast(err.Error(), env.ToastError)
				return
			}
			e.Toast(p.Title()+" deleted", env.ToastSuccess)
			e.Nav.Back()
		})
	})
}

func (p *Page) setSort(e *env.Env, sortValue model.PlaylistSort) {
	p.sortPending = true
	act.Call[protocol.Null](e, protocol.MethodSetPlaylistSort, protocol.SetPlaylistSortParams{PlaylistID: p.route.ID, Sort: sortValue}, func(_ protocol.Null, err error) {
		p.sortPending = false
		if err != nil {
			e.Toast(err.Error(), env.ToastError)
			return
		}
		if p.page.SortMenu == nil {
			p.page.SortMenu = &model.PlaylistSortMenu{Editable: true}
		}
		p.page.SortMenu.Selected = &sortValue
		p.Refresh(e)
	})
}

func (p *Page) selectedSort() model.PlaylistSort {
	if p.page.SortMenu == nil || p.page.SortMenu.Selected == nil {
		return model.PlaylistSortDefault
	}
	return *p.page.SortMenu.Selected
}

func sortIndex(value model.PlaylistSort) int {
	for i, candidate := range sortValues {
		if candidate == value {
			return i
		}
	}
	return 0
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
			e.Toast("Could not load more", env.ToastError)
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
		if strings.Contains(strings.ToLower(track.Title), query) || strings.Contains(strings.ToLower(track.Artists), query) || strings.Contains(strings.ToLower(value(track.Album)), query) {
			items = append(items, track)
			indices = append(indices, i)
		}
	}
	return items, indices
}

func playlistKicker(id string) string {
	if id == "VLLM" || id == "spotify:playlist:liked" || id == "liked" {
		return "Your library"
	}
	switch id {
	case model.OnRepeatPlaylistID, model.RecentlyPlayedPlaylistID, model.RediscoverPlaylistID:
		return "Smart playlist"
	}
	if isDevice(id) {
		if id == model.LocalLikedPlaylistID {
			return "Your library"
		}
		return "Local playlist"
	}
	return "Playlist"
}

func playlistEditable(id string, owned bool) bool {
	switch id {
	case "VLLM", "spotify:playlist:liked", "liked", model.LocalLikedPlaylistID,
		model.OnRepeatPlaylistID, model.RecentlyPlayedPlaylistID, model.RediscoverPlaylistID:
		return false
	default:
		return owned || isDevice(id)
	}
}

func playlistFacts(items []model.Track) []string {
	facts := []string{fmt.Sprintf("%d tracks", len(items))}
	if seconds := totalDuration(items); seconds > 0 {
		facts = append(facts, widgets.Duration(float64(seconds)))
	}
	return facts
}

func totalDuration(items []model.Track) int {
	total := 0
	for _, item := range items {
		if item.Duration == nil {
			continue
		}
		seconds := 0
		valid := true
		for _, part := range strings.Split(*item.Duration, ":") {
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

func isDevice(id string) bool { return strings.HasPrefix(id, model.LocalPlaylistPrefix) }

func value(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
