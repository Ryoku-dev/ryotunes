package settings

import (
	"strings"

	"gioui.org/layout"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryotunes/internal/client/state"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/act"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

func (p *page) layoutPlaylists(gtx layout.Context, e *env.Env, snapshot state.State) layout.Dimensions {
	p.playlists.exportPath.sync("ryotunes-queue.json")
	return p.settingRows(gtx,
		p.settingRow(e.Th, "Import a playlist", "Choose a Ryotunes playlist file; its tracks land in a new library playlist. Sign in first.", func(gtx layout.Context) layout.Dimensions {
			p.playlists.importPath.field.Mono = true
			p.playlists.importPath.field.Placeholder = "/home/you/Music/playlist.json"
			p.playlists.importButton.Kind, p.playlists.importButton.Label, p.playlists.importButton.Busy = kit.PlaqueLine, "Import", p.playlists.busy
			if p.playlists.importButton.Clicked(gtx) && !p.playlists.busy {
				path := strings.TrimSpace(p.playlists.importPath.field.Text())
				if path == "" {
					e.Toast("Choose a playlist file", env.ToastError)
				} else {
					p.importPlaylist(e, path)
				}
			}
			return transferControl(gtx, e, &p.playlists.importPath.field, &p.playlists.importButton)
		}),
		p.settingRow(e.Th, "Export the queue", "Write the tracks now in your queue to a portable JSON file.", func(gtx layout.Context) layout.Dimensions {
			p.playlists.exportPath.field.Mono = true
			p.playlists.exportPath.field.Placeholder = "ryotunes-queue.json"
			p.playlists.exportButton.Kind, p.playlists.exportButton.Label, p.playlists.exportButton.Busy = kit.PlaqueLine, "Export", p.playlists.busy
			if p.playlists.exportButton.Clicked(gtx) && !p.playlists.busy {
				path := strings.TrimSpace(p.playlists.exportPath.field.Text())
				if path == "" {
					e.Toast("Choose where to export the queue", env.ToastError)
				} else if len(snapshot.Queue.Items) == 0 {
					e.Toast("The queue has no tracks to export", env.ToastError)
				} else {
					p.playlists.busy = true
					act.Call[bool](e, protocol.MethodExportPlaylistFile, protocol.ExportPlaylistFileParams{Title: "Ryotunes queue", Items: snapshot.Queue.Items, Path: path}, func(_ bool, err error) {
						p.playlists.busy = false
						if err != nil {
							toastError(e, err, "Could not export the queue")
							return
						}
						e.Toast("Queue exported", env.ToastSuccess)
					})
				}
			}
			return transferControl(gtx, e, &p.playlists.exportPath.field, &p.playlists.exportButton)
		}))
}

func transferControl(gtx layout.Context, e *env.Env, field *kit.TextField, button *kit.Plaque) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical, Alignment: layout.End}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return field.Layout(gtx, e.Th.Theme) }),
		layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return button.Layout(gtx, e.Th.Theme) }),
	)
}

func (p *page) importPlaylist(e *env.Env, path string) {
	p.playlists.busy = true
	act.Call[protocol.ImportPlaylistFileResult](e, protocol.MethodImportPlaylistFile, protocol.PathParams{Path: path}, func(transfer protocol.ImportPlaylistFileResult, err error) {
		if err != nil {
			p.playlists.busy = false
			toastError(e, err, "Could not import that playlist")
			return
		}
		if len(transfer.Items) == 0 {
			p.playlists.busy = false
			e.Toast("That playlist file has no tracks", env.ToastError)
			return
		}
		title := strings.TrimSpace(transfer.Title)
		if title == "" {
			title = "Imported playlist"
		}
		act.Call[protocol.PlaylistIDResult](e, protocol.MethodCreatePlaylist, protocol.CreatePlaylistParams{Title: title}, func(id protocol.PlaylistIDResult, err error) {
			if err != nil {
				p.playlists.busy = false
				toastError(e, err, "Could not create the imported playlist")
				return
			}
			p.addImportedTracks(e, string(id), transfer.Items, 0)
		})
	})
}

func (p *page) addImportedTracks(e *env.Env, playlistID string, tracks []model.Track, index int) {
	if index >= len(tracks) {
		p.playlists.busy = false
		e.Toast("Imported "+pluralCount(len(tracks), "song"), env.ToastSuccess)
		return
	}
	done := func(err error) {
		if err != nil {
			p.playlists.busy = false
			toastError(e, err, "Could not add tracks to the imported playlist")
			return
		}
		p.addImportedTracks(e, playlistID, tracks, index+1)
	}
	if strings.HasPrefix(playlistID, model.LocalPlaylistPrefix) {
		act.Call[bool](e, protocol.MethodAddToLocalPlaylist, protocol.AddToLocalPlaylistParams{PlaylistID: playlistID, Item: tracks[index]}, func(_ bool, err error) { done(err) })
		return
	}
	act.Call[*bool](e, protocol.MethodAddToPlaylist, protocol.PlaylistTrackParams{PlaylistID: playlistID, VideoID: tracks[index].VideoID}, func(_ *bool, err error) { done(err) })
}
