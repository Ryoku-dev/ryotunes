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
		settingRow(e.Th, "Audio quality", "Preferred stream quality when resolving a track. Changing it clears cached URLs.", func(gtx layout.Context) layout.Dimensions {
			selected := indexOf(qualityValues, quality, 2)
			dims := p.playback.qualityPicker.Layout(gtx, e.Th, "", qualityLabels, selected)
			if changed, ok := p.playback.qualityPicker.Changed(gtx); ok {
				setSetting(e, model.SettingQuality, qualityValues[changed], "", func() {
					act.Call[protocol.ClearCachesResult](e, protocol.MethodClearCaches, protocol.ClearCachesParams{Rotate: new(false)}, func(_ protocol.ClearCachesResult, _ error) {
						e.Toast("Audio quality updated", env.ToastSuccess)
					})
				})
			}
			return dims
		}),
		toggleSettingRow(e, &p.playback.autoplay, snapshot.Settings, model.SettingAutoplay, true,
			"Autoplay", "Continue with related music when the queue reaches its end."),
		toggleSettingRow(e, &p.playback.duplicates, snapshot.Settings, model.SettingPreventDuplicates, false,
			"Prevent duplicate tracks", "Keep the same track from being added to the queue more than once."),
		settingRow(e.Th, "Stop after current", "Stop when the current track finishes. This is a one-shot playback choice.", func(gtx layout.Context) layout.Dimensions {
			p.playback.stopAfter.Value = snapshot.Playback.StopAfterCurrent
			before := p.playback.stopAfter.Value
			dims := p.playback.stopAfter.Layout(gtx, e.Th, "")
			if p.playback.stopAfter.Value != before {
				callNull(e, protocol.MethodSetStopAfterCurrent, protocol.SetStopAfterCurrentParams{Enabled: p.playback.stopAfter.Value}, "Could not update", nil)
			}
			return dims
		}),
		toggleSettingRow(e, &p.playback.history, snapshot.Settings, model.SettingEnableHistory, true,
			"Enable history", historyDetail),
		toggleSettingRow(e, &p.playback.wordByWord, snapshot.Settings, model.SettingLyricsBoidu, false,
			"Word-by-word lyrics", "Use word timing when the lyrics service provides it."),
	}

	disabled := settingSet(snapshot.Settings, model.SettingDisabledStreamClients)
	if len(p.streamClients) > 0 {
		rows = append(rows, settingRow(e.Th, "Stream clients", "Choose which YouTube playback clients may resolve streams. At least one should remain enabled.", nil))
	}
	for _, client := range p.streamClients {
		client := client
		toggle := p.playback.clients[client]
		if toggle == nil {
			toggle = &lithic.Toggle{}
			p.playback.clients[client] = toggle
		}
		enabled := !disabled[string(client)]
		rows = append(rows, settingRow(e.Th, streamClientLabel(client), "YouTube stream resolver client.", func(gtx layout.Context) layout.Dimensions {
			toggle.Value = enabled
			before := toggle.Value
			dims := toggle.Layout(gtx, e.Th, "")
			if toggle.Value != before {
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
	return settingRows(gtx, rows...)
}

func toggleSettingRow(e *env.Env, toggle *lithic.Toggle, settings model.SettingsSnapshot, key model.SettingKey, fallback bool, title, detail string) layout.Widget {
	return settingRow(e.Th, title, detail, func(gtx layout.Context) layout.Dimensions {
		toggle.Value = settingBool(settings, key, fallback)
		before := toggle.Value
		dims := toggle.Layout(gtx, e.Th, "")
		if toggle.Value != before {
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
