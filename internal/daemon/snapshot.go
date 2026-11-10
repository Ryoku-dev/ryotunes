package daemon

import (
	"github.com/ryoku-dev/ryotunes/internal/eq"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

// Snapshot returns the complete state replayed to every subscriber. It answers
// from memory: the start-up browser re-read announces its result as events.
func (a *App) Snapshot() protocol.Snapshot {
	var out protocol.Snapshot
	if a.Queue != nil {
		out.Playback = a.Queue.Playback()
		out.Queue = a.Queue.Snapshot()
	}
	if a.Store != nil {
		out.Settings = a.Store.SettingsSnapshot()
		out.Personal = a.Store.Personal()
	}
	if a.Player != nil {
		out.AudioFx = a.Player.Fx()
		out.Equalizer = a.Player.Equalizer()
	} else {
		out.AudioFx = model.DefaultAudioFx()
		out.Equalizer = eq.Default()
	}
	a.mu.Lock()
	out.Provider = model.Provider(a.provider)
	a.mu.Unlock()
	if out.Provider == "" {
		out.Provider = model.ProviderYouTube
	}
	if a.YouTube != nil {
		out.Auth = a.YouTube.CurrentAccount()
	}
	if a.Spotify != nil {
		out.Spotify = a.Spotify.Status()
	}
	if a.SoundCloud != nil {
		out.SoundCloud = a.SoundCloud.Status()
	}
	if a.Tidal != nil {
		out.Tidal = a.Tidal.Status()
	}
	if a.Downloads != nil {
		out.Downloads = a.Downloads.Snapshot()
	}
	if a.LT != nil {
		out.LT = a.LT.State()
	}
	return out
}
