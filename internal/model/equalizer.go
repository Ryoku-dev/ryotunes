package model

// EQBandType is the shape of one parametric band.
type EQBandType string

// The band shapes: a bell, two shelves, two pass filters, a notch and a band pass.
const (
	EQPeak      EQBandType = "peak"
	EQLowShelf  EQBandType = "lowshelf"
	EQHighShelf EQBandType = "highshelf"
	EQLowPass   EQBandType = "lowpass"
	EQHighPass  EQBandType = "highpass"
	EQNotch     EQBandType = "notch"
	EQBandPass  EQBandType = "bandpass"
)

// The parametric equalizer's limits.
const (
	EQMaxBands     = 20
	EQMinFrequency = 20.0
	EQMaxFrequency = 20000.0
	EQMinGain      = -12.0
	EQMaxGain      = 12.0
	EQMinQ         = 0.1
	EQMaxQ         = 18.0
	EQMinPreamp    = -24.0
	EQMaxPreamp    = 12.0
)

// EQBand is one parametric filter: its shape, centre or corner frequency in Hz,
// gain in dB (ignored by the pass, notch and band-pass shapes) and Q.
type EQBand struct {
	Type      EQBandType `json:"type"`
	Frequency float64    `json:"frequency"`
	Gain      float64    `json:"gain"`
	Q         float64    `json:"q"`
	Enabled   bool       `json:"enabled"`
}

// HasGain reports whether the band's shape uses its gain.
func (band EQBand) HasGain() bool {
	switch band.Type {
	case EQPeak, EQLowShelf, EQHighShelf:
		return true
	default:
		return false
	}
}

// Equalizer is the complete parametric equalizer the daemon applies: up to
// EQMaxBands bands after a preamp. Preset names the curve it was loaded from
// and is cleared by any edit.
type Equalizer struct {
	Enabled bool     `json:"enabled"`
	Preamp  float64  `json:"preamp"`
	Bands   []EQBand `json:"bands"`
	Preset  string   `json:"preset,omitempty"`
}

// EQPreset is a named curve: a built-in one or one the listener saved.
type EQPreset struct {
	Name    string   `json:"name"`
	Builtin bool     `json:"builtin,omitempty"`
	Preamp  float64  `json:"preamp"`
	Bands   []EQBand `json:"bands"`
}
