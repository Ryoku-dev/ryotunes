package settings

import (
	"gioui.org/layout"

	"github.com/ryoku-dev/ryotunes/internal/client/lithic"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/act"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

func (p *page) layoutData(gtx layout.Context, e *env.Env) layout.Dimensions {
	return settingRows(gtx,
		settingRow(e.Th, "Rotate playback identity", "Ask the daemon to establish a fresh YouTube playback identity after clearing caches.", func(gtx layout.Context) layout.Dimensions {
			return p.data.rotate.Layout(gtx, e.Th, "Rotate playback identity")
		}),
		settingRow(e.Th, "Playback caches", "Clear cached stream URLs and downloaded audio bytes, and reset the YouTube playback identity. Fixes tracks that fail with “YouTube rejected the stream link”.", func(gtx layout.Context) layout.Dimensions {
			p.data.clear.Busy = p.clearing
			if p.data.clear.Clicked(gtx) && !p.clearing {
				p.clearing = true
				act.Call[protocol.ClearCachesResult](e, protocol.MethodClearCaches, protocol.ClearCachesParams{Rotate: new(p.data.rotate.Value)}, func(result protocol.ClearCachesResult, err error) {
					p.clearing = false
					if err != nil {
						toastError(e, err, "Could not clear caches")
						return
					}
					if !result.VisitorDataRefreshed && p.data.rotate.Value {
						e.Toast("Caches cleared, offline, playback ID will retry on next play", env.ToastInfo)
					} else {
						e.Toast("Playback caches cleared", env.ToastSuccess)
					}
				})
			}
			label := "Force clear caches"
			if p.clearing {
				label = "Clearing…"
			}
			return p.data.clear.Layout(gtx, e.Th, lithic.DangerLine, nil, label)
		}),
		settingRow(e.Th, "Playlist index", "Refresh the track-to-playlist index used by library menus and saved-state checks.", func(gtx layout.Context) layout.Dimensions {
			p.data.sync.Busy = p.syncing
			if p.data.sync.Clicked(gtx) && !p.syncing {
				p.syncing = true
				act.Call[model.PlaylistIndex](e, protocol.MethodSyncPlaylistIndex, protocol.Null{}, func(_ model.PlaylistIndex, err error) {
					p.syncing = false
					if err != nil {
						toastError(e, err, "Could not sync playlist index")
						return
					}
					e.Toast("Playlist index synced", env.ToastSuccess)
				})
			}
			label := "Sync now"
			if p.syncing {
				label = "Syncing…"
			}
			return p.data.sync.Layout(gtx, e.Th, lithic.Line, nil, label)
		}),
	)
}

func (p *page) layoutLocal(gtx layout.Context, e *env.Env) layout.Dimensions {
	return settingRows(gtx,
		settingRow(e.Th, "Local library", "Add folders, remove folders and rescan your files in Library > Local.", func(gtx layout.Context) layout.Dimensions {
			if p.localOpen.Clicked(gtx) {
				e.Nav.Push(env.Route{Kind: env.RouteLibrary, Params: "local"})
			}
			return p.localOpen.Layout(gtx, e.Th, lithic.Line, nil, "Open local library")
		}),
	)
}
