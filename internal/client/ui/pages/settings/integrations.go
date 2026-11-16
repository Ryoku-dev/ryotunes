package settings

import (
	"strings"

	"gioui.org/layout"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/palette"
	"github.com/ryoku-dev/ryotunes/internal/client/state"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

func (p *page) layoutIntegrations(gtx layout.Context, e *env.Env, snapshot state.State) layout.Dimensions {
	presence := settingString(snapshot.Settings, model.SettingDiscordPresenceName, "Ryotunes")
	p.integrations.presence.sync(presence)
	p.integrations.presence.field.MaxLen = 128
	p.integrations.listenURL.sync(snapshot.LT.ServerURL)
	return p.settingRows(gtx,
		p.discordRow(e, snapshot),
		p.discordTitleRow(e),
		p.lastfmRow(e, snapshot),
		p.mprisRow(e),
		func(gtx layout.Context) layout.Dimensions {
			return p.integrations.more.layout(gtx, e.Th, "More options", func(gtx layout.Context) layout.Dimensions {
				return stackRows(gtx, []layout.Widget{p.listenTogetherRow(e)})
			})
		})
}

// discordRow is the Discord Rich Presence toggle (owner ruling R5): the
// connection's claim beside it, and the whole row toggles it.
func (p *page) discordRow(e *env.Env, snapshot state.State) layout.Widget {
	enabled := p.valueBool(snapshot.Settings, model.SettingDiscordRPC)
	tone, claim := discordClaim(p.discord, enabled)
	set := func(on bool) { p.write(e, model.SettingDiscordRPC, boolString(on), "", nil) }
	return p.row(e.Th, rowSpec{
		title: "Discord rich presence", detail: "Show what you are listening to on your Discord profile",
		changed:  p.settingChanged(snapshot.Settings, model.SettingDiscordRPC),
		reset:    func() { set(false) },
		activate: func() { set(!enabled) },
		control: func(gtx layout.Context) layout.Dimensions {
			p.integrations.discord.Value = enabled
			return accountControl(gtx, e, tone, claim, func(gtx layout.Context) layout.Dimensions {
				dims := p.integrations.discord.Layout(gtx, e.Th.Theme)
				if p.integrations.discord.Changed() {
					set(p.integrations.discord.Value)
				}
				return dims
			})
		},
	})
}

func (p *page) discordTitleRow(e *env.Env) layout.Widget {
	return p.settingRow(e.Th, "Discord presence title", "The text Discord renders as “Listening to …”.", func(gtx layout.Context) layout.Dimensions {
		p.integrations.presence.field.Placeholder = "Ryotunes"
		p.integrations.presence.field.Help = "Listening to " + p.integrations.presence.field.Text()
		p.integrations.savePresence.Kind, p.integrations.savePresence.Small, p.integrations.savePresence.Label = kit.PlaqueQuiet, true, "Save"
		if p.integrations.savePresence.Clicked(gtx) {
			name := strings.TrimSpace(p.integrations.presence.field.Text())
			if len(name) < 2 || len(name) > 128 {
				e.Toast("Discord presence title must be between 2 and 128 characters", env.ToastError)
			} else {
				p.write(e, model.SettingDiscordPresenceName, name, "Discord now shows “Listening to "+name+"”.", nil)
			}
		}
		return layout.Flex{Axis: layout.Vertical, Alignment: layout.End}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return p.integrations.presence.field.Layout(gtx, e.Th.Theme)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return p.integrations.savePresence.Layout(gtx, e.Th.Theme) }),
		)
	})
}

func (p *page) lastfmRow(e *env.Env, snapshot state.State) layout.Widget {
	configured := p.lastfm.Configured || snapshot.Lastfm.Configured
	connected := p.lastfm.Connected || snapshot.Lastfm.Connected
	username := p.lastfm.Username
	if snapshot.Lastfm.Username != nil {
		username = snapshot.Lastfm.Username
	}
	tone, claim, detail := palette.Idle, "Not connected", "Connect Last.fm to scrobble what you play."
	if !configured {
		tone, claim, detail = palette.Attention, "Not configured", "Last.fm credentials are not configured on this daemon."
	} else if connected {
		tone, claim, detail = palette.Alive, "Connected", valueOr(username, "Last.fm")
	} else if snapshot.Lastfm.Error != nil && *snapshot.Lastfm.Error != "" {
		tone, claim, detail = palette.Failed, "Failed", *snapshot.Lastfm.Error
	}
	return p.settingRow(e.Th, "Last.fm", detail, func(gtx layout.Context) layout.Dimensions {
		return accountControl(gtx, e, tone, claim, func(gtx layout.Context) layout.Dimensions {
			if connected {
				return providerButton(gtx, e, &p.integrations.lastfmDisconnect, kit.PlaqueQuiet, "Disconnect", func() {
					confirm(e, "Disconnect Last.fm?", "Ryotunes stops scrobbling what you play until you connect again.", "Disconnect", func() {
						callNull(e, protocol.MethodLastfmDisconnect, protocol.Null{}, "Could not disconnect Last.fm", nil)
					})
				})
			}
			buttonGTX := gtx
			if !configured {
				buttonGTX = buttonGTX.Disabled()
			}
			return providerButton(buttonGTX, e, &p.integrations.lastfmConnect, kit.PlaqueQuiet, "Connect", func() {
				if configured {
					callNull(e, protocol.MethodLastfmConnect, protocol.Null{}, "Could not connect Last.fm", nil)
				}
			})
		})
	})
}

func (p *page) mprisRow(e *env.Env) layout.Widget {
	tone, claim, detail := palette.Attention, "Unavailable", "Desktop media controls are unavailable in this session."
	if p.diagnostics.MPRIS {
		tone, claim, detail = palette.Alive, "Connected", "Desktop media keys and player controls can operate Ryotunes."
	}
	return p.settingRow(e.Th, "MPRIS", detail, func(gtx layout.Context) layout.Dimensions {
		return kit.Claim(e.Th.Theme, tone, claim)(gtx)
	})
}

func (p *page) listenTogetherRow(e *env.Env) layout.Widget {
	return p.settingRow(e.Th, "Listen Together server", "The ws:// or wss:// relay used to host and join synchronized rooms.", func(gtx layout.Context) layout.Dimensions {
		p.integrations.listenURL.field.Mono = true
		p.integrations.listenURL.field.Placeholder = "wss://sync.example"
		p.integrations.saveListenURL.Kind, p.integrations.saveListenURL.Small, p.integrations.saveListenURL.Label = kit.PlaqueQuiet, true, "Save server"
		if p.integrations.saveListenURL.Clicked(gtx) {
			callNull(e, protocol.MethodLTSetServerURL, protocol.LTServerURLParams{URL: strings.TrimSpace(p.integrations.listenURL.field.Text())}, "Could not save Listen Together server", func() {
				e.Toast("Listen Together server saved.", env.ToastSuccess)
			})
		}
		return layout.Flex{Axis: layout.Vertical, Alignment: layout.End}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return p.integrations.listenURL.field.Layout(gtx, e.Th.Theme)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return p.integrations.saveListenURL.Layout(gtx, e.Th.Theme)
			}),
		)
	})
}

func discordClaim(status model.DiscordStatus, enabled bool) (palette.Tone, string) {
	if !enabled {
		return palette.Idle, "Off"
	}
	switch strings.ToLower(status.Status) {
	case "connected":
		return palette.Alive, "Connected"
	case "connecting":
		return palette.Becoming, "Connecting"
	default:
		return palette.Attention, "Discord is not running"
	}
}
