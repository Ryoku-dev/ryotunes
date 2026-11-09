// Package library implements the retained Library page.
package library

import (
	"fmt"
	"path/filepath"
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

var libraryTabs = []string{"Playlists", "Songs", "Albums", "Artists", "Local"}

type folderActions struct {
	rescan kit.Plaque
	remove kit.Plaque
}

// Page is the retained Library surface.
type Page struct {
	tabs       kit.Tabs
	selected   int
	scroll     look.Scroller
	filter     kit.Search
	filterOpen bool
	focusNext  bool

	playlistsGrid widgets.Grid
	albumsGrid    widgets.Grid
	artistsGrid   widgets.Grid
	tracks        widgets.TrackTable
	localTracks   widgets.TrackTable

	filterButton kit.IconPlaque
	newButton    kit.Plaque
	importButton kit.IconPlaque
	addFolder    kit.Plaque
	retry        kit.Plaque
	folders      map[string]*folderActions

	newDialog    newPlaylistDialog
	importDialog importPlaylistDialog
	folderDialog folderDialog

	playlists       []model.BrowseItem
	playlistItems   []model.BrowseItem
	liked           []model.Track
	likedID         string
	albums          []model.BrowseItem
	artists         []model.BrowseItem
	local           model.LocalLibrary
	filteredCards   []model.BrowseItem
	filteredTracks  []model.Track
	filteredIndices []int
	filteredFolders []string
	filterKey       string

	loaded          bool
	loading         bool
	localLoaded     bool
	localLoading    bool
	rescanRequested bool
	errText         string
	localError      string
	request         uint64
	localRequest    uint64
	pending         int
	provider        model.Provider
	libraryRev      uint64
	localRev        uint64
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
	return &Page{selected: selected, folders: make(map[string]*folderActions)}
}

// Title returns the page title.
func (p *Page) Title() string { return "Library" }

// FocusFilter opens and focuses the current-tab filter for Ctrl F.
func (p *Page) FocusFilter(gtx layout.Context) {
	p.filterOpen = true
	p.filter.Focus(gtx)
}

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
	p.invalidateFilter()
	req := p.request

	act.Call[[]model.BrowseItem](e, protocol.MethodGetLibrary, protocol.Null{}, func(items []model.BrowseItem, err error) {
		if req != p.request {
			return
		}
		if err == nil {
			p.playlists = items
			p.rebuildPlaylists()
		}
		p.finishRequest(err)
	})
	act.Call[[]model.BrowseItem](e, protocol.MethodGetLibraryAlbums, protocol.Null{}, func(items []model.BrowseItem, err error) {
		if req != p.request {
			return
		}
		if err == nil {
			p.albums = items
			p.invalidateFilter()
		}
		p.finishRequest(err)
	})
	act.Call[[]model.BrowseItem](e, protocol.MethodGetLibraryArtists, protocol.Null{}, func(items []model.BrowseItem, err error) {
		if req != p.request {
			return
		}
		if err == nil {
			p.artists = items
			p.invalidateFilter()
		}
		p.finishRequest(err)
	})
	if p.likedID == "" {
		p.liked = nil
		p.rebuildPlaylists()
		p.finishRequest(nil)
	} else {
		act.Call[model.PlaylistPage](e, protocol.MethodGetPlaylist, protocol.PlaylistParams{ID: p.likedID}, func(page model.PlaylistPage, err error) {
			if req != p.request {
				return
			}
			if err == nil {
				p.liked = page.Items
				p.rebuildPlaylists()
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

func (p *Page) rebuildPlaylists() {
	p.playlistItems = p.playlistItems[:0]
	if p.likedID != "" {
		subtitle := pluralSongs(len(p.liked))
		for _, item := range p.playlists {
			if !widgets.IsLikedCollection(item, p.provider) {
				continue
			}
			if candidate := value(item.Subtitle); candidate != "" {
				subtitle = candidate
				break
			}
		}
		p.playlistItems = append(p.playlistItems, model.BrowseItem{Kind: "playlist", ID: p.likedID, Title: "Liked songs", Subtitle: &subtitle})
	}
	for _, item := range p.playlists {
		if !widgets.IsLikedCollection(item, p.provider) {
			p.playlistItems = append(p.playlistItems, item)
		}
	}
	p.invalidateFilter()
}

func (p *Page) refreshLocal(e *env.Env) {
	state := e.Store.Snapshot()
	p.localLoaded = true
	p.localLoading = true
	p.localError = ""
	p.localRequest++
	p.localRev = state.LocalRevision
	p.invalidateFilter()
	req := p.localRequest
	act.Call[model.LocalLibrary](e, protocol.MethodGetLocalLibrary, protocol.Null{}, func(local model.LocalLibrary, err error) {
		if req != p.localRequest {
			return
		}
		p.localLoading = false
		if err != nil {
			p.localError = err.Error()
			if p.rescanRequested {
				e.Toast(err.Error(), env.ToastError)
				p.rescanRequested = false
			}
			return
		}
		p.local = local
		p.syncFolderButtons()
		p.invalidateFilter()
		if p.rescanRequested {
			e.Toast("Local music rescanned", env.ToastSuccess)
			p.rescanRequested = false
		}
	})
}

func (p *Page) syncFolderButtons() {
	keep := make(map[string]struct{}, len(p.local.Folders))
	for _, folder := range p.local.Folders {
		keep[folder] = struct{}{}
		if p.folders[folder] == nil {
			p.folders[folder] = new(folderActions)
		}
	}
	for folder := range p.folders {
		if _, ok := keep[folder]; !ok {
			delete(p.folders, folder)
		}
	}
}

// Layout renders Library through one virtualised scroller and protected dialogs.
func (p *Page) Layout(gtx layout.Context, e *env.Env) layout.Dimensions {
	state := e.Store.Snapshot()
	if !p.loaded || p.provider != state.Provider || p.libraryRev != state.LibraryRevision {
		p.Refresh(e)
	}
	if p.selected == 4 && (!p.localLoaded || p.localRev != state.LocalRevision) && !p.localLoading {
		p.refreshLocal(e)
	}
	p.handleActions(gtx, e)
	if index, ok := p.tabs.Changed(); ok && index >= 0 && index < len(libraryTabs) {
		p.selected = index
		p.invalidateFilter()
		if index == 4 && !p.localLoaded {
			p.refreshLocal(e)
		}
	}

	var dims layout.Dimensions
	if p.loading && len(p.playlistItems)+len(p.liked)+len(p.albums)+len(p.artists) == 0 {
		dims = widgets.Loading(e, widgets.LoadingGrid)(gtx)
	} else if p.errText != "" && len(p.playlistItems)+len(p.liked)+len(p.albums)+len(p.artists) == 0 && !p.libraryNeedsSignIn(e) {
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
		if p.selected == 4 {
			p.refreshLocal(e)
		} else {
			p.Refresh(e)
		}
	}
	if p.filterButton.Clicked(gtx) {
		p.filterOpen = !p.filterOpen
		if p.filterOpen {
			p.focusNext = true
		} else {
			p.filter.SetText("")
			p.invalidateFilter()
		}
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
		actions := p.folders[folder]
		if actions == nil {
			continue
		}
		if actions.rescan.Clicked(gtx) && !p.localLoading {
			p.rescanRequested = true
			e.Toast("Rescanning local music", env.ToastInfo)
			p.refreshLocal(e)
		}
		if actions.remove.Clicked(gtx) {
			path := folder
			act.Call[model.LocalLibrary](e, protocol.MethodRemoveLocalFolder, protocol.PathParams{Path: path}, func(local model.LocalLibrary, err error) {
				if err != nil {
					e.Toast(err.Error(), env.ToastError)
					return
				}
				p.local = local
				p.syncFolderButtons()
				p.invalidateFilter()
			})
		}
	}
}

func (p *Page) layoutPage(gtx layout.Context, e *env.Env) layout.Dimensions {
	query := strings.TrimSpace(p.filter.Text())
	key := fmt.Sprintf("%d\x00%s", p.selected, strings.ToLower(query))
	if key != p.filterKey {
		p.rebuildFilter(query)
	}
	count := p.elementCount(gtx, e)
	return p.scroll.Layout(gtx, e.Th, count, func(gtx layout.Context, index int) layout.Dimensions {
		switch index {
		case 0:
			return widgets.PageHeader(e, "Library", "", nil)(gtx)
		case 1:
			return p.layoutTabs(gtx, e)
		default:
			return p.layoutTabElement(gtx, e, index-2)
		}
	})
}

func (p *Page) layoutTabs(gtx layout.Context, e *env.Env) layout.Dimensions {
	p.tabs.Labels, p.tabs.Selected = libraryTabs, p.selected
	p.filterButton.Kind, p.filterButton.Icon, p.filterButton.Description = kit.PlaqueQuiet, look.Icon.Filter, "Filter library"
	if p.filterOpen {
		p.filterButton.Kind, p.filterButton.Description = kit.PlaqueOn, "Close library filter"
	}
	p.newButton.Kind, p.newButton.Icon, p.newButton.Label = kit.PlaqueLine, look.Icon.Add, "New playlist"
	p.importButton.Kind, p.importButton.Icon, p.importButton.Description = kit.PlaqueQuiet, look.Icon.OpenInNew, "Import playlist"
	p.filter.Placeholder = "Filter " + strings.ToLower(libraryTabs[p.selected])
	if p.focusNext {
		p.filter.Focus(gtx)
		p.focusNext = false
	}
	actions := func(gtx layout.Context) layout.Dimensions {
		search := layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = min(gtx.Constraints.Max.X, gtx.Dp(280))
			gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, gtx.Dp(280))
			return p.filter.Layout(gtx, e.Th.Theme)
		})
		filter := layout.Rigid(func(gtx layout.Context) layout.Dimensions { return p.filterButton.Layout(gtx, e.Th.Theme) })
		if p.filterOpen && p.selected == 0 {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				search,
				layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
				filter,
				layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { return p.importButton.Layout(gtx, e.Th.Theme) }),
				layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { return p.newButton.Layout(gtx, e.Th.Theme) }),
			)
		}
		if p.filterOpen {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				search,
				layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
				filter,
			)
		}
		if p.selected == 0 {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				filter,
				layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { return p.importButton.Layout(gtx, e.Th.Theme) }),
				layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { return p.newButton.Layout(gtx, e.Th.Theme) }),
			)
		}
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, filter)
	}
	return p.inset(e, 13, 13, func(gtx layout.Context) layout.Dimensions {
		if e.Th.Class == look.Compact {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return p.tabs.Layout(gtx, e.Th.Theme)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
				layout.Rigid(actions),
			)
		}
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return p.tabs.Layout(gtx, e.Th.Theme)
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(13)}.Layout),
			layout.Rigid(actions),
		)
	})(gtx)
}

func (p *Page) inset(e *env.Env, top, bottom unit.Dp, body layout.Widget) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		inset := look.ContentInset(e.Th.Class)
		return layout.Inset{Left: inset, Right: inset, Top: top, Bottom: bottom}.Layout(gtx, body)
	}
}

func (p *Page) elementCount(gtx layout.Context, e *env.Env) int {
	inner := max(1, gtx.Constraints.Max.X-2*gtx.Dp(look.ContentInset(e.Th.Class)))
	switch p.selected {
	case 0:
		if len(p.filteredCards) == 0 {
			return 3
		}
		return 2 + p.playlistsGrid.Rows(inner, len(p.filteredCards))
	case 1:
		if len(p.filteredTracks) == 0 {
			return 3
		}
		return 3 + len(p.filteredTracks)
	case 2:
		if len(p.filteredCards) == 0 {
			return 3
		}
		return 2 + p.albumsGrid.Rows(inner, len(p.filteredCards))
	case 3:
		if len(p.filteredCards) == 0 {
			return 3
		}
		return 2 + p.artistsGrid.Rows(inner, len(p.filteredCards))
	default:
		return 2 + p.localElementCount()
	}
}

func (p *Page) layoutTabElement(gtx layout.Context, e *env.Env, index int) layout.Dimensions {
	switch p.selected {
	case 0:
		return p.layoutGridElement(gtx, e, &p.playlistsGrid, p.filteredCards, index)
	case 1:
		return p.layoutTrackElement(gtx, e, p.filteredTracks, p.filteredIndices, p.likedID, "Liked songs", &p.tracks, index)
	case 2:
		return p.layoutGridElement(gtx, e, &p.albumsGrid, p.filteredCards, index)
	case 3:
		return p.layoutGridElement(gtx, e, &p.artistsGrid, p.filteredCards, index)
	default:
		return p.layoutLocalElement(gtx, e, index)
	}
}

func (p *Page) layoutGridElement(gtx layout.Context, e *env.Env, grid *widgets.Grid, items []model.BrowseItem, index int) layout.Dimensions {
	if len(items) == 0 {
		return p.inset(e, 13, 34, widgets.EmptyState(e, look.Icon.Library, p.emptyTitle(e), "Save something and it will appear here."))(gtx)
	}
	bottom := unit.Dp(0)
	inner := max(1, gtx.Constraints.Max.X-2*gtx.Dp(look.ContentInset(e.Th.Class)))
	if index+1 == grid.Rows(inner, len(items)) {
		bottom = 34
	}
	return p.inset(e, 0, bottom, func(gtx layout.Context) layout.Dimensions { return grid.Row(gtx, e, items, index) })(gtx)
}

func (p *Page) layoutTrackElement(gtx layout.Context, e *env.Env, items []model.Track, indices []int, sourceID, sourceName string, table *widgets.TrackTable, index int) layout.Dimensions {
	if len(items) == 0 {
		body := "Like a song and it will appear here."
		if strings.TrimSpace(p.filter.Text()) != "" {
			body = "Clear the filter to see every song."
		}
		return p.inset(e, 13, 34, widgets.EmptyState(e, look.Icon.HeartOutline, p.emptyTitle(e), body))(gtx)
	}
	opts := widgets.TrackTableOpts{
		Columns:       widgets.ColArt | widgets.ColAlbum,
		Context:       "library",
		SourceID:      sourceID,
		SourceName:    sourceName,
		SourceItems:   p.trackSource(),
		DisplayItems:  items,
		SourceIndices: indices,
	}
	if index == 0 {
		return p.inset(e, 0, 0, func(gtx layout.Context) layout.Dimensions { return table.Header(gtx, e, opts) })(gtx)
	}
	row := index - 1
	bottom := unit.Dp(0)
	if row+1 == len(items) {
		bottom = 34
	}
	return p.inset(e, 0, bottom, func(gtx layout.Context) layout.Dimensions { return table.Row(gtx, e, items, row, opts) })(gtx)
}

func (p *Page) trackSource() []model.Track {
	if p.selected == 4 {
		return p.local.Songs
	}
	return p.liked
}

func (p *Page) localElementCount() int {
	if p.localLoading && len(p.local.Folders)+len(p.local.Songs) == 0 || p.localError != "" && len(p.local.Folders)+len(p.local.Songs) == 0 {
		return 1
	}
	folders := p.visibleFolders()
	folderRows := len(folders)
	if folderRows == 0 {
		folderRows = 1
	}
	trackRows := 1
	if len(p.filteredTracks) > 0 {
		trackRows += len(p.filteredTracks)
	}
	return 1 + folderRows + trackRows
}

func (p *Page) layoutLocalElement(gtx layout.Context, e *env.Env, index int) layout.Dimensions {
	if p.localLoading && len(p.local.Folders)+len(p.local.Songs) == 0 {
		return p.inset(e, 13, 34, widgets.Loading(e, widgets.LoadingTable))(gtx)
	}
	if p.localError != "" && len(p.local.Folders)+len(p.local.Songs) == 0 {
		return p.inset(e, 13, 34, widgets.Failed(e, p.localError, &p.retry))(gtx)
	}
	if index == 0 {
		p.addFolder.Kind, p.addFolder.Icon, p.addFolder.Label = kit.PlaqueLine, look.Icon.Add, "Add folder"
		return p.inset(e, 13, 13, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Flexed(1, kit.Text(e.Th.Theme, e.Th.T.Section, e.Th.R.Fg, "Watched folders")),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { return p.addFolder.Layout(gtx, e.Th.Theme) }),
			)
		})(gtx)
	}
	index--
	folders := p.visibleFolders()
	folderRows := len(folders)
	if folderRows == 0 {
		folderRows = 1
	}
	if index < folderRows {
		if len(folders) == 0 {
			return p.inset(e, 0, 21, kit.Text(e.Th.Theme, e.Th.T.Body, e.Th.R.Fg2, "No folders yet. Add the one your music sits in."))(gtx)
		}
		return p.layoutFolder(gtx, e, folders[index])
	}
	index -= folderRows
	if len(p.filteredTracks) == 0 {
		return p.inset(e, 21, 34, widgets.EmptyState(e, look.Icon.Folder, "No local music yet", "Add or rescan a folder that contains music."))(gtx)
	}
	return p.layoutTrackElement(gtx, e, p.filteredTracks, p.filteredIndices, "local", "Local files", &p.localTracks, index)
}

func (p *Page) layoutFolder(gtx layout.Context, e *env.Env, folder string) layout.Dimensions {
	actions := p.folders[folder]
	actions.rescan.Kind, actions.rescan.Icon, actions.rescan.Label = kit.PlaqueQuiet, look.Icon.Refresh, "Rescan"
	actions.rescan.Busy = p.localLoading
	actions.remove.Kind, actions.remove.Icon, actions.remove.Label = kit.PlaqueDangerLine, look.Icon.Remove, "Remove"
	count := folderTrackCount(folder, p.local.Songs)
	return p.inset(e, 0, 4, func(gtx layout.Context) layout.Dimensions {
		return kit.SlabContent(gtx, e.Th.Theme, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return kit.Label{Style: e.Th.T.Mono, Color: e.Th.R.Fg, MaxLines: 1}.Layout(gtx, e.Th.Theme, folder)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(13)}.Layout),
				layout.Rigid(kit.Text(e.Th.Theme, e.Th.T.Mono, e.Th.R.Fg3, pluralSongs(count))),
				layout.Rigid(layout.Spacer{Width: unit.Dp(13)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { return actions.rescan.Layout(gtx, e.Th.Theme) }),
				layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { return actions.remove.Layout(gtx, e.Th.Theme) }),
			)
		})
	})(gtx)
}

func (p *Page) visibleFolders() []string {
	if strings.TrimSpace(p.filter.Text()) == "" {
		return p.local.Folders
	}
	return p.filteredFolders
}

func (p *Page) rebuildFilter(query string) {
	p.filterKey = fmt.Sprintf("%d\x00%s", p.selected, strings.ToLower(strings.TrimSpace(query)))
	p.filteredCards = p.filteredCards[:0]
	p.filteredTracks = p.filteredTracks[:0]
	p.filteredIndices = p.filteredIndices[:0]
	p.filteredFolders = p.filteredFolders[:0]
	lower := strings.ToLower(strings.TrimSpace(query))
	var cards []model.BrowseItem
	var tracks []model.Track
	switch p.selected {
	case 0:
		cards = p.playlistItems
	case 1:
		tracks = p.liked
	case 2:
		cards = p.albums
	case 3:
		cards = p.artists
	default:
		tracks = p.local.Songs
	}
	if p.selected == 4 && lower != "" {
		for _, folder := range p.local.Folders {
			if strings.Contains(strings.ToLower(folder), lower) {
				p.filteredFolders = append(p.filteredFolders, folder)
			}
		}
	}
	if lower == "" {
		p.filteredCards = cards
		p.filteredTracks = tracks
		p.filteredIndices = p.filteredIndices[:0]
		for index := range tracks {
			p.filteredIndices = append(p.filteredIndices, index)
		}
		return
	}
	for _, item := range cards {
		if strings.Contains(strings.ToLower(item.Title), lower) || strings.Contains(strings.ToLower(value(item.Subtitle)), lower) {
			p.filteredCards = append(p.filteredCards, item)
		}
	}
	for index, track := range tracks {
		if strings.Contains(strings.ToLower(track.Title), lower) || strings.Contains(strings.ToLower(track.Artists), lower) || strings.Contains(strings.ToLower(value(track.Album)), lower) {
			p.filteredTracks = append(p.filteredTracks, track)
			p.filteredIndices = append(p.filteredIndices, index)
		}
	}
}

func (p *Page) invalidateFilter() { p.filterKey = "\x00invalid" }

func folderTrackCount(folder string, tracks []model.Track) int {
	clean := filepath.Clean(folder)
	prefix := clean + string(filepath.Separator)
	count := 0
	for _, track := range tracks {
		path, ok := strings.CutPrefix(track.VideoID, "LOCAL:")
		if !ok {
			continue
		}
		path = filepath.Clean(path)
		if path == clean || strings.HasPrefix(path, prefix) {
			count++
		}
	}
	return count
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
		return "Sign in to see your library"
	}
	if strings.TrimSpace(p.filter.Text()) != "" {
		return "No matches"
	}
	return "Nothing saved yet"
}

func value(v *string) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(*v)
}
