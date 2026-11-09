package library

import (
	"strings"

	"gioui.org/layout"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryotunes/internal/client/lithic"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/act"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

type newPlaylistDialog struct {
	dialog lithic.Dialog
	name   lithic.TextField
	cancel lithic.Button
	create lithic.Button
	err    string
	busy   bool
}

func (d *newPlaylistDialog) Open() {
	d.dialog.Open = true
	d.name.SetText("")
	d.err = ""
}

func (d *newPlaylistDialog) Layout(gtx layout.Context, e *env.Env, p *Page) {
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
	body := func(gtx layout.Context) layout.Dimensions {
		return d.name.Layout(gtx, e.Th, "Playlist name", "New playlist", "", d.err)
	}
	cancel := func(gtx layout.Context) layout.Dimensions {
		return d.cancel.Layout(gtx, e.Th, lithic.Quiet, nil, "Cancel")
	}
	create := func(gtx layout.Context) layout.Dimensions {
		if d.busy {
			gtx = gtx.Disabled()
		}
		return d.create.Layout(gtx, e.Th, lithic.Gold, lithic.Icon("add"), "Create")
	}
	d.dialog.Layout(gtx, e.Th, "New playlist", body, cancel, create)
}

type importPlaylistDialog struct {
	dialog       lithic.Dialog
	path         lithic.TextField
	cancel       lithic.Button
	importButton lithic.Button
	err          string
	busy         bool
}

func (d *importPlaylistDialog) Open() {
	d.dialog.Open = true
	d.path.SetText("")
	d.err = ""
}

func (d *importPlaylistDialog) Layout(gtx layout.Context, e *env.Env, p *Page) {
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
	body := func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return lithic.Text(e.Th, e.Th.T.Body, e.Th.R.Fg2, "Choose a Ryotunes playlist file; its tracks land in a new library playlist.")(gtx)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(21)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				d.path.Mono = true
				return d.path.Layout(gtx, e.Th, "Playlist file", "/home/you/Music/playlist.json", "Ryotunes playlist files end in .json.", d.err)
			}),
		)
	}
	cancel := func(gtx layout.Context) layout.Dimensions {
		return d.cancel.Layout(gtx, e.Th, lithic.Quiet, nil, "Cancel")
	}
	importAction := func(gtx layout.Context) layout.Dimensions {
		if d.busy {
			gtx = gtx.Disabled()
		}
		return d.importButton.Layout(gtx, e.Th, lithic.Gold, lithic.Icon("upload_file"), "Import")
	}
	d.dialog.Layout(gtx, e.Th, "Import playlist", body, cancel, importAction)
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
		act.Call[bool](e, protocol.MethodAddToLocalPlaylist, protocol.AddToLocalPlaylistParams{PlaylistID: playlistID, Item: items[index]}, func(_ bool, err error) {
			done(err)
		})
		return
	}
	act.Call[*bool](e, protocol.MethodAddToPlaylist, protocol.PlaylistTrackParams{PlaylistID: playlistID, VideoID: items[index].VideoID}, func(_ *bool, err error) {
		done(err)
	})
}

type folderDialog struct {
	dialog lithic.Dialog
	path   lithic.TextField
	cancel lithic.Button
	add    lithic.Button
	err    string
	busy   bool
}

func (d *folderDialog) Open() {
	d.dialog.Open = true
	d.path.SetText("")
	d.err = ""
}

func (d *folderDialog) Layout(gtx layout.Context, e *env.Env, p *Page) {
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
				d.dialog.Open = false
			})
		}
	}
	body := func(gtx layout.Context) layout.Dimensions {
		d.path.Mono = true
		return d.path.Layout(gtx, e.Th, "Music folder", "/home/you/Music", "Folders Ryotunes watches for files on disk. The daemon rescans them on demand.", d.err)
	}
	cancel := func(gtx layout.Context) layout.Dimensions {
		return d.cancel.Layout(gtx, e.Th, lithic.Quiet, nil, "Cancel")
	}
	add := func(gtx layout.Context) layout.Dimensions {
		if d.busy {
			gtx = gtx.Disabled()
		}
		return d.add.Layout(gtx, e.Th, lithic.Gold, lithic.Icon("add"), "Add folder")
	}
	d.dialog.Layout(gtx, e.Th, "Add local music", body, cancel, add)
}
