package widgets

import (
	"image"
	"strings"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/act"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

// Dialog widths, in dp: the Hub's dialog is 520; an editor takes 640.
const (
	dialogWidth       = 520
	dialogEditorWidth = 640
)

// AddToPlaylistDialog adds one or more tracks to an existing or new playlist.
// The playlists stand as the Hub's ruled list: rows that wash and lean under
// the pointer and press deeper, a mono ordinal and the playlist's name, a
// vein rule between; the wait for them is the Hub's (nothing for a second,
// then the flakes with a word). Create is the dialog's one gold.
type AddToPlaylistDialog struct {
	Open   bool
	Tracks []model.Track

	dialog     kit.Dialog
	newTitle   kit.TextField
	create     kit.Plaque
	cancel     kit.Plaque
	scroll     look.Scroller
	rows       map[string]*playlistRow
	playlists  []model.BrowseItem
	loading    bool
	loaded     bool
	err        string
	wasOpen    bool
	createOnly bool
}

// playlistRow is one playlist's retained row in the dialog.
type playlistRow struct {
	click   kit.Click
	wash    look.RowWash
	ordinal string
	at      [2]int
}

func (d *AddToPlaylistDialog) load(e *env.Env) {
	if d.loading || d.loaded {
		return
	}
	d.loading = true
	act.Call[[]model.BrowseItem](e, protocol.MethodGetLibrary, protocol.Null{}, func(items []model.BrowseItem, err error) {
		d.loading = false
		d.loaded = true
		if err != nil {
			d.err = err.Error()
			return
		}
		d.playlists = d.playlists[:0]
		for _, item := range items {
			if model.IsRyotunesOwn(item.ID) && !strings.HasPrefix(item.ID, model.LocalPlaylistPrefix) {
				continue
			}
			if strings.EqualFold(item.Kind, "playlist") || strings.HasPrefix(item.ID, model.LocalPlaylistPrefix) {
				d.playlists = append(d.playlists, item)
			}
		}
	})
}

func addTracksToPlaylist(e *env.Env, playlistID string, tracks []model.Track, index int, done func(bool)) {
	if index >= len(tracks) {
		if e.Toast != nil {
			e.Toast("Added to playlist", env.ToastSuccess)
		}
		done(true)
		return
	}
	finish := func(_ protocol.Null, err error) {
		if err != nil {
			if e.Toast != nil {
				e.Toast(err.Error(), env.ToastError)
			}
			done(false)
			return
		}
		addTracksToPlaylist(e, playlistID, tracks, index+1, done)
	}
	if strings.HasPrefix(playlistID, model.LocalPlaylistPrefix) {
		act.Call[protocol.Null](e, protocol.MethodAddToLocalPlaylist, protocol.AddToLocalPlaylistParams{PlaylistID: playlistID, Item: tracks[index]}, finish)
		return
	}
	act.Call[protocol.Null](e, protocol.MethodAddToPlaylist, protocol.PlaylistTrackParams{PlaylistID: playlistID, VideoID: tracks[index].VideoID}, finish)
}

func (d *AddToPlaylistDialog) add(e *env.Env, playlistID string) {
	if playlistID == "" || len(d.Tracks) == 0 {
		return
	}
	tracks := append([]model.Track(nil), d.Tracks...)
	addTracksToPlaylist(e, playlistID, tracks, 0, func(ok bool) {
		if ok {
			d.Open = false
			d.dialog.Open = false
		}
	})
}

func (d *AddToPlaylistDialog) createPlaylist(e *env.Env) {
	title := strings.TrimSpace(d.newTitle.Text())
	if title == "" {
		title = "New playlist"
	}
	tracks := append([]model.Track(nil), d.Tracks...)
	act.Call[protocol.PlaylistIDResult](e, protocol.MethodCreatePlaylist, protocol.CreatePlaylistParams{Title: title}, func(id protocol.PlaylistIDResult, err error) {
		if err != nil {
			if e.Toast != nil {
				e.Toast(err.Error(), env.ToastError)
			}
			return
		}
		if len(tracks) > 0 {
			d.add(e, string(id))
			return
		}
		if e.Toast != nil {
			e.Toast("Playlist created", env.ToastSuccess)
		}
		d.Open = false
		d.dialog.Open = false
		if e.Nav != nil {
			e.Nav.Push(env.Route{Kind: env.RoutePlaylist, ID: string(id), Title: title})
		}
	})
}

func (d *AddToPlaylistDialog) playlistRow(id string) *playlistRow {
	if d.rows == nil {
		d.rows = make(map[string]*playlistRow)
	}
	row := d.rows[id]
	if row == nil {
		row = new(playlistRow)
		d.rows[id] = row
	}
	return row
}

// playlistRowLayout draws one playlist row: the wash with its lean and
// press, the ordinal in mono fg3, the playlist glyph and name, and a vein
// rule under it.
func playlistRowLayout(gtx layout.Context, e *env.Env, row *playlistRow, index, count int, title string) layout.Dimensions {
	th := e.Th
	digits := ordinalDigits(count)
	if row.ordinal == "" || row.at != [2]int{index, digits} {
		row.at = [2]int{index, digits}
		row.ordinal = padOrdinal(index+1, digits)
	}
	row.wash.Update(gtx, th, row.click.Hovered(), row.click.Pressed())
	height := gtx.Dp(44)
	size := image.Pt(gtx.Constraints.Max.X, height)
	rowGTX := gtx
	rowGTX.Constraints = layout.Exact(size)
	return row.click.Layout(rowGTX, func(gtx layout.Context) layout.Dimensions {
		bounds := image.Rectangle{Max: size}
		hair := max(1, gtx.Dp(1))
		if index+1 < count {
			paint.FillShape(gtx.Ops, th.R.Line(), clip.Rect(image.Rect(0, height-hair, size.X, height)).Op())
		}
		row.wash.Paint(gtx, th, image.Rect(0, hair, size.X, height-hair), gtx.Dp(th.Cut.Small))
		lean := op.Offset(image.Pt(row.wash.Lean(gtx)+gtx.Dp(8), 0)).Push(gtx.Ops)
		x := 0
		place := func(w layout.Widget) {
			cell := gtx
			cell.Constraints = layout.Constraints{Max: image.Pt(max(0, size.X-x-gtx.Dp(16)), height)}
			record := op.Record(gtx.Ops)
			dims := w(cell)
			call := record.Stop()
			offset := op.Offset(image.Pt(x, (height-dims.Size.Y)/2)).Push(gtx.Ops)
			call.Add(gtx.Ops)
			offset.Pop()
			x += dims.Size.X
		}
		place(func(gtx layout.Context) layout.Dimensions {
			return kit.Label{Style: th.T.Mono, Color: th.R.Fg3, MaxLines: 1}.Layout(gtx, th.Theme, row.ordinal)
		})
		x += gtx.Dp(13)
		place(func(gtx layout.Context) layout.Dimensions {
			return drawScaledIcon(gtx, look.Icon.Playlist, th.R.Fg2, 18, row.wash.GlyphScale())
		})
		x += gtx.Dp(12)
		place(func(gtx layout.Context) layout.Dimensions {
			return kit.Label{Style: th.T.Label, Color: th.R.Fg, MaxLines: 1}.Layout(gtx, th.Theme, title)
		})
		lean.Pop()
		if gtx.Focused(&row.click.Clickable) {
			kit.FocusRing(gtx, th.Theme, bounds, gtx.Dp(th.Cut.Small))
		}
		return layout.Dimensions{Size: size}
	})
}

// loadingPlaylists is the dialog's own wait, apart from any page's.
const loadingPlaylists LoadingShape = 100

func (d *AddToPlaylistDialog) body(gtx layout.Context, e *env.Env) layout.Dimensions {
	if d.create.Clicked(gtx) {
		d.createPlaylist(e)
	}
	d.newTitle.Label = "New playlist"
	d.newTitle.Placeholder = "Playlist title"
	d.newTitle.Help = "Create a playlist and add these songs."
	if d.createOnly {
		d.newTitle.Help = "Create an empty playlist."
	}
	d.create.Kind, d.create.Icon, d.create.Label = kit.PlaqueGold, look.Icon.Add, "Create playlist"
	children := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return d.newTitle.Layout(gtx, e.Th.Theme) }),
		layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return d.create.Layout(gtx, e.Th.Theme) }),
	}
	if d.createOnly {
		return layout.Flex{Axis: layout.Vertical, Alignment: layout.Start}.Layout(gtx, children...)
	}
	children = append(children,
		layout.Rigid(layout.Spacer{Height: unit.Dp(21)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = gtx.Constraints.Max.X
			mark := ""
			if d.loaded && len(d.playlists) > 0 {
				mark = kit.Pad2(len(d.playlists))
			}
			return RuledHead(gtx, e, "Your playlists", mark, nil)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
	)
	switch {
	case d.loading:
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			elapsed, clock := waitFor(gtx, e, loadingPlaylists)
			height := gtx.Dp(44)
			if elapsed < waitQuiet {
				return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, height)}
			}
			line := workLine(gtx, e, clock, elapsed, "Loading your playlists")
			return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, max(height, line.Size.Y))}
		}))
	case d.err != "":
		children = append(children, layout.Rigid(kit.Text(e.Th.Theme, e.Th.T.Detail, e.Th.R.Jasper, d.err)))
	case len(d.playlists) == 0:
		children = append(children, layout.Rigid(kit.Text(e.Th.Theme, e.Th.T.Body, e.Th.R.Fg2, "You have no playlists yet. Create one above.")))
	default:
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Max.Y = min(gtx.Constraints.Max.Y, gtx.Dp(280))
			return d.scroll.Layout(gtx, e.Th, len(d.playlists), func(gtx layout.Context, index int) layout.Dimensions {
				playlist := d.playlists[index]
				row := d.playlistRow(playlist.ID)
				if row.click.Clicked(gtx) {
					d.add(e, playlist.ID)
				}
				return playlistRowLayout(gtx, e, row, index, len(d.playlists), playlist.Title)
			})
		}))
	}
	return layout.Flex{Axis: layout.Vertical, Alignment: layout.Start}.Layout(gtx, children...)
}

// Layout draws the Add to playlist dialog and dispatches its actions.
func (d *AddToPlaylistDialog) Layout(gtx layout.Context, e *env.Env) layout.Dimensions {
	if d.Open && !d.wasOpen {
		d.loaded, d.loading, d.err = false, false, ""
		d.playlists = nil
		d.rows = nil
		d.newTitle.SetText("New playlist")
	}
	d.wasOpen = d.Open
	if d.Open && !d.createOnly {
		d.load(e)
	}
	if d.cancel.Clicked(gtx) {
		d.Open = false
	}
	d.cancel.Kind, d.cancel.Label = kit.PlaqueQuiet, "Cancel"
	d.dialog.Open = d.Open
	d.dialog.Width = dialogWidth
	title := "Add to playlist"
	if d.createOnly {
		title = "New playlist"
	}
	dims := d.dialog.Layout(gtx, e.Th.Theme, title,
		func(gtx layout.Context) layout.Dimensions { return d.body(gtx, e) },
		func(gtx layout.Context) layout.Dimensions { return d.cancel.Layout(gtx, e.Th.Theme) },
	)
	if d.dialog.Closed() {
		d.Open = false
	}
	d.Open = d.dialog.Open
	if !d.Open {
		d.wasOpen = false
	}
	return dims
}

// NewPlaylistDialog creates an empty playlist and opens it.
type NewPlaylistDialog struct {
	Open bool
	add  AddToPlaylistDialog
}

// Layout draws the new playlist dialog and dispatches its actions.
func (d *NewPlaylistDialog) Layout(gtx layout.Context, e *env.Env) layout.Dimensions {
	d.add.Open = d.Open
	d.add.Tracks = nil
	d.add.createOnly = true
	dimensions := d.add.Layout(gtx, e)
	d.Open = d.add.Open
	return dimensions
}

// EditPlaylistDialog edits owned playlist metadata and protected deletion.
type EditPlaylistDialog struct {
	Open bool
	Page *model.PlaylistPage

	dialog      kit.Dialog
	title       kit.TextField
	description kit.TextField
	public      kit.Toggle
	save        kit.Plaque
	cover       kit.Plaque
	delete      kit.Plaque
	cancel      kit.Plaque
	initialized *model.PlaylistPage
}

func playlistTitle(page *model.PlaylistPage) string {
	if page != nil && page.Title != nil && strings.TrimSpace(*page.Title) != "" {
		return strings.TrimSpace(*page.Title)
	}
	return "Playlist"
}

func (d *EditPlaylistDialog) initialize() {
	if d.Page == nil || d.initialized == d.Page {
		return
	}
	d.initialized = d.Page
	d.title.SetText(playlistTitle(d.Page))
	d.description.SetText(textValue(d.Page.Description))
	d.public.Value = d.Page.Privacy != nil && strings.EqualFold(*d.Page.Privacy, "PUBLIC")
}

func currentPlaylistID(e *env.Env) string {
	if e.Nav == nil {
		return ""
	}
	return e.Nav.Current().ID
}

func (d *EditPlaylistDialog) savePlaylist(e *env.Env) {
	id := currentPlaylistID(e)
	name := strings.TrimSpace(d.title.Text())
	if name == "" {
		d.title.Error = "Name this playlist"
		return
	}
	d.title.Error = ""
	description := strings.TrimSpace(d.description.Text())
	public := d.public.Value
	act.Call[protocol.Null](e, protocol.MethodEditPlaylistDetails, protocol.EditPlaylistDetailsParams{
		PlaylistID: id, Name: &name, Description: &description, Public: &public,
	}, func(_ protocol.Null, err error) {
		if err != nil {
			if e.Toast != nil {
				e.Toast(err.Error(), env.ToastError)
			}
			return
		}
		if d.Page != nil {
			d.Page.Title = &name
			d.Page.Description = &description
			privacy := "PRIVATE"
			if public {
				privacy = "PUBLIC"
			}
			d.Page.Privacy = &privacy
		}
		if e.Toast != nil {
			e.Toast("Playlist saved", env.ToastSuccess)
		}
		d.Open = false
		d.dialog.Open = false
	})
}

func (d *EditPlaylistDialog) changeCover(e *env.Env) {
	act.Call[protocol.SetPlaylistCoverResult](e, protocol.MethodSetPlaylistCover, protocol.SetPlaylistCoverParams{
		PlaylistID: currentPlaylistID(e), Pick: true,
	}, func(result protocol.SetPlaylistCoverResult, err error) {
		if err != nil {
			if e.Toast != nil {
				e.Toast(err.Error(), env.ToastError)
			}
			return
		}
		if d.Page != nil {
			d.Page.Cover = result.Cover
			d.Page.Thumbnail = result.Thumbnail
		}
		if e.Toast != nil {
			e.Toast("Playlist cover changed", env.ToastSuccess)
		}
	})
}

func deleteConfirmCopy(id, name string) (title, body, proof string) {
	if !strings.HasPrefix(id, model.LocalPlaylistPrefix) {
		return "Delete this playlist?", "This removes it from your account. It can't be undone.", ""
	}
	return "Delete playlist", "This deletes " + name + " and removes it from your library.", name
}

func (d *EditPlaylistDialog) deletePlaylist(e *env.Env) {
	if e.Confirm == nil {
		return
	}
	name := playlistTitle(d.Page)
	id := currentPlaylistID(e)
	title, body, proof := deleteConfirmCopy(id, name)
	e.Confirm(title, body, "Delete", proof, func() {
		act.Call[protocol.Null](e, protocol.MethodDeletePlaylist, protocol.PlaylistIDParams{PlaylistID: id}, func(_ protocol.Null, err error) {
			if err != nil {
				if e.Toast != nil {
					e.Toast(err.Error(), env.ToastError)
				}
				return
			}
			if e.Toast != nil {
				e.Toast(name+" deleted", env.ToastSuccess)
			}
			d.Open = false
			d.dialog.Open = false
			if e.Nav != nil {
				e.Nav.Back()
			}
		})
	})
}

func (d *EditPlaylistDialog) body(gtx layout.Context, e *env.Env) layout.Dimensions {
	d.title.Label, d.title.Placeholder = "Title", "Playlist title"
	d.description.Label, d.description.Placeholder = "Description", "Describe this playlist"
	d.public.Label = "Public playlist"
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return d.title.Layout(gtx, e.Th.Theme) }),
		layout.Rigid(layout.Spacer{Height: unit.Dp(21)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return d.description.Layout(gtx, e.Th.Theme) }),
		layout.Rigid(layout.Spacer{Height: unit.Dp(21)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return d.public.Layout(gtx, e.Th.Theme) }),
	)
}

// Layout draws the owned-playlist editor and its actions.
func (d *EditPlaylistDialog) Layout(gtx layout.Context, e *env.Env) layout.Dimensions {
	if d.Open {
		d.initialize()
	}
	if d.save.Clicked(gtx) {
		d.savePlaylist(e)
	}
	if d.cover.Clicked(gtx) {
		d.changeCover(e)
	}
	if d.delete.Clicked(gtx) {
		d.deletePlaylist(e)
	}
	if d.cancel.Clicked(gtx) {
		d.Open = false
	}
	d.delete.Kind, d.delete.Icon, d.delete.Label = kit.PlaqueDangerLine, look.Icon.Remove, "Delete"
	d.cover.Kind, d.cover.Icon, d.cover.Label = kit.PlaqueQuiet, look.Icon.Album, "Change cover"
	d.cancel.Kind, d.cancel.Label = kit.PlaqueQuiet, "Cancel"
	d.save.Kind, d.save.Label = kit.PlaqueGold, "Save"
	d.dialog.Open = d.Open
	d.dialog.Width = dialogEditorWidth
	dims := d.dialog.Layout(gtx, e.Th.Theme, "Edit playlist",
		func(gtx layout.Context) layout.Dimensions { return d.body(gtx, e) },
		func(gtx layout.Context) layout.Dimensions { return d.delete.Layout(gtx, e.Th.Theme) },
		func(gtx layout.Context) layout.Dimensions { return d.cover.Layout(gtx, e.Th.Theme) },
		func(gtx layout.Context) layout.Dimensions { return d.cancel.Layout(gtx, e.Th.Theme) },
		func(gtx layout.Context) layout.Dimensions { return d.save.Layout(gtx, e.Th.Theme) },
	)
	if d.dialog.Closed() {
		d.Open = false
	}
	d.Open = d.dialog.Open
	if !d.Open {
		d.initialized = nil
	}
	return dims
}
