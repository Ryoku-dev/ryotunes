package settings

import (
	"strings"

	"gioui.org/layout"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/act"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

func (p *page) layoutData(gtx layout.Context, e *env.Env) layout.Dimensions {
	snapshot := e.Store.Snapshot()
	p.data.proxy.sync(settingString(snapshot.Settings, model.SettingProxy, ""))
	return settingRows(gtx,
		settingRow(e.Th, "Proxy", "Route HTTP and HTTPS YouTube traffic through this proxy. Restart Ryotunes after saving.", func(gtx layout.Context) layout.Dimensions {
			p.data.proxy.field.Mono = true
			p.data.proxy.field.Placeholder = "http://host:port (blank = none)"
			p.data.saveProxy.Kind, p.data.saveProxy.Label = kit.PlaqueLine, "Save proxy"
			if p.data.saveProxy.Clicked(gtx) {
				setSetting(e, model.SettingProxy, strings.TrimSpace(p.data.proxy.field.Text()), "Proxy saved, restart to apply", nil)
			}
			return layout.Flex{Axis: layout.Vertical, Alignment: layout.End}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { return p.data.proxy.field.Layout(gtx, e.Th.Theme) }),
				layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { return p.data.saveProxy.Layout(gtx, e.Th.Theme) }),
			)
		}),
		settingRow(e.Th, "Rotate playback identity", "Ask the daemon to establish a fresh YouTube playback identity after clearing caches.", func(gtx layout.Context) layout.Dimensions {
			p.data.rotate.Label = "Rotate playback identity"
			return p.data.rotate.Layout(gtx, e.Th.Theme)
		}),
		settingRow(e.Th, "Playback caches", "Clear cached stream URLs and downloaded audio bytes, and reset the YouTube playback identity.", func(gtx layout.Context) layout.Dimensions {
			p.data.clear.Kind, p.data.clear.Busy = kit.PlaqueDangerLine, p.clearing
			p.data.clear.Label = "Force clear caches"
			if p.clearing {
				p.data.clear.Label = "Clearing…"
			}
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
			return p.data.clear.Layout(gtx, e.Th.Theme)
		}),
		settingRow(e.Th, "Playlist index", "Refresh saved-state checks and the track-to-playlist index.", func(gtx layout.Context) layout.Dimensions {
			p.data.sync.Kind, p.data.sync.Busy, p.data.sync.Label = kit.PlaqueLine, p.syncing, "Sync now"
			if p.syncing {
				p.data.sync.Label = "Syncing…"
			}
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
			return p.data.sync.Layout(gtx, e.Th.Theme)
		}),
	)
}
