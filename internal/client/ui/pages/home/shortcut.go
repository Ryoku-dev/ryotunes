package home

import (
	"strings"

	"gioui.org/layout"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryotunes/internal/client/lithic"
	"github.com/ryoku-dev/ryotunes/internal/client/state"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/act"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/widgets"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

type shortcutTile struct {
	card   widgets.Card
	pin    lithic.Button
	remove lithic.Button
}

type shortcutDialog struct {
	dialog  lithic.Dialog
	query   lithic.TextField
	open    lithic.Button
	close   lithic.Button
	search  lithic.Button
	preview lithic.Button
	list    layout.List

	loading   bool
	error     string
	request   uint64
	remote    []model.BrowseItem
	link      *model.BrowseItem
	linkInput string
	add       map[string]*lithic.Button

	library        []model.BrowseItem
	libraryLoading bool
	libraryLoaded  bool
	libraryPending int
	libraryRequest uint64
}

func (d *shortcutDialog) handle(gtx layout.Context, e *env.Env, page *page, personal state.Personal) {
	if d.open.Clicked(gtx) {
		d.dialog.Open = true
		d.error = ""
		if !d.libraryLoaded && !d.libraryLoading {
			d.loadLibrary(e)
		}
	}
	if d.close.Clicked(gtx) {
		d.dialog.Open = false
	}
	if d.search.Clicked(gtx) {
		d.searchProvider(e)
	}
	if d.preview.Clicked(gtx) {
		d.previewLink(e)
	}
	if d.add == nil {
		d.add = make(map[string]*lithic.Button)
	}
	for _, item := range d.candidates(personal) {
		button := d.add[item.ID]
		if button != nil && button.Clicked(gtx) {
			page.addShortcut(e, personal, item)
		}
	}
}

func (p *page) shortcutsLayout(e *env.Env, personal state.Personal) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		children := []layout.FlexChild{
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return p.shortcut.open.Layout(gtx, e.Th, lithic.Line, lithic.Icon("add"), "Add shortcut")
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(13)}.Layout),
		}
		if len(personal.Picks) == 0 {
			children = append(children, layout.Rigid(lithic.Text(e.Th, e.Th.T.Body, e.Th.R.Fg2, "No shortcuts yet. Add something you return to often.")))
		} else {
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return lithic.Columns(gtx, unit.Dp(180), unit.Dp(13), len(personal.Picks), func(gtx layout.Context, index int) layout.Dimensions {
					pick := personal.Picks[index]
					tile := p.shortcuts[pick.ID]
					if tile == nil {
						tile = &shortcutTile{}
						p.shortcuts[pick.ID] = tile
					}
					pinned := containsID(personal.Pins, pick.ID)
					if tile.pin.Clicked(gtx) {
						p.togglePin(e, pick.ID, pinned)
					}
					if tile.remove.Clicked(gtx) {
						p.removeShortcut(e, pick.ID)
					}
					dims := layout.Flex{Axis: layout.Vertical, Alignment: layout.Start}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							dims := tile.card.Layout(gtx, e, pick.BrowseItem, unit.Dp(148))
							if tile.card.Clicked() {
								p.mutatePersonal(e, func(personal *state.Personal) bool {
									return state.TouchPick(personal, pick.ID, e.Now.UnixMilli())
								})
								act.OpenCard(e, pick.BrowseItem)
							}
							return dims
						}),
						layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							label := "Pin"
							kind := lithic.Quiet
							if pinned {
								label, kind = "Pinned", lithic.On
							}
							return layout.Flex{Axis: layout.Horizontal, Gap: gtx.Dp(8)}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return tile.pin.Layout(gtx, e.Th, kind, lithic.Icon("pin"), label)
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return tile.remove.Layout(gtx, e.Th, lithic.DangerLine, lithic.Icon("remove"), "Remove")
								}),
							)
						}),
					)
					return dims
				})
			}))
		}
		return layout.Flex{Axis: layout.Vertical, Alignment: layout.Start}.Layout(gtx, children...)
	}
}

func (d *shortcutDialog) layoutDialog(gtx layout.Context, e *env.Env, personal state.Personal) layout.Dimensions {
	body := func(gtx layout.Context) layout.Dimensions {
		candidates := d.candidates(personal)
		children := []layout.FlexChild{
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return d.query.Layout(gtx, e.Th, "Find a shortcut", "Filter your listening library, search the provider, or paste a playlist link", "", d.error)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(13)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Gap: gtx.Dp(8)}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return d.search.Layout(gtx, e.Th, lithic.Line, lithic.Icon("search"), "Search provider")
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return d.preview.Layout(gtx, e.Th, lithic.Quiet, lithic.Icon("link"), "Preview link")
					}),
				)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(21)}.Layout),
		}
		if d.loading {
			children = append(children, layout.Rigid(lithic.Text(e.Th, e.Th.T.Body, e.Th.R.Fg2, "Loading shortcut choices…")))
		} else if len(candidates) == 0 && d.libraryLoading {
			children = append(children, layout.Rigid(lithic.Text(e.Th, e.Th.T.Body, e.Th.R.Fg2, "Your listening library is still loading")))
		} else if len(candidates) == 0 {
			children = append(children, layout.Rigid(lithic.Text(e.Th, e.Th.T.Body, e.Th.R.Fg2, "No matching shortcuts.")))
		} else {
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Max.Y = min(gtx.Constraints.Max.Y, gtx.Dp(300))
				return d.list.Layout(gtx, len(candidates), func(gtx layout.Context, index int) layout.Dimensions {
					item := candidates[index]
					button := d.add[item.ID]
					if button == nil {
						button = &lithic.Button{}
						d.add[item.ID] = button
					}
					label := item.Title
					if item.Subtitle != nil && strings.TrimSpace(*item.Subtitle) != "" {
						label += " · " + strings.TrimSpace(*item.Subtitle)
					}
					return layout.Inset{Bottom: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return button.Layout(gtx, e.Th, lithic.Quiet, lithic.Icon("add"), label)
					})
				})
			}))
		}
		return layout.Flex{Axis: layout.Vertical, Alignment: layout.Start}.Layout(gtx, children...)
	}
	return d.dialog.Layout(gtx, e.Th, "Add shortcut", body, func(gtx layout.Context) layout.Dimensions {
		return d.close.Layout(gtx, e.Th, lithic.Quiet, nil, "Close")
	})
}

func (d *shortcutDialog) loadLibrary(e *env.Env) {
	d.libraryLoading = true
	d.libraryPending = 3
	d.libraryRequest++
	request := d.libraryRequest
	d.library = nil
	done := func(items []model.BrowseItem, err error) {
		if request != d.libraryRequest {
			return
		}
		if err == nil {
			d.library = dedupeBrowse(append(d.library, items...))
		}
		d.libraryPending--
		if d.libraryPending == 0 {
			d.libraryLoading = false
			d.libraryLoaded = true
		}
	}
	act.Call[[]model.BrowseItem](e, protocol.MethodGetLibrary, protocol.Null{}, done)
	act.Call[[]model.BrowseItem](e, protocol.MethodGetLibraryAlbums, protocol.Null{}, done)
	act.Call[[]model.BrowseItem](e, protocol.MethodGetLibraryArtists, protocol.Null{}, done)
}

func (d *shortcutDialog) searchProvider(e *env.Env) {
	query := strings.Join(strings.Fields(d.query.Text()), " ")
	if query == "" {
		d.error = "Enter something to search for."
		return
	}
	d.link = nil
	d.loading = true
	d.error = ""
	d.request++
	request := d.request
	act.Call[model.SearchAll](e, protocol.MethodSearchAll, protocol.QueryParams{Query: query}, func(result model.SearchAll, err error) {
		if request != d.request {
			return
		}
		d.loading = false
		if err != nil {
			d.error = err.Error()
			return
		}
		d.remote = appendSearchAll(nil, result)
	})
}

func (d *shortcutDialog) previewLink(e *env.Env) {
	input := strings.TrimSpace(d.query.Text())
	link := state.ParsePlaylistLink(input)
	if !link.OK {
		d.error = link.Message
		if d.error == "" {
			d.error = "Paste a YouTube or YouTube Music playlist link."
		}
		return
	}
	d.loading = true
	d.error = ""
	d.request++
	request := d.request
	act.Call[model.PlaylistPage](e, protocol.MethodGetPlaylist, protocol.PlaylistParams{ID: link.ID}, func(result model.PlaylistPage, err error) {
		if request != d.request {
			return
		}
		d.loading = false
		if err != nil {
			d.error = err.Error()
			return
		}
		title := "Playlist"
		if result.Title != nil && strings.TrimSpace(*result.Title) != "" {
			title = strings.TrimSpace(*result.Title)
		}
		thumbnail := result.Thumbnail
		if thumbnail == nil {
			thumbnail = result.Cover
		}
		d.link = &model.BrowseItem{Kind: "playlist", ID: link.ID, Title: title, Subtitle: result.Subtitle, Thumbnail: thumbnail}
		d.linkInput = input
	})
}

func (d *shortcutDialog) candidates(personal state.Personal) []model.BrowseItem {
	input := strings.TrimSpace(d.query.Text())
	query := strings.ToLower(input)
	linkActive := d.link != nil && input == d.linkInput
	items := make([]model.BrowseItem, 0, len(personal.Recent)+len(personal.Saved)+len(d.library)+len(d.remote)+1)
	if linkActive {
		items = append(items, *d.link)
	}
	for _, recent := range state.RecentItems(personal, 24) {
		items = append(items, recent.BrowseItem)
	}
	items = append(items, personal.Saved...)
	items = append(items, d.library...)
	items = append(items, d.remote...)
	seen := make(map[string]struct{}, len(items))
	out := items[:0]
	for _, item := range items {
		if item.ID == "" || item.Title == "" {
			continue
		}
		if query != "" && !linkActive && !strings.Contains(strings.ToLower(item.Title+" "+subtitle(item)), query) {
			continue
		}
		if _, exists := seen[item.ID]; exists {
			continue
		}
		seen[item.ID] = struct{}{}
		out = append(out, item)
		if len(out) == 32 {
			break
		}
	}
	return out
}

func dedupeBrowse(items []model.BrowseItem) []model.BrowseItem {
	seen := make(map[string]struct{}, len(items))
	out := make([]model.BrowseItem, 0, len(items))
	for _, item := range items {
		if item.ID == "" {
			continue
		}
		if _, exists := seen[item.ID]; exists {
			continue
		}
		seen[item.ID] = struct{}{}
		out = append(out, item)
	}
	return out
}

func appendSearchAll(items []model.BrowseItem, result model.SearchAll) []model.BrowseItem {
	items = append(items, result.Top...)
	items = append(items, result.Songs...)
	items = append(items, result.Albums...)
	items = append(items, result.Artists...)
	return append(items, result.Playlists...)
}

func (p *page) addShortcut(e *env.Env, personal state.Personal, item model.BrowseItem) {
	for _, pick := range personal.Picks {
		if pick.ID == item.ID {
			e.Toast("Already a shortcut", env.ToastInfo)
			return
		}
	}
	if len(personal.Picks) >= 18 {
		e.Toast("Shortcuts are full, remove one first", env.ToastError)
		return
	}
	manual := true
	if p.mutatePersonal(e, func(personal *state.Personal) bool {
		return state.AddPick(personal, state.Pick{BrowseItem: item, Manual: &manual}, e.Now.UnixMilli())
	}) {
		e.Toast("Pinned "+item.Title, env.ToastSuccess)
		p.shortcut.dialog.Open = false
	}
}

func (p *page) removeShortcut(e *env.Env, id string) {
	p.mutatePersonal(e, func(personal *state.Personal) bool {
		for _, pick := range personal.Picks {
			if pick.ID == id {
				state.RemovePick(personal, id)
				personal.Pins = removeID(personal.Pins, id)
				return true
			}
		}
		return false
	})
}

func (p *page) togglePin(e *env.Env, id string, pinned bool) {
	result := ""
	changed := p.mutatePersonal(e, func(personal *state.Personal) bool {
		result = state.TogglePin(personal, id)
		return result != "full"
	})
	if result == "full" {
		e.Toast("Only three shortcuts can be pinned", env.ToastInfo)
	} else if changed && pinned {
		e.Toast("Shortcut unpinned", env.ToastInfo)
	} else if changed {
		e.Toast("Shortcut pinned", env.ToastSuccess)
	}
}
func removeID(ids []string, id string) []string {
	out := ids[:0]
	for _, candidate := range ids {
		if candidate != id {
			out = append(out, candidate)
		}
	}
	return out
}

func containsID(ids []string, id string) bool {
	for _, candidate := range ids {
		if candidate == id {
			return true
		}
	}
	return false
}

func subtitle(item model.BrowseItem) string {
	if item.Subtitle == nil {
		return ""
	}
	return *item.Subtitle
}
