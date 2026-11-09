package settings

import (
	"strings"

	"gioui.org/layout"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryotunes/internal/client/state"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/act"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

var (
	qualityLabels = []string{"Low", "Auto", "High"}
	qualityValues = []string{string(model.QualityLow), string(model.QualityAuto), string(model.QualityHigh)}
)

func (p *page) layoutPlayback(gtx layout.Context, e *env.Env, snapshot state.State) layout.Dimensions {
	historyDetail := "Sign in to YouTube Music to register watch history."
	if snapshot.Auth.SignedIn {
		historyDetail = "Register what you play so recommendations and listening history can improve."
	}
	quality := settingString(snapshot.Settings, model.SettingQuality, string(model.QualityHigh))
	rows := []layout.Widget{
		p.settingRow(e.Th, "Audio quality", "Preferred stream quality. Changing it clears cached stream URLs.", func(gtx layout.Context) layout.Dimensions {
			selected := indexOf(qualityValues, quality, 2)
			dims := p.playback.quality.Layout(gtx, e.Th, e.Overlays, qualityLabels, selected)
			if changed, ok := p.playback.quality.Changed(); ok {
				setSetting(e, model.SettingQuality, qualityValues[changed], "", func() {
					rotate := false
					act.Call[protocol.ClearCachesResult](e, protocol.MethodClearCaches, protocol.ClearCachesParams{Rotate: &rotate}, func(_ protocol.ClearCachesResult, err error) {
						if err != nil {
							toastError(e, err, "Could not clear cached streams")
							return
						}
						e.Toast("Audio quality updated", env.ToastSuccess)
					})
				})
			}
			return dims
		}),
		p.toggleSettingRow(e, &p.playback.autoplay, snapshot.Settings, model.SettingAutoplay, true,
			"Autoplay", "Continue with related music when the queue reaches its end."),
		p.toggleSettingRow(e, &p.playback.duplicates, snapshot.Settings, model.SettingPreventDuplicates, false,
			"Prevent duplicate tracks in queue", "Keep the same track from being added to the queue more than once."),
		p.settingRow(e.Th, "Stop after current", "Stop when the current track finishes. This is a one-shot playback choice.", func(gtx layout.Context) layout.Dimensions {
			p.playback.stopAfter.Value = snapshot.Playback.StopAfterCurrent
			dims := p.playback.stopAfter.Layout(gtx, e.Th.Theme)
			if p.playback.stopAfter.Changed() {
				callNull(e, protocol.MethodSetStopAfterCurrent, protocol.SetStopAfterCurrentParams{Enabled: p.playback.stopAfter.Value}, "Could not update playback", nil)
			}
			return dims
		}),
		p.toggleSettingRow(e, &p.playback.history, snapshot.Settings, model.SettingEnableHistory, true,
			"Watch history", historyDetail),
		p.toggleSettingRow(e, &p.appearance.tray, snapshot.Settings, model.SettingCloseToTray, true,
			"Close to tray", "Closing the window keeps music playing in the background."),
		p.toggleSettingRow(e, &p.appearance.autostart, snapshot.Settings, model.SettingAutostart, false,
			"Start on login", "Launch Ryotunes automatically when you log in."),
	}

	disabled := settingSet(snapshot.Settings, model.SettingDisabledStreamClients)
	if len(p.streamClients) > 0 {
		rows = append(rows, p.settingRow(e.Th, "Stream clients", "Choose which YouTube playback clients may resolve streams. At least one should remain enabled.", nil))
	}
	for _, client := range p.streamClients {
		client := client
		toggle := p.playback.clients[client]
		if toggle == nil {
			toggle = new(kit.Toggle)
			p.playback.clients[client] = toggle
		}
		rows = append(rows, p.settingRow(e.Th, streamClientLabel(client), "YouTube stream resolver client.", func(gtx layout.Context) layout.Dimensions {
			toggle.Value = !disabled[string(client)]
			dims := toggle.Layout(gtx, e.Th.Theme)
			if toggle.Changed() {
				next := make([]string, 0, len(p.streamClients))
				for _, candidate := range p.streamClients {
					isDisabled := disabled[string(candidate)]
					if candidate == client {
						isDisabled = !toggle.Value
					}
					if isDisabled {
						next = append(next, string(candidate))
					}
				}
				setSetting(e, model.SettingDisabledStreamClients, strings.Join(next, ","), "", nil)
			}
			return dims
		}))
	}
	return p.settingRows(gtx, rows...)
}

func (p *page) layoutSound(gtx layout.Context, e *env.Env, snapshot state.State) layout.Dimensions {
	return p.settingRows(gtx, p.settingRow(e.Th, "Playback sound", "Shape tempo, pitch, reverb, bass and stereo width. Current preset: "+snapshot.AudioFx.PresetName()+".", func(gtx layout.Context) layout.Dimensions {
		p.sound.Kind, p.sound.Icon, p.sound.Label = kit.PlaqueLine, look.Icon.Tune, "Open sound"
		dims := p.sound.Layout(gtx, e.Th.Theme)
		if p.sound.Clicked(gtx) {
			e.Nav.OpenSound(e.Overlays.AnchorOf(&p.sound.Clickable, dims.Size))
		}
		return dims
	}))
}

func (p *page) layoutLyrics(gtx layout.Context, e *env.Env, snapshot state.State) layout.Dimensions {
	return p.settingRows(gtx,
		p.toggleSettingRow(e, &p.lyrics.wordByWord, snapshot.Settings, model.SettingLyricsBoidu, false,
			"Word-by-word lyrics", "Use word timing when the lyrics service provides it."),
		p.toggleSettingRow(e, &p.lyrics.romanize, snapshot.Settings, model.SettingLyricsRomanize, false,
			"Romanized lyrics", "Show romanized readings when the lyrics service provides them."))
}

func (p *page) toggleSettingRow(e *env.Env, toggle *kit.Toggle, settings model.SettingsSnapshot, key model.SettingKey, fallback bool, title, detail string) layout.Widget {
	return p.settingRow(e.Th, title, detail, func(gtx layout.Context) layout.Dimensions {
		toggle.Value = settingBool(settings, key, fallback)
		dims := toggle.Layout(gtx, e.Th.Theme)
		if toggle.Changed() {
			setSetting(e, key, boolString(toggle.Value), "", nil)
		}
		return dims
	})
}

func streamClientLabel(client model.StreamClient) string {
	words := strings.Fields(strings.ReplaceAll(strings.ToLower(string(client)), "_", " "))
	for i, word := range words {
		switch word {
		case "web":
			words[i] = "Web"
		case "tvhtml5":
			words[i] = "TV HTML5"
		case "vr":
			words[i] = "VR"
		default:
			words[i] = strings.ToUpper(word[:1]) + word[1:]
		}
	}
	return strings.Join(words, " ")
}
