package look

import (
	"math"
	"testing"

	"gioui.org/layout"
)

func closeEnough(first, second float32) bool {
	return math.Abs(float64(first-second)) < .001
}

func TestScrollDeltaWaylandAppliesImmediately(t *testing.T) {
	if got, want := mapScrollDelta(150, 1.5, false), float32(225); !closeEnough(got, want) {
		t.Fatalf("mapScrollDelta(150, 1.5, false) = %v, want %v", got, want)
	}
	if got, want := mapScrollDelta(7.25, 2, false), float32(14.5); !closeEnough(got, want) {
		t.Fatalf("mapScrollDelta(7.25, 2, false) = %v, want %v", got, want)
	}
}

func TestScrollDeltaX11Notch(t *testing.T) {
	if got, want := mapScrollDelta(10, 1.25, true), float32(60); !closeEnough(got, want) {
		t.Fatalf("mapScrollDelta(10, 1.25, true) = %v, want %v", got, want)
	}
	if got, want := mapScrollDelta(-20, 1.25, true), float32(-120); !closeEnough(got, want) {
		t.Fatalf("mapScrollDelta(-20, 1.25, true) = %v, want %v", got, want)
	}
}

func TestScrollRangeOnlyAdvertisesAvailableDirections(t *testing.T) {
	tests := []struct {
		name     string
		position layout.Position
		backward bool
		forward  bool
	}{
		{name: "empty"},
		{name: "start", position: layout.Position{BeforeEnd: true}, forward: true},
		{name: "middle", position: layout.Position{First: 3, BeforeEnd: true}, backward: true, forward: true},
		{name: "offset", position: layout.Position{Offset: 1}, backward: true},
		{name: "end", position: layout.Position{First: 8}, backward: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			bounds := verticalScrollRange(test.position)
			if got := bounds.Min < 0; got != test.backward {
				t.Fatalf("backward range = %v, want %v", got, test.backward)
			}
			if got := bounds.Max > 0; got != test.forward {
				t.Fatalf("forward range = %v, want %v", got, test.forward)
			}
		})
	}
}

func TestScrollFractionalAccumulation(t *testing.T) {
	position := layout.Position{BeforeEnd: true}
	fraction := float32(0)
	if applied := addListOffset(&position, &fraction, .6); applied != 0 {
		t.Fatalf("first applied = %d, want 0", applied)
	}
	if applied := addListOffset(&position, &fraction, .6); applied != 1 {
		t.Fatalf("second applied = %d, want 1", applied)
	}
	if position.Offset != 1 || !closeEnough(fraction, .2) {
		t.Fatalf("position = %d, fraction = %v, want 1 and .2", position.Offset, fraction)
	}
}
