package eq

import (
	"strings"

	"github.com/ryoku-dev/ryotunes/internal/model"
)

var builtinPresets = makeBuiltins()

// Builtins returns independent copies of Ryotunes' factory curves.
func Builtins() []model.EQPreset {
	out := make([]model.EQPreset, len(builtinPresets))
	for index, preset := range builtinPresets {
		out[index] = preset
		out[index].Bands = append([]model.EQBand(nil), preset.Bands...)
	}
	return out
}

// IsBuiltin reports whether name is reserved by a factory curve.
func IsBuiltin(name string) bool {
	for _, preset := range builtinPresets {
		if strings.EqualFold(name, preset.Name) {
			return true
		}
	}
	return false
}

func makeBuiltins() []model.EQPreset {
	presets := []model.EQPreset{
		preset("Flat",
			band(model.EQLowShelf, 60, 0, .707), band(model.EQPeak, 250, 0, 1),
			band(model.EQPeak, 1000, 0, 1), band(model.EQPeak, 4000, 0, 1),
			band(model.EQHighShelf, 12000, 0, .707)),
		preset("Bass boost",
			band(model.EQLowShelf, 60, 6, .707), band(model.EQPeak, 150, 4, 1),
			band(model.EQPeak, 400, 1, 1), band(model.EQPeak, 1000, 0, 1),
			band(model.EQHighShelf, 12000, 0, .707)),
		preset("Bass reducer",
			band(model.EQLowShelf, 60, -6, .707), band(model.EQPeak, 150, -4, 1),
			band(model.EQPeak, 400, -1, 1), band(model.EQPeak, 1000, 0, 1),
			band(model.EQHighShelf, 12000, 0, .707)),
		preset("Treble boost",
			band(model.EQLowShelf, 60, 0, .707), band(model.EQPeak, 1000, 0, 1),
			band(model.EQPeak, 4000, 3, 1), band(model.EQPeak, 8000, 5, 1),
			band(model.EQHighShelf, 12000, 6, .707)),
		preset("Vocal",
			band(model.EQLowShelf, 80, -2, .707), band(model.EQPeak, 250, 1, 1),
			band(model.EQPeak, 1500, 4, 1.2), band(model.EQPeak, 4000, 3, 1),
			band(model.EQHighShelf, 12000, 1, .707)),
		preset("Loudness",
			band(model.EQLowShelf, 60, 5, .707), band(model.EQPeak, 400, 2, 1),
			band(model.EQPeak, 1000, -1, 1), band(model.EQPeak, 4000, 2, 1),
			band(model.EQHighShelf, 12000, 5, .707)),
		preset("V-shape",
			band(model.EQLowShelf, 70, 5, .707), band(model.EQPeak, 250, 2, 1),
			band(model.EQPeak, 1000, -3, 1), band(model.EQPeak, 4000, 2, 1),
			band(model.EQHighShelf, 11000, 5, .707)),
		preset("Acoustic",
			band(model.EQLowShelf, 80, 2, .707), band(model.EQPeak, 300, -1, 1),
			band(model.EQPeak, 1200, 2, 1), band(model.EQPeak, 5000, 2, 1),
			band(model.EQHighShelf, 12000, 1, .707)),
		preset("Electronic",
			band(model.EQLowShelf, 60, 4, .707), band(model.EQPeak, 500, -1, 1),
			band(model.EQPeak, 2000, 2, 1), band(model.EQPeak, 7000, 4, 1),
			band(model.EQHighShelf, 14000, 2, .707)),
		preset("Speech",
			band(model.EQHighPass, 70, 0, .707), band(model.EQPeak, 180, -3, 1),
			band(model.EQPeak, 1200, 4, 1), band(model.EQPeak, 3500, 3, 1),
			band(model.EQLowPass, 14000, 0, .707)),
	}
	for index := range presets {
		presets[index].Preamp = curveHeadroom(presets[index].Bands)
	}
	return presets
}

func preset(name string, bands ...model.EQBand) model.EQPreset {
	return model.EQPreset{Name: name, Builtin: true, Bands: bands}
}

func band(typ model.EQBandType, frequency, gain, q float64) model.EQBand {
	return model.EQBand{Type: typ, Frequency: frequency, Gain: gain, Q: q, Enabled: true}
}

func curveHeadroom(bands []model.EQBand) float64 {
	curve := Curve(0, bands, LogFrequencies(2048), nil)
	peak := 0.0
	for _, gain := range curve {
		peak = max(peak, gain)
	}
	if peak == 0 {
		return 0
	}
	return clamp(-peak, model.EQMinPreamp, 0)
}
