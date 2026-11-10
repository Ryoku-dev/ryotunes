package chrome

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"strconv"
	"strings"

	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/anim"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/palette"
	"github.com/ryoku-dev/ryotunes/internal/client/state"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/act"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/widgets"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

type sidebarDestination struct {
	label string
	kind  env.RouteKind
	icon  *widget.Icon
	click kit.Click
	hover look.Hover
	tip   look.Tip
}

type sidebarCollectionRow struct {
	click      kit.Click
	hover      look.Hover
	tip        look.Tip
	art        look.ArtView
	contextTag struct{}
	menu       widgets.ItemMenu
}

type sidebarCollection struct {
	card  model.BrowseItem
	route env.Route
	liked bool
}

// Sidebar owns expanded and medium destination and collection navigation.
type Sidebar struct {
	destinations         [4]sidebarDestination
	settings             sidebarDestination
	add                  kit.IconPlaque
	addTip               look.Tip
	newPlaylist          widgets.NewPlaylistDialog
	newPlaylistRequested bool
	scroll               look.Scroller
	rows                 map[string]*sidebarCollectionRow

	destinationSelection  anim.Tween
	collectionSelection   anim.Tween
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

func (sidebar *Sidebar) initialize() {
	if sidebar.destinations[0].label != "" {
		return
	}
	sidebar.destinations = [4]sidebarDestination{
		{label: "Home", kind: env.RouteHome, icon: look.Icon.Home},
		{label: "Library", kind: env.RouteLibrary, icon: look.Icon.Library},
		{label: "Radio", kind: env.RouteRadio, icon: look.Icon.Radio},
		{label: "Downloads", kind: env.RouteDownloads, icon: look.Icon.Downloads},
	}
	sidebar.settings = sidebarDestination{label: "Settings", kind: env.RouteSettings, icon: look.Icon.Settings}
	sidebar.rows = make(map[string]*sidebarCollectionRow)
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
		e.Toast("Could not load sidebar playlists: "+err.Error(), env.ToastError)
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
			e.Toast("Could not load sidebar playlists: "+err.Error(), env.ToastError)
		}
	}
	act.Call[[]model.BrowseItem](e, protocol.MethodGetLibrary, protocol.Null{}, func(items []model.BrowseItem, err error) {
		if request != sidebar.request {
			return
		}
		if err == nil {
			sidebar.playlists = append(sidebar.playlists[:0], items...)
			for _, item := range items {
				if !widgets.IsLikedCollection(item, snapshot.Provider) {
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
			sidebar.likedCount = len(page.Items)
		}
		done(err)
	})
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

func sidebarIcon(gtx layout.Context, icon *widget.Icon, ink color.NRGBA, size unit.Dp) layout.Dimensions {
	pixels := gtx.Dp(size)
	gtx.Constraints = layout.Exact(image.Pt(pixels, pixels))
	if icon == nil {
		return layout.Dimensions{Size: image.Pt(pixels, pixels)}
	}
	return icon.Layout(gtx, ink)
}

func sidebarAt(gtx layout.Context, rectangle image.Rectangle, widget layout.Widget) layout.Dimensions {
	if rectangle.Empty() {
		return layout.Dimensions{}
	}
	move := op.Offset(rectangle.Min).Push(gtx.Ops)
	child := gtx
	child.Constraints = layout.Exact(rectangle.Size())
	dimensions := widget(child)
	move.Pop()
	return dimensions
}

func sidebarProgress(value float32) float32 {
	return min(max(value, 0), 1)
}

func sidebarLerp(from, to int, progress float32) int {
	return from + int(float32(to-from)*sidebarProgress(progress)+.5)
}

func sidebarLabelAlpha(progress float32) float32 {
	return max(0, 1-2*sidebarProgress(progress))
}

func sidebarFlake(gtx layout.Context, e *env.Env, size unit.Dp) layout.Dimensions {
	pixels := gtx.Dp(size)
	width := max(1, int(float32(pixels)*1.4+0.5))
	height := max(1, int(float32(pixels)*0.7+0.5))
	gtx.Constraints = layout.Exact(image.Pt(width, height))
	return kit.Flake(gtx, e.Th.Theme, palette.Becoming, 1, size)
}

func sidebarDestinationIndex(current env.Route) int {
	switch current.Kind {
	case env.RouteHome:
		return 0
	case env.RouteLibrary:
		return 1
	case env.RouteRadio:
		return 2
	case env.RouteDownloads:
		return 3
	default:
		return -1
	}
}

func sidebarNavigate(e *env.Env, route env.Route) {
	if e.Nav.Current() == route {
		return
	}
	e.Nav.Push(route)
}

func (sidebar *Sidebar) layoutDestination(gtx layout.Context, e *env.Env, destination *sidebarDestination, index, selectedIndex int, rail float32, snapshot state.State) layout.Dimensions {
	if destination.click.Clicked(gtx) {
		sidebar.navigated = true
		sidebarNavigate(e, env.Route{Kind: destination.kind, Title: destination.label})
	}
	rail = sidebarProgress(rail)
	selected := index == selectedIndex
	height := sidebarLerp(gtx.Dp(40), gtx.Dp(48), rail)
	size := image.Pt(gtx.Constraints.Max.X, height)
	gtx.Constraints = layout.Exact(size)
	body := func(gtx layout.Context) layout.Dimensions {
		return destination.click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			record := op.Record(gtx.Ops)
			dimensions, hover := destination.hover.Layout(gtx, e.Th, func(gtx layout.Context) layout.Dimensions {
				ink := e.Th.R.Fg2
				if destination.click.Hovered() {
					ink = e.Th.R.Fg
				}
				if selected {
					ink = e.Th.R.OnSel
				}
				iconSize := gtx.Dp(20)
				iconX := sidebarLerp(gtx.Dp(13), (size.X-iconSize)/2, rail)
				sidebarAt(gtx, image.Rect(iconX, (height-iconSize)/2, iconX+iconSize, (height+iconSize)/2), func(gtx layout.Context) layout.Dimensions {
					return sidebarIcon(gtx, destination.icon, ink, 20)
				})
				alpha := sidebarLabelAlpha(rail)
				if alpha > 0 {
					labelX := iconX + iconSize + gtx.Dp(13)
					opacity := paint.PushOpacity(gtx.Ops, alpha)
					sidebarAt(gtx, image.Rect(labelX, 0, max(labelX, size.X-gtx.Dp(13)), height), func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								gtx.Constraints.Min.Y = 0
								return kit.Label{Style: e.Th.T.Label, Color: ink, MaxLines: 1}.Layout(gtx, e.Th.Theme, destination.label)
							}),
						)
					})
					opacity.Pop()
				}
				if destination.kind == env.RouteDownloads {
					count := snapshot.Downloads.ActiveCount + snapshot.Downloads.QueuedCount
					if count > 0 {
						if alpha > 0 {
							opacity := paint.PushOpacity(gtx.Ops, alpha)
							sidebarAt(gtx, image.Rect(max(0, size.X-gtx.Dp(34)), 0, size.X-gtx.Dp(13), height), func(gtx layout.Context) layout.Dimensions {
								return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
									layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
										gtx.Constraints.Min.Y = 0
										return kit.Label{Style: e.Th.T.Mono, Color: e.Th.R.Fg2, MaxLines: 1, Align: text.End}.Layout(gtx, e.Th.Theme, fmt.Sprint(count))
									}),
								)
							})
							opacity.Pop()
						}
						flakeAlpha := max(rail, 1-alpha)
						opacity := paint.PushOpacity(gtx.Ops, flakeAlpha)
						sidebarAt(gtx, image.Rect(size.X-gtx.Dp(18), gtx.Dp(6), size.X, height), func(gtx layout.Context) layout.Dimensions {
							return sidebarFlake(gtx, e, 9)
						})
						opacity.Pop()
					}
				}
				return layout.Dimensions{Size: size}
			})
			content := record.Stop()
			area := clip.Rect(image.Rectangle{Max: dimensions.Size}).Push(gtx.Ops)
			if selectedIndex >= 0 {
				sidebar.destinationSelection.Duration = e.Th.Motion.Select
				sidebar.destinationSelection.Ease = anim.EaseChisel
				sidebar.destinationSelection.Set(gtx, float32(selectedIndex))
				at := sidebar.destinationSelection.Value(gtx)
				y := int(math.Round(float64((at - float32(index)) * float32(height+gtx.Dp(4)))))
				kit.FillCut(gtx.Ops, image.Rect(0, y, dimensions.Size.X, y+height), gtx.Dp(6), e.Th.R.Sel)
			}
			if hover > 0 && !selected {
				kit.FillCut(gtx.Ops, image.Rectangle{Max: dimensions.Size}, gtx.Dp(6), palette.WithAlpha(e.Th.R.Stone3, hover))
			}
			content.Add(gtx.Ops)
			area.Pop()
			return dimensions
		})
	}
	if rail >= .999 {
		return destination.tip.Layout(gtx, e.Th, e.Overlays, destination.label, body)
	}
	return body(gtx)
}

func (sidebar *Sidebar) layoutDestinations(gtx layout.Context, e *env.Env, rail float32, snapshot state.State) layout.Dimensions {
	selected := sidebarDestinationIndex(e.Nav.Current())
	rowWidth := gtx.Constraints.Max.X - gtx.Dp(16)
	return layout.Inset{Top: 13, Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		children := make([]layout.FlexChild, 0, len(sidebar.destinations)*2)
		for index := range sidebar.destinations {
			index := index
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Max.X = rowWidth
				return sidebar.layoutDestination(gtx, e, &sidebar.destinations[index], index, selected, rail, snapshot)
			}))
			if index+1 < len(sidebar.destinations) {
				children = append(children, layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout))
			}
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
	})
}

func (sidebar *Sidebar) layoutGroup(gtx layout.Context, e *env.Env, rail float32) layout.Dimensions {
	if sidebar.add.Clicked(gtx) {
		sidebar.newPlaylistRequested = true
	}
	sidebar.add.Kind, sidebar.add.Icon, sidebar.add.Description, sidebar.add.Small = kit.PlaqueQuiet, look.Icon.Add, "New playlist", true
	rail = sidebarProgress(rail)
	width := gtx.Constraints.Max.X
	top := sidebarLerp(gtx.Dp(21), gtx.Dp(13), rail)
	height := top + gtx.Dp(32) + gtx.Dp(8)
	gtx.Constraints = layout.Exact(image.Pt(width, height))
	alpha := sidebarLabelAlpha(rail)
	if alpha > 0 {
		opacity := paint.PushOpacity(gtx.Ops, alpha)
		sidebarAt(gtx, image.Rect(gtx.Dp(21), top, max(gtx.Dp(21), width-gtx.Dp(48)), top+gtx.Dp(32)), func(gtx layout.Context) layout.Dimensions {
			return layout.W.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return kit.Label{Style: e.Th.T.Group, Color: e.Th.R.Fg3, MaxLines: 1}.Layout(gtx, e.Th.Theme, "Playlists")
			})
		})
		opacity.Pop()
	}
	expandedX := width - gtx.Dp(8) - gtx.Dp(32)
	railX := (width - gtx.Dp(32)) / 2
	x := sidebarLerp(expandedX, railX, rail)
	sidebarAt(gtx, image.Rect(x, top, x+gtx.Dp(32), top+gtx.Dp(32)), func(gtx layout.Context) layout.Dimensions {
		if rail >= .999 {
			return sidebar.addTip.Layout(gtx, e.Th, e.Overlays, "New playlist", func(gtx layout.Context) layout.Dimensions {
				return sidebar.add.Layout(gtx, e.Th.Theme)
			})
		}
		return sidebar.add.Layout(gtx, e.Th.Theme)
	})
	return layout.Dimensions{Size: image.Pt(width, height)}
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

func sidebarCaption(e *env.Env, item sidebarCollection, likedCount int, selected bool) layout.Widget {
	ink := e.Th.R.Fg3
	if selected {
		ink = e.Th.R.OnSel2
	}
	if item.liked {
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

func (sidebar *Sidebar) layoutCollectionArt(gtx layout.Context, e *env.Env, row *sidebarCollectionRow, item sidebarCollection, size int) layout.Dimensions {
	gtx.Constraints = layout.Exact(image.Pt(size, size))
	if item.liked {
		bounds := image.Rectangle{Max: image.Pt(size, size)}
		kit.FillCut(gtx.Ops, bounds, gtx.Dp(3), e.Th.Accent.Field)
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return sidebarIcon(gtx, look.Icon.Heart, e.Th.Accent.Ink, 18)
		})
	}
	thumbnail := ""
	if item.card.Thumbnail != nil {
		thumbnail = *item.card.Thumbnail
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

func (sidebar *Sidebar) layoutCollection(gtx layout.Context, e *env.Env, item sidebarCollection, index, selectedIndex int, rail float32, snapshot state.State) layout.Dimensions {
	key := sidebarItemKey(item, index)
	row := sidebar.collectionRow(key)
	if row.click.Clicked(gtx) {
		sidebarNavigate(e, item.route)
	}
	for {
		raw, ok := gtx.Event(pointer.Filter{Target: &row.contextTag, Kinds: pointer.Press})
		if !ok {
			break
		}
		if press, ok := raw.(pointer.Event); ok && press.Buttons.Contain(pointer.ButtonSecondary) {
			sidebar.openCollectionMenu(e, row, item)
		}
	}
	rail = sidebarProgress(rail)
	selected := index == selectedIndex
	playing := sidebarPlaying(snapshot, item)
	height := gtx.Dp(48)
	size := image.Pt(gtx.Constraints.Max.X, height)
	gtx.Constraints = layout.Exact(size)
	body := func(gtx layout.Context) layout.Dimensions {
		return row.click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			record := op.Record(gtx.Ops)
			dimensions, hover := row.hover.Layout(gtx, e.Th, func(gtx layout.Context) layout.Dimensions {
				ink := e.Th.R.Fg
				if selected {
					ink = e.Th.R.OnSel
				}
				if playing {
					ink = e.Th.Accent.Ink
				}
				artSize := min(gtx.Dp(32), height-gtx.Dp(16))
				artX := sidebarLerp(gtx.Dp(8), (size.X-artSize)/2, rail)
				sidebarAt(gtx, image.Rect(artX, (height-artSize)/2, artX+artSize, (height+artSize)/2), func(gtx layout.Context) layout.Dimensions {
					return sidebar.layoutCollectionArt(gtx, e, row, item, artSize)
				})
				alpha := sidebarLabelAlpha(rail)
				if alpha > 0 {
					labelX := artX + artSize + gtx.Dp(13)
					labelRight := size.X - gtx.Dp(8)
					if playing {
						labelRight -= gtx.Dp(22)
					}
					opacity := paint.PushOpacity(gtx.Ops, alpha)
					sidebarAt(gtx, image.Rect(labelX, 0, max(labelX, labelRight), height), func(gtx layout.Context) layout.Dimensions {
						return layout.W.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return kit.Label{Style: e.Th.T.Label, Color: ink, MaxLines: 1}.Layout(gtx, e.Th.Theme, item.card.Title)
								}),
								layout.Rigid(sidebarCaption(e, item, sidebar.likedCount, selected)),
							)
						})
					})
					if playing {
						sidebarAt(gtx, image.Rect(size.X-gtx.Dp(26), 0, size.X-gtx.Dp(8), height), func(gtx layout.Context) layout.Dimensions {
							return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return sidebarIcon(gtx, look.Icon.VolumeUp, e.Th.Accent.Ink, 18)
							})
						})
					}
					opacity.Pop()
				}
				return layout.Dimensions{Size: size}
			})
			content := record.Stop()
			area := clip.Rect(image.Rectangle{Max: dimensions.Size}).Push(gtx.Ops)
			inset := sidebarLerp(0, gtx.Dp(8), rail)
			if selectedIndex >= 0 {
				sidebar.collectionSelection.Duration = e.Th.Motion.Select
				sidebar.collectionSelection.Ease = anim.EaseChisel
				sidebar.collectionSelection.Set(gtx, float32(selectedIndex))
				at := sidebar.collectionSelection.Value(gtx)
				y := int(math.Round(float64((at - float32(index)) * float32(height))))
				kit.FillCut(gtx.Ops, image.Rect(inset, y, dimensions.Size.X-inset, y+height), gtx.Dp(6), e.Th.R.Sel)
			}
			if hover > 0 && !selected {
				kit.FillCut(gtx.Ops, image.Rect(inset, 0, dimensions.Size.X-inset, dimensions.Size.Y), gtx.Dp(6), palette.WithAlpha(e.Th.R.Stone3, hover))
			}
			content.Add(gtx.Ops)
			area.Pop()
			pass := pointer.PassOp{}.Push(gtx.Ops)
			contextArea := clip.Rect(image.Rectangle{Max: dimensions.Size}).Push(gtx.Ops)
			event.Op(gtx.Ops, &row.contextTag)
			contextArea.Pop()
			pass.Pop()
			return dimensions
		})
	}
	if rail >= .999 {
		return row.tip.Layout(gtx, e.Th, e.Overlays, item.card.Title, body)
	}
	return body(gtx)
}

func (sidebar *Sidebar) layoutCollections(gtx layout.Context, e *env.Env, rail float32, snapshot state.State) layout.Dimensions {
	items := sidebar.collections(snapshot)
	selected := -1
	for index, item := range items {
		if sidebarRouteMatches(e.Nav.Current(), item.route) {
			selected = index
			break
		}
	}
	return sidebar.scroll.Layout(gtx, e.Th, len(items), func(gtx layout.Context, index int) layout.Dimensions {
		return sidebar.layoutCollection(gtx, e, items[index], index, selected, rail, snapshot)
	})
}

func (sidebar *Sidebar) layoutFooter(gtx layout.Context, e *env.Env, rail float32, snapshot state.State) layout.Dimensions {
	selected := -1
	if e.Nav.Current().Kind == env.RouteSettings {
		selected = 0
	}
	return layout.Inset{Top: 8, Right: 8, Bottom: 13, Left: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		width := gtx.Constraints.Max.X
		paint.FillShape(gtx.Ops, e.Th.R.Vein, clip.Rect(image.Rect(0, 0, width, max(1, gtx.Dp(1)))).Op())
		return layout.Inset{Top: 9}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return sidebar.layoutDestination(gtx, e, &sidebar.settings, 0, selected, rail, snapshot)
		})
	})
}

// ConsumeNavigation reports and clears a destination activation from this view.
func (sidebar *Sidebar) ConsumeNavigation() bool {
	navigated := sidebar.navigated
	sidebar.navigated = false
	return navigated
}

// ConsumeNewPlaylistRequest reports and clears an add-plaque activation.
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

// Layout draws navigation for expanded and medium classes inside the shell-painted pane.
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
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return sidebar.layoutDestinations(gtx, e, rail, snapshot) }),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return sidebar.layoutGroup(gtx, e, rail) }),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return sidebar.layoutCollections(gtx, e, rail, snapshot) }),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return sidebar.layoutFooter(gtx, e, rail, snapshot) }),
	)
}
