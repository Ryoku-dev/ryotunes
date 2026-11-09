package settings

import (
	"strings"

	"gioui.org/layout"

	"github.com/ryoku-dev/ryotunes/internal/client/lithic"
	"github.com/ryoku-dev/ryotunes/internal/client/state"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/act"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

func (p *page) layoutProviders(gtx layout.Context, e *env.Env, snapshot state.State) layout.Dimensions {
	return settingRows(gtx,
		p.youtubeRow(e, snapshot),
		p.youtubeIdentitiesRow(e),
		p.spotifyRow(e, snapshot),
		p.soundCloudRow(e, snapshot),
		p.tidalRow(e, snapshot),
		p.tidalCredentialsRow(e),
		p.lastfmRow(e, snapshot),
	)
}

func (p *page) youtubeRow(e *env.Env, snapshot state.State) layout.Widget {
	name := "Not signed in"
	detail := "Sign in to sync your YouTube Music library."
	tone, claim := lithic.Idle, "Not signed in"
	if snapshot.Auth.SignedIn {
		name = valueOr(snapshot.Auth.Name, "YouTube Music")
		detail = valueOr(snapshot.Auth.Handle, "YouTube Music")
		tone, claim = lithic.Alive, "Connected"
	}
	return settingRow(e.Th, "YouTube account", name+" · "+detail, func(gtx layout.Context) layout.Dimensions {
		return accountControl(gtx, e.Th, tone, claim, func(gtx layout.Context) layout.Dimensions {
			if snapshot.Auth.SignedIn {
				if p.providers.youtubeSignOut.Clicked(gtx) {
					callNull(e, protocol.MethodSignOut, protocol.Null{}, "Could not sign out", nil)
				}
				return p.providers.youtubeSignOut.Layout(gtx, e.Th, lithic.DangerLine, nil, "Sign out")
			}
			if p.providers.youtubeSignIn.Clicked(gtx) {
				callNull(e, protocol.MethodSignIn, protocol.Null{}, "Could not sign in", nil)
			}
			return p.providers.youtubeSignIn.Layout(gtx, e.Th, lithic.Gold, nil, "Sign in via browser cookies")
		})
	})
}

func (p *page) youtubeIdentitiesRow(e *env.Env) layout.Widget {
	detail := "Sign in to see the channels on this account."
	if len(p.identities) == 1 {
		detail = "This account has a single channel."
	}
	if len(p.identities) > 1 {
		detail = "Choose which YouTube identity Ryotunes uses."
	}
	return settingRow(e.Th, "YouTube identity", detail, func(gtx layout.Context) layout.Dimensions {
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
		dims := p.providers.identity.Layout(gtx, e.Th, "", labels, selected)
		if changed, ok := p.providers.identity.Changed(gtx); ok {
			identity := p.identities[changed]
			act.Call[model.Account](e, protocol.MethodSwitchAccount, protocol.SwitchAccountParams{SelectionKey: identity.SelectionKey}, func(_ model.Account, err error) {
				if err != nil {
					toastError(e, err, "Could not switch account")
				}
			})
		}
		return dims
	})
}

func (p *page) spotifyRow(e *env.Env, snapshot state.State) layout.Widget {
	tone, claim := lithic.Idle, "Not signed in"
	detail := "Sign in to browse and play Spotify."
	if snapshot.Spotify.Flow == "browser" {
		tone, claim = lithic.Busy, "Waiting"
		detail = "Waiting for your browser…"
		if snapshot.Spotify.URL != "" {
			detail += " " + snapshot.Spotify.URL
		}
	} else if snapshot.Spotify.Error != "" {
		tone, claim, detail = lithic.Bad, "Failed", snapshot.Spotify.Error
	} else if snapshot.Spotify.SignedIn {
		tone, claim = lithic.Alive, "Connected"
		detail = valueOr(snapshot.Spotify.Name, "Spotify")
		if snapshot.Spotify.Premium != nil && !*snapshot.Spotify.Premium {
			detail += " · Premium is required for playback."
		} else {
			detail += " · Premium"
		}
	}
	return settingRow(e.Th, "Spotify", detail, func(gtx layout.Context) layout.Dimensions {
		return accountControl(gtx, e.Th, tone, claim, func(gtx layout.Context) layout.Dimensions {
			if snapshot.Spotify.SignedIn {
				if p.providers.spotifySignOut.Clicked(gtx) {
					callProviderSignOut(e, protocol.MethodSpotifySignOut)
				}
				return p.providers.spotifySignOut.Layout(gtx, e.Th, lithic.DangerLine, nil, "Sign out")
			}
			if p.providers.spotifySignIn.Clicked(gtx) {
				callProviderSignIn(e, protocol.MethodSpotifySignIn)
			}
			return p.providers.spotifySignIn.Layout(gtx, e.Th, lithic.Gold, nil, "Sign in")
		})
	})
}

func (p *page) soundCloudRow(e *env.Env, snapshot state.State) layout.Widget {
	tone, claim := lithic.Idle, "Guest access"
	detail := "No account needed · SoundCloud's public catalogue plays for everyone."
	if snapshot.SoundCloud.Flow == "waiting" {
		tone, claim = lithic.Busy, "Waiting"
		detail = "Finish signing in at soundcloud.com in your browser. Ryotunes picks the session up automatically."
	} else if snapshot.SoundCloud.Flow == "expired" {
		tone, claim = lithic.Attention, "Expired"
		detail = "No SoundCloud sign-in was completed in the browser."
	} else if snapshot.SoundCloud.Error != "" {
		tone, claim, detail = lithic.Bad, "Failed", snapshot.SoundCloud.Error
	} else if snapshot.SoundCloud.SignedIn {
		tone, claim = lithic.Alive, "Connected"
		detail = valueOr(snapshot.SoundCloud.Name, "SoundCloud")
	}
	return settingRow(e.Th, "SoundCloud", detail, func(gtx layout.Context) layout.Dimensions {
		return accountControl(gtx, e.Th, tone, claim, func(gtx layout.Context) layout.Dimensions {
			if snapshot.SoundCloud.SignedIn {
				if p.providers.soundCloudSignOut.Clicked(gtx) {
					callProviderSignOut(e, protocol.MethodSoundCloudSignOut)
				}
				return p.providers.soundCloudSignOut.Layout(gtx, e.Th, lithic.DangerLine, nil, "Sign out")
			}
			if p.providers.soundCloudSignIn.Clicked(gtx) {
				callProviderSignIn(e, protocol.MethodSoundCloudSignIn)
			}
			return p.providers.soundCloudSignIn.Layout(gtx, e.Th, lithic.Gold, nil, "Sign in")
		})
	})
}

func (p *page) tidalRow(e *env.Env, snapshot state.State) layout.Widget {
	tone, claim := lithic.Idle, "Not signed in"
	detail := "TIDAL has no guest catalogue · sign in to browse and play."
	if snapshot.Tidal.Code != "" || snapshot.Tidal.URL != "" {
		tone, claim = lithic.Busy, "Waiting"
		detail = "Enter " + fallback(snapshot.Tidal.Code, "the code") + " at " + fallback(snapshot.Tidal.URL, "link.tidal.com")
	} else if snapshot.Tidal.Error != "" {
		tone, claim, detail = lithic.Bad, "Failed", snapshot.Tidal.Error
	} else if snapshot.Tidal.SignedIn {
		tone, claim = lithic.Alive, "Connected"
		detail = valueOr(snapshot.Tidal.Name, "TIDAL")
		if snapshot.Tidal.Quality != nil && *snapshot.Tidal.Quality != "" {
			detail += " · " + *snapshot.Tidal.Quality
		}
	} else if snapshot.Tidal.HasCredentials {
		tone, claim = lithic.Attention, "Ready to sign in"
	}
	return settingRow(e.Th, "TIDAL", detail, func(gtx layout.Context) layout.Dimensions {
		return accountControl(gtx, e.Th, tone, claim, func(gtx layout.Context) layout.Dimensions {
			if snapshot.Tidal.SignedIn {
				if p.providers.tidalSignOut.Clicked(gtx) {
					callProviderSignOut(e, protocol.MethodTidalSignOut)
				}
				return p.providers.tidalSignOut.Layout(gtx, e.Th, lithic.DangerLine, nil, "Sign out")
			}
			if p.providers.tidalSignIn.Clicked(gtx) {
				callProviderSignIn(e, protocol.MethodTidalSignIn)
			}
			return p.providers.tidalSignIn.Layout(gtx, e.Th, lithic.Gold, nil, "Sign in")
		})
	})
}

func (p *page) tidalCredentialsRow(e *env.Env) layout.Widget {
	p.providers.tidalDeviceSecret.field.Secret = true
	return settingRow(e.Th, "TIDAL credentials", "Optional device and PKCE credentials for a custom TIDAL application.", func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical, Alignment: layout.End, Gap: gtx.Dp(8)}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return p.providers.tidalDeviceID.field.Layout(gtx, e.Th, "Device ID", "Device ID", "", "")
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return p.providers.tidalDeviceSecret.field.Layout(gtx, e.Th, "Device secret", "Device secret", "", "")
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return p.providers.tidalPKCEID.field.Layout(gtx, e.Th, "PKCE ID", "PKCE ID", "", "")
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if p.providers.tidalSave.Clicked(gtx) {
					params := protocol.TidalCredentialsParams{
						DeviceID:     new(strings.TrimSpace(p.providers.tidalDeviceID.field.Text())),
						DeviceSecret: new(strings.TrimSpace(p.providers.tidalDeviceSecret.field.Text())),
						PKCEID:       new(strings.TrimSpace(p.providers.tidalPKCEID.field.Text())),
					}
					act.Call[model.TidalStatus](e, protocol.MethodTidalSetCredentials, params, func(_ model.TidalStatus, err error) {
						if err != nil {
							toastError(e, err, "Could not save TIDAL credentials")
						}
					})
				}
				return p.providers.tidalSave.Layout(gtx, e.Th, lithic.Gold, nil, "Save credentials")
			}),
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
	tone, claim, detail := lithic.Idle, "Not connected", "Connect Last.fm to scrobble what you play."
	if !configured {
		tone, claim, detail = lithic.Attention, "Not configured", "Last.fm credentials are not configured on this daemon."
	} else if connected {
		tone, claim, detail = lithic.Alive, "Connected", valueOr(username, "Last.fm")
	} else if snapshot.Lastfm.Error != nil && *snapshot.Lastfm.Error != "" {
		tone, claim, detail = lithic.Bad, "Failed", *snapshot.Lastfm.Error
	}
	return settingRow(e.Th, "Last.fm", detail, func(gtx layout.Context) layout.Dimensions {
		return accountControl(gtx, e.Th, tone, claim, func(gtx layout.Context) layout.Dimensions {
			if connected {
				if p.providers.lastfmDisconnect.Clicked(gtx) {
					callNull(e, protocol.MethodLastfmDisconnect, protocol.Null{}, "Could not disconnect Last.fm", nil)
				}
				return p.providers.lastfmDisconnect.Layout(gtx, e.Th, lithic.DangerLine, nil, "Disconnect")
			}
			buttonGTX := gtx
			if !configured {
				buttonGTX = buttonGTX.Disabled()
			}
			if p.providers.lastfmConnect.Clicked(gtx) && configured {
				callNull(e, protocol.MethodLastfmConnect, protocol.Null{}, "Could not connect Last.fm", nil)
			}
			return p.providers.lastfmConnect.Layout(buttonGTX, e.Th, lithic.Gold, nil, "Connect")
		})
	})
}

func accountControl(gtx layout.Context, th *lithic.Theme, tone lithic.Tone, claim string, action layout.Widget) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical, Alignment: layout.End, Gap: gtx.Dp(8)}.Layout(gtx,
		layout.Rigid(lithic.Claim(th, tone, claim)),
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
