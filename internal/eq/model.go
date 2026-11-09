package eq

import (
	"math"
	"strings"

	"github.com/ryoku-dev/ryotunes/internal/model"
)

// Default returns the bypassed five-band curve used by a new installation.
func Default() model.Equalizer {
	return model.Equalizer{
		Enabled: false,
		Preset:  "Flat",
		Bands: []model.EQBand{
			band(model.EQLowShelf, 60, 0, .707),
			band(model.EQPeak, 250, 0, 1),
			band(model.EQPeak, 1000, 0, 1),
			band(model.EQPeak, 4000, 0, 1),
			band(model.EQHighShelf, 12000, 0, .707),
		},
	}
}

// Normalize returns a finite equalizer within the model's limits.
func Normalize(in model.Equalizer) model.Equalizer {
	out := model.Equalizer{
		Enabled: in.Enabled,
		Preamp:  finiteOr(in.Preamp, 0),
		Preset:  strings.TrimSpace(in.Preset),
	}
	out.Preamp = clamp(out.Preamp, model.EQMinPreamp, model.EQMaxPreamp)
	count := min(len(in.Bands), model.EQMaxBands)
	out.Bands = make([]model.EQBand, 0, count)
	for _, raw := range in.Bands[:count] {
		typ := raw.Type
		if !knownType(typ) {
			typ = model.EQPeak
		}
		out.Bands = append(out.Bands, model.EQBand{
			Type:      typ,
			Frequency: clamp(finiteOr(raw.Frequency, 1000), model.EQMinFrequency, model.EQMaxFrequency),
			Gain:      clamp(finiteOr(raw.Gain, 0), model.EQMinGain, model.EQMaxGain),
			Q:         clamp(finiteOr(raw.Q, 1), model.EQMinQ, model.EQMaxQ),
			Enabled:   raw.Enabled,
		})
	}
	return out
}

func knownType(typ model.EQBandType) bool {
	switch typ {
	case model.EQPeak, model.EQLowShelf, model.EQHighShelf, model.EQLowPass,
		model.EQHighPass, model.EQNotch, model.EQBandPass:
		return true
	default:
		return false
	}
}

func finiteOr(value, fallback float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return fallback
	}
	return value
}

func clamp(value, low, high float64) float64 {
	return math.Max(low, math.Min(high, value))
}
