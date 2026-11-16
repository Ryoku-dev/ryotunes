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
	quality := p.value(snapshot.Settings, model.SettingQuality)
	rows := []layout.Widget{
		p.row(e.Th, rowSpec{
			title: "Audio quality", detail: "Preferred stream quality. Changing it clears cached stream URLs.",
			changed: p.settingChanged(snapshot.Settings, model.SettingQuality),
			reset:   func() { p.setQuality(e, settingDefaults[model.SettingQuality]) },
			control: func(gtx layout.Context) layout.Dimensions {
				selected := indexOf(qualityValues, quality, 2)
				dims := p.playback.quality.Layout(gtx, e.Th, e.Overlays, qualityLabels, selected)
				if changed, ok := p.playback.quality.Changed(); ok {
					p.setQuality(e, qualityValues[changed])
				}
				return dims
			},
		}),
		p.toggleSettingRow(e, &p.playback.autoplay, snapshot.Settings, model.SettingAutoplay,
			"Autoplay", "Continue with related music when the queue reaches its end."),
		p.toggleSettingRow(e, &p.playback.duplicates, snapshot.Settings, model.SettingPreventDuplicates,
			"Prevent duplicate tracks in queue", "Keep the same track from being added to the queue more than once."),
		p.toggleRow(e.Th, &p.playback.stopAfter, snapshot.Playback.StopAfterCurrent, false, rowSpec{
			title: "Stop after current", detail: "Stop when the current track finishes. This is a one-shot playback choice.",
		}, func(on bool) {
			callNull(e, protocol.MethodSetStopAfterCurrent, protocol.SetStopAfterCurrentParams{Enabled: on}, "Could not update playback", nil)
		}),
		p.toggleSettingRow(e, &p.playback.history, snapshot.Settings, model.SettingEnableHistory,
			"Watch history", historyDetail),
		p.toggleSettingRow(e, &p.appearance.tray, snapshot.Settings, model.SettingCloseToTray,
			"Close to tray", "Closing the window keeps music playing in the background."),
		p.toggleSettingRow(e, &p.appearance.autostart, snapshot.Settings, model.SettingAutostart,
			"Start on login", "Launch Ryotunes automatically when you log in."),
	}
	if len(p.streamClients) > 0 {
		rows = append(rows, func(gtx layout.Context) layout.Dimensions {
			return p.playback.more.layout(gtx, e.Th, "More options", func(gtx layout.Context) layout.Dimensions {
				return stackRows(gtx, p.streamClientRows(e, snapshot))
			})
		})
	}
	return p.settingRows(gtx, rows...)
}

// setQuality writes the stream quality and clears the cached stream links
// resolved at the old one.
func (p *page) setQuality(e *env.Env, value string) {
	p.write(e, model.SettingQuality, value, "", func() {
		rotate := false
		act.Call[protocol.ClearCachesResult](e, protocol.MethodClearCaches, protocol.ClearCachesParams{Rotate: &rotate}, func(_ protocol.ClearCachesResult, err error) {
			if err != nil {
				toastError(e, err, "Could not clear cached streams")
				return
			}
			e.Toast("Audio quality updated.", env.ToastSuccess)
		})
	})
}

// streamClientRows are the YouTube stream resolver clients, each a toggle,
// under Playback's More options.
func (p *page) streamClientRows(e *env.Env, snapshot state.State) []layout.Widget {
	disabled := listSet(p.value(snapshot.Settings, model.SettingDisabledStreamClients))
	rows := make([]layout.Widget, 0, len(p.streamClients)+1)
	rows = append(rows, p.row(e.Th, rowSpec{
		title:   "Stream clients",
		detail:  "Choose which YouTube playback clients may resolve streams. At least one should remain enabled.",
		changed: len(disabled) > 0,
		reset: func() {
			p.write(e, model.SettingDisabledStreamClients, "", "", nil)
		},
	}))
	for _, client := range p.streamClients {
		toggle := p.playback.clients[client]
		if toggle == nil {
			toggle = new(kit.Toggle)
			p.playback.clients[client] = toggle
		}
		rows = append(rows, p.toggleRow(e.Th, toggle, !disabled[string(client)], false, rowSpec{
			title: streamClientLabel(client), detail: "YouTube stream resolver client.",
		}, func(on bool) {
			next := make([]string, 0, len(p.streamClients))
			for _, candidate := range p.streamClients {
				off := disabled[string(candidate)]
				if candidate == client {
					off = !on
				}
				if off {
					next = append(next, string(candidate))
				}
			}
			p.write(e, model.SettingDisabledStreamClients, strings.Join(next, ","), "", nil)
		}))
	}
	return rows
}

func (p *page) layoutSound(gtx layout.Context, e *env.Env, snapshot state.State) layout.Dimensions {
	return p.settingRows(gtx,
		p.settingRow(e.Th, "Playback sound", "Shape tempo, pitch, reverb, bass and stereo width. Current preset: "+snapshot.AudioFx.PresetName()+".", func(gtx layout.Context) layout.Dimensions {
			p.sound.open.Kind, p.sound.open.Small, p.sound.open.Icon, p.sound.open.Label = kit.PlaqueQuiet, true, look.Icon.Tune, "Open sound"
			dims := p.sound.open.Layout(gtx, e.Th.Theme)
			if p.sound.open.Clicked(gtx) {
				e.Nav.OpenSound(e.Overlays.AnchorOf(&p.sound.open.Clickable, dims.Size))
			}
			return dims
		}),
		p.row(e.Th, rowSpec{
			title: "Equalizer", detail: equalizerProof(snapshot.Equalizer),
			activate: func() { e.Nav.Push(env.Route{Kind: env.RouteEqualizer}) },
			control: func(gtx layout.Context) layout.Dimensions {
				p.sound.equalizer.Kind, p.sound.equalizer.Small, p.sound.equalizer.Icon, p.sound.equalizer.Label = kit.PlaqueQuiet, true, look.Icon.Equalizer, "Open equalizer"
				if p.sound.equalizer.Clicked(gtx) {
					e.Nav.Push(env.Route{Kind: env.RouteEqualizer})
				}
				return p.sound.equalizer.Layout(gtx, e.Th.Theme)
			},
		}))
}

func equalizerProof(equalizer model.Equalizer) string {
	if !equalizer.Enabled {
		return "Bypassed"
	}
	preset := strings.TrimSpace(equalizer.Preset)
	if preset == "" {
		preset = "Custom"
	}
	return "On · " + preset
}

func (p *page) layoutLyrics(gtx layout.Context, e *env.Env, snapshot state.State) layout.Dimensions {
	preferences := snapshot.Client
	return p.settingRows(gtx,
		p.toggleRow(e.Th, &p.lyrics.wordByWord, preferences.WordByWordOn(), false, rowSpec{
			title: "Word by word", detail: "Highlight each word as it's sung, estimated when the lyrics only time each line",
			changed: wordByWordChanged(preferences),
			reset: func() {
				next := e.Store.Snapshot().Client
				next.WordByWord = nil
				saveClientPreferences(e, next)
			},
		}, func(on bool) {
			next := e.Store.Snapshot().Client
			next.WordByWord = &on
			saveClientPreferences(e, next)
		}),
		p.toggleSettingRow(e, &p.lyrics.timedSource, snapshot.Settings, model.SettingLyricsBoidu,
			"Word-timed lyrics", "Ask Boidu first for lyrics timed word by word"),
		p.toggleSettingRow(e, &p.lyrics.romanize, snapshot.Settings, model.SettingLyricsRomanize,
			"Romanized lyrics", "Show romanized readings when the lyrics service provides them."))
}

// toggleSettingRow is a toggle row for a daemon setting: it reads the value
// (a write on its way first), marks it changed against its default and
// resets to it.
func (p *page) toggleSettingRow(e *env.Env, toggle *kit.Toggle, settings model.SettingsSnapshot, key model.SettingKey, title, detail string) layout.Widget {
	return p.toggleRow(e.Th, toggle, p.valueBool(settings, key), false, rowSpec{
		title: title, detail: detail,
		changed: p.settingChanged(settings, key),
		reset:   func() { p.write(e, key, settingDefaults[key], "", nil) },
	}, func(on bool) {
		p.write(e, key, boolString(on), "", nil)
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
