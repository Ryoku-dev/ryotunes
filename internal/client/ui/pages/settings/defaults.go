package settings

import (
	"strconv"

	"github.com/ryoku-dev/ryotunes/internal/client/state"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/model"
)

// settingDefaults are the values the page shows for daemon settings that
// were never written; a row whose value differs reads "Changed" and offers a
// reset to it, and the page head counts them.
var settingDefaults = map[model.SettingKey]string{
	model.SettingQuality:               string(model.QualityHigh),
	model.SettingAutoplay:              "true",
	model.SettingPreventDuplicates:     "false",
	model.SettingEnableHistory:         "true",
	model.SettingCloseToTray:           "true",
	model.SettingAutostart:             "false",
	model.SettingLyricsBoidu:           "true",
	model.SettingLyricsRomanize:        "false",
	model.SettingLowResourceMode:       "false",
	model.SettingDiscordRPC:            "false",
	model.SettingUIScale:               "100",
	model.SettingDisabledStreamClients: "",
}

// settingDefaultKeys orders settingDefaults for counting.
var settingDefaultKeys = []model.SettingKey{
	model.SettingQuality, model.SettingAutoplay, model.SettingPreventDuplicates, model.SettingEnableHistory,
	model.SettingCloseToTray, model.SettingAutostart, model.SettingLyricsBoidu, model.SettingLyricsRomanize,
	model.SettingLowResourceMode, model.SettingDiscordRPC, model.SettingUIScale, model.SettingDisabledStreamClients,
}

// value is a daemon setting as the page shows it: a write still on its way,
// else the daemon's value, else its default.
func (p *page) value(settings model.SettingsSnapshot, key model.SettingKey) string {
	if pending, ok := p.pendingSettings[key]; ok {
		return pending
	}
	if value, ok := settings[string(key)]; ok {
		return value
	}
	return settingDefaults[key]
}

// valueBool is value read as a boolean, the default when it does not parse.
func (p *page) valueBool(settings model.SettingsSnapshot, key model.SettingKey) bool {
	value, err := strconv.ParseBool(p.value(settings, key))
	if err != nil {
		return settingDefaults[key] == "true"
	}
	return value
}

// settingChanged reports whether a daemon setting stands away from its
// default.
func (p *page) settingChanged(settings model.SettingsSnapshot, key model.SettingKey) bool {
	return p.value(settings, key) != settingDefaults[key]
}

// write saves a daemon setting and shows it at once: the page reads the
// written value until the daemon answers, so a toggle moves once and stays.
func (p *page) write(e *env.Env, key model.SettingKey, value, success string, done func()) {
	p.pendingSettings[key] = value
	setSettingThen(e, key, value, success, func(ok bool) {
		if current, still := p.pendingSettings[key]; still && current == value {
			delete(p.pendingSettings, key)
		}
		if ok && done != nil {
			done()
		}
	})
}

// forgetSettledWrites drops pending writes the store already shows.
func (p *page) forgetSettledWrites(snapshot state.State) {
	for key, value := range p.pendingSettings {
		if snapshot.Settings[string(key)] == value {
			delete(p.pendingSettings, key)
		}
	}
}

// clientChanges are the client preferences that can stand away from their
// defaults (each missing preference is its default).
var clientChanges = []func(state.ClientPreferences) bool{
	densityChanged, visualizerChanged, visualizerStyleChanged, waveChanged,
	accentChanged, coverTintChanged, wordByWordChanged, openFloatingChanged,
}

func densityChanged(p state.ClientPreferences) bool {
	return p.Density != "" && p.Density != "comfortable"
}

func visualizerChanged(p state.ClientPreferences) bool      { return !p.VisualizerOn() }
func visualizerStyleChanged(p state.ClientPreferences) bool { return p.Style() != state.VisualizerBars }
func waveChanged(p state.ClientPreferences) bool            { return !p.WaveOn() }
func accentChanged(p state.ClientPreferences) bool          { return p.AccentMode() != state.AccentArt }
func coverTintChanged(p state.ClientPreferences) bool       { return p.CoverTintOn() }
func wordByWordChanged(p state.ClientPreferences) bool      { return !p.WordByWordOn() }
func openFloatingChanged(p state.ClientPreferences) bool    { return !p.OpenFloatingOn() }

// changedCount is how many settings stand away from their defaults, daemon
// and client together, for the page head.
func (p *page) changedCount(snapshot state.State) int {
	count := 0
	for _, key := range settingDefaultKeys {
		if p.settingChanged(snapshot.Settings, key) {
			count++
		}
	}
	for _, changed := range clientChanges {
		if changed(snapshot.Client) {
			count++
		}
	}
	return count
}

// resettableClient are the client preferences a row reset can move: same
// compares two readings as the page shows them, take copies the stored field.
var resettableClient = []struct {
	same func(a, b state.ClientPreferences) bool
	take func(to *state.ClientPreferences, from state.ClientPreferences)
}{
	{func(a, b state.ClientPreferences) bool { return a.Density == b.Density }, func(to *state.ClientPreferences, from state.ClientPreferences) { to.Density = from.Density }},
	{func(a, b state.ClientPreferences) bool { return a.VisualizerOn() == b.VisualizerOn() }, func(to *state.ClientPreferences, from state.ClientPreferences) { to.Visualizer = from.Visualizer }},
	{func(a, b state.ClientPreferences) bool { return a.Style() == b.Style() }, func(to *state.ClientPreferences, from state.ClientPreferences) {
		to.VisualizerStyle = from.VisualizerStyle
	}},
	{func(a, b state.ClientPreferences) bool { return a.WaveOn() == b.WaveOn() }, func(to *state.ClientPreferences, from state.ClientPreferences) { to.WaveProgress = from.WaveProgress }},
	{func(a, b state.ClientPreferences) bool { return a.AccentMode() == b.AccentMode() }, func(to *state.ClientPreferences, from state.ClientPreferences) { to.Accent = from.Accent }},
	{func(a, b state.ClientPreferences) bool { return a.CoverTintOn() == b.CoverTintOn() }, func(to *state.ClientPreferences, from state.ClientPreferences) { to.CoverTint = from.CoverTint }},
	{func(a, b state.ClientPreferences) bool { return a.WordByWordOn() == b.WordByWordOn() }, func(to *state.ClientPreferences, from state.ClientPreferences) { to.WordByWord = from.WordByWord }},
	{func(a, b state.ClientPreferences) bool { return a.OpenFloatingOn() == b.OpenFloatingOn() }, func(to *state.ClientPreferences, from state.ClientPreferences) { to.OpenFloating = from.OpenFloating }},
}

// resetWithUndo puts a row back to its default and offers the Hub's Undo
// notice. Undo returns only what the reset itself moved, and only where
// nobody has changed it since: a setting changed after the reset stays as
// it was set. Daemon settings are written back (a write still on its way
// counts as written), client preferences restored field by field, so
// nothing else in client.json moves.
func (p *page) resetWithUndo(title string, reset func()) {
	e := p.env
	if e == nil || e.Store == nil {
		reset()
		return
	}
	before := e.Store.Snapshot()
	prior := make([]string, len(settingDefaultKeys))
	for index, key := range settingDefaultKeys {
		prior[index] = p.value(before.Settings, key)
	}
	reset()
	if e.Undoable == nil {
		return
	}
	after := e.Store.Snapshot()
	settled := make([]string, len(settingDefaultKeys))
	for index, key := range settingDefaultKeys {
		settled[index] = p.value(after.Settings, key)
	}
	e.Undoable(title+" is back to its default.", func() {
		now := e.Store.Snapshot()
		for index, key := range settingDefaultKeys {
			if prior[index] == settled[index] || p.value(now.Settings, key) != settled[index] {
				continue
			}
			if key == model.SettingQuality {
				p.setQuality(e, prior[index])
				continue
			}
			p.write(e, key, prior[index], "", nil)
		}
		restored, moved := now.Client, false
		for _, field := range resettableClient {
			if field.same(before.Client, after.Client) || !field.same(now.Client, after.Client) {
				continue
			}
			field.take(&restored, before.Client)
			moved = true
		}
		if !moved {
			return
		}
		if restored.Density != now.Client.Density {
			setDensity(e, restored, restored.Density)
			return
		}
		saveClientPreferences(e, restored)
	})
}
