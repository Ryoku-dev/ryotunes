package settings

import (
	"strings"

	"gioui.org/layout"

	"github.com/ryoku-dev/ryotunes/internal/client/lithic"
	"github.com/ryoku-dev/ryotunes/internal/client/state"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

func (p *page) layoutIntegrations(gtx layout.Context, e *env.Env, snapshot state.State) layout.Dimensions {
	presence := settingString(snapshot.Settings, model.SettingDiscordPresenceName, "Ryotunes")
	proxy := settingString(snapshot.Settings, model.SettingProxy, "")
	p.integrations.presence.sync(presence)
	p.integrations.presence.field.MaxLen = 128
	p.integrations.proxy.sync(proxy)
	p.integrations.proxy.field.Mono = true
	p.integrations.listenURL.sync(snapshot.LT.ServerURL)
	p.integrations.listenURL.field.Mono = true

	return settingRows(gtx,
		settingRow(e.Th, "Discord rich presence", discordDetail(p.discord, settingBool(snapshot.Settings, model.SettingDiscordRPC, false)), func(gtx layout.Context) layout.Dimensions {
			enabled := settingBool(snapshot.Settings, model.SettingDiscordRPC, false)
			p.integrations.discord.Value = enabled
			before := p.integrations.discord.Value
			tone, claim := discordClaim(p.discord, enabled)
			return accountControl(gtx, e.Th, tone, claim, func(gtx layout.Context) layout.Dimensions {
				dims := p.integrations.discord.Layout(gtx, e.Th, "")
				if p.integrations.discord.Value != before {
					setSetting(e, model.SettingDiscordRPC, boolString(p.integrations.discord.Value), "", nil)
				}
				return dims
			})
		}),
		settingRow(e.Th, "Discord presence title", "The text Discord renders as “Listening to …”.", func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical, Alignment: layout.End, Gap: gtx.Dp(8)}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return p.integrations.presence.field.Layout(gtx, e.Th, "", "Ryotunes", "Listening to "+p.integrations.presence.field.Text(), "")
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if p.integrations.savePresence.Clicked(gtx) {
						name := strings.TrimSpace(p.integrations.presence.field.Text())
						if name == "" {
							name = "Ryotunes"
						}
						setSetting(e, model.SettingDiscordPresenceName, name, "Discord now shows “Listening to "+name+"”", nil)
					}
					return p.integrations.savePresence.Layout(gtx, e.Th, lithic.Gold, nil, "Save")
				}),
			)
		}),
		toggleSettingRow(e, &p.integrations.tray, snapshot.Settings, model.SettingCloseToTray, true,
			"Close to tray", "Closing the window keeps music playing in the background."),
		toggleSettingRow(e, &p.integrations.autostart, snapshot.Settings, model.SettingAutostart, false,
			"Start on login", "Launch Ryotunes automatically when you log in."),
		settingRow(e.Th, "Proxy", "Route HTTP and HTTPS YouTube traffic through this proxy. Restart Ryotunes after saving.", func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical, Alignment: layout.End, Gap: gtx.Dp(8)}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return p.integrations.proxy.field.Layout(gtx, e.Th, "", "http://host:port (blank = none)", "", "")
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if p.integrations.saveProxy.Clicked(gtx) {
						setSetting(e, model.SettingProxy, strings.TrimSpace(p.integrations.proxy.field.Text()), "Proxy saved, restart to apply", nil)
					}
					return p.integrations.saveProxy.Layout(gtx, e.Th, lithic.Gold, nil, "Save proxy")
				}),
			)
		}),
		settingRow(e.Th, "Listen Together server", "The ws:// or wss:// relay used to host and join synchronized rooms.", func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical, Alignment: layout.End, Gap: gtx.Dp(8)}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return p.integrations.listenURL.field.Layout(gtx, e.Th, "", "wss://sync.example", "", "")
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if p.integrations.saveListenURL.Clicked(gtx) {
						callNull(e, protocol.MethodLTSetServerURL, protocol.LTServerURLParams{URL: strings.TrimSpace(p.integrations.listenURL.field.Text())}, "Could not save Listen Together server", func() {
							e.Toast("Listen Together server saved", env.ToastSuccess)
						})
					}
					return p.integrations.saveListenURL.Layout(gtx, e.Th, lithic.Gold, nil, "Save server")
				}),
			)
		}),
	)
}

func discordClaim(status model.DiscordStatus, enabled bool) (lithic.Tone, string) {
	if !enabled {
		return lithic.Idle, "Disabled"
	}
	switch status.Status {
	case "connected":
		return lithic.Alive, "Connected"
	case "connecting":
		return lithic.Busy, "Connecting"
	default:
		return lithic.Attention, "Unavailable"
	}
}

func discordDetail(status model.DiscordStatus, enabled bool) string {
	if !enabled {
		return "Disabled"
	}
	switch status.Status {
	case "connected":
		return "Connected"
	case "connecting":
		return "Connecting…"
	default:
		return "Discord not running / unavailable"
	}
}
