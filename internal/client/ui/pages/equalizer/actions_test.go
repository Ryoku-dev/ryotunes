package equalizer

import (
	"math"
	"testing"

	"github.com/ryoku-dev/ryotunes/internal/model"
)

func TestWidestGapFrequencyUsesLogarithmicDistance(t *testing.T) {
	bands := []model.EQBand{{Frequency: 100}, {Frequency: 1000}, {Frequency: 10000}}
	got := widestGapFrequency(bands)
	want := math.Round(math.Sqrt(100 * 1000))
	if got != want {
		t.Fatalf("widest gap frequency = %.0f, want %.0f", got, want)
	}
}

func TestInterpolateBandKeepsShapeUntilMidpoint(t *testing.T) {
	from := model.EQBand{Type: model.EQPeak, Frequency: 100, Gain: -4, Q: 1, Enabled: true}
	target := model.EQBand{Type: model.EQHighShelf, Frequency: 1000, Gain: 6, Q: 2, Enabled: false}
	before := interpolateBand(from, target, .25)
	if before.Type != from.Type || before.Enabled != from.Enabled {
		t.Fatalf("shape switched before midpoint: %#v", before)
	}
	if before.Frequency != 325 || before.Gain != -1.5 || before.Q != 1.25 {
		t.Fatalf("quarter interpolation = %#v", before)
	}
	after := interpolateBand(from, target, .75)
	if after.Type != target.Type || after.Enabled != target.Enabled {
		t.Fatalf("shape did not switch after midpoint: %#v", after)
	}
}
