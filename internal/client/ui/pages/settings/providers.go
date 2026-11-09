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
	return settingRows(gtx,
		p.youtubeRow(e, snapshot),
		p.youtubeIdentitiesRow(e),
		p.spotifyRow(e, snapshot),
		p.soundCloudRow(e, snapshot),
		p.tidalRow(e, snapshot),
		p.tidalCredentialsRow(e),
	)
}

func (p *page) youtubeRow(e *env.Env, snapshot state.State) layout.Widget {
	detail, tone, claim := "Sign in to sync your YouTube Music library.", palette.Idle, "Not signed in"
	if snapshot.Auth.SignedIn {
		detail = valueOr(snapshot.Auth.Name, "YouTube Music")
		if handle := valueOr(snapshot.Auth.Handle, ""); handle != "" {
			detail += " · " + handle
		}
		tone, claim = palette.Alive, "Connected"
	}
	return settingRow(e.Th, "YouTube Music", detail, func(gtx layout.Context) layout.Dimensions {
		return accountControl(gtx, e, tone, claim, func(gtx layout.Context) layout.Dimensions {
			if snapshot.Auth.SignedIn {
				p.accounts.youtubeSignOut.Kind, p.accounts.youtubeSignOut.Label = kit.PlaqueDangerLine, "Sign out"
				if p.accounts.youtubeSignOut.Clicked(gtx) {
					callNull(e, protocol.MethodSignOut, protocol.Null{}, "Could not sign out", nil)
				}
				return p.accounts.youtubeSignOut.Layout(gtx, e.Th.Theme)
			}
			p.accounts.youtubeSignIn.Kind, p.accounts.youtubeSignIn.Label = kit.PlaqueLine, "Sign in with Google"
			if p.accounts.youtubeSignIn.Clicked(gtx) {
				callNull(e, protocol.MethodSignIn, protocol.Null{}, "Could not sign in", nil)
			}
			return p.accounts.youtubeSignIn.Layout(gtx, e.Th.Theme)
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
	return settingRow(e.Th, "YouTube channel", detail, func(gtx layout.Context) layout.Dimensions {
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
	tone, claim, detail := palette.Idle, "Not signed in", "Sign in to browse and play Spotify."
	if snapshot.Spotify.Flow == "browser" {
		tone, claim, detail = palette.Becoming, "Waiting", "Waiting for your browser…"
	} else if snapshot.Spotify.Error != "" {
		tone, claim, detail = palette.Failed, "Failed", snapshot.Spotify.Error
	} else if snapshot.Spotify.SignedIn {
		tone, claim = palette.Alive, "Connected"
		detail = valueOr(snapshot.Spotify.Name, "Spotify")
		if snapshot.Spotify.Premium != nil && !*snapshot.Spotify.Premium {
			detail += " · Premium is required for playback."
		} else {
			detail += " · Premium"
		}
	}
	return settingRow(e.Th, "Spotify", detail, func(gtx layout.Context) layout.Dimensions {
		return accountControl(gtx, e, tone, claim, func(gtx layout.Context) layout.Dimensions {
			if snapshot.Spotify.SignedIn {
				return providerButton(gtx, e, &p.accounts.spotifySignOut, kit.PlaqueDangerLine, "Sign out", func() { callProviderSignOut(e, protocol.MethodSpotifySignOut) })
			}
			return providerButton(gtx, e, &p.accounts.spotifySignIn, kit.PlaqueLine, "Sign in", func() { callProviderSignIn(e, protocol.MethodSpotifySignIn) })
		})
	})
}

func (p *page) soundCloudRow(e *env.Env, snapshot state.State) layout.Widget {
	tone, claim := palette.Idle, "Guest access"
	detail := "No account needed · SoundCloud's public catalogue plays for everyone."
	if snapshot.SoundCloud.Flow == "waiting" {
		tone, claim, detail = palette.Becoming, "Waiting", "Finish signing in at soundcloud.com."
	} else if snapshot.SoundCloud.Flow == "expired" {
		tone, claim, detail = palette.Attention, "Expired", "No SoundCloud sign-in was completed."
	} else if snapshot.SoundCloud.Error != "" {
		tone, claim, detail = palette.Failed, "Failed", snapshot.SoundCloud.Error
	} else if snapshot.SoundCloud.SignedIn {
		tone, claim, detail = palette.Alive, "Connected", valueOr(snapshot.SoundCloud.Name, "SoundCloud")
	}
	return settingRow(e.Th, "SoundCloud", detail, func(gtx layout.Context) layout.Dimensions {
		return accountControl(gtx, e, tone, claim, func(gtx layout.Context) layout.Dimensions {
			if snapshot.SoundCloud.SignedIn {
				return providerButton(gtx, e, &p.accounts.soundCloudSignOut, kit.PlaqueDangerLine, "Sign out", func() { callProviderSignOut(e, protocol.MethodSoundCloudSignOut) })
			}
			return providerButton(gtx, e, &p.accounts.soundCloudSignIn, kit.PlaqueLine, "Sign in", func() { callProviderSignIn(e, protocol.MethodSoundCloudSignIn) })
		})
	})
}

func (p *page) tidalRow(e *env.Env, snapshot state.State) layout.Widget {
	tone, claim, detail := palette.Idle, "Not signed in", "TIDAL has no guest catalogue · sign in to browse and play."
	if snapshot.Tidal.Code != "" || snapshot.Tidal.URL != "" {
		tone, claim = palette.Becoming, "Waiting"
		detail = "Enter " + fallback(snapshot.Tidal.Code, "the code") + " at " + fallback(snapshot.Tidal.URL, "link.tidal.com")
	} else if snapshot.Tidal.Error != "" {
		tone, claim, detail = palette.Failed, "Failed", snapshot.Tidal.Error
	} else if snapshot.Tidal.SignedIn {
		tone, claim, detail = palette.Alive, "Connected", valueOr(snapshot.Tidal.Name, "TIDAL")
		if snapshot.Tidal.Quality != nil && *snapshot.Tidal.Quality != "" {
			detail += " · " + *snapshot.Tidal.Quality
		}
	} else if snapshot.Tidal.HasCredentials {
		tone, claim = palette.Attention, "Ready to sign in"
	}
	return settingRow(e.Th, "TIDAL", detail, func(gtx layout.Context) layout.Dimensions {
		return accountControl(gtx, e, tone, claim, func(gtx layout.Context) layout.Dimensions {
			if snapshot.Tidal.SignedIn {
				return providerButton(gtx, e, &p.accounts.tidalSignOut, kit.PlaqueDangerLine, "Sign out", func() { callProviderSignOut(e, protocol.MethodTidalSignOut) })
			}
			return providerButton(gtx, e, &p.accounts.tidalSignIn, kit.PlaqueLine, "Sign in", func() { callProviderSignIn(e, protocol.MethodTidalSignIn) })
		})
	})
}

func (p *page) tidalCredentialsRow(e *env.Env) layout.Widget {
	return settingRow(e.Th, "TIDAL credentials", "Optional device and PKCE credentials for a custom TIDAL application.", func(gtx layout.Context) layout.Dimensions {
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
	return layout.Flex{Axis: layout.Vertical, Alignment: layout.End}.Layout(gtx,
		layout.Rigid(kit.Claim(e.Th.Theme, tone, claim)),
		layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
		layout.Rigid(action),
	)
}

func callProviderSignIn(e *env.Env, method string) {
	act.Call[protocol.StartedResult](e, method, protocol.Null{}, func(_ protocol.StartedResult, err error) {
		if err != nil {
			toastError(e, err, "Could not sign in")
		}
	})
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

func fallback(value, other string) string {
	if value != "" {
		return value
	}
	return other
}
