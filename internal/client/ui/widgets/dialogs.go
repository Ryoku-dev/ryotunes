package widgets

import (
	"strings"

	"gioui.org/layout"

	"github.com/ryoku-dev/ryotunes/internal/client/lithic"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/act"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

// AddToPlaylistDialog lets a track be added to an existing or new playlist.
type AddToPlaylistDialog struct {
	Open  bool
	Track *model.Track

	dialog    lithic.Dialog
	menu      lithic.Menu
	newTitle  lithic.TextField
	create    lithic.Button
	cancel    lithic.Button
	playlists []model.BrowseItem
	loading   bool
	loaded    bool
	err       string
	wasOpen   bool
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
		d.playlists = items[:0]
		for _, item := range items {
			if model.IsRyotunesOwn(item.ID) && !strings.HasPrefix(item.ID, model.LocalPlaylistPrefix) {
				continue
			}
			d.playlists = append(d.playlists, item)
		}
	})
}

func addTrackToPlaylist(e *env.Env, playlistID string, track model.Track, done func(bool)) {
	finish := func(_ protocol.Null, err error) {
		if err != nil {
			if e.Toast != nil {
				e.Toast(err.Error(), env.ToastError)
			}
			done(false)
			return
		}
		if e.Toast != nil {
			e.Toast("Added to playlist", env.ToastSuccess)
		}
		done(true)
	}
	if strings.HasPrefix(playlistID, model.LocalPlaylistPrefix) {
		act.Call[protocol.Null](e, protocol.MethodAddToLocalPlaylist, protocol.AddToLocalPlaylistParams{PlaylistID: playlistID, Item: track}, finish)
		return
	}
	act.Call[protocol.Null](e, protocol.MethodAddToPlaylist, protocol.PlaylistTrackParams{PlaylistID: playlistID, VideoID: track.VideoID}, finish)
}

func (d *AddToPlaylistDialog) add(e *env.Env, playlistID string) {
	if d.Track == nil || playlistID == "" {
		return
	}
	track := *d.Track
	addTrackToPlaylist(e, playlistID, track, func(ok bool) {
		if ok {
			d.Open = false
			d.dialog.Open = false
		}
	})
}

func (d *AddToPlaylistDialog) createPlaylist(e *env.Env) {
	if d.Track == nil {
		return
	}
	title := strings.TrimSpace(d.newTitle.Text())
	if title == "" {
		title = "New playlist"
	}
	act.Call[protocol.PlaylistIDResult](e, protocol.MethodCreatePlaylist, protocol.CreatePlaylistParams{Title: title}, func(id protocol.PlaylistIDResult, err error) {
		if err != nil {
			if e.Toast != nil {
				e.Toast(err.Error(), env.ToastError)
			}
			return
		}
		d.add(e, string(id))
	})
}

func (d *AddToPlaylistDialog) body(gtx layout.Context, e *env.Env) layout.Dimensions {
	if d.create.Clicked(gtx) {
		d.createPlaylist(e)
	}
	children := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return d.newTitle.Layout(gtx, e.Th, "New playlist", "New playlist", "Create a playlist and add this song.", "")
		}),
		layout.Rigid(layout.Spacer{Height: e.Th.Sp.S2}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return d.create.Layout(gtx, e.Th, lithic.Line, lithic.Icon("add"), "Create playlist")
		}),
		layout.Rigid(layout.Spacer{Height: e.Th.Sp.S4}.Layout),
	}
	switch {
	case d.loading:
		children = append(children, layout.Rigid(lithic.Text(e.Th, e.Th.T.Body, e.Th.R.Fg2, "Loading playlists…")))
	case d.err != "":
		children = append(children, layout.Rigid(lithic.Text(e.Th, e.Th.T.Body, e.Th.R.Jasper, d.err)))
	case len(d.playlists) == 0:
		children = append(children, layout.Rigid(lithic.Text(e.Th, e.Th.T.Body, e.Th.R.Fg2, "No playlists yet.")))
	default:
		items := make([]lithic.MenuItem, len(d.playlists))
		for i, playlist := range d.playlists {
			items[i] = lithic.MenuItem{Icon: lithic.Icon("playlist_play"), Label: playlist.Title}
		}
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			dims := d.menu.Layout(gtx, e.Th, items)
			if index, ok := d.menu.Clicked(gtx); ok && index >= 0 && index < len(d.playlists) {
				d.add(e, d.playlists[index].ID)
			}
			return dims
		}))
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

// Layout renders the Add to playlist dialog and performs its daemon calls.
func (d *AddToPlaylistDialog) Layout(gtx layout.Context, e *env.Env) layout.Dimensions {
	if d.Open && !d.wasOpen {
		d.loaded, d.loading, d.err = false, false, ""
		d.playlists = nil
		d.newTitle.SetText("New playlist")
	}
	d.wasOpen = d.Open
	if d.Open {
		d.load(e)
	}
	if d.cancel.Clicked(gtx) {
		d.Open = false
	}
	d.dialog.Open = d.Open
	dims := d.dialog.Layout(gtx, e.Th, "Add to playlist", func(gtx layout.Context) layout.Dimensions {
		return d.body(gtx, e)
	}, func(gtx layout.Context) layout.Dimensions {
		return d.cancel.Layout(gtx, e.Th, lithic.Quiet, nil, "Cancel")
	})
	if d.dialog.Closed(gtx) {
		d.Open = false
	}
	d.Open = d.dialog.Open
	if !d.Open {
		d.wasOpen = false
	}
	return dims
}

// EditPlaylistDialog edits owned playlist metadata, cover, and deletion.
type EditPlaylistDialog struct {
	Open bool
	Page *model.PlaylistPage

	dialog      lithic.Dialog
	title       lithic.TextField
	description lithic.TextField
	public      lithic.Toggle
	save        lithic.Button
	cover       lithic.Button
	delete      lithic.Button
	cancel      lithic.Button
	initialized *model.PlaylistPage
}

func playlistTitle(page *model.PlaylistPage) string {
	if page != nil && page.Title != nil {
		return *page.Title
	}
	return "Playlist"
}

func (d *EditPlaylistDialog) initialize() {
	if d.Page == nil || d.initialized == d.Page {
		return
	}
	d.initialized = d.Page
	d.title.SetText(playlistTitle(d.Page))
	if d.Page.Description != nil {
		d.description.SetText(*d.Page.Description)
	} else {
		d.description.SetText("")
	}
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

func deleteConfirmCopy(id, name string) (title, body, typeToConfirm string) {
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
	title, body, typeToConfirm := deleteConfirmCopy(id, name)
	e.Confirm(title, body, "Delete", typeToConfirm, func() {
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
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return d.title.Layout(gtx, e.Th, "Title", "Playlist title", "", "")
		}),
		layout.Rigid(layout.Spacer{Height: e.Th.Sp.S4}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return d.description.Layout(gtx, e.Th, "Description", "Describe this playlist", "", "")
		}),
		layout.Rigid(layout.Spacer{Height: e.Th.Sp.S4}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return d.public.Layout(gtx, e.Th, "Public playlist")
		}),
	)
}

// Layout renders the owned-playlist editor and protected delete action.
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
	d.dialog.Open = d.Open
	dims := d.dialog.Layout(gtx, e.Th, "Edit playlist", func(gtx layout.Context) layout.Dimensions {
		return d.body(gtx, e)
	},
		func(gtx layout.Context) layout.Dimensions {
			return d.delete.Layout(gtx, e.Th, lithic.DangerLine, lithic.Icon("delete"), "Delete")
		},
		func(gtx layout.Context) layout.Dimensions {
			return d.cover.Layout(gtx, e.Th, lithic.Quiet, lithic.Icon("album"), "Change cover")
		},
		func(gtx layout.Context) layout.Dimensions {
			return d.cancel.Layout(gtx, e.Th, lithic.Quiet, nil, "Cancel")
		},
		func(gtx layout.Context) layout.Dimensions {
			return d.save.Layout(gtx, e.Th, lithic.Gold, nil, "Save")
		},
	)
	if d.dialog.Closed(gtx) {
		d.Open = false
	}
	d.Open = d.dialog.Open
	if !d.Open {
		d.initialized = nil
	}
	return dims
}
