// Package library implements the retained Library page.
package library

import (
	"fmt"
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

var libraryTabs = []string{"Playlists", "Songs", "Albums", "Artists", "Local music"}

// Page is the retained Library surface.
type Page struct {
	tabs     lithic.Tabs
	selected int
	list     widget.List

	playlistsGrid widgets.CardGrid
	albumsGrid    widgets.CardGrid
	artistsGrid   widgets.CardGrid
	localAlbums   widgets.CardGrid
	localArtists  widgets.CardGrid
	tracks        widgets.TrackList

	newButton    lithic.Button
	importButton lithic.Button
	addFolder    lithic.Button
	retry        lithic.Button
	remove       map[string]*lithic.Button

	newDialog    newPlaylistDialog
	importDialog importPlaylistDialog
	folderDialog folderDialog

	playlists []model.BrowseItem
	liked     []model.Track
	likedID   string
	albums    []model.BrowseItem
	artists   []model.BrowseItem
	local     model.LocalLibrary

	loaded       bool
	loading      bool
	localLoaded  bool
	localLoading bool
	errText      string
	localError   string
	request      uint64
	localRequest uint64
	pending      int
	provider     model.Provider
	libraryRev   uint64
	localRev     uint64
}

// New constructs a retained Library page.
func New(r env.Route) env.Page {
	selected := 0
	switch strings.ToLower(strings.TrimSpace(r.Params)) {
	case "songs":
		selected = 1
	case "albums":
		selected = 2
	case "artists":
		selected = 3
	case "local", "local music":
		selected = 4
	}
	return &Page{
		selected: selected,
		list:     widget.List{List: layout.List{Axis: layout.Vertical}},
		remove:   make(map[string]*lithic.Button),
		provider: "",
	}
}

// Title returns the page title.
func (p *Page) Title() string { return "Library" }

// Refresh reloads all provider library collections.
func (p *Page) Refresh(e *env.Env) {
	state := e.Store.Snapshot()
	p.loaded = true
	p.loading = true
	p.errText = ""
	p.request++
	p.pending = 4
	p.provider = state.Provider
	p.likedID = likedPlaylistFor(state.Provider)
	p.libraryRev = state.LibraryRevision
	req := p.request

	act.Call[[]model.BrowseItem](e, protocol.MethodGetLibrary, protocol.Null{}, func(items []model.BrowseItem, err error) {
		if req != p.request {
			return
		}
		if err == nil {
			p.playlists = items
		}
		p.finishRequest(err)
	})
	act.Call[[]model.BrowseItem](e, protocol.MethodGetLibraryAlbums, protocol.Null{}, func(items []model.BrowseItem, err error) {
		if req != p.request {
			return
		}
		if err == nil {
			p.albums = items
		}
		p.finishRequest(err)
	})
	act.Call[[]model.BrowseItem](e, protocol.MethodGetLibraryArtists, protocol.Null{}, func(items []model.BrowseItem, err error) {
		if req != p.request {
			return
		}
		if err == nil {
			p.artists = items
		}
		p.finishRequest(err)
	})
	if p.likedID == "" {
		p.liked = nil
		p.finishRequest(nil)
	} else {
		act.Call[model.PlaylistPage](e, protocol.MethodGetPlaylist, protocol.PlaylistParams{ID: p.likedID}, func(page model.PlaylistPage, err error) {
			if req != p.request {
				return
			}
			if err == nil {
				p.liked = page.Items
			}
			p.finishRequest(err)
		})
	}

	if p.selected == 4 {
		p.refreshLocal(e)
	}
}

func (p *Page) finishRequest(err error) {
	if err != nil && p.errText == "" {
		p.errText = err.Error()
	}
	p.pending--
	if p.pending == 0 {
		p.loading = false
	}
}

func (p *Page) refreshLocal(e *env.Env) {
	state := e.Store.Snapshot()
	p.localLoaded = true
	p.localLoading = true
	p.localError = ""
	p.localRequest++
	p.localRev = state.LocalRevision
	req := p.localRequest
	act.Call[model.LocalLibrary](e, protocol.MethodGetLocalLibrary, protocol.Null{}, func(local model.LocalLibrary, err error) {
		if req != p.localRequest {
			return
		}
		p.localLoading = false
		if err != nil {
			p.localError = err.Error()
			return
		}
		p.local = local
		p.syncFolderButtons()
	})
}

func (p *Page) syncFolderButtons() {
	keep := make(map[string]struct{}, len(p.local.Folders))
	for _, folder := range p.local.Folders {
		keep[folder] = struct{}{}
		if p.remove[folder] == nil {
			p.remove[folder] = new(lithic.Button)
		}
	}
	for folder := range p.remove {
		if _, ok := keep[folder]; !ok {
			delete(p.remove, folder)
		}
	}
}

// Layout renders the Library page and its path-entry dialogs.
func (p *Page) Layout(gtx layout.Context, e *env.Env) layout.Dimensions {
	state := e.Store.Snapshot()
	if !p.loaded || p.provider != state.Provider || p.libraryRev != state.LibraryRevision {
		p.Refresh(e)
	}
	if p.selected == 4 && (!p.localLoaded || p.localRev != state.LocalRevision) && !p.localLoading {
		p.refreshLocal(e)
	}

	p.handleActions(gtx, e)
	if index, ok := p.tabs.Changed(gtx); ok && index >= 0 && index < len(libraryTabs) {
		p.selected = index
		if index == 4 && !p.localLoaded {
			p.refreshLocal(e)
		}
	}

	var dims layout.Dimensions
	if p.loading && len(p.playlists)+len(p.liked)+len(p.albums)+len(p.artists) == 0 {
		dims = widgets.Loading(e)(gtx)
	} else if p.libraryNeedsSignIn(e) && len(p.playlists)+len(p.liked)+len(p.albums)+len(p.artists) == 0 {
		dims = p.layoutPage(gtx, e)
	} else if p.errText != "" && len(p.playlists)+len(p.liked)+len(p.albums)+len(p.artists) == 0 {
		dims = widgets.Failed(e, p.errText, &p.retry)(gtx)
	} else {
		dims = p.layoutPage(gtx, e)
	}

	p.newDialog.Layout(gtx, e, p)
	p.importDialog.Layout(gtx, e, p)
	p.folderDialog.Layout(gtx, e, p)
	return dims
}

func (p *Page) handleActions(gtx layout.Context, e *env.Env) {
	if p.retry.Clicked(gtx) {
		p.Refresh(e)
	}
	if p.newButton.Clicked(gtx) {
		p.newDialog.Open()
	}
	if p.importButton.Clicked(gtx) {
		p.importDialog.Open()
	}
	if p.addFolder.Clicked(gtx) {
		p.folderDialog.Open()
	}
	for _, folder := range p.local.Folders {
		button := p.remove[folder]
		if button != nil && button.Clicked(gtx) {
			path := folder
			act.Call[model.LocalLibrary](e, protocol.MethodRemoveLocalFolder, protocol.PathParams{Path: path}, func(local model.LocalLibrary, err error) {
				if err != nil {
					e.Toast(err.Error(), env.ToastError)
					return
				}
				p.local = local
				p.syncFolderButtons()
			})
		}
	}
}

func (p *Page) layoutPage(gtx layout.Context, e *env.Env) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(widgets.PageHeader(e, "Library", "Everything you have saved, plus music on this device.", p.layoutToolbar(e))),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: 34, Right: 34, Top: 13}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return p.tabs.Layout(gtx, e.Th, libraryTabs, p.selected)
			})
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(13)}.Layout),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: 34, Right: 34, Bottom: 34}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				switch p.selected {
				case 0:
					return p.layoutPlaylists(gtx, e)
				case 1:
					return p.layoutSongs(gtx, e)
				case 2:
					return p.layoutCards(gtx, e, p.albums, &p.albumsGrid)
				case 3:
					return p.layoutCards(gtx, e, p.artists, &p.artistsGrid)
				default:
					return p.layoutLocal(gtx, e)
				}
			})
		}),
	)
}

func (p *Page) layoutToolbar(e *env.Env) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return p.importButton.Layout(gtx, e.Th, lithic.Quiet, lithic.Icon("upload_file"), "Import")
	}
}

func (p *Page) layoutPlaylists(gtx layout.Context, e *env.Env) layout.Dimensions {
	return p.list.Layout(gtx, 1, func(gtx layout.Context, _ int) layout.Dimensions {
		children := []layout.FlexChild{
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return p.newButton.Layout(gtx, e.Th, lithic.Gold, lithic.Icon("add"), "New playlist")
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(21)}.Layout),
		}
		if len(p.playlists) == 0 {
			children = append(children, layout.Rigid(widgets.EmptyState(e, "library_music", p.emptyTitle(e), "Make a playlist or save one to find it here.")))
		} else {
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return p.playlistsGrid.Layout(gtx, e, p.playlists)
			}))
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
	})
}

func (p *Page) layoutSongs(gtx layout.Context, e *env.Env) layout.Dimensions {
	if len(p.liked) == 0 {
		return widgets.EmptyState(e, "favorite", p.emptyTitle(e), "Like a song and it will appear here.")(gtx)
	}
	return p.tracks.Layout(gtx, e, p.liked, widgets.TrackListOpts{
		ShowArt:   true,
		ShowAlbum: true,
		Context:   p.likedID,
		OnPlay: func(i int) {
			act.PlayCollection(e, p.liked, i, p.likedID, "Liked Songs", false, nil)
		},
	})
}

func (p *Page) layoutCards(gtx layout.Context, e *env.Env, items []model.BrowseItem, grid *widgets.CardGrid) layout.Dimensions {
	if len(items) == 0 {
		return widgets.EmptyState(e, "library_music", p.emptyTitle(e), "Save something and it will appear here.")(gtx)
	}
	return p.list.Layout(gtx, 1, func(gtx layout.Context, _ int) layout.Dimensions {
		return grid.Layout(gtx, e, items)
	})
}

func (p *Page) layoutLocal(gtx layout.Context, e *env.Env) layout.Dimensions {
	if p.localLoading && len(p.local.Folders)+len(p.local.Songs) == 0 {
		return widgets.Loading(e)(gtx)
	}
	if p.localError != "" && len(p.local.Folders)+len(p.local.Songs) == 0 {
		return widgets.Failed(e, p.localError, &p.retry)(gtx)
	}
	return p.list.Layout(gtx, 1, func(gtx layout.Context, _ int) layout.Dimensions {
		children := []layout.FlexChild{
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Flexed(1, lithic.Text(e.Th, e.Th.T.Section, e.Th.R.Fg, "Folders")),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return p.addFolder.Layout(gtx, e.Th, lithic.Gold, lithic.Icon("add"), "Add folder")
					}),
				)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(13)}.Layout),
			layout.Rigid(p.layoutFolders(e)),
		}
		if len(p.local.Albums) > 0 {
			children = append(children,
				layout.Rigid(layout.Spacer{Height: unit.Dp(34)}.Layout),
				layout.Rigid(lithic.Text(e.Th, e.Th.T.Section, e.Th.R.Fg, "Albums")),
				layout.Rigid(layout.Spacer{Height: unit.Dp(13)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { return p.localAlbums.Layout(gtx, e, p.local.Albums) }),
			)
		}
		if len(p.local.Artists) > 0 {
			children = append(children,
				layout.Rigid(layout.Spacer{Height: unit.Dp(34)}.Layout),
				layout.Rigid(lithic.Text(e.Th, e.Th.T.Section, e.Th.R.Fg, "Artists")),
				layout.Rigid(layout.Spacer{Height: unit.Dp(13)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { return p.localArtists.Layout(gtx, e, p.local.Artists) }),
			)
		}
		if len(p.local.Songs) == 0 && len(p.local.Albums) == 0 && len(p.local.Artists) == 0 {
			children = append(children,
				layout.Rigid(layout.Spacer{Height: unit.Dp(34)}.Layout),
				layout.Rigid(widgets.EmptyState(e, "folder_music", "No local music yet.", "Add a folder that contains your music.")),
			)
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
	})
}

func (p *Page) layoutFolders(e *env.Env) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		if len(p.local.Folders) == 0 {
			return lithic.Text(e.Th, e.Th.T.Body, e.Th.R.Fg2, "No folders yet. Add the one your music sits in.")(gtx)
		}
		rows := make([]layout.FlexChild, 0, len(p.local.Folders)*2)
		for i, folder := range p.local.Folders {
			folder := folder
			rows = append(rows, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return lithic.Slab(gtx, e.Th.R.Stone1, e.Th.Cut.Mid, func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: 13, Bottom: 13, Left: 21, Right: 13}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
							layout.Flexed(1, lithic.Text(e.Th, e.Th.T.Mono, e.Th.R.Fg, folder)),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return p.remove[folder].Layout(gtx, e.Th, lithic.DangerLine, lithic.Icon("remove"), "Remove")
							}),
						)
					})
				})
			}))
			if i+1 < len(p.local.Folders) {
				rows = append(rows, layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout))
			}
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, rows...)
	}
}

func pluralSongs(n int) string {
	if n == 1 {
		return "1 song"
	}
	return fmt.Sprintf("%d songs", n)
}

func isDevicePlaylist(id string) bool { return strings.HasPrefix(id, model.LocalPlaylistPrefix) }

func likedPlaylistFor(provider model.Provider) string {
	switch provider {
	case model.ProviderSpotify:
		return "spotify:playlist:liked"
	case model.ProviderLocal:
		return model.LocalLikedPlaylistID
	case model.ProviderTidal, model.ProviderSoundCloud, model.ProviderRadio:
		return ""
	default:
		return "VLLM"
	}
}

func (p *Page) libraryNeedsSignIn(e *env.Env) bool {
	state := e.Store.Snapshot()
	switch p.provider {
	case model.ProviderSpotify:
		return !state.Spotify.SignedIn
	case model.ProviderYouTube:
		return !state.Auth.SignedIn
	default:
		return false
	}
}

func (p *Page) emptyTitle(e *env.Env) string {
	if p.libraryNeedsSignIn(e) {
		return "Sign in to see your library."
	}
	return "Nothing saved yet."
}
