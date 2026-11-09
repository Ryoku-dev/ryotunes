package look

import (
	"testing"
	"time"
)

func TestShapeVisualizerLevelEndpointsAndMonotonicity(t *testing.T) {
	if got := shapeVisualizerLevel(0); got != 0 {
		t.Fatalf("shapeVisualizerLevel(0) = %v, want 0", got)
	}
	if got := shapeVisualizerLevel(.03); got != 0 {
		t.Fatalf("shapeVisualizerLevel(.03) = %v, want 0", got)
	}
	if got := shapeVisualizerLevel(1); !closeEnough(got, 1) {
		t.Fatalf("shapeVisualizerLevel(1) = %v, want 1", got)
	}
	previous := float32(0)
	for step := 1; step <= 1000; step++ {
		level := float32(step) / 1000
		shaped := shapeVisualizerLevel(level)
		if shaped < previous {
			t.Fatalf("shape fell from %v to %v at input %v", previous, shaped, level)
		}
		previous = shaped
	}
}

func TestBeatDetectorSteadySignalDoesNotFire(t *testing.T) {
	var detector beatDetector
	now := time.Unix(100, 0)
	for range 180 {
		if kick := detector.update(now, .6, true); kick != 0 {
			t.Fatalf("steady signal produced kick=%v", kick)
		}
		now = now.Add(time.Second / 60)
	}
}

func TestBeatDetectorStepFiresOnceAndRespectsRefractory(t *testing.T) {
	var detector beatDetector
	now := time.Unix(200, 0)
	for range 120 {
		detector.update(now, .1, true)
		now = now.Add(time.Second / 60)
	}
	if kick := detector.update(now, .8, true); kick != 1 {
		t.Fatalf("bass step kick=%v, want 1", kick)
	}
	firstBeat := now
	if kick := detector.update(firstBeat.Add(20*time.Millisecond), .1, true); kick >= 1 {
		t.Fatalf("falling energy reset kick to %v", kick)
	}
	if kick := detector.update(firstBeat.Add(100*time.Millisecond), .8, true); kick >= 1 {
		t.Fatalf("rise inside refractory period reset kick to %v", kick)
	}
	if kick := detector.update(firstBeat.Add(120*time.Millisecond), .1, true); kick >= 1 {
		t.Fatalf("rearm signal reset kick to %v", kick)
	}
	if kick := detector.update(firstBeat.Add(170*time.Millisecond), .8, true); kick != 1 {
		t.Fatalf("new rise after refractory period kick=%v, want 1", kick)
	}
}

func TestBeatKickSettlesAfterOneSecond(t *testing.T) {
	var detector beatDetector
	now := time.Unix(300, 0)
	detector.update(now, .1, true)
	now = now.Add(time.Second)
	if kick := detector.update(now, .8, true); kick != 1 {
		t.Fatalf("beat start kick=%v, want 1", kick)
	}
	if kick := detector.update(now.Add(time.Second), .1, true); kick != 0 {
		t.Fatalf("kick after one second = %v, want 0", kick)
	}
}

func TestPeakCapStartsRestingAtZero(t *testing.T) {
	var cap peakCap
	now := time.Unix(350, 0)
	moving, wake := cap.update(now, 0, 100, 2)
	if moving || !wake.IsZero() || cap.height != 0 || cap.peak != 0 || !cap.resting {
		t.Fatalf("zero cap moving=%v wake=%v height=%v peak=%v resting=%v", moving, wake, cap.height, cap.peak, cap.resting)
	}
}

func TestPeakCapHoldsThenFalls(t *testing.T) {
	var cap peakCap
	now := time.Unix(400, 0)
	if moving, wake := cap.update(now, 98, 100, 2); moving || !wake.Equal(now.Add(peakCapHold)) {
		t.Fatalf("initial peak moving=%v wake=%v", moving, wake)
	}
	if moving, _ := cap.update(now.Add(peakCapHold-time.Millisecond), 0, 100, 2); moving || cap.height != 100 {
		t.Fatalf("cap before hold expiry moving=%v height=%v", moving, cap.height)
	}
	if moving, _ := cap.update(now.Add(peakCapHold), 0, 100, 2); !moving || cap.height != 100 {
		t.Fatalf("cap at fall start moving=%v height=%v", moving, cap.height)
	}
	halfFall := now.Add(peakCapHold + peakCapFall/2)
	if moving, _ := cap.update(halfFall, 0, 100, 2); !moving || cap.height < 74.9 || cap.height > 75.1 {
		t.Fatalf("cap halfway through fall moving=%v height=%v, want about 75", moving, cap.height)
	}
	if moving, _ := cap.update(now.Add(peakCapHold+peakCapFall), 0, 100, 2); moving || cap.height != 0 {
		t.Fatalf("cap after full fall moving=%v height=%v", moving, cap.height)
	}
}

func TestVisualizerAlphaStepBuckets(t *testing.T) {
	tests := []struct {
		level, want float32
	}{
		{0, .55}, {.249, .55}, {.25, .7}, {.499, .7},
		{.5, .85}, {.749, .85}, {.75, 1}, {1, 1},
	}
	for _, test := range tests {
		if got := visualizerAlphaSteps[visualizerAlphaBucket(test.level)]; got != test.want {
			t.Errorf("alpha step for level %v = %v, want %v", test.level, got, test.want)
		}
	}
}
