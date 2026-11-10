package shell

import (
	"image"
	"testing"
	"time"

	"gioui.org/io/input"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/anim"
)

func TestPlayerFrameReservationTracksLoadedState(t *testing.T) {
	tests := []struct {
		name                          string
		compact                       bool
		progress                      float32
		wantTop, wantBottom, wantPane int
	}{
		{name: "wide empty", progress: 0, wantTop: 792, wantBottom: 792, wantPane: 792},
		{name: "wide loaded", progress: 1, wantTop: 704, wantBottom: 792, wantPane: 696},
		{name: "compact empty", compact: true, progress: 0, wantTop: 696, wantBottom: 696, wantPane: 696},
		{name: "compact loaded", compact: true, progress: 1, wantTop: 632, wantBottom: 696, wantPane: 624},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			height, bottomNav, windowHeight := 88, 0, 800
			if test.compact {
				height, bottomNav, windowHeight = 64, 56, 760
			}
			top, bottom, pane := resolvePlayerGeometry(windowHeight, 56, 8, 8, height, bottomNav, test.compact, test.progress)
			if top != test.wantTop || bottom != test.wantBottom || pane != test.wantPane {
				t.Fatalf("geometry = (%d, %d, %d), want (%d, %d, %d)", top, bottom, pane, test.wantTop, test.wantBottom, test.wantPane)
			}
		})
	}
}
func TestPlayerBarMidUnfoldKeepsFullLayoutHeight(t *testing.T) {
	tests := []struct {
		name                    string
		width, height, nav, bar int
		compact                 bool
		wantVisible             int
	}{
		{name: "wide", width: 1280, height: 800, bar: 88, wantVisible: 44},
		{name: "compact", width: 700, height: 760, nav: 56, bar: 64, compact: true, wantVisible: 32},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			top, bottom, _ := resolvePlayerGeometry(test.height, 56, 8, 8, test.bar, test.nav, test.compact, .5)
			rects := resolvePlayerReveal(test.width, 8, top, bottom, test.bar)
			if got := rects.Visible.Dy(); got != test.wantVisible {
				t.Fatalf("visible height = %d, want %d", got, test.wantVisible)
			}
			if got := rects.Full.Dy(); got != test.bar {
				t.Fatalf("content layout height = %d, want full %d", got, test.bar)
			}
			if rects.Full.Min != rects.Visible.Min {
				t.Fatalf("content origin = %v, reveal origin = %v", rects.Full.Min, rects.Visible.Min)
			}
		})
	}
}

func TestPlayerBarFirstSnapshotSnapsThenTransitionsAndSettles(t *testing.T) {
	const duration = 200 * time.Millisecond
	var operations op.Ops
	var router input.Router
	now := time.Unix(100, 0)
	context := func() layout.Context {
		return layout.Context{
			Ops: &operations, Now: now, Source: router.Source(),
			Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(1280, 800)),
		}
	}

	var restored uiShell
	if got := restored.playerBarProgress(context(), duration, false, false); got != 0 {
		t.Fatalf("before snapshot progress = %v, want 0", got)
	}
	now = now.Add(time.Millisecond)
	if got := restored.playerBarProgress(context(), duration, true, true); got != 1 {
		t.Fatalf("restored first snapshot progress = %v, want 1", got)
	}

	var live uiShell
	if got := live.playerBarProgress(context(), duration, true, false); got != 0 {
		t.Fatalf("empty first snapshot progress = %v, want 0", got)
	}
	now = now.Add(time.Millisecond)
	operations.Reset()
	if got := live.playerBarProgress(context(), duration, true, true); got != 0 {
		t.Fatalf("transition start progress = %v, want 0", got)
	}
	now = now.Add(duration / 2)
	operations.Reset()
	middle := live.playerBarProgress(context(), duration, true, true)
	if middle <= 0 || middle >= 1 {
		t.Fatalf("mid-transition progress = %v, want between 0 and 1", middle)
	}
	now = now.Add(duration)
	operations.Reset()
	if got := live.playerBarProgress(context(), duration, true, true); got != 1 {
		t.Fatalf("settled progress = %v, want 1", got)
	}
	router.Frame(&operations)
	router.WakeupTime()
	operations.Reset()
	now = now.Add(time.Millisecond)
	live.playerBarProgress(context(), duration, true, true)
	router.Frame(&operations)
	if wake, ok := router.WakeupTime(); ok {
		t.Fatalf("settled player bar requested an idle frame at %v", wake)
	}
}

func TestPlayerBarReducedMotionSnaps(t *testing.T) {
	reduced := anim.Reduced()
	anim.SetReduce(true)
	defer anim.SetReduce(reduced)

	var operations op.Ops
	gtx := layout.Context{Ops: &operations, Now: time.Unix(200, 0)}
	var shell uiShell
	if got := shell.playerBarProgress(gtx, 200*time.Millisecond, true, false); got != 0 {
		t.Fatalf("empty progress = %v, want 0", got)
	}
	gtx.Now = gtx.Now.Add(time.Millisecond)
	if got := shell.playerBarProgress(gtx, 200*time.Millisecond, true, true); got != 1 {
		t.Fatalf("loaded progress = %v, want 1", got)
	}
}
