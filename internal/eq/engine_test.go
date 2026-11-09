package eq

import (
	"math"
	"testing"

	"github.com/ryoku-dev/ryotunes/internal/model"
)

func TestDefaultAndNormalize(t *testing.T) {
	defaultEQ := Default()
	if defaultEQ.Enabled || defaultEQ.Preset != "Flat" || len(defaultEQ.Bands) != 5 {
		t.Fatalf("default = %#v", defaultEQ)
	}
	bands := make([]model.EQBand, model.EQMaxBands+3)
	bands[0] = model.EQBand{Type: "other", Frequency: math.NaN(), Gain: math.Inf(1), Q: math.Inf(-1), Enabled: true}
	for index := 1; index < len(bands); index++ {
		bands[index] = model.EQBand{Type: model.EQPeak, Frequency: 30000, Gain: -30, Q: 30, Enabled: true}
	}
	got := Normalize(model.Equalizer{Enabled: true, Preamp: math.Inf(1), Preset: "  Mine  ", Bands: bands})
	if len(got.Bands) != model.EQMaxBands {
		t.Fatalf("bands = %d", len(got.Bands))
	}
	if got.Preamp != 0 || got.Preset != "Mine" {
		t.Fatalf("normalized header = %#v", got)
	}
	if got.Bands[0] != (model.EQBand{Type: model.EQPeak, Frequency: 1000, Gain: 0, Q: 1, Enabled: true}) {
		t.Fatalf("non-finite band = %#v", got.Bands[0])
	}
	if got.Bands[1].Frequency != model.EQMaxFrequency || got.Bands[1].Gain != model.EQMinGain || got.Bands[1].Q != model.EQMaxQ {
		t.Fatalf("clamped band = %#v", got.Bands[1])
	}
}

func TestBuiltinsHaveCurveHeadroom(t *testing.T) {
	wantNames := []string{"Flat", "Bass boost", "Bass reducer", "Treble boost", "Vocal", "Loudness", "V-shape", "Acoustic", "Electronic", "Speech"}
	presets := Builtins()
	if len(presets) != len(wantNames) {
		t.Fatalf("presets = %d", len(presets))
	}
	for index, preset := range presets {
		if preset.Name != wantNames[index] || !preset.Builtin {
			t.Fatalf("preset %d = %#v", index, preset)
		}
		peak := math.Inf(-1)
		for _, value := range Curve(preset.Preamp, preset.Bands, LogFrequencies(2048), nil) {
			peak = max(peak, value)
		}
		if peak > 1e-9 {
			t.Errorf("%s peak after preamp = %g", preset.Name, peak)
		}
	}
	presets[0].Bands[0].Gain = 12
	if Builtins()[0].Bands[0].Gain != 0 {
		t.Fatal("Builtins returned shared band storage")
	}
}

func TestChainMixedBands(t *testing.T) {
	equalizer := model.Equalizer{Enabled: true, Preamp: -3, Bands: []model.EQBand{
		{Type: model.EQPeak, Frequency: 1000, Gain: 2.5, Q: 1.2, Enabled: true},
		{Type: model.EQLowShelf, Frequency: 80, Gain: 4, Q: .707, Enabled: false},
		{Type: model.EQHighShelf, Frequency: 12000, Gain: -2, Q: .707, Enabled: true},
		{Type: model.EQLowPass, Frequency: 18000, Q: .707, Enabled: true},
		{Type: model.EQHighPass, Frequency: 30, Q: .8, Enabled: true},
		{Type: model.EQNotch, Frequency: 60, Q: 8, Enabled: true},
		{Type: model.EQBandPass, Frequency: 2500, Q: 2, Enabled: true},
	}}
	want := "@rteq00:lavfi=[volume=volume=-3dB]," +
		"@rteq01:lavfi=[equalizer=f=1000:t=q:w=1.2:g=2.5]," +
		"@rteq03:lavfi=[highshelf=f=12000:t=q:w=0.707:g=-2:p=2]," +
		"@rteq04:lavfi=[lowpass=f=18000:t=q:w=0.707:p=2]," +
		"@rteq05:lavfi=[highpass=f=30:t=q:w=0.8:p=2]," +
		"@rteq06:lavfi=[bandreject=f=60:t=q:w=8]," +
		"@rteq07:lavfi=[bandpass=f=2500:t=q:w=2:csg=0]"
	if got := Chain(equalizer); got != want {
		t.Fatalf("chain\n got: %s\nwant: %s", got, want)
	}
	equalizer.Enabled = false
	if got := Chain(equalizer); got != "" {
		t.Fatalf("bypassed chain = %q", got)
	}
}
