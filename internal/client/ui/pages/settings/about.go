package settings

import (
	"gioui.org/layout"

	"github.com/ryoku-dev/ryotunes/internal/client/lithic"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/act"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
	"github.com/ryoku-dev/ryotunes/internal/version"
)

var aboutLinks = []struct {
	label string
	url   string
}{
	{label: "Website", url: "https://github.com/ryoku-dev/ryotunes"},
	{label: "Issues", url: "https://github.com/ryoku-dev/ryotunes/issues"},
	{label: "License", url: "https://github.com/ryoku-dev/ryotunes/blob/main/LICENSE"},
	{label: "Ryoku", url: "https://github.com/ryoku-dev/ryoku-arch"},
}

func (p *page) layoutAbout(gtx layout.Context, e *env.Env) layout.Dimensions {
	rows := []layout.Widget{
		settingRow(e.Th, "Ryotunes", "A focused Ryoku desktop music instrument: your library, local media, queue, lyrics and playback engine in one Lithic surface.", func(gtx layout.Context) layout.Dimensions {
			return lithic.Text(e.Th, e.Th.T.Mono, e.Th.R.Fg, version.Version)(gtx)
		}),
		settingRow(e.Th, "Daemon", "The background playback and provider service.", func(gtx layout.Context) layout.Dimensions {
			value := p.daemonVersion
			if value == "" {
				value = "-"
			}
			return lithic.Text(e.Th, e.Th.T.Mono, e.Th.R.Fg, value)(gtx)
		}),
		settingRow(e.Th, "Client", "Native Gio renderer using the Lithic UI kit.", func(gtx layout.Context) layout.Dimensions {
			return lithic.Text(e.Th, e.Th.T.Mono, e.Th.R.Fg, "Gio / Lithic")(gtx)
		}),
		p.updateRow(e),
	}
	for i, link := range aboutLinks {
		i, link := i, link
		rows = append(rows, settingRow(e.Th, link.label, link.url, func(gtx layout.Context) layout.Dimensions {
			if p.about.links[i].Clicked(gtx) {
				openExternal(e, link.url)
			}
			return p.about.links[i].Layout(gtx, e.Th, lithic.Line, nil, "Open")
		}))
	}
	rows = append(rows, settingRow(e.Th, "Diagnostics", "Inspect the daemon connection, playback state, provider state and recent failure evidence.", func(gtx layout.Context) layout.Dimensions {
		if p.about.diagnostics.Clicked(gtx) {
			e.Nav.Push(env.Route{Kind: env.RouteDiagnostics})
		}
		return p.about.diagnostics.Layout(gtx, e.Th, lithic.Line, nil, "Open diagnostics")
	}))
	return settingRows(gtx, rows...)
}

func (p *page) updateRow(e *env.Env) layout.Widget {
	detail := "Updates are checked only when you ask."
	if p.update != nil {
		switch {
		case p.update.RestartPending:
			detail = "Restart to finish update. The background service reloads only on a fresh launch."
		case p.update.Available && p.update.LatestVersion != nil:
			detail = "Ryotunes " + *p.update.LatestVersion + " is available."
			if p.update.Notes != "" {
				detail += " " + p.update.Notes
			}
		case p.update.UnsupportedReason != nil && *p.update.UnsupportedReason != "":
			detail = *p.update.UnsupportedReason
		default:
			detail = "Ryotunes is up to date."
		}
	}
	if p.installing {
		detail = "Downloading and verifying the update, then asking for administrator approval. Keep Ryotunes open until it finishes."
	}
	return settingRow(e.Th, "Updates", detail, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical, Alignment: layout.End, Gap: gtx.Dp(8)}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if p.installing {
					return lithic.BladeProgress(gtx, e.Th, 0, true)
				}
				return layout.Dimensions{}
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if p.update != nil && p.update.Available && p.update.CanInstall && p.update.LatestVersion != nil {
					p.about.install.Busy = p.installing
					if p.about.install.Clicked(gtx) && !p.installing {
						p.installing = true
						version := *p.update.LatestVersion
						act.Call[model.UpdateResult](e, protocol.MethodInstallUpdate, protocol.InstallUpdateParams{Version: version}, func(result model.UpdateResult, err error) {
							p.installing = false
							if err != nil {
								toastError(e, err, "Could not install update")
								return
							}
							updated := *p.update
							updated.Available = false
							updated.CurrentVersion = result.Version
							updated.RestartPending = result.RestartRequired
							p.update = &updated
							e.Toast("Ryotunes "+result.Version+" installed, restart Ryotunes to finish", env.ToastSuccess)
						})
					}
					label := "Install update"
					if p.installing {
						label = "Installing…"
					}
					return p.about.install.Layout(gtx, e.Th, lithic.Gold, nil, label)
				}
				p.about.check.Busy = p.checking
				if p.about.check.Clicked(gtx) && !p.checking {
					p.checking = true
					e.Toast("Checking for a new version…", env.ToastInfo)
					act.Call[model.UpdateInfo](e, protocol.MethodCheckForUpdates, protocol.Null{}, func(update model.UpdateInfo, err error) {
						p.checking = false
						if err != nil {
							toastError(e, err, "Could not check for updates")
							return
						}
						p.update = &update
					})
				}
				label := "Check for updates"
				if p.checking {
					label = "Checking…"
				}
				return p.about.check.Layout(gtx, e.Th, lithic.Line, nil, label)
			}),
		)
	})
}

func openExternal(e *env.Env, url string) {
	callNull(e, protocol.MethodOpenExternal, protocol.URLParams{URL: url}, "Could not open link", nil)
}
