package chrome

import (
	"fmt"
	"image"
	"strconv"
	"strings"

	"gioui.org/layout"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryotunes/internal/client/state"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/act"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/widgets"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

// sidebarCollectionRow is one collection leaf's state: its line, cover and menu.
type sidebarCollectionRow struct {
	row        treeRow
	art        look.ArtView
	contextTag struct{}
	menu       widgets.ItemMenu
}

type sidebarCollection struct {
	card  model.BrowseItem
	route env.Route
	liked bool
}

// Sidebar is the Hub's tree for Ryotunes: the numbered places, the Playlists folder with
// the collections, and Settings at the foot with the open page's outline. It folds to the
// rail in a wide window and stands alone in the medium class's drawer.
type Sidebar struct {
	tree                 tree
	newPlaylist          widgets.NewPlaylistDialog
	newPlaylistRequested bool
	scroll               look.Scroller
	rows                 map[string]*sidebarCollectionRow

	navigated             bool
	playlists             []model.BrowseItem
	likedCount            int
	likedCountFromLibrary bool
	loaded                bool
	loading               bool
	loadedProvider        model.Provider
	loadedSignedIn        bool
	libraryRev            uint64
	request               uint64
}

// sidebarLoadFailed is the notice for a library that did not load.
const sidebarLoadFailed = "Your playlists did not load. Ryotunes tries again when the library changes."

func (sidebar *Sidebar) initialize() {
	if sidebar.rows == nil {
		sidebar.rows = make(map[string]*sidebarCollectionRow)
	}
}

func sidebarLikedPlaylist(provider model.Provider) string {
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

func (sidebar *Sidebar) finishLoad(e *env.Env, request uint64, err error) {
	if request != sidebar.request {
		return
	}
	sidebar.loading = false
	if err != nil && e.Toast != nil {
		e.Toast(sidebarLoadFailed, env.ToastError)
	}
}

func (sidebar *Sidebar) refresh(e *env.Env, snapshot state.State) {
	if sidebar.loading {
		return
	}
	signedIn := providerSignedIn(snapshot, snapshot.Provider)
	sidebar.loaded = true
	sidebar.loadedProvider = snapshot.Provider
	sidebar.loadedSignedIn = signedIn
	sidebar.libraryRev = snapshot.LibraryRevision
	sidebar.request++
	sidebar.likedCountFromLibrary = false
	// unknown until the provider says; a device's liked list is not the provider's
	sidebar.likedCount = likedCountUnknown
	if !signedIn {
		// signed out there is no library to load, and Home already offers sign in
		sidebar.playlists = sidebar.playlists[:0]
		sidebar.likedCount = 0
		return
	}
	sidebar.loading = true
	request := sidebar.request
	pending := 1
	likedID := sidebarLikedPlaylist(snapshot.Provider)
	if likedID != "" {
		pending++
	}
	done := func(err error) {
		pending--
		if pending == 0 {
			sidebar.finishLoad(e, request, err)
		} else if err != nil && e.Toast != nil {
			e.Toast(sidebarLoadFailed, env.ToastError)
		}
	}
	act.Call[[]model.BrowseItem](e, protocol.MethodGetLibrary, protocol.Null{}, func(items []model.BrowseItem, err error) {
		if request != sidebar.request {
			return
		}
		if err == nil {
			sidebar.playlists = append(sidebar.playlists[:0], items...)
			for _, item := range items {
				if !widgets.IsLikedCollection(item, snapshot.Provider) || !sameLikedCollection(item.ID, likedID) {
					continue
				}
				if count, ok := sidebarCollectionCount(item.Subtitle); ok {
					sidebar.likedCount = count
					sidebar.likedCountFromLibrary = true
					break
				}
			}
		}
		done(err)
	})
	if likedID == "" {
		sidebar.likedCount = 0
		return
	}
	act.Call[model.PlaylistPage](e, protocol.MethodGetPlaylist, protocol.PlaylistParams{ID: likedID}, func(page model.PlaylistPage, err error) {
		if request != sidebar.request {
			return
		}
		if err == nil && !sidebar.likedCountFromLibrary {
			// a first page with more to come is not the count: say none rather than a wrong one
			sidebar.likedCount = likedCountUnknown
			if page.Continuation == nil || strings.TrimSpace(*page.Continuation) == "" {
				sidebar.likedCount = len(page.Items)
			}
		}
		done(err)
	})
}

// likedCountUnknown is a liked count the provider has not given: the leaf shows no count.
const likedCountUnknown = -1

// sameLikedCollection reports whether a library card is the provider's own liked list the
// leaf opens (YouTube's VLLM is also LM), not the device's liked list beside it.
func sameLikedCollection(id, likedID string) bool {
	id = strings.TrimSpace(id)
	if strings.EqualFold(id, likedID) {
		return true
	}
	return likedID == "VLLM" && strings.EqualFold(id, "LM")
}

func sidebarCollectionKind(item model.BrowseItem) string {
	switch strings.ToLower(strings.TrimSpace(item.Kind)) {
	case "album", "single", "ep":
		return "Album"
	case "artist", "channel", "user":
		return "Artist"
	default:
		return "Playlist"
	}
}

func sidebarCollectionRoute(item model.BrowseItem) env.Route {
	switch strings.ToLower(strings.TrimSpace(item.Kind)) {
	case "album", "single", "ep":
		return env.Route{Kind: env.RouteAlbum, ID: item.ID, Title: item.Title}
	case "artist", "channel", "user":
		return env.Route{Kind: env.RouteArtist, ID: item.ID, Title: item.Title}
	default:
		return env.Route{Kind: env.RoutePlaylist, ID: item.ID, Title: item.Title}
	}
}

func sidebarPinned(personal state.Personal, library []model.BrowseItem) []model.BrowseItem {
	byID := make(map[string]model.BrowseItem)
	for _, item := range library {
		byID[item.ID] = item
	}
	for _, item := range personal.Picks {
		byID[item.ID] = item.BrowseItem
	}
	for _, item := range personal.Saved {
		byID[item.ID] = item
	}
	for _, item := range personal.Recent {
		byID[item.ID] = item.BrowseItem
	}
	out := make([]model.BrowseItem, 0, len(personal.Pins))
	for _, id := range personal.Pins {
		if item, ok := byID[id]; ok {
			out = append(out, item)
		}
	}
	return out
}

func sidebarCollectionCount(caption *string) (int, bool) {
	if caption == nil {
		return 0, false
	}
	for _, field := range strings.Fields(*caption) {
		value := strings.ReplaceAll(strings.Trim(field, "(),"), ",", "")
		if count, err := strconv.Atoi(value); err == nil && count >= 0 {
			return count, true
		}
	}
	return 0, false
}

func (sidebar *Sidebar) collections(snapshot state.State) []sidebarCollection {
	likedID := sidebarLikedPlaylist(snapshot.Provider)
	likedRoute := env.Route{Kind: env.RouteLibrary, Params: "songs", Title: "Liked songs"}
	if likedID != "" {
		likedRoute = env.Route{Kind: env.RoutePlaylist, ID: likedID, Title: "Liked songs"}
	}
	liked := model.BrowseItem{Kind: "playlist", ID: likedID, Title: "Liked songs"}
	items := []sidebarCollection{{card: liked, route: likedRoute, liked: true}}
	seen := map[string]struct{}{likedID: {}}
	for _, item := range sidebarPinned(snapshot.Personal, sidebar.playlists) {
		if item.ID == "" || widgets.IsLikedCollection(item, snapshot.Provider) {
			continue
		}
		if _, exists := seen[item.ID]; exists {
			continue
		}
		seen[item.ID] = struct{}{}
		items = append(items, sidebarCollection{card: item, route: sidebarCollectionRoute(item)})
	}
	for _, item := range sidebar.playlists {
		if item.ID == "" || widgets.IsLikedCollection(item, snapshot.Provider) {
			continue
		}
		if _, exists := seen[item.ID]; exists {
			continue
		}
		seen[item.ID] = struct{}{}
		items = append(items, sidebarCollection{card: item, route: sidebarCollectionRoute(item)})
	}
	return items
}

func sidebarRouteMatches(current, candidate env.Route) bool {
	if candidate.ID != "" {
		return current.Kind == candidate.Kind && current.ID == candidate.ID
	}
	return current.Kind == candidate.Kind && current.Params == candidate.Params
}

func (sidebar *Sidebar) collectionRow(key string) *sidebarCollectionRow {
	row := sidebar.rows[key]
	if row == nil {
		row = new(sidebarCollectionRow)
		sidebar.rows[key] = row
	}
	return row
}

func sidebarNavigate(e *env.Env, route env.Route) {
	if e.Nav.Current() == route {
		return
	}
	e.Nav.Push(route)
}

func sidebarItemKey(item sidebarCollection, index int) string {
	if item.card.ID != "" {
		return item.card.ID
	}
	return fmt.Sprintf("%s:%d", item.card.Title, index)
}

func sidebarPlaying(snapshot state.State, item sidebarCollection) bool {
	return snapshot.Queue.SourceName != nil && strings.EqualFold(strings.TrimSpace(*snapshot.Queue.SourceName), strings.TrimSpace(item.card.Title))
}

func sidebarCardCount(item model.BrowseItem) (int, bool) {
	if count, ok := sidebarCollectionCount(item.Subtitle); ok {
		return count, true
	}
	return sidebarCollectionCount(item.PlayCount)
}

func sidebarAlbumArtist(item model.BrowseItem) string {
	for _, artist := range item.ArtistRuns {
		name := strings.TrimSpace(artist.Text)
		if name != "" && !strings.EqualFold(name, strings.TrimSpace(item.Title)) {
			return name
		}
	}
	if item.Subtitle == nil {
		return ""
	}
	name := strings.TrimSpace(*item.Subtitle)
	if name == "" || strings.EqualFold(name, strings.TrimSpace(item.Title)) || strings.EqualFold(name, "Album") {
		return ""
	}
	return name
}

func sidebarSongsSuffix(count int) string {
	if count == 1 {
		return " song"
	}
	return " songs"
}

// sidebarCaption is a leaf's second line in caption, fg3 (fg2 on the open page's line).
func sidebarCaption(e *env.Env, item sidebarCollection, likedCount int, selected bool) layout.Widget {
	ink := e.Th.R.Fg3
	if selected {
		ink = e.Th.R.Fg2
	}
	if item.liked {
		if likedCount < 0 {
			return func(layout.Context) layout.Dimensions { return layout.Dimensions{} }
		}
		if likedCount == 0 {
			return kit.Text(e.Th.Theme, e.Th.T.Caption, ink, "No songs yet")
		}
		return func(gtx layout.Context) layout.Dimensions {
			return kit.Sentence(gtx, e.Th.Theme, e.Th.T.Caption, ink, []kit.Span{
				{Text: widgets.Count(likedCount), Mono: true},
				{Text: sidebarSongsSuffix(likedCount)},
			})
		}
	}
	kind := sidebarCollectionKind(item.card)
	switch kind {
	case "Playlist":
		if count, ok := sidebarCardCount(item.card); ok {
			return func(gtx layout.Context) layout.Dimensions {
				return kit.Sentence(gtx, e.Th.Theme, e.Th.T.Caption, ink, []kit.Span{
					{Text: "Playlist · "},
					{Text: widgets.Count(count), Mono: true},
					{Text: sidebarSongsSuffix(count)},
				})
			}
		}
	case "Album":
		if artist := sidebarAlbumArtist(item.card); artist != "" {
			return kit.Text(e.Th.Theme, e.Th.T.Caption, ink, "Album · "+artist)
		}
	}
	return kit.Text(e.Th.Theme, e.Th.T.Caption, ink, kind)
}

// layoutCollectionArt draws a leaf's cover size pixels square; Liked songs is the
// music's field with its heart.
func (sidebar *Sidebar) layoutCollectionArt(gtx layout.Context, e *env.Env, row *sidebarCollectionRow, item sidebarCollection, size int) layout.Dimensions {
	gtx.Constraints = layout.Exact(image.Pt(size, size))
	if item.liked {
		bounds := image.Rectangle{Max: image.Pt(size, size)}
		look.FillArt(gtx, e.Th, bounds.Size(), look.Shape{Cut: 3}, e.Th.Accent.Field)
		glyph(gtx, look.Icon.Heart, e.Th.Accent.Ink, gtx.Dp(18), size/2, size/2, 0, 1)
		return layout.Dimensions{Size: bounds.Size()}
	}
	thumbnail := ""
	if item.card.Thumbnail != nil {
		thumbnail = *item.card.Thumbnail
	}
	if strings.TrimSpace(thumbnail) == "" {
		// a playlist with nothing to show wears the empty plate, not the loading hatch
		return widgets.EmptyCover(gtx, e.Th, image.Pt(size, size), look.Shape{Cut: 3}, look.Icon.Playlist)
	}
	return row.art.Layout(gtx, e.Th, e.Art, thumbnail, image.Pt(size, size), look.Shape{Cut: 3}, e.Th.Motion.Field)
}

func (sidebar *Sidebar) openCollectionMenu(e *env.Env, row *sidebarCollectionRow, item sidebarCollection) {
	if item.liked && item.card.ID == "" {
		return
	}
	card := item.card
	row.menu.OpenAt(e, env.MenuTarget{Card: &card}, e.Overlays.Press())
}

// ConsumeNavigation reports and clears a destination activation from this view.
func (sidebar *Sidebar) ConsumeNavigation() bool {
	navigated := sidebar.navigated
	sidebar.navigated = false
	return navigated
}

// ConsumeNewPlaylistRequest reports and clears a New playlist activation.
func (sidebar *Sidebar) ConsumeNewPlaylistRequest() bool {
	requested := sidebar.newPlaylistRequested
	sidebar.newPlaylistRequested = false
	return requested
}

// OpenNewPlaylistDialog opens the sidebar-owned dialog for window-level layout.
func (sidebar *Sidebar) OpenNewPlaylistDialog() {
	sidebar.newPlaylist.Open = true
}

// LayoutDialogs draws sidebar dialogs once with the window's constraints.
func (sidebar *Sidebar) LayoutDialogs(gtx layout.Context, e *env.Env) layout.Dimensions {
	return sidebar.newPlaylist.Layout(gtx, e)
}

// Layout draws the tree across gtx.Constraints.Max on the window's paper: rail is the
// fold, 0 the tree at 272, 1 the 72 dp rail.
func (sidebar *Sidebar) Layout(gtx layout.Context, e *env.Env, class look.Class, rail float32) layout.Dimensions {
	if e == nil || e.Th == nil || e.Nav == nil || e.Store == nil || class == look.Compact {
		return layout.Dimensions{}
	}
	sidebar.initialize()
	snapshot := e.Store.Snapshot()
	if !sidebar.loaded || sidebar.loadedProvider != snapshot.Provider || sidebar.libraryRev != snapshot.LibraryRevision ||
		sidebar.loadedSignedIn != providerSignedIn(snapshot, snapshot.Provider) {
		sidebar.refresh(e, snapshot)
	}
	return sidebar.layoutTree(gtx, e, rail, snapshot)
}
