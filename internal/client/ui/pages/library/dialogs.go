package library

import (
	"strings"

	"gioui.org/layout"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/act"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

type newPlaylistDialog struct {
	dialog kit.Dialog
	name   kit.TextField
	cancel kit.Plaque
	create kit.Plaque
	err    string
	busy   bool
}

func (d *newPlaylistDialog) Open() {
	d.dialog.Open = true
	d.name.SetText("")
	d.err = ""
}

func (d *newPlaylistDialog) Layout(gtx layout.Context, e *env.Env, p *Page) layout.Dimensions {
	if d.cancel.Clicked(gtx) {
		d.dialog.Open = false
	}
	if d.create.Clicked(gtx) && !d.busy {
		title := strings.TrimSpace(d.name.Text())
		if title == "" {
			d.err = "Name the playlist first."
		} else {
			d.busy = true
			d.err = ""
			act.Call[protocol.PlaylistIDResult](e, protocol.MethodCreatePlaylist, protocol.CreatePlaylistParams{Title: title}, func(id protocol.PlaylistIDResult, err error) {
				d.busy = false
				if err != nil {
					d.err = err.Error()
					return
				}
				d.dialog.Open = false
				e.Toast("Playlist created", env.ToastSuccess)
				e.Nav.Push(env.Route{Kind: env.RoutePlaylist, ID: string(id), Title: title})
				p.Refresh(e)
			})
		}
	}
	d.name.Label, d.name.Placeholder, d.name.Error = "Playlist name", "New playlist", d.err
	d.cancel.Kind, d.cancel.Label = kit.PlaqueQuiet, "Cancel"
	d.create.Kind, d.create.Icon, d.create.Label, d.create.Busy = kit.PlaqueLine, look.Icon.Add, "Create", d.busy
	d.dialog.Width = 520
	return d.dialog.Layout(gtx, e.Th.Theme, "New playlist",
		func(gtx layout.Context) layout.Dimensions { return d.name.Layout(gtx, e.Th.Theme) },
		func(gtx layout.Context) layout.Dimensions { return d.cancel.Layout(gtx, e.Th.Theme) },
		func(gtx layout.Context) layout.Dimensions { return d.create.Layout(gtx, e.Th.Theme) },
	)
}

type importPlaylistDialog struct {
	dialog       kit.Dialog
	path         kit.TextField
	cancel       kit.Plaque
	importButton kit.Plaque
	err          string
	busy         bool
}

func (d *importPlaylistDialog) Open() {
	d.dialog.Open = true
	d.path.SetText("")
	d.err = ""
}

func (d *importPlaylistDialog) Layout(gtx layout.Context, e *env.Env, p *Page) layout.Dimensions {
	if d.cancel.Clicked(gtx) {
		d.dialog.Open = false
	}
	if d.importButton.Clicked(gtx) && !d.busy {
		path := strings.TrimSpace(d.path.Text())
		if path == "" {
			d.err = "Enter the playlist file path."
		} else {
			d.busy = true
			d.err = ""
			act.Call[protocol.ImportPlaylistFileResult](e, protocol.MethodImportPlaylistFile, protocol.PathParams{Path: path}, func(transfer protocol.ImportPlaylistFileResult, err error) {
				if err != nil {
					d.busy = false
					d.err = err.Error()
					return
				}
				if len(transfer.Items) == 0 {
					d.busy = false
					d.err = "That playlist file has no tracks."
					return
				}
				title := strings.TrimSpace(transfer.Title)
				if title == "" {
					title = "Imported playlist"
				}
				d.createImported(e, p, title, transfer.Items)
			})
		}
	}
	d.path.Label = "Playlist file"
	d.path.Placeholder = "/home/you/Music/playlist.json"
	d.path.Help = "Ryotunes playlist files end in .json."
	d.path.Error = d.err
	d.path.Mono = true
	d.cancel.Kind, d.cancel.Label = kit.PlaqueQuiet, "Cancel"
	d.importButton.Kind, d.importButton.Icon, d.importButton.Label, d.importButton.Busy = kit.PlaqueLine, look.Icon.OpenInNew, "Import", d.busy
	d.dialog.Width = 620
	return d.dialog.Layout(gtx, e.Th.Theme, "Import playlist",
		func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(kit.Text(e.Th.Theme, e.Th.T.Body, e.Th.R.Fg2, "Choose a Ryotunes playlist file; its tracks land in a new library playlist.")),
				layout.Rigid(layout.Spacer{Height: unit.Dp(21)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { return d.path.Layout(gtx, e.Th.Theme) }),
			)
		},
		func(gtx layout.Context) layout.Dimensions { return d.cancel.Layout(gtx, e.Th.Theme) },
		func(gtx layout.Context) layout.Dimensions { return d.importButton.Layout(gtx, e.Th.Theme) },
	)
}

func (d *importPlaylistDialog) createImported(e *env.Env, p *Page, title string, items []model.Track) {
	act.Call[protocol.PlaylistIDResult](e, protocol.MethodCreatePlaylist, protocol.CreatePlaylistParams{Title: title}, func(id protocol.PlaylistIDResult, err error) {
		if err != nil {
			d.busy = false
			d.err = err.Error()
			return
		}
		d.addImported(e, p, string(id), title, items, 0)
	})
}

func (d *importPlaylistDialog) addImported(e *env.Env, p *Page, playlistID, title string, items []model.Track, index int) {
	if index >= len(items) {
		d.busy = false
		d.dialog.Open = false
		e.Toast("Imported "+pluralSongs(len(items)), env.ToastSuccess)
		e.Nav.Push(env.Route{Kind: env.RoutePlaylist, ID: playlistID, Title: title})
		p.Refresh(e)
		return
	}
	done := func(err error) {
		if err != nil {
			d.busy = false
			d.err = err.Error()
			return
		}
		d.addImported(e, p, playlistID, title, items, index+1)
	}
	if isDevicePlaylist(playlistID) {
		act.Call[bool](e, protocol.MethodAddToLocalPlaylist, protocol.AddToLocalPlaylistParams{PlaylistID: playlistID, Item: items[index]}, func(_ bool, err error) { done(err) })
		return
	}
	act.Call[*bool](e, protocol.MethodAddToPlaylist, protocol.PlaylistTrackParams{PlaylistID: playlistID, VideoID: items[index].VideoID}, func(_ *bool, err error) { done(err) })
}

type folderDialog struct {
	dialog kit.Dialog
	path   kit.TextField
	cancel kit.Plaque
	add    kit.Plaque
	err    string
	busy   bool
}

func (d *folderDialog) Open() {
	d.dialog.Open = true
	d.path.SetText("")
	d.err = ""
}

func (d *folderDialog) Layout(gtx layout.Context, e *env.Env, p *Page) layout.Dimensions {
	if d.cancel.Clicked(gtx) {
		d.dialog.Open = false
	}
	if d.add.Clicked(gtx) && !d.busy {
		path := strings.TrimSpace(d.path.Text())
		if path == "" {
			d.err = "Enter the folder path."
		} else {
			d.busy = true
			d.err = ""
			act.Call[model.LocalLibrary](e, protocol.MethodAddLocalFolder, protocol.PathParams{Path: path}, func(local model.LocalLibrary, err error) {
				d.busy = false
				if err != nil {
					d.err = err.Error()
					return
				}
				p.local = local
				p.localLoaded = true
				p.syncFolderButtons()
				p.invalidateFilter()
				d.dialog.Open = false
			})
		}
	}
	d.path.Label = "Music folder"
	d.path.Placeholder = "/home/you/Music"
	d.path.Help = "Ryotunes watches this folder for music and scans it on demand."
	d.path.Error = d.err
	d.path.Mono = true
	d.cancel.Kind, d.cancel.Label = kit.PlaqueQuiet, "Cancel"
	d.add.Kind, d.add.Icon, d.add.Label, d.add.Busy = kit.PlaqueLine, look.Icon.Add, "Add folder", d.busy
	d.dialog.Width = 620
	return d.dialog.Layout(gtx, e.Th.Theme, "Add local music",
		func(gtx layout.Context) layout.Dimensions { return d.path.Layout(gtx, e.Th.Theme) },
		func(gtx layout.Context) layout.Dimensions { return d.cancel.Layout(gtx, e.Th.Theme) },
		func(gtx layout.Context) layout.Dimensions { return d.add.Layout(gtx, e.Th.Theme) },
	)
}
