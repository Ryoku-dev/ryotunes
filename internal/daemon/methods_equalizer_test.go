package daemon

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/ryoku-dev/ryotunes/internal/bus"
	"github.com/ryoku-dev/ryotunes/internal/eq"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
	"github.com/ryoku-dev/ryotunes/internal/provider"
)

type fakeEqualizerPlayer struct {
	value model.Equalizer
	err   error
}

func (player *fakeEqualizerPlayer) SetEqualizer(value model.Equalizer) error {
	if player.err != nil {
		return player.err
	}
	player.value = eq.Normalize(value)
	return nil
}

func (player *fakeEqualizerPlayer) Equalizer() model.Equalizer {
	value := player.value
	value.Bands = append([]model.EQBand(nil), value.Bands...)
	return value
}

func TestEqualizerSetEventAndPersistenceRoundTrip(t *testing.T) {
	kv := provider.MapKV{}
	engine := &fakeEqualizerPlayer{value: eq.Default()}
	app := &App{equalizerPlayer: engine, equalizerStore: kv, Bus: &bus.Fanout{}}
	var event model.Equalizer
	app.Bus.Attach(bus.SinkFunc(func(name string, value any) {
		if name == protocol.EventEqualizer {
			event = value.(model.Equalizer)
		}
	}))
	requested := model.Equalizer{Enabled: true, Preamp: -99, Preset: " Mine ", Bands: []model.EQBand{{
		Type: "unknown", Frequency: 50000, Gain: 99, Q: 0, Enabled: true,
	}}}
	params, err := json.Marshal(requested)
	if err != nil {
		t.Fatal(err)
	}
	result, rpcErr := app.Dispatch(context.Background(), protocol.MethodSetEqualizer, params)
	if rpcErr != nil {
		t.Fatal(rpcErr)
	}
	accepted := result.(model.Equalizer)
	if accepted.Preamp != model.EQMinPreamp || accepted.Bands[0].Type != model.EQPeak || accepted.Bands[0].Frequency != model.EQMaxFrequency || accepted.Bands[0].Gain != model.EQMaxGain || accepted.Bands[0].Q != model.EQMinQ {
		t.Fatalf("accepted = %#v", accepted)
	}
	if !reflect.DeepEqual(event, accepted) {
		t.Fatalf("event = %#v, accepted = %#v", event, accepted)
	}
	if _, ok := kv[equalizerStateKey]; !ok {
		t.Fatal("equalizer state was not persisted")
	}

	restartedEngine := &fakeEqualizerPlayer{}
	restarted := &App{equalizerPlayer: restartedEngine, equalizerStore: kv}
	if err := restarted.restoreEqualizer(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(restartedEngine.Equalizer(), accepted) {
		t.Fatalf("restored = %#v, want %#v", restartedEngine.Equalizer(), accepted)
	}
}

func TestEqualizerPresetSaveDeleteRules(t *testing.T) {
	kv := provider.MapKV{}
	engine := &fakeEqualizerPlayer{value: model.Equalizer{Enabled: true, Bands: []model.EQBand{{Type: model.EQPeak, Frequency: 1000, Gain: 3, Q: 1, Enabled: true}}}}
	app := &App{equalizerPlayer: engine, equalizerStore: kv, Bus: &bus.Fanout{}}
	var event model.Equalizer
	app.Bus.Attach(bus.SinkFunc(func(name string, value any) {
		if name == protocol.EventEqualizer {
			event = value.(model.Equalizer)
		}
	}))

	presets, err := saveEqualizerPreset(context.Background(), app, protocol.EqualizerPresetParams{Name: "Headphones"})
	if err != nil {
		t.Fatal(err)
	}
	if len(presets) != len(eq.Builtins())+1 || presets[len(presets)-1].Name != "Headphones" {
		t.Fatalf("presets = %#v", presets)
	}
	if engine.Equalizer().Preset != "Headphones" || event.Preset != "Headphones" {
		t.Fatalf("active=%#v event=%#v", engine.Equalizer(), event)
	}
	if _, ok := kv[equalizerStateKey]; !ok {
		t.Fatal("active equalizer was not persisted")
	}
	if _, ok := kv[equalizerPresetsKey]; !ok {
		t.Fatal("preset catalogue was not persisted separately")
	}
	if _, err := saveEqualizerPreset(context.Background(), app, protocol.EqualizerPresetParams{Name: "bass BOOST"}); err == nil {
		t.Fatal("built-in preset was replaced")
	}
	if _, err := deleteEqualizerPreset(context.Background(), app, protocol.EqualizerPresetParams{Name: "Flat"}); err == nil {
		t.Fatal("built-in preset was deleted")
	}
	presets, err = deleteEqualizerPreset(context.Background(), app, protocol.EqualizerPresetParams{Name: "headphones"})
	if err != nil {
		t.Fatal(err)
	}
	if len(presets) != len(eq.Builtins()) {
		t.Fatalf("presets after delete = %d", len(presets))
	}
	if _, err := deleteEqualizerPreset(context.Background(), app, protocol.EqualizerPresetParams{Name: "Headphones"}); err == nil {
		t.Fatal("missing preset deletion succeeded")
	}
}

func TestImportEqualizerAppliesAndOptionallySaves(t *testing.T) {
	kv := provider.MapKV{}
	engine := &fakeEqualizerPlayer{value: eq.Default()}
	app := &App{equalizerPlayer: engine, equalizerStore: kv, Bus: &bus.Fanout{}}
	result, err := importEqualizer(context.Background(), app, protocol.ImportEqualizerParams{
		Name: "Studio cans",
		Text: "Preamp: -30 dB\nFilter 1: ON LSC Fc 10 Hz Gain 4 dB Q 0.707\nFilter 2: OFF HSC Fc 12000 Hz Gain -2 dB Q 0.8\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Equalizer.Enabled || result.Equalizer.Preset != "Studio cans" || len(result.Equalizer.Bands) != 2 || len(result.Warnings) != 2 {
		t.Fatalf("import result = %#v", result)
	}
	presets, err := listEqualizerPresets(context.Background(), app, protocol.Null{})
	if err != nil {
		t.Fatal(err)
	}
	if presets[len(presets)-1].Name != "Studio cans" {
		t.Fatalf("saved presets = %#v", presets)
	}

	restarted := &App{equalizerPlayer: &fakeEqualizerPlayer{}, equalizerStore: kv}
	if err := restarted.restoreEqualizer(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(restarted.equalizerPlayer.Equalizer(), result.Equalizer) || len(restarted.equalizerPresets) != 1 {
		t.Fatalf("restart active=%#v presets=%#v", restarted.equalizerPlayer.Equalizer(), restarted.equalizerPresets)
	}
}

func TestImportEqualizerRejectsZeroFiltersWithoutChangingState(t *testing.T) {
	kv := provider.MapKV{}
	before := model.Equalizer{Enabled: true, Preamp: -4, Bands: []model.EQBand{{
		Type: model.EQPeak, Frequency: 1000, Gain: 2, Q: 1, Enabled: true,
	}}}
	engine := &fakeEqualizerPlayer{value: before}
	app := &App{equalizerPlayer: engine, equalizerStore: kv}
	if _, err := importEqualizer(context.Background(), app, protocol.ImportEqualizerParams{Text: "Preamp: -6 dB\n"}); err == nil {
		t.Fatal("zero-filter import succeeded")
	}
	if !reflect.DeepEqual(engine.Equalizer(), before) {
		t.Fatalf("state changed to %#v", engine.Equalizer())
	}
	if _, ok := kv[equalizerStateKey]; ok {
		t.Fatal("zero-filter import persisted state")
	}
}

func TestEqualizerActiveWritesDoNotRewritePresets(t *testing.T) {
	kv := provider.MapKV{}
	engine := &fakeEqualizerPlayer{value: eq.Default()}
	app := &App{equalizerPlayer: engine, equalizerStore: kv}
	if _, err := saveEqualizerPreset(context.Background(), app, protocol.EqualizerPresetParams{Name: "Saved"}); err != nil {
		t.Fatal(err)
	}
	presetsJSON := kv[equalizerPresetsKey]
	active := engine.Equalizer()
	active.Preset = ""
	active.Bands[0].Gain = 4
	if _, err := setEqualizer(context.Background(), app, active); err != nil {
		t.Fatal(err)
	}
	if kv[equalizerPresetsKey] != presetsJSON {
		t.Fatal("active update rewrote preset data")
	}
}

func TestEqualizerMigratesCombinedPersistence(t *testing.T) {
	legacy := persistedEqualizerPresets{Version: equalizerStateVersion, Presets: []model.EQPreset{{
		Name: "Legacy", Bands: []model.EQBand{{Type: model.EQPeak, Frequency: 1000, Q: 1, Enabled: true}},
	}}}
	combined, err := json.Marshal(legacyPersistedEqualizer{
		Version: legacy.Version,
		Active:  model.Equalizer{Enabled: true, Preamp: -3, Bands: legacy.Presets[0].Bands, Preset: "Legacy"},
		Presets: legacy.Presets,
	})
	if err != nil {
		t.Fatal(err)
	}
	kv := provider.MapKV{equalizerStateKey: string(combined)}
	engine := &fakeEqualizerPlayer{}
	app := &App{equalizerPlayer: engine, equalizerStore: kv}
	if err := app.restoreEqualizer(); err != nil {
		t.Fatal(err)
	}
	if len(app.equalizerPresets) != 1 || app.equalizerPresets[0].Name != "Legacy" {
		t.Fatalf("presets = %#v", app.equalizerPresets)
	}
	if _, ok := kv[equalizerPresetsKey]; !ok {
		t.Fatal("legacy presets were not split into their own key")
	}
}

func TestEqualizerLeavesUnsupportedPersistenceUntouched(t *testing.T) {
	activeRaw := `{"version":2,"active":{"enabled":true}}`
	presetsRaw := `{"version":99,"presets":[{"name":"Future"}]}`
	kv := provider.MapKV{equalizerStateKey: activeRaw, equalizerPresetsKey: presetsRaw}
	app := &App{equalizerPlayer: &fakeEqualizerPlayer{}, equalizerStore: kv}
	if err := app.restoreEqualizer(); err != nil {
		t.Fatal(err)
	}
	if _, err := setEqualizer(context.Background(), app, eq.Default()); err == nil {
		t.Fatal("unsupported active state was overwritten")
	}
	if _, err := saveEqualizerPreset(context.Background(), app, protocol.EqualizerPresetParams{Name: "Mine"}); err == nil {
		t.Fatal("unsupported preset state was overwritten")
	}
	if kv[equalizerStateKey] != activeRaw || kv[equalizerPresetsKey] != presetsRaw {
		t.Fatalf("unsupported values changed: %q %q", kv[equalizerStateKey], kv[equalizerPresetsKey])
	}
}

func TestEqualizerLeavesCorruptPersistenceUntouched(t *testing.T) {
	const raw = `{"version":1,"active":`
	kv := provider.MapKV{equalizerStateKey: raw}
	app := &App{equalizerPlayer: &fakeEqualizerPlayer{}, equalizerStore: kv}
	if err := app.restoreEqualizer(); err != nil {
		t.Fatal(err)
	}
	if _, err := setEqualizer(context.Background(), app, eq.Default()); err == nil {
		t.Fatal("corrupt active state was overwritten")
	}
	if kv[equalizerStateKey] != raw {
		t.Fatalf("corrupt value changed to %q", kv[equalizerStateKey])
	}
}
