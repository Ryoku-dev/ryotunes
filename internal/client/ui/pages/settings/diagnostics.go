package settings

import (
	"fmt"

	"gioui.org/layout"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
)

func (p *page) layoutDiagnostics(gtx layout.Context, e *env.Env) layout.Dimensions {
	state := "Disconnected"
	if e.Conn != nil && e.Conn.Connected() {
		state = "Connected"
	}
	detail := state
	if p.daemonVersion != "" {
		detail += " · daemon " + p.daemonVersion
	}
	if p.protocol > 0 {
		detail += fmt.Sprintf(" · protocol %d", p.protocol)
	}
	if p.diagnostics.EngineVersion != "" {
		detail += " · " + p.diagnostics.EngineVersion
	}
	return p.settingRows(gtx, p.row(e.Th, rowSpec{
		title: "Daemon summary", detail: detail,
		activate: func() { e.Nav.Push(env.Route{Kind: env.RouteDiagnostics}) },
		control: func(gtx layout.Context) layout.Dimensions {
			p.diagnosticsUI.open.Kind, p.diagnosticsUI.open.Small, p.diagnosticsUI.open.Icon, p.diagnosticsUI.open.Label = kit.PlaqueQuiet, true, look.Icon.Forward, "Open diagnostics"
			if p.diagnosticsUI.open.Clicked(gtx) {
				e.Nav.Push(env.Route{Kind: env.RouteDiagnostics})
			}
			return p.diagnosticsUI.open.Layout(gtx, e.Th.Theme)
		},
	}))
}
