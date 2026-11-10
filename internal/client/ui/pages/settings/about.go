package settings

import (
	"os"
	"runtime/debug"

	"gioui.org/layout"
	"gioui.org/unit"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/act"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
	"github.com/ryoku-dev/ryotunes/internal/version"
)

const (
	copyrightLine        = "Copyright (C) 2026 Ryoku-dev and contributors"
	installedLicensesURL = "file:///usr/share/licenses/ryotunes/"
	repositoryNoticeURL  = "https://github.com/ryoku-dev/ryotunes/blob/hyperion/NOTICE"
)

var aboutLinks = []struct {
	label string
	url   string
}{
	{label: "Website", url: "https://github.com/ryoku-dev/ryotunes"},
	{label: "Issues", url: "https://github.com/ryoku-dev/ryotunes/issues"},
	{label: "Ryoku", url: "https://github.com/ryoku-dev/ryoku"},
}

func (p *page) layoutAbout(gtx layout.Context, e *env.Env) layout.Dimensions {
	daemon := p.daemonVersion
	if daemon == "" {
		daemon = "Not connected"
	}
	engine := p.diagnostics.EngineVersion
	if engine == "" {
		engine = "Unavailable"
	}
	rows := []layout.Widget{
		p.settingRow(e.Th, "Ryotunes", "A focused Ryoku desktop music instrument: your library, local media, queue, lyrics and playback engine in one Lithic surface.", func(gtx layout.Context) layout.Dimensions {
			return look.RyokuMark(unit.Dp(40), e.Th.R.Fg)(gtx)
		}),
		p.versionRow(e, "App", version.Version),
		p.versionRow(e, "Daemon", daemon),
		p.versionRow(e, "Client", clientVersion()),
		p.versionRow(e, "Engine", engine),
		p.licenseRow(e),
		p.updateRow(e),
	}
	for i, link := range aboutLinks {
		i, link := i, link
		rows = append(rows, p.settingRow(e.Th, link.label, link.url, func(gtx layout.Context) layout.Dimensions {
			p.about.links[i].Kind, p.about.links[i].Label = kit.PlaqueLine, "Open"
			if p.about.links[i].Clicked(gtx) {
				openExternal(e, link.url)
			}
			return p.about.links[i].Layout(gtx, e.Th.Theme)
		}))
	}
	return p.settingRows(gtx, rows...)
}

func (p *page) licenseRow(e *env.Env) layout.Widget {
	return p.settingRow(e.Th, "License", copyrightLine, func(gtx layout.Context) layout.Dimensions {
		p.about.licenses.Kind, p.about.licenses.Label = kit.PlaqueLine, "Licenses"
		if p.about.licenses.Clicked(gtx) {
			act.OpenURL(e, licensesURL())
		}
		return layout.Flex{Axis: layout.Vertical, Alignment: layout.End}.Layout(gtx,
			layout.Rigid(kit.Text(e.Th.Theme, e.Th.T.Mono, e.Th.R.Fg, "GPL-3.0-or-later")),
			layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return p.about.licenses.Layout(gtx, e.Th.Theme)
			}),
		)
	})
}

func licensesURL() string {
	if info, err := os.Stat("/usr/share/licenses/ryotunes"); err == nil && info.IsDir() {
		return installedLicensesURL
	}
	return repositoryNoticeURL
}

func (p *page) versionRow(e *env.Env, label, value string) layout.Widget {
	return p.settingRow(e.Th, label, "Version", func(gtx layout.Context) layout.Dimensions {
		return kit.Text(e.Th.Theme, e.Th.T.Mono, e.Th.R.Fg, value)(gtx)
	})
}

func clientVersion() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, dependency := range info.Deps {
			if dependency.Path == "gioui.org" && dependency.Version != "" {
				return "Gio " + dependency.Version
			}
		}
	}
	return "Gio"
}

func (p *page) updateRow(e *env.Env) layout.Widget {
	detail := "Updates are checked only when you ask."
	if p.update != nil {
		switch {
		case p.update.RestartPending:
			detail = "Restart to finish the update."
		case p.update.Available && p.update.LatestVersion != nil:
			detail = "Ryotunes " + *p.update.LatestVersion + " is available."
		case p.update.UnsupportedReason != nil && *p.update.UnsupportedReason != "":
			detail = *p.update.UnsupportedReason
		default:
			detail = "Ryotunes is up to date."
		}
	}
	if p.installing {
		detail = "Downloading and verifying the update, then asking for administrator approval. Keep Ryotunes open until it finishes."
	}
	return p.settingRow(e.Th, "Updates", detail, func(gtx layout.Context) layout.Dimensions {
		p.about.check.Kind, p.about.check.Busy, p.about.check.Label = kit.PlaqueLine, p.checking, "Check"
		if p.checking {
			p.about.check.Label = "Checking…"
		}
		p.about.install.Kind, p.about.install.Busy, p.about.install.Label = kit.PlaqueLine, p.installing, "Install"
		if p.installing {
			p.about.install.Label = "Installing…"
		}
		p.about.changelog.Kind, p.about.changelog.Label = kit.PlaqueLine, "View changelog"
		p.about.notes.Kind = kit.PlaqueQuiet
		if p.about.showNotes {
			p.about.notes.Label = "Hide release notes"
		} else {
			p.about.notes.Label = "Show release notes"
		}
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
		if p.about.install.Clicked(gtx) && p.update != nil && p.update.Available && p.update.CanInstall && p.update.LatestVersion != nil && !p.installing {
			p.installing = true
			latest := *p.update.LatestVersion
			act.Call[model.UpdateResult](e, protocol.MethodInstallUpdate, protocol.InstallUpdateParams{Version: latest}, func(result model.UpdateResult, err error) {
				p.installing = false
				if err != nil {
					toastError(e, err, "Could not install update")
					return
				}
				updated := *p.update
				updated.Available, updated.CurrentVersion, updated.RestartPending = false, result.Version, result.RestartRequired
				p.update = &updated
				e.Toast("Ryotunes "+result.Version+" installed, restart Ryotunes to finish", env.ToastSuccess)
			})
		}
		if p.about.changelog.Clicked(gtx) {
			openExternal(e, "https://github.com/ryoku-dev/ryotunes/blob/gio/CHANGELOG.md")
		}
		if p.about.notes.Clicked(gtx) && p.update != nil && p.update.Notes != "" {
			p.about.showNotes = !p.about.showNotes
		}
		children := []layout.FlexChild{
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return p.about.check.Layout(gtx, e.Th.Theme) }),
			layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
		}
		if p.update != nil && p.update.Available && p.update.CanInstall {
			children = append(children,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { return p.about.install.Layout(gtx, e.Th.Theme) }),
				layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
			)
		}
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions { return p.about.changelog.Layout(gtx, e.Th.Theme) }))
		if p.update != nil && p.update.Notes != "" {
			children = append(children,
				layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { return p.about.notes.Layout(gtx, e.Th.Theme) }),
			)
		}
		if p.about.showNotes && p.update != nil && p.update.Notes != "" {
			children = append(children,
				layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return kit.Label{Style: e.Th.T.Caption, Color: e.Th.R.Fg2, MaxLines: 12}.Layout(gtx, e.Th.Theme, p.update.Notes)
				}),
			)
		}
		return layout.Flex{Axis: layout.Vertical, Alignment: layout.End}.Layout(gtx, children...)
	})
}

func openExternal(e *env.Env, url string) {
	callNull(e, protocol.MethodOpenExternal, protocol.URLParams{URL: url}, "Could not open link", nil)
}
