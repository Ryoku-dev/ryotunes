package look

import (
	"math"
	"testing"

	"gioui.org/f32"
)

func TestOscilloscopeTriggerKeepsSinePhase(t *testing.T) {
	const (
		period = 128
		frames = 5000
	)
	makeSignal := func(offset int) []float32 {
		samples := make([]float32, frames*2)
		for frame := range frames {
			value := float32(math.Sin(2 * math.Pi * float64(offset+frame) / period))
			samples[frame*2], samples[frame*2+1] = value, value
		}
		return samples
	}
	firstOffset, secondOffset := 0, 37
	first := oscilloscopeTrigger(makeSignal(firstOffset), oscilloscopeWindow, oscilloscopeHysteresis)
	second := oscilloscopeTrigger(makeSignal(secondOffset), oscilloscopeWindow, oscilloscopeHysteresis)
	if phaseA, phaseB := (firstOffset+first)%period, (secondOffset+second)%period; phaseA != phaseB {
		t.Fatalf("successive triggers start at phases %d and %d", phaseA, phaseB)
	}
}

func TestVectorscopeMapping(t *testing.T) {
	tests := []struct {
		name         string
		left, right  float32
		wantX, wantY float32
	}{
		{name: "left only", left: 1, wantX: .70710677, wantY: .70710677},
		{name: "mono", left: 1, right: 1, wantX: 0, wantY: 1.4142135},
		{name: "inverted", left: 1, right: -1, wantX: 1.4142135, wantY: 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			point := vectorscopePoint(test.left, test.right)
			if !closeEnough(point.X, test.wantX) || !closeEnough(point.Y, test.wantY) {
				t.Fatalf("vectorscopePoint(%v, %v) = (%v, %v), want (%v, %v)", test.left, test.right, point.X, point.Y, test.wantX, test.wantY)
			}
		})
	}
}

func TestMonotoneCurveNeverDropsBelowFloor(t *testing.T) {
	points := []f32.Point{
		f32.Pt(0, 80),
		f32.Pt(20, 30),
		f32.Pt(40, 60),
		f32.Pt(60, 20),
		f32.Pt(80, 70),
	}
	slopes := make([]float32, len(points)-1)
	tangents := make([]float32, len(points))
	monotoneTangents(points, slopes, tangents)
	for segment := range len(points) - 1 {
		left, right := points[segment], points[segment+1]
		width := right.X - left.X
		controlA := left.Y + tangents[segment]*width/3
		controlB := right.Y - tangents[segment+1]*width/3
		low, high := min(left.Y, right.Y), max(left.Y, right.Y)
		for step := range 101 {
			time := float32(step) / 100
			inverse := 1 - time
			y := inverse*inverse*inverse*left.Y +
				3*inverse*inverse*time*controlA +
				3*inverse*time*time*controlB +
				time*time*time*right.Y
			if y < low-.001 || y > high+.001 || y > 80.001 {
				t.Fatalf("segment %d at %.2f has y=%v outside [%v, %v] or below floor", segment, time, y, low, high)
			}
		}
	}
}
