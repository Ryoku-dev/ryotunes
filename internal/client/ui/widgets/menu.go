package widgets

import (
	"image"
	"strings"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/tokens"
	"github.com/ryoku-dev/ryotunes/internal/client/state"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/act"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

type menuEntry struct {
	item kit.MenuItem
	run  func()
	keep bool
}

// ItemMenu provides context actions for tracks and browse collections, in
// the Hub's menu language: a head naming what it acts on (its kind in small
// mono capitals over its title, a vein rule under them), then rows that wash
// and lean under the pointer as the Hub's rows do, press deeper with the
// glyph at .86, and a rule before the destructive ones.
type ItemMenu struct {
	rows       []*menuRow
	measure    op.Ops
	add        AddToPlaylistDialog
	edit       EditPlaylistDialog
	target     env.MenuTarget
	done       bool
	subdialog  bool
	overlayTag int
}

// menuRow is one row's retained input and motion.
type menuRow struct {
	click kit.Click
	wash  look.RowWash
}

// Done reports and clears completion of the current menu flow.
func (m *ItemMenu) Done() bool {
	done := m.done
	m.done = false
	return done
}

// OpenAdd starts the full-window Add to playlist dialog without opening a menu.
func (m *ItemMenu) OpenAdd(tracks ...model.Track) {
	m.add.Tracks = append(m.add.Tracks[:0], tracks...)
	m.add.Open = true
	m.edit.Open = false
	m.subdialog = true
	m.done = false
}

// OpenAt opens this menu at a window press point through the overlay host.
func (m *ItemMenu) OpenAt(e *env.Env, target env.MenuTarget, at image.Point) {
	if e == nil || e.Overlays == nil {
		return
	}
	if target.Track != nil {
		track := *target.Track
		target.Track = &track
	}
	if target.Card != nil {
		card := *target.Card
		target.Card = &card
	}
	m.target = target
	m.done = false
	m.subdialog = false
	m.add.Open = false
	m.edit.Open = false
	e.Overlays.OpenAt(&m.overlayTag, at, func(gtx layout.Context) layout.Dimensions {
		return m.Layout(gtx, e, m.target)
	})
}

func menuItem(icon *widget.Icon, label string, run func()) menuEntry {
	return menuEntry{item: kit.MenuItem{Icon: icon, Label: label}, run: run}
}

func dangerItem(icon *widget.Icon, label string, run func()) menuEntry {
	entry := menuItem(icon, label, run)
	entry.item.Danger = true
	return entry
}

func toastCall(e *env.Env, method string, params any, success string) {
	act.Call[protocol.Null](e, method, params, func(_ protocol.Null, err error) {
		if err != nil {
			if e.Toast != nil {
				e.Toast(err.Error(), env.ToastError)
			}
			return
		}
		if success != "" && e.Toast != nil {
			e.Toast(success, env.ToastSuccess)
		}
	})
}

// offerUndo shows message with Undo (the Hub's notice), or as a plain notice
// where the window has no Undo.
func offerUndo(e *env.Env, message string, undo func()) {
	switch {
	case e.Undoable != nil:
		e.Undoable(message, undo)
	case e.Toast != nil:
		e.Toast(message, env.ToastSuccess)
	}
}

// undoableCall runs one daemon call and, once the daemon has done it, offers
// undo with message; a failed call shows its error and offers nothing.
func undoableCall(e *env.Env, method string, params any, message string, undo func()) {
	act.Call[protocol.Null](e, method, params, func(_ protocol.Null, err error) {
		if err != nil {
			if e.Toast != nil {
				e.Toast(err.Error(), env.ToastError)
			}
			return
		}
		offerUndo(e, message, undo)
	})
}

// ToggleLike likes track, or, when liked, takes it out of the liked songs
// and, once the daemon has done it, offers Undo ("Removed from liked
// songs."), which likes it again. Every heart and Like/Unlike row uses it.
func ToggleLike(e *env.Env, track model.Track, liked bool) {
	if !liked {
		act.Rate(e, track, model.RatingLike)
		return
	}
	undoableCall(e, protocol.MethodRate, protocol.RateParams{VideoID: track.VideoID, Rating: model.RatingIndifferent},
		"Removed from liked songs.", func() { act.Rate(e, track, model.RatingLike) })
}

func collectionTracks(e *env.Env, card model.BrowseItem, done func([]model.Track, error)) {
	switch strings.ToLower(card.Kind) {
	case "album", "single", "ep":
		act.Call[model.AlbumPage](e, protocol.MethodGetAlbum, protocol.IDParams{ID: card.ID}, func(page model.AlbumPage, err error) {
			done(page.Items, err)
		})
	case "playlist", "mix":
		act.Call[model.PlaylistPage](e, protocol.MethodGetPlaylist, protocol.PlaylistParams{ID: card.ID}, func(page model.PlaylistPage, err error) {
			done(page.Items, err)
		})
	case "artist", "channel", "user":
		method := protocol.MethodGetArtist
		if model.ProviderOf(card.ID) == model.ProviderSoundCloud {
			method = protocol.MethodGetSCUser
		}
		act.Call[model.ArtistPage](e, method, protocol.IDParams{ID: card.ID}, func(page model.ArtistPage, err error) {
			done(page.TopSongs, err)
		})
	default:
		done(nil, nil)
	}
}

func withCollection(e *env.Env, card model.BrowseItem, run func([]model.Track)) {
	collectionTracks(e, card, func(items []model.Track, err error) {
		if err != nil {
			if e.Toast != nil {
				e.Toast(err.Error(), env.ToastError)
			}
			return
		}
		if len(items) == 0 {
			if e.Toast != nil {
				e.Toast("This collection has no songs", env.ToastInfo)
			}
			return
		}
		run(items)
	})
}

func removeQueue(e *env.Env, index int) {
	toastCall(e, protocol.MethodRemoveFromQueue, protocol.IndexParams{Index: index}, "Removed from queue")
}

func trackEntries(m *ItemMenu, e *env.Env, target env.MenuTarget) []menuEntry {
	track := *target.Track
	provider := model.ProviderOf(track.VideoID)
	entries := []menuEntry{
		menuItem(look.Icon.Next, "Play next", func() { act.PlayNext(e, []model.Track{track}, target.Context) }),
		menuItem(look.Icon.Queue, "Add to queue", func() { act.AddToQueue(e, []model.Track{track}, target.Context) }),
	}
	if provider != model.ProviderLocal && provider != model.ProviderRadio {
		entries = append(entries, menuItem(look.Icon.Radio, "Start radio", func() {
			act.StartRadio(e, "song", track.VideoID, track.Title)
		}))
	}
	add := menuItem(look.Icon.PlaylistAdd, "Add to playlist", func() {
		m.add.Tracks = []model.Track{track}
		m.add.Open = true
		m.subdialog = true
	})
	add.keep = true
	entries = append(entries, add)

	artistID, artistName := "", track.Artists
	if track.ArtistID != nil {
		artistID = *track.ArtistID
	}
	for _, artist := range track.ArtistRuns {
		if artistID == "" && artist.ID != nil {
			artistID, artistName = *artist.ID, artist.Text
			break
		}
	}
	if artistID != "" {
		entries = append(entries, menuItem(look.Icon.Person, "Go to artist", func() { act.OpenArtist(e, artistID, artistName) }))
	}
	if track.AlbumID != nil && *track.AlbumID != "" {
		entries = append(entries, menuItem(look.Icon.Album, "Go to album", func() {
			act.OpenAlbum(e, *track.AlbumID, textValue(track.Album))
		}))
	}
	if provider != model.ProviderLocal && provider != model.ProviderRadio {
		entries = append(entries, menuItem(look.Icon.Download, "Download", func() { act.Download(e, track) }))
	}
	if provider == model.ProviderYouTube || provider == model.ProviderSpotify || provider == model.ProviderSoundCloud || provider == model.ProviderTidal {
		entries = append(entries, menuItem(look.Icon.Share, "Copy link", func() { act.CopyLink(e, track) }))
	}
	if target.Context == "queue" {
		entries = append(entries, dangerItem(look.Icon.Remove, "Remove from queue", func() { removeQueue(e, target.QueueIndex) }))
	} else if target.Context != "" && target.Context != "search" && target.Context != "album" && target.Context != "library" {
		entries = append(entries, dangerItem(look.Icon.Remove, "Remove from playlist", func() {
			act.RemoveFromPlaylist(e, target.Context, track)
		}))
	}
	if trackRateable(track) {
		if trackLiked(track) {
			entries = append(entries, menuItem(look.Icon.Heart, "Unlike", func() { ToggleLike(e, track, true) }))
		} else {
			entries = append(entries, menuItem(look.Icon.HeartOutline, "Like", func() { ToggleLike(e, track, false) }))
		}
	}
	return entries
}

func personalSaved(e *env.Env, card model.BrowseItem) bool {
	if e.Store == nil {
		return false
	}
	for _, saved := range e.Store.Snapshot().Personal.Saved {
		if saved.Kind == card.Kind && saved.ID == card.ID {
			return true
		}
	}
	return false
}

// SetPersonalSaved saves card in or removes it from the personal library and
// returns where it stood (-1 when nothing was removed).
func SetPersonalSaved(e *env.Env, card model.BrowseItem, save bool) int {
	removed := -1
	if e.Store == nil {
		return removed
	}
	e.Store.UpdatePersonal(func(personal *state.Personal) bool {
		for index, item := range personal.Saved {
			if item.Kind != card.Kind || item.ID != card.ID {
				continue
			}
			if save {
				return false
			}
			personal.Saved = append(personal.Saved[:index], personal.Saved[index+1:]...)
			removed = index
			return true
		}
		if !save {
			return false
		}
		personal.Saved = append(personal.Saved, card)
		return true
	})
	return removed
}

// RestorePersonalSaved puts card back in the personal library where it
// stood, unless it is there again already.
func RestorePersonalSaved(e *env.Env, card model.BrowseItem, at int) {
	if e.Store == nil {
		return
	}
	e.Store.UpdatePersonal(func(personal *state.Personal) bool {
		for _, item := range personal.Saved {
			if item.Kind == card.Kind && item.ID == card.ID {
				return false
			}
		}
		at = min(max(at, 0), len(personal.Saved))
		personal.Saved = append(personal.Saved, model.BrowseItem{})
		copy(personal.Saved[at+1:], personal.Saved[at:])
		personal.Saved[at] = card
		return true
	})
}

// LibraryRemoved is the notice for a collection taken out of the library.
func LibraryRemoved(card model.BrowseItem) string {
	if title := strings.TrimSpace(card.Title); title != "" {
		return "Removed " + title + " from your library."
	}
	return "Removed from your library."
}

func collectionLink(card model.BrowseItem) string {
	provider := model.ProviderOf(card.ID)
	kind := strings.ToLower(card.Kind)
	switch provider {
	case model.ProviderSpotify:
		id := card.ID
		for _, prefix := range []string{"spotify:album:", "spotify:playlist:", "spotify:artist:"} {
			id = strings.TrimPrefix(id, prefix)
		}
		return "https://open.spotify.com/" + kind + "/" + id
	case model.ProviderTidal:
		id := card.ID
		for _, prefix := range []string{"tidal:album:", "tidal:playlist:", "tidal:artist:"} {
			id = strings.TrimPrefix(id, prefix)
		}
		return "https://tidal.com/browse/" + kind + "/" + id
	case model.ProviderSoundCloud:
		if strings.HasPrefix(card.ID, "sc:system:") {
			return "https://soundcloud.com/" + strings.TrimPrefix(card.ID, "sc:system:")
		}
	case model.ProviderYouTube:
		if kind == "playlist" || kind == "mix" {
			return "https://music.youtube.com/playlist?list=" + strings.TrimPrefix(card.ID, "VL")
		}
		return "https://music.youtube.com/browse/" + card.ID
	}
	return ""
}

func collectionEntries(e *env.Env, card model.BrowseItem) []menuEntry {
	kind := strings.ToLower(card.Kind)
	provider := model.ProviderOf(card.ID)
	entries := []menuEntry{
		menuItem(look.Icon.Play, "Play", func() {
			withCollection(e, card, func(items []model.Track) { act.PlayCollection(e, items, 0, card.ID, card.Title, false, nil) })
		}),
		menuItem(look.Icon.OpenInNew, "Open", func() { act.OpenCard(e, card) }),
		menuItem(look.Icon.Shuffle, "Shuffle", func() {
			withCollection(e, card, func(items []model.Track) { act.PlayCollection(e, items, 0, card.ID, card.Title, true, nil) })
		}),
		menuItem(look.Icon.Queue, "Add to queue", func() {
			withCollection(e, card, func(items []model.Track) { act.AddToQueue(e, items, card.Title) })
		}),
	}
	if provider != model.ProviderLocal && provider != model.ProviderRadio {
		entries = append(entries, menuItem(look.Icon.Radio, "Start radio", func() { act.StartRadio(e, kind, card.ID, card.Title) }))
	}
	if (kind == "album" || kind == "playlist" || kind == "mix") && provider != model.ProviderLocal && provider != model.ProviderRadio {
		label := "Download album"
		if kind != "album" {
			label = "Download playlist"
		}
		entries = append(entries, menuItem(look.Icon.Download, label, func() {
			withCollection(e, card, func(items []model.Track) { act.DownloadCollection(e, card.Title, items) })
		}))
	}
	saved := personalSaved(e, card)
	label, icon := "Save to library", look.Icon.HeartOutline
	if saved {
		label, icon = "Remove from library", look.Icon.Heart
	}
	entries = append(entries, menuItem(icon, label, func() {
		if kind == "album" {
			if !saved {
				toastCall(e, protocol.MethodSetAlbumSaved, protocol.SetAlbumSavedParams{PlaylistID: card.ID, Saved: true}, "Saved to your library.")
				return
			}
			undoableCall(e, protocol.MethodSetAlbumSaved, protocol.SetAlbumSavedParams{PlaylistID: card.ID, Saved: false}, LibraryRemoved(card), func() {
				toastCall(e, protocol.MethodSetAlbumSaved, protocol.SetAlbumSavedParams{PlaylistID: card.ID, Saved: true}, "")
			})
			return
		}
		if at := SetPersonalSaved(e, card, !saved); at >= 0 {
			offerUndo(e, LibraryRemoved(card), func() { RestorePersonalSaved(e, card, at) })
		}
	}))
	if link := collectionLink(card); link != "" {
		entries = append(entries, menuItem(look.Icon.Share, "Copy link", func() { act.CopyText(e, link, "Link copied") }))
	}
	return entries
}

func targetEntries(m *ItemMenu, e *env.Env, target env.MenuTarget) []menuEntry {
	if target.Track != nil {
		return trackEntries(m, e, target)
	}
	if target.Card != nil {
		if browseIsTrack(*target.Card) {
			track := act.TrackFromCard(*target.Card)
			target.Track = &track
			return trackEntries(m, e, target)
		}
		return collectionEntries(e, *target.Card)
	}
	if target.Playlist == nil {
		return nil
	}
	id, title := target.Context, playlistTitle(target.Playlist)
	card := model.BrowseItem{Kind: "playlist", ID: id, Title: title, Thumbnail: target.Playlist.Thumbnail}
	entries := collectionEntries(e, card)
	if !target.Playlist.Owned {
		return entries
	}
	edit := menuItem(nil, "Edit playlist", func() {
		m.edit.Page = target.Playlist
		m.edit.Open = true
		m.subdialog = true
	})
	edit.keep = true
	entries = append(entries, edit,
		menuItem(look.Icon.Album, "Change cover", func() {
			act.Call[protocol.SetPlaylistCoverResult](e, protocol.MethodSetPlaylistCover, protocol.SetPlaylistCoverParams{PlaylistID: id, Pick: true}, func(result protocol.SetPlaylistCoverResult, err error) {
				if err != nil {
					if e.Toast != nil {
						e.Toast(err.Error(), env.ToastError)
					}
					return
				}
				target.Playlist.Cover = result.Cover
				target.Playlist.Thumbnail = result.Thumbnail
				if e.Toast != nil {
					e.Toast("Playlist cover changed", env.ToastSuccess)
				}
			})
		}),
		dangerItem(look.Icon.Remove, "Delete playlist", func() {
			if e.Confirm == nil {
				return
			}
			confirmTitle, body, proof := deleteConfirmCopy(id, title)
			e.Confirm(confirmTitle, body, "Delete", proof, func() {
				act.Call[protocol.Null](e, protocol.MethodDeletePlaylist, protocol.PlaylistIDParams{PlaylistID: id}, func(_ protocol.Null, err error) {
					if err != nil {
						if e.Toast != nil {
							e.Toast(err.Error(), env.ToastError)
						}
						return
					}
					if e.Nav != nil {
						e.Nav.Back()
					}
				})
			})
		}),
	)
	return entries
}

// Item menu geometry, in dp: the overlay host keeps the menu inset (16 at
// the sides, 6 above and below) on its plate; a row's wash reaches to 6 from
// the plate's edges, as the Hub's list washes stand inside their slab.
const (
	menuWashReach  unit.Dp = tokens.SizeMenuInset - 6
	menuHeadGap    unit.Dp = 2
	menuRuleGap    unit.Dp = 8
	menuHeadInk            = 1.2
	menuMaxEntries         = 32
)

// menuHead is what the menu acts on: its kind, title and a quiet line.
func menuHead(target env.MenuTarget) (kind, title, detail string) {
	switch {
	case target.Track != nil:
		kind = "Song"
		if target.Context == "queue" {
			kind = "In the queue"
		}
		return kind, target.Track.Title, target.Track.Artists
	case target.Card != nil:
		return browseKind(*target.Card), target.Card.Title, textValue(target.Card.Subtitle)
	case target.Playlist != nil:
		return "Playlist", playlistTitle(target.Playlist), ""
	}
	return "", "", ""
}

// row returns the retained row at index.
func (m *ItemMenu) row(index int) *menuRow {
	for len(m.rows) <= index {
		m.rows = append(m.rows, new(menuRow))
	}
	return m.rows[index]
}

// width is the menu's width: its widest row or head, 220 to 360.
func (m *ItemMenu) width(gtx layout.Context, e *env.Env, entries []menuEntry, title string) int {
	th := e.Th
	maximum := min(gtx.Dp(tokens.SizeMenuWidthMax), gtx.Constraints.Max.X)
	widest := min(gtx.Dp(tokens.SizeMenuWidthMin), maximum)
	measure := gtx
	measure.Ops = &m.measure
	measure.Constraints = layout.Constraints{Max: image.Pt(maximum, gtx.Constraints.Max.Y)}
	m.measure.Reset()
	if title != "" {
		widest = max(widest, min(maximum, kit.Label{Style: th.T.Label, MaxLines: 1}.Layout(measure, th.Theme, title).Size.X))
	}
	for _, entry := range entries {
		fixed := 0
		if entry.item.Icon != nil {
			fixed = gtx.Dp(tokens.SizeMenuIcon + tokens.SizeMenuIconGap)
		}
		m.measure.Reset()
		size := kit.Label{Style: th.T.Label, MaxLines: 1}.Layout(measure, th.Theme, entry.item.Label).Size
		widest = max(widest, min(maximum, fixed+size.X))
	}
	return widest
}

// Layout draws the current menu. Full-window subdialogs are drawn by LayoutDialogs.
func (m *ItemMenu) Layout(gtx layout.Context, e *env.Env, target env.MenuTarget) layout.Dimensions {
	if m.subdialog {
		return layout.Dimensions{}
	}
	th := e.Th
	entries := targetEntries(m, e, target)
	if len(entries) > menuMaxEntries {
		entries = entries[:menuMaxEntries]
	}
	for index := range entries {
		if m.row(index).click.Clicked(gtx) {
			m.choose(e, entries[index])
			return layout.Dimensions{}
		}
	}
	m.keys(gtx, len(entries))
	kind, title, detail := menuHead(target)
	width := m.width(gtx, e, entries, title)
	gtx.Constraints = layout.Constraints{Max: image.Pt(width, gtx.Constraints.Max.Y)}
	reach := gtx.Dp(menuWashReach)
	y := 0
	if title != "" {
		y = m.head(gtx, e, kind, title, detail)
		y += gtx.Dp(menuRuleGap)
		hair := max(1, gtx.Dp(1))
		rule := gtx.Dp(tokens.SizeMenuInset)
		paint.FillShape(gtx.Ops, th.R.Vein, clip.Rect(image.Rect(-rule, y, width+rule, y+hair)).Op())
		y += hair + gtx.Dp(tokens.SizeMenuPaddingVertical)
	}
	ruled := false
	for index, entry := range entries {
		if entry.item.Danger && !ruled && index > 0 {
			ruled = true
			y += gtx.Dp(tokens.SizeMenuPaddingVertical)
			hair := max(1, gtx.Dp(1))
			paint.FillShape(gtx.Ops, th.R.Vein, clip.Rect(image.Rect(0, y, width, y+hair)).Op())
			y += hair + gtx.Dp(tokens.SizeMenuPaddingVertical)
		}
		offset := op.Offset(image.Pt(0, y)).Push(gtx.Ops)
		y += m.rowLayout(gtx, e, m.row(index), entry, width, reach).Y
		offset.Pop()
	}
	return layout.Dimensions{Size: image.Pt(width, y)}
}

// head draws the menu's head and returns its height.
func (m *ItemMenu) head(gtx layout.Context, e *env.Env, kind, title, detail string) int {
	th := e.Th
	y := 0
	if kind != "" {
		y += kit.LayoutSpacedCaps(gtx, th.Theme, th.T.MonoSm, th.R.Fg3, menuHeadInk, kind).Size.Y + gtx.Dp(menuHeadGap)
	}
	offset := op.Offset(image.Pt(0, y)).Push(gtx.Ops)
	y += kit.Label{Style: th.T.Label, Color: th.R.Fg, MaxLines: 1}.Layout(gtx, th.Theme, title).Size.Y
	offset.Pop()
	if detail != "" {
		offset := op.Offset(image.Pt(0, y)).Push(gtx.Ops)
		y += kit.Label{Style: th.T.Caption, Color: th.R.Fg3, MaxLines: 1}.Layout(gtx, th.Theme, detail).Size.Y
		offset.Pop()
	}
	return y
}

// rowLayout draws one row: its wash reaching reach past the menu's sides,
// the row leaning with it, glyph and label in fg (jasper when destructive,
// fg3 when disabled), and the focus ring while focused from the keys.
func (m *ItemMenu) rowLayout(gtx layout.Context, e *env.Env, row *menuRow, entry menuEntry, width, reach int) image.Point {
	th := e.Th
	height := gtx.Dp(tokens.SizeMenuItem)
	size := image.Pt(width, height)
	disabled := entry.item.Disabled
	hovered := !disabled && (row.click.Hovered() || entry.item.Preview == kit.StateHover)
	pressed := !disabled && (row.click.Pressed() || entry.item.Preview == kit.StatePressed)
	row.wash.Update(gtx, th, hovered, pressed)
	bounds := image.Rect(-reach, 0, width+reach, height)
	row.wash.Paint(gtx, th, bounds, gtx.Dp(th.Cut.Small))
	ink := th.R.Fg
	switch {
	case disabled:
		ink = th.R.Fg3
	case entry.item.Danger:
		ink = th.R.Jasper
	}
	rowGTX := gtx
	rowGTX.Constraints = layout.Exact(size)
	if disabled {
		rowGTX = rowGTX.Disabled()
	}
	row.click.Layout(rowGTX, func(gtx layout.Context) layout.Dimensions {
		lean := op.Offset(image.Pt(row.wash.Lean(gtx), 0)).Push(gtx.Ops)
		x := 0
		if entry.item.Icon != nil {
			icon := gtx.Dp(tokens.SizeMenuIcon)
			offset := op.Offset(image.Pt(0, (height-icon)/2)).Push(gtx.Ops)
			drawScaledIcon(gtx, entry.item.Icon, ink, tokens.SizeMenuIcon, row.wash.GlyphScale())
			offset.Pop()
			x = icon + gtx.Dp(tokens.SizeMenuIconGap)
		}
		label := gtx
		label.Constraints = layout.Constraints{Max: image.Pt(max(0, width-x), height)}
		record := op.Record(gtx.Ops)
		text := kit.Label{Style: th.T.Label, Color: ink, MaxLines: 1}.Layout(label, th.Theme, entry.item.Label)
		call := record.Stop()
		offset := op.Offset(image.Pt(x, (height-text.Size.Y)/2)).Push(gtx.Ops)
		call.Add(gtx.Ops)
		offset.Pop()
		lean.Pop()
		return layout.Dimensions{Size: size}
	})
	if gtx.Focused(&row.click.Clickable) {
		kit.FocusRing(gtx, th.Theme, bounds, gtx.Dp(th.Cut.Small))
	}
	return size
}

// keys moves focus through the rows: Up and Down wrap, Home and End go to
// the ends; Return, Enter and Space choose through the row's click.
func (m *ItemMenu) keys(gtx layout.Context, count int) {
	for index := range count {
		tag := &m.row(index).click.Clickable
		for {
			value, ok := gtx.Event(
				key.Filter{Focus: tag, Name: key.NameUpArrow},
				key.Filter{Focus: tag, Name: key.NameDownArrow},
				key.Filter{Focus: tag, Name: key.NameHome},
				key.Filter{Focus: tag, Name: key.NameEnd},
			)
			if !ok {
				break
			}
			pressed, ok := value.(key.Event)
			if !ok || pressed.State != key.Press {
				continue
			}
			next := index
			switch pressed.Name {
			case key.NameUpArrow:
				next = (index - 1 + count) % count
			case key.NameDownArrow:
				next = (index + 1) % count
			case key.NameHome:
				next = 0
			case key.NameEnd:
				next = count - 1
			}
			gtx.Execute(key.FocusCmd{Tag: &m.row(next).click.Clickable})
		}
	}
}

// choose runs entry and closes the menu; a row that opens a dialog keeps
// the flow going until the dialog ends.
func (m *ItemMenu) choose(e *env.Env, entry menuEntry) {
	if entry.run != nil {
		entry.run()
	}
	if !entry.keep {
		m.done = true
	}
	if e.Overlays != nil {
		e.Overlays.Close(&m.overlayTag)
	}
}

// LayoutDialogs draws menu-owned dialogs in full-window coordinates.
func (m *ItemMenu) LayoutDialogs(gtx layout.Context, e *env.Env) layout.Dimensions {
	if !m.subdialog {
		return layout.Dimensions{}
	}
	var dimensions layout.Dimensions
	switch {
	case m.add.Open:
		dimensions = m.add.Layout(gtx, e)
	case m.edit.Open:
		dimensions = m.edit.Layout(gtx, e)
	}
	if !m.add.Open && !m.edit.Open {
		m.subdialog = false
		m.done = true
	}
	return dimensions
}
