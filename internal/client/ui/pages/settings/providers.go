package settings

import (
	"strings"

	"gioui.org/layout"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/palette"
	"github.com/ryoku-dev/ryotunes/internal/client/state"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/act"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

func (p *page) layoutAccount(gtx layout.Context, e *env.Env, snapshot state.State) layout.Dimensions {
	return p.settingRows(gtx,
		p.youtubeRow(e, snapshot),
		p.youtubeIdentitiesRow(e),
		p.spotifyRow(e, snapshot),
		p.soundCloudRow(e, snapshot),
		p.tidalRow(e, snapshot),
		p.tidalCredentialsRow(e))
}

func (p *page) youtubeRow(e *env.Env, snapshot state.State) layout.Widget {
	flow := snapshot.AuthFlows[model.ProviderYouTube]
	detail, tone, claim := "Sign in to sync your YouTube Music library.", palette.Idle, "Not signed in"
	switch {
	case snapshot.Auth.SignedIn:
		detail = valueOr(snapshot.Auth.Name, "YouTube Music")
		if handle := valueOr(snapshot.Auth.Handle, ""); handle != "" {
			detail += " · " + handle
		}
		tone, claim = palette.Alive, "Connected"
	case flow.Error != "":
		detail, tone, claim = flow.Error, palette.Failed, "Failed"
	case flow.Waiting:
		detail, tone, claim = "Waiting for your browser", palette.Becoming, "Waiting"
	}
	return p.settingRow(e.Th, "YouTube Music", detail, func(gtx layout.Context) layout.Dimensions {
		return accountControl(gtx, e, tone, claim, func(gtx layout.Context) layout.Dimensions {
			if snapshot.Auth.SignedIn {
				p.accounts.youtubeSignOut.Kind, p.accounts.youtubeSignOut.Label = kit.PlaqueDangerLine, "Sign out"
				if p.accounts.youtubeSignOut.Clicked(gtx) {
					callNull(e, protocol.MethodSignOut, protocol.Null{}, "Could not sign out", nil)
				}
				return p.accounts.youtubeSignOut.Layout(gtx, e.Th.Theme)
			}
			if flow.Waiting || flow.URL != "" {
				return p.layoutAuthFlow(gtx, e, model.ProviderYouTube, flow, &p.accounts.youtubeFlow)
			}
			return providerButton(gtx, e, &p.accounts.youtubeSignIn, kit.PlaqueLine, "Sign in with Google", func() {
				act.SignIn(e, model.ProviderYouTube)
			})
		})
	})
}

func (p *page) youtubeIdentitiesRow(e *env.Env) layout.Widget {
	detail := "Sign in to see the channels on this account."
	if len(p.identities) == 1 {
		detail = "This account has a single channel."
	} else if len(p.identities) > 1 {
		detail = "Choose which YouTube channel Ryotunes uses."
	}
	return p.settingRow(e.Th, "YouTube channel", detail, func(gtx layout.Context) layout.Dimensions {
		if len(p.identities) < 2 {
			return layout.Dimensions{}
		}
		labels := make([]string, len(p.identities))
		selected := 0
		for i, identity := range p.identities {
			labels[i] = identity.Name
			if identity.Handle != nil && *identity.Handle != "" {
				labels[i] += " " + *identity.Handle
			}
			if identity.Selected {
				selected = i
			}
		}
		dims := p.accounts.identity.Layout(gtx, e.Th, e.Overlays, labels, selected)
		if changed, ok := p.accounts.identity.Changed(); ok {
			identity := p.identities[changed]
			act.Call[model.Account](e, protocol.MethodSwitchAccount, protocol.SwitchAccountParams{SelectionKey: identity.SelectionKey}, func(_ model.Account, err error) {
				if err != nil {
					toastError(e, err, "Could not switch channel")
				}
			})
		}
		return dims
	})
}

func (p *page) spotifyRow(e *env.Env, snapshot state.State) layout.Widget {
	flow := snapshot.AuthFlows[model.ProviderSpotify]
	tone, claim, detail := palette.Idle, "Not signed in", "Sign in to browse and play Spotify."
	switch {
	case snapshot.Spotify.SignedIn:
		tone, claim = palette.Alive, "Connected"
		detail = valueOr(snapshot.Spotify.Name, "Spotify")
		if snapshot.Spotify.Premium != nil && !*snapshot.Spotify.Premium {
			detail += " · Premium is required for playback."
		} else {
			detail += " · Premium"
		}
	case flow.Error != "":
		tone, claim, detail = palette.Failed, "Failed", flow.Error
	case flow.Waiting:
		tone, claim, detail = palette.Becoming, "Waiting", "Waiting for your browser"
	case snapshot.Spotify.Error != "":
		tone, claim, detail = palette.Failed, "Failed", snapshot.Spotify.Error
	}
	return p.settingRow(e.Th, "Spotify", detail, func(gtx layout.Context) layout.Dimensions {
		return accountControl(gtx, e, tone, claim, func(gtx layout.Context) layout.Dimensions {
			if snapshot.Spotify.SignedIn {
				return providerButton(gtx, e, &p.accounts.spotifySignOut, kit.PlaqueDangerLine, "Sign out", func() {
					callProviderSignOut(e, protocol.MethodSpotifySignOut)
				})
			}
			if flow.Waiting || flow.URL != "" {
				return p.layoutAuthFlow(gtx, e, model.ProviderSpotify, flow, &p.accounts.spotifyFlow)
			}
			return providerButton(gtx, e, &p.accounts.spotifySignIn, kit.PlaqueLine, "Sign in", func() {
				act.SignIn(e, model.ProviderSpotify)
			})
		})
	})
}

func (p *page) soundCloudRow(e *env.Env, snapshot state.State) layout.Widget {
	flow := snapshot.AuthFlows[model.ProviderSoundCloud]
	tone, claim := palette.Idle, "Guest access"
	detail := "No account needed · SoundCloud's public catalogue plays for everyone."
	switch {
	case snapshot.SoundCloud.SignedIn:
		tone, claim, detail = palette.Alive, "Connected", valueOr(snapshot.SoundCloud.Name, "SoundCloud")
	case flow.Error != "":
		tone, claim, detail = palette.Failed, "Failed", flow.Error
	case flow.Waiting:
		tone, claim, detail = palette.Becoming, "Waiting", "Waiting for your browser"
	case snapshot.SoundCloud.Error != "":
		tone, claim, detail = palette.Failed, "Failed", snapshot.SoundCloud.Error
	case snapshot.SoundCloud.Flow == "expired":
		tone, claim, detail = palette.Attention, "Expired", "No SoundCloud sign-in was completed."
	}
	return p.settingRow(e.Th, "SoundCloud", detail, func(gtx layout.Context) layout.Dimensions {
		return accountControl(gtx, e, tone, claim, func(gtx layout.Context) layout.Dimensions {
			if snapshot.SoundCloud.SignedIn {
				return providerButton(gtx, e, &p.accounts.soundCloudSignOut, kit.PlaqueDangerLine, "Sign out", func() {
					callProviderSignOut(e, protocol.MethodSoundCloudSignOut)
				})
			}
			if flow.Waiting || flow.URL != "" {
				return p.layoutAuthFlow(gtx, e, model.ProviderSoundCloud, flow, &p.accounts.soundCloudFlow)
			}
			return providerButton(gtx, e, &p.accounts.soundCloudSignIn, kit.PlaqueLine, "Sign in", func() {
				act.SignIn(e, model.ProviderSoundCloud)
			})
		})
	})
}

func (p *page) tidalRow(e *env.Env, snapshot state.State) layout.Widget {
	flow := snapshot.AuthFlows[model.ProviderTidal]
	tone, claim, detail := palette.Idle, "Not signed in", "TIDAL has no guest catalogue · sign in to browse and play."
	switch {
	case snapshot.Tidal.SignedIn:
		tone, claim, detail = palette.Alive, "Connected", valueOr(snapshot.Tidal.Name, "TIDAL")
		if snapshot.Tidal.Quality != nil && *snapshot.Tidal.Quality != "" {
			detail += " · " + *snapshot.Tidal.Quality
		}
	case flow.Error != "":
		tone, claim, detail = palette.Failed, "Failed", flow.Error
	case flow.Waiting:
		tone, claim, detail = palette.Becoming, "Waiting", "Waiting for your browser"
	case snapshot.Tidal.Error != "":
		tone, claim, detail = palette.Failed, "Failed", snapshot.Tidal.Error
	case snapshot.Tidal.HasCredentials:
		tone, claim = palette.Attention, "Ready to sign in"
	}
	return p.settingRow(e.Th, "TIDAL", detail, func(gtx layout.Context) layout.Dimensions {
		return accountControl(gtx, e, tone, claim, func(gtx layout.Context) layout.Dimensions {
			if snapshot.Tidal.SignedIn {
				return providerButton(gtx, e, &p.accounts.tidalSignOut, kit.PlaqueDangerLine, "Sign out", func() {
					callProviderSignOut(e, protocol.MethodTidalSignOut)
				})
			}
			if flow.Waiting || flow.URL != "" {
				return p.layoutAuthFlow(gtx, e, model.ProviderTidal, flow, &p.accounts.tidalFlow)
			}
			return providerButton(gtx, e, &p.accounts.tidalSignIn, kit.PlaqueLine, "Sign in", func() {
				act.SignIn(e, model.ProviderTidal)
			})
		})
	})
}

func (p *page) tidalCredentialsRow(e *env.Env) layout.Widget {
	return p.settingRow(e.Th, "TIDAL credentials", "Optional device and PKCE credentials for a custom TIDAL application.", func(gtx layout.Context) layout.Dimensions {
		p.accounts.tidalDeviceID.field.Label, p.accounts.tidalDeviceID.field.Placeholder = "Device ID", "Device ID"
		p.accounts.tidalDeviceSecret.field.Label, p.accounts.tidalDeviceSecret.field.Placeholder = "Device secret", "Device secret"
		p.accounts.tidalDeviceSecret.field.Secret = true
		p.accounts.tidalPKCEID.field.Label, p.accounts.tidalPKCEID.field.Placeholder = "PKCE ID", "PKCE ID"
		p.accounts.tidalSave.Kind, p.accounts.tidalSave.Label = kit.PlaqueLine, "Save credentials"
		if p.accounts.tidalSave.Clicked(gtx) {
			params := protocol.TidalCredentialsParams{
				DeviceID: new(strings.TrimSpace(p.accounts.tidalDeviceID.field.Text())), DeviceSecret: new(strings.TrimSpace(p.accounts.tidalDeviceSecret.field.Text())),
				PKCEID: new(strings.TrimSpace(p.accounts.tidalPKCEID.field.Text())),
			}
			act.Call[model.TidalStatus](e, protocol.MethodTidalSetCredentials, params, func(_ model.TidalStatus, err error) {
				if err != nil {
					toastError(e, err, "Could not save TIDAL credentials")
				}
			})
		}
		return layout.Flex{Axis: layout.Vertical, Alignment: layout.End}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return p.accounts.tidalDeviceID.field.Layout(gtx, e.Th.Theme)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return p.accounts.tidalDeviceSecret.field.Layout(gtx, e.Th.Theme)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return p.accounts.tidalPKCEID.field.Layout(gtx, e.Th.Theme)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return p.accounts.tidalSave.Layout(gtx, e.Th.Theme) }),
		)
	})
}

func providerButton(gtx layout.Context, e *env.Env, button *kit.Plaque, kind kit.PlaqueKind, label string, action func()) layout.Dimensions {
	button.Kind, button.Label = kind, label
	if button.Clicked(gtx) {
		action()
	}
	return button.Layout(gtx, e.Th.Theme)
}

func accountControl(gtx layout.Context, e *env.Env, tone palette.Tone, claim string, action layout.Widget) layout.Dimensions {
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Start}.Layout(gtx,
		layout.Rigid(kit.Claim(e.Th.Theme, tone, claim)),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Rigid(action),
	)
}

func (p *page) layoutAuthFlow(gtx layout.Context, e *env.Env, provider model.Provider, flow state.AuthFlow, controls *authFlowControls) layout.Dimensions {
	controls.open.Kind, controls.open.Label, controls.open.Small = kit.PlaqueQuiet, "Open again", true
	controls.copy.Kind, controls.copy.Label, controls.copy.Small = kit.PlaqueQuiet, "Copy link", true
	controls.cancel.Kind, controls.cancel.Label, controls.cancel.Small = kit.PlaqueLine, "Cancel", true
	if flow.URL != "" && controls.open.Clicked(gtx) {
		act.OpenURL(e, flow.URL)
	}
	if flow.URL != "" && controls.copy.Clicked(gtx) {
		act.CopyText(e, flow.URL, "Link copied")
	}
	if controls.cancel.Clicked(gtx) {
		e.Store.CancelAuthFlow(provider)
	}
	buttons := func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if flow.URL == "" {
					gtx = gtx.Disabled()
				}
				return controls.open.Layout(gtx, e.Th.Theme)
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if flow.URL == "" {
					gtx = gtx.Disabled()
				}
				return controls.copy.Layout(gtx, e.Th.Theme)
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return controls.cancel.Layout(gtx, e.Th.Theme)
			}),
		)
	}
	if flow.Code == "" {
		return buttons(gtx)
	}
	return layout.Flex{Axis: layout.Vertical, Alignment: layout.End}.Layout(gtx,
		layout.Rigid(kit.Text(e.Th.Theme, e.Th.T.Mono, e.Th.R.Fg, flow.Code)),
		layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
		layout.Rigid(buttons),
	)
}

func callProviderSignOut(e *env.Env, method string) {
	act.Call[protocol.SignedOutResult](e, method, protocol.Null{}, func(_ protocol.SignedOutResult, err error) {
		if err != nil {
			toastError(e, err, "Could not sign out")
		}
	})
}

func valueOr(value *string, fallback string) string {
	if value != nil && *value != "" {
		return *value
	}
	return fallback
}
