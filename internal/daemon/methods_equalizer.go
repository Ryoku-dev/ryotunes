package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ryoku-dev/ryotunes/internal/eq"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

const (
	equalizerStateKey     = "equalizer_json"
	equalizerPresetsKey   = "equalizer_presets_json"
	equalizerStateVersion = 1
)

type equalizerPlayer interface {
	SetEqualizer(model.Equalizer) error
	Equalizer() model.Equalizer
}

type persistedEqualizer struct {
	Version int             `json:"version"`
	Active  model.Equalizer `json:"active"`
}

type persistedEqualizerPresets struct {
	Version int              `json:"version"`
	Presets []model.EQPreset `json:"presets,omitempty"`
}

type legacyPersistedEqualizer struct {
	Version int              `json:"version"`
	Active  model.Equalizer  `json:"active"`
	Presets []model.EQPreset `json:"presets,omitempty"`
}

func init() {
	register(protocol.MethodGetEqualizer, typed(getEqualizer))
	register(protocol.MethodSetEqualizer, typed(setEqualizer))
	register(protocol.MethodListEqualizerPresets, typed(listEqualizerPresets))
	register(protocol.MethodSaveEqualizerPreset, typed(saveEqualizerPreset))
	register(protocol.MethodDeleteEqualizerPreset, typed(deleteEqualizerPreset))
	register(protocol.MethodImportEqualizer, typed(importEqualizer))
}

func (a *App) restoreEqualizer() error {
	a.equalizerMu.Lock()
	defer a.equalizerMu.Unlock()
	active := eq.Default()
	var presets []model.EQPreset
	var legacyPresets []model.EQPreset
	a.equalizerActiveBlocked = false
	a.equalizerPresetsBlocked = false
	if a.equalizerStore != nil {
		if raw, ok := a.equalizerStore.Get(equalizerStateKey); ok {
			var state legacyPersistedEqualizer
			if err := json.Unmarshal([]byte(raw), &state); err != nil || state.Version != equalizerStateVersion {
				a.equalizerActiveBlocked = true
				if a.Log != nil {
					a.Log.Warn("leaving unsupported equalizer state untouched", "error", err, "version", state.Version)
				}
			} else {
				active = eq.Normalize(state.Active)
				legacyPresets = normalizeUserPresets(state.Presets)
			}
		}
		if raw, ok := a.equalizerStore.Get(equalizerPresetsKey); ok {
			var state persistedEqualizerPresets
			if err := json.Unmarshal([]byte(raw), &state); err != nil || state.Version != equalizerStateVersion {
				a.equalizerPresetsBlocked = true
				if a.Log != nil {
					a.Log.Warn("leaving unsupported equalizer presets untouched", "error", err, "version", state.Version)
				}
			} else {
				presets = normalizeUserPresets(state.Presets)
			}
		} else if len(legacyPresets) != 0 {
			presets = legacyPresets
			a.equalizerPresets = presets
			if err := a.persistPresetsLocked(presets); err != nil {
				a.equalizerActiveBlocked = true
				a.equalizerPresetsBlocked = true
				if a.Log != nil {
					a.Log.Warn("keeping legacy equalizer state after preset migration failed", "error", err)
				}
			}
		} else if a.equalizerActiveBlocked {
			// The unreadable legacy value may contain presets. Protect it as a unit.
			a.equalizerPresetsBlocked = true
		}
	}
	if a.equalizerPlayer == nil {
		return fmt.Errorf("equalizer player is unavailable")
	}
	if err := a.equalizerPlayer.SetEqualizer(active); err != nil {
		return fmt.Errorf("restore equalizer: %w", err)
	}
	a.equalizerPresets = presets
	return nil
}

func getEqualizer(_ context.Context, a *App, _ protocol.Null) (model.Equalizer, error) {
	if a.equalizerPlayer == nil {
		return model.Equalizer{}, fmt.Errorf("equalizer player is unavailable")
	}
	return a.equalizerPlayer.Equalizer(), nil
}

func setEqualizer(_ context.Context, a *App, requested model.Equalizer) (model.Equalizer, error) {
	a.equalizerMu.Lock()
	accepted := eq.Normalize(requested)
	previous := a.equalizerPlayer.Equalizer()
	if err := a.equalizerPlayer.SetEqualizer(accepted); err != nil {
		a.equalizerMu.Unlock()
		return model.Equalizer{}, err
	}
	if err := a.persistActiveLocked(accepted); err != nil {
		rollbackErr := a.equalizerPlayer.SetEqualizer(previous)
		a.equalizerMu.Unlock()
		if rollbackErr != nil {
			return model.Equalizer{}, fmt.Errorf("persist equalizer: %w; rollback failed: %v", err, rollbackErr)
		}
		return model.Equalizer{}, err
	}
	a.equalizerMu.Unlock()
	a.emitEqualizer(accepted)
	return accepted, nil
}

func listEqualizerPresets(_ context.Context, a *App, _ protocol.Null) ([]model.EQPreset, error) {
	a.equalizerMu.Lock()
	presets := combinedPresets(a.equalizerPresets)
	a.equalizerMu.Unlock()
	return presets, nil
}

func saveEqualizerPreset(_ context.Context, a *App, params protocol.EqualizerPresetParams) ([]model.EQPreset, error) {
	name := strings.TrimSpace(params.Name)
	if err := validateUserPresetName(name); err != nil {
		return nil, err
	}
	a.equalizerMu.Lock()
	previous := a.equalizerPlayer.Equalizer()
	active := previous
	active.Preset = name
	preset := model.EQPreset{Name: name, Preamp: active.Preamp, Bands: cloneBands(active.Bands)}
	next := upsertUserPreset(a.equalizerPresets, preset)
	if err := a.equalizerPlayer.SetEqualizer(active); err != nil {
		a.equalizerMu.Unlock()
		return nil, err
	}
	if err := a.persistBothLocked(active, next); err != nil {
		rollbackErr := a.equalizerPlayer.SetEqualizer(previous)
		a.equalizerMu.Unlock()
		if rollbackErr != nil {
			return nil, fmt.Errorf("persist equalizer preset: %w; rollback failed: %v", err, rollbackErr)
		}
		return nil, err
	}
	a.equalizerPresets = next
	presets := combinedPresets(next)
	a.equalizerMu.Unlock()
	a.emitEqualizer(active)
	return presets, nil
}

func deleteEqualizerPreset(_ context.Context, a *App, params protocol.EqualizerPresetParams) ([]model.EQPreset, error) {
	name := strings.TrimSpace(params.Name)
	if err := validateUserPresetName(name); err != nil {
		return nil, err
	}
	a.equalizerMu.Lock()
	index := presetIndex(a.equalizerPresets, name)
	if index < 0 {
		a.equalizerMu.Unlock()
		return nil, fmt.Errorf("Equalizer preset %q was not found.", name)
	}
	next := make([]model.EQPreset, 0, len(a.equalizerPresets)-1)
	next = append(next, a.equalizerPresets[:index]...)
	next = append(next, a.equalizerPresets[index+1:]...)
	previous := a.equalizerPlayer.Equalizer()
	active := previous
	changed := strings.EqualFold(active.Preset, name)
	if changed {
		active.Preset = ""
		if err := a.equalizerPlayer.SetEqualizer(active); err != nil {
			a.equalizerMu.Unlock()
			return nil, err
		}
	}
	var persistErr error
	if changed {
		persistErr = a.persistBothLocked(active, next)
	} else {
		persistErr = a.persistPresetsLocked(next)
	}
	if persistErr != nil {
		var rollbackErr error
		if changed {
			rollbackErr = a.equalizerPlayer.SetEqualizer(previous)
		}
		a.equalizerMu.Unlock()
		if rollbackErr != nil {
			if a.Log != nil {
				a.Log.Error("equalizer delete rollback failed", "error", rollbackErr)
			}
			return nil, fmt.Errorf("persist equalizer preset deletion: %w; rollback failed: %v", persistErr, rollbackErr)
		}
		return nil, persistErr
	}
	a.equalizerPresets = next
	presets := combinedPresets(next)
	a.equalizerMu.Unlock()
	if changed {
		a.emitEqualizer(active)
	}
	return presets, nil
}

func importEqualizer(_ context.Context, a *App, params protocol.ImportEqualizerParams) (protocol.ImportEqualizerResult, error) {
	preamp, bands, warnings, err := eq.Parse(params.Text)
	if err != nil {
		return protocol.ImportEqualizerResult{}, err
	}
	name := strings.TrimSpace(params.Name)
	if name != "" {
		if err := validateUserPresetName(name); err != nil {
			return protocol.ImportEqualizerResult{}, err
		}
	}
	accepted := eq.Normalize(model.Equalizer{Enabled: true, Preamp: preamp, Bands: bands, Preset: name})
	a.equalizerMu.Lock()
	previous := a.equalizerPlayer.Equalizer()
	next := a.equalizerPresets
	if name != "" {
		next = upsertUserPreset(next, model.EQPreset{Name: name, Preamp: accepted.Preamp, Bands: cloneBands(accepted.Bands)})
	}
	if err := a.equalizerPlayer.SetEqualizer(accepted); err != nil {
		a.equalizerMu.Unlock()
		return protocol.ImportEqualizerResult{}, err
	}
	if name != "" {
		err = a.persistBothLocked(accepted, next)
	} else {
		err = a.persistActiveLocked(accepted)
	}
	if err != nil {
		rollbackErr := a.equalizerPlayer.SetEqualizer(previous)
		a.equalizerMu.Unlock()
		if rollbackErr != nil {
			return protocol.ImportEqualizerResult{}, fmt.Errorf("persist imported equalizer: %w; rollback failed: %v", err, rollbackErr)
		}
		return protocol.ImportEqualizerResult{}, err
	}
	a.equalizerPresets = next
	a.equalizerMu.Unlock()
	a.emitEqualizer(accepted)
	return protocol.ImportEqualizerResult{Equalizer: accepted, Warnings: warnings}, nil
}

func (a *App) persistActiveLocked(active model.Equalizer) error {
	if a.equalizerStore == nil {
		return fmt.Errorf("equalizer store is unavailable")
	}
	if a.equalizerActiveBlocked {
		return fmt.Errorf("Stored equalizer data is unsupported and was left untouched.")
	}
	state := persistedEqualizer{Version: equalizerStateVersion, Active: eq.Normalize(active)}
	encoded, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("encode equalizer state: %w", err)
	}
	if err := a.equalizerStore.Set(equalizerStateKey, string(encoded)); err != nil {
		return fmt.Errorf("persist equalizer state: %w", err)
	}
	return nil
}

func (a *App) persistPresetsLocked(presets []model.EQPreset) error {
	if a.equalizerStore == nil {
		return fmt.Errorf("equalizer store is unavailable")
	}
	if a.equalizerPresetsBlocked {
		return fmt.Errorf("Stored equalizer preset data is unsupported and was left untouched.")
	}
	state := persistedEqualizerPresets{Version: equalizerStateVersion, Presets: normalizeUserPresets(presets)}
	encoded, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("encode equalizer presets: %w", err)
	}
	if err := a.equalizerStore.Set(equalizerPresetsKey, string(encoded)); err != nil {
		return fmt.Errorf("persist equalizer presets: %w", err)
	}
	return nil
}

func (a *App) persistBothLocked(active model.Equalizer, presets []model.EQPreset) error {
	if a.equalizerStore == nil {
		return fmt.Errorf("equalizer store is unavailable")
	}
	if a.equalizerActiveBlocked || a.equalizerPresetsBlocked {
		if a.equalizerActiveBlocked {
			return fmt.Errorf("Stored equalizer data is unsupported and was left untouched.")
		}
		return fmt.Errorf("Stored equalizer preset data is unsupported and was left untouched.")
	}
	oldPresets, hadPresets := a.equalizerStore.Get(equalizerPresetsKey)
	if err := a.persistPresetsLocked(presets); err != nil {
		return err
	}
	if err := a.persistActiveLocked(active); err != nil {
		var rollbackErr error
		if hadPresets {
			rollbackErr = a.equalizerStore.Set(equalizerPresetsKey, oldPresets)
		} else {
			rollbackErr = a.equalizerStore.Delete(equalizerPresetsKey)
		}
		if rollbackErr != nil && a.Log != nil {
			a.Log.Error("equalizer preset persistence rollback failed", "error", rollbackErr)
		}
		return err
	}
	return nil
}

func (a *App) emitEqualizer(equalizer model.Equalizer) {
	if a.Bus != nil {
		a.Bus.Emit(protocol.EventEqualizer, equalizer)
	}
}

func validateUserPresetName(name string) error {
	if name == "" {
		return fmt.Errorf("Equalizer preset name cannot be empty.")
	}
	if eq.IsBuiltin(name) {
		return fmt.Errorf("Equalizer preset %q is built in and cannot be changed.", name)
	}
	return nil
}

func normalizeUserPresets(in []model.EQPreset) []model.EQPreset {
	out := make([]model.EQPreset, 0, len(in))
	for _, raw := range in {
		name := strings.TrimSpace(raw.Name)
		if validateUserPresetName(name) != nil {
			continue
		}
		normalized := eq.Normalize(model.Equalizer{Preamp: raw.Preamp, Bands: raw.Bands})
		preset := model.EQPreset{Name: name, Preamp: normalized.Preamp, Bands: normalized.Bands}
		out = upsertUserPreset(out, preset)
	}
	return out
}

func upsertUserPreset(presets []model.EQPreset, preset model.EQPreset) []model.EQPreset {
	out := clonePresets(presets)
	if index := presetIndex(out, preset.Name); index >= 0 {
		out[index] = clonePreset(preset)
		return out
	}
	return append(out, clonePreset(preset))
}

func presetIndex(presets []model.EQPreset, name string) int {
	for index := range presets {
		if strings.EqualFold(presets[index].Name, name) {
			return index
		}
	}
	return -1
}

func combinedPresets(users []model.EQPreset) []model.EQPreset {
	builtins := eq.Builtins()
	out := make([]model.EQPreset, 0, len(builtins)+len(users))
	out = append(out, builtins...)
	out = append(out, clonePresets(users)...)
	return out
}

func clonePresets(in []model.EQPreset) []model.EQPreset {
	out := make([]model.EQPreset, len(in))
	for index := range in {
		out[index] = clonePreset(in[index])
	}
	return out
}

func clonePreset(in model.EQPreset) model.EQPreset {
	in.Bands = cloneBands(in.Bands)
	return in
}

func cloneBands(in []model.EQBand) []model.EQBand {
	return append([]model.EQBand(nil), in...)
}
