package shell

import (
	"context"
	"image"
	"sync/atomic"
	"testing"
	"time"

	"gioui.org/io/input"
	"gioui.org/layout"
	"gioui.org/op"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/anim"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/design"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/palette"
	libtheme "github.com/ryoku-dev/ryoku/docs/lithic/library/go/theme"
	"github.com/ryoku-dev/ryotunes/internal/client/state"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
)

func TestDesktopArrivingMidFrameIsWornOnTheNextFrame(t *testing.T) {
	defer anim.SetReduce(false)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runtime := &clientRuntime{ctx: ctx, theme: look.NewTheme(palette.Signature(true), libtheme.Comfortable)}
	var invalidations atomic.Int32
	runtime.hub.Add(func() { invalidations.Add(1) })
	preferences := state.DefaultClientPreferences()
	var ops op.Ops
	var router input.Router
	now := time.Unix(9000, 0)
	frame := func() *look.Theme {
		ops.Reset()
		gtx := layout.Context{Ops: &ops, Source: router.Source(), Now: now, Constraints: layout.Exact(image.Pt(100, 100))}
		theme := runtime.applyClientTheme(preferences)
		// the desktop changes while this frame is being drawn
		if now.Equal(time.Unix(9000, 0)) {
			fluent := palette.Desktop{Design: design.Fluent, Gloss: true, Roles: palette.Signature(false)}
			runtime.offerDesktop(fluent)
		}
		runtime.themeFrameEnd(gtx)
		router.Frame(&ops)
		return theme
	}

	if theme := frame(); theme.Design == design.Fluent {
		t.Fatal("the reading was worn in the frame it arrived during")
	}
	if wake, ok := router.WakeupTime(); !ok || wake.After(now) {
		t.Fatalf("a reading left pending at the frame's end asks for %v (%v), want the next frame at once", wake, ok)
	}
	now = now.Add(time.Millisecond)
	if theme := frame(); theme.Design != design.Fluent || theme.R.Dark {
		t.Fatalf("the next frame wears design %v dark %v, want the new reading", theme.Design, theme.R.Dark)
	}
	if _, ok := router.WakeupTime(); ok {
		t.Fatal("a frame with nothing pending asks for another")
	}

	// the retry asks the window again until a frame takes the reading, then stops
	runtime.offerDesktop(palette.Desktop{Gloss: true, Roles: palette.Signature(true)})
	deadline := time.Now().Add(time.Second)
	for invalidations.Load() < 3 && time.Now().Before(deadline) {
		time.Sleep(desktopRetry)
	}
	if invalidations.Load() < 3 {
		t.Fatalf("a pending reading asked for %d frames, want it to keep asking", invalidations.Load())
	}
	now = now.Add(time.Millisecond)
	frame()
	time.Sleep(3 * desktopRetry)
	settled := invalidations.Load()
	time.Sleep(5 * desktopRetry)
	if got := invalidations.Load(); got != settled {
		t.Fatalf("frames still asked for after the reading was taken: %d then %d", settled, got)
	}
	runtime.themeMu.Lock()
	retrying := runtime.desktopRetrying
	runtime.themeMu.Unlock()
	if retrying {
		t.Fatal("the retry is still running after the reading was taken")
	}
}
