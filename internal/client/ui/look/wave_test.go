package look

import (
	"math"
	"testing"
)

func TestWavePolygonKeepsPerpendicularThickness(t *testing.T) {
	const thickness = float32(2)
	points := wavePolygon(nil, 0, 45, 20, 3, 20, 0, thickness)
	upper := len(points) / 2
	if upper < 4 || len(points)%2 != 0 {
		t.Fatalf("polygon has %d points, want paired sharp edges", len(points))
	}
	slope := float32(12.0 / 20.0)
	for index := range upper {
		top := points[index]
		bottom := points[len(points)-1-index]
		if top.X != bottom.X {
			t.Fatalf("edge pair %d has x %v and %v", index, top.X, bottom.X)
		}
		perpendicular := (bottom.Y - top.Y) / float32(math.Sqrt(float64(1+slope*slope)))
		if math.Abs(float64(perpendicular-thickness)) > .001 {
			t.Fatalf("edge pair %d thickness = %v, want %v", index, perpendicular, thickness)
		}
	}
}

func TestWaveDefaultsUseSlimSeekGeometry(t *testing.T) {
	thickness, amplitude, wavelength := waveDefaults(0, 0, 0)
	if thickness != 2 || amplitude != 3 || wavelength != 20 {
		t.Fatalf("waveDefaults(0, 0, 0) = (%v, %v, %v), want (2, 3, 20)", thickness, amplitude, wavelength)
	}
}

func TestWavePolygonHasSharpVerticesAndVerticalCuts(t *testing.T) {
	points := wavePolygon(nil, 0, 37, 20, 3, 20, 0, 4)
	upper := len(points) / 2
	if upper != 5 {
		t.Fatalf("upper edge has %d points, want endpoints and three triangle vertices", upper)
	}
	for index, wantX := range []float32{0, 10, 20, 30, 37} {
		if points[index].X != wantX {
			t.Fatalf("upper point %d x = %v, want %v", index, points[index].X, wantX)
		}
	}
	firstSlope := (points[2].Y - points[1].Y) / (points[2].X - points[1].X)
	secondSlope := (points[3].Y - points[2].Y) / (points[3].X - points[2].X)
	if firstSlope*secondSlope >= 0 {
		t.Fatalf("adjacent slopes %v and %v do not meet in a sharp vertex", firstSlope, secondSlope)
	}
	endCount := 0
	for _, point := range points {
		if point.X < 0 || point.X > 37 {
			t.Fatalf("point x %v escaped the played clip", point.X)
		}
		if point.X == 37 {
			endCount++
		}
	}
	if endCount != 2 {
		t.Fatalf("played end has %d points, want a vertical two-point cut", endCount)
	}
}

func TestWavePolygonFlat(t *testing.T) {
	points := wavePolygon(nil, 2, 18, 10, 0, 20, 7, 4)
	want := [][2]float32{{2, 8}, {18, 8}, {18, 12}, {2, 12}}
	if len(points) != len(want) {
		t.Fatalf("flat polygon has %d points, want %d", len(points), len(want))
	}
	for index, point := range points {
		if point.X != want[index][0] || point.Y != want[index][1] {
			t.Fatalf("point %d = (%v,%v), want (%v,%v)", index, point.X, point.Y, want[index][0], want[index][1])
		}
	}
}
