package look

import (
	"testing"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/palette"
)

func TestScrubberPressPositionToFraction(t *testing.T) {
	tests := []struct {
		name  string
		x     float32
		width int
		want  float32
	}{
		{name: "start", x: 0, width: 200, want: 0},
		{name: "quarter", x: 50, width: 200, want: .25},
		{name: "end", x: 200, width: 200, want: 1},
		{name: "before", x: -20, width: 200, want: 0},
		{name: "after", x: 250, width: 200, want: 1},
		{name: "zero width", x: 10, width: 0, want: 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := scrubberFraction(test.x, test.width); !closeEnough(got, test.want) {
				t.Fatalf("scrubberFraction(%v, %d) = %v, want %v", test.x, test.width, got, test.want)
			}
		})
	}
}

func TestScrubberWheelStepsAndAccumulation(t *testing.T) {
	value, remainder := scrubberWheel(.5, -75, 0, .05, 150)
	if !closeEnough(value, .5) || !closeEnough(remainder, -.5) {
		t.Fatalf("first half-notch = (%v, %v), want (.5, -.5)", value, remainder)
	}
	value, remainder = scrubberWheel(value, -75, remainder, .05, 150)
	if !closeEnough(value, .55) || !closeEnough(remainder, 0) {
		t.Fatalf("second half-notch = (%v, %v), want (.55, 0)", value, remainder)
	}
	value, _ = scrubberWheel(.98, -300, 0, .05, 150)
	if !closeEnough(value, 1) {
		t.Fatalf("upper clamp = %v, want 1", value)
	}
	value, _ = scrubberWheel(.02, 300, 0, .05, 150)
	if !closeEnough(value, 0) {
		t.Fatalf("lower clamp = %v, want 0", value)
	}
}

func TestScrubberTrackContrast(t *testing.T) {
	for _, test := range []struct {
		name string
		dark bool
	}{
		{name: "dark", dark: true},
		{name: "light", dark: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			roles := palette.Signature(test.dark)
			ratio := palette.ContrastRatio(scrubberTrackInk(roles), roles.Stone1)
			if ratio < 1.5 {
				t.Fatalf("track contrast against stone1 = %.3f, want at least 1.5", ratio)
			}
		})
	}
}
