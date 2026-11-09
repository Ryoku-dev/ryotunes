package eq

import (
	"math"
	"testing"

	"github.com/ryoku-dev/ryotunes/internal/model"
)

func near(t *testing.T, name string, got, want, tolerance float64) {
	t.Helper()
	if math.Abs(got-want) > tolerance {
		t.Fatalf("%s = %.3f dB, want %.3f dB (±%.2f)", name, got, want, tolerance)
	}
}

func TestBandShapesHitTheirDefiningGains(t *testing.T) {
	peak := model.EQBand{Type: model.EQPeak, Frequency: 1000, Gain: 6, Q: 1, Enabled: true}
	near(t, "peak at centre", BandResponse(peak, 1000), 6, .01)
	near(t, "peak two decades away", BandResponse(peak, 20), 0, .05)

	low := model.EQBand{Type: model.EQLowShelf, Frequency: 200, Gain: -6, Q: .707, Enabled: true}
	near(t, "low shelf floor", BandResponse(low, 20), -6, .2)
	near(t, "low shelf top", BandResponse(low, 10000), 0, .05)

	high := model.EQBand{Type: model.EQHighShelf, Frequency: 4000, Gain: 4, Q: .707, Enabled: true}
	near(t, "high shelf top", BandResponse(high, 19000), 4, .3)
	near(t, "high shelf floor", BandResponse(high, 40), 0, .05)

	pass := model.EQBand{Type: model.EQHighPass, Frequency: 100, Q: .707, Enabled: true}
	near(t, "Butterworth high pass corner", BandResponse(pass, 100), -3.01, .05)
	if BandResponse(pass, 25) > -20 {
		t.Fatalf("high pass two octaves below its corner = %.2f dB, want under -20 dB", BandResponse(pass, 25))
	}

	notch := model.EQBand{Type: model.EQNotch, Frequency: 1000, Q: 4, Enabled: true}
	if BandResponse(notch, 1000) > -60 {
		t.Fatalf("notch centre = %.2f dB, want a deep cut", BandResponse(notch, 1000))
	}
}

func TestCurveSumsEnabledBandsOverThePreamp(t *testing.T) {
	bands := []model.EQBand{
		{Type: model.EQPeak, Frequency: 1000, Gain: 6, Q: 1, Enabled: true},
		{Type: model.EQPeak, Frequency: 1000, Gain: 3, Q: 1, Enabled: true},
		{Type: model.EQPeak, Frequency: 1000, Gain: 12, Q: 1, Enabled: false},
	}
	curve := Curve(-4, bands, []float64{1000, 20}, nil)
	near(t, "summed centre", curve[0], 5, .02)
	near(t, "preamp alone far away", curve[1], -4, .1)
}

func TestLogFrequenciesSpanTheAudibleRange(t *testing.T) {
	freqs := LogFrequencies(4)
	near(t, "first", freqs[0], model.EQMinFrequency, 1e-9)
	near(t, "last", freqs[3], model.EQMaxFrequency, 1e-6)
	near(t, "even log steps", freqs[1]/freqs[0], freqs[3]/freqs[2], 1e-9)
}
