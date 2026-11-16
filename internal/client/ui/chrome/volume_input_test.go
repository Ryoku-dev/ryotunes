package chrome

import (
	"image"
	"os"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"

	"github.com/ryoku-dev/ryotunes/internal/client/state"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

// TestVolumeKeysAndWheelAreExact drives the volume scrubber through the real
// router: arrow keys pressed at key-repeat rate each land one whole step from
// the last (none is lost to the chase), and a wheel burst never draws the
// handle back on its way to where it settles.
func TestVolumeKeysAndWheelAreExact(t *testing.T) {
	h := newPlayerHarness(t)
	store := state.New(protocol.Snapshot{
		Provider: model.ProviderYouTube,
		Playback: model.PlaybackSnapshot{Volume: 40},
	})
	e := &env.Env{Th: h.theme, Overlays: new(look.Overlays), Store: store}
	var volume volumeControl
	draw := func(gtx layout.Context) {
		gtx.Constraints = layout.Constraints{Max: image.Pt(400, 32)}
		volume.Layout(gtx, e, 100)
	}
	h.frame(draw)

	// the scrubber starts after the 32 plaque and 4 apart: x 76 is 40 %
	at := f32.Pt(36+40, 16)
	h.router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, PointerID: 1, Position: at})
	h.frame(draw)
	h.router.Queue(
		pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 1, Buttons: pointer.ButtonPrimary, Position: at},
		pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 1, Position: at},
	)
	h.frame(draw)
	h.frame(draw)
	if volume.target != 40 {
		t.Fatalf("a click at 40 %% set %d", volume.target)
	}

	// three presses 33 ms apart, as a held arrow repeats: 5 points each
	for press, want := range []int{45, 50, 55} {
		h.advance(33 * time.Millisecond)
		h.router.Queue(key.Event{Name: key.NameRightArrow, State: key.Press})
		h.frame(draw)
		if volume.target != want || !sameFraction(volume.scrubber.Shown(), float32(want)/100) {
			t.Fatalf("press %d: volume %d showing %v, want %d exactly", press+1, volume.target, volume.scrubber.Shown(), want)
		}
	}

	// a burst of three wheel notches up, 30 ms apart, then the settle
	last := volume.scrubber.Shown()
	for range 3 {
		h.advance(30 * time.Millisecond)
		h.router.Queue(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, PointerID: 1, Position: at, Scroll: f32.Pt(0, -wheelNotch())})
		h.frame(draw)
		if shown := volume.scrubber.Shown(); shown < last-fractionNoise {
			t.Fatalf("the wheel drew the handle back from %v to %v", last, shown)
		} else {
			last = shown
		}
	}
	for range 40 {
		h.advance(16 * time.Millisecond)
		h.frame(draw)
		if shown := volume.scrubber.Shown(); shown < last-fractionNoise {
			t.Fatalf("the settling wheel drew the handle back from %v to %v", last, shown)
		} else {
			last = shown
		}
	}
	if volume.target != 70 || !sameFraction(last, .7) {
		t.Fatalf("three notches up from 55 settled at %d showing %v, want 70", volume.target, last)
	}
}

// fractionNoise is float32 rounding in sums of steps; a backward move by the
// chase or a stale value is at least a whole percent.
const fractionNoise = 1e-4

func sameFraction(a, b float32) bool { return a-b < fractionNoise && b-a < fractionNoise }

// wheelNotch is one wheel notch as Gio reports it: 10 under X11 (no
// WAYLAND_DISPLAY, as in CI), 150 under Wayland (look/scroll.go).
func wheelNotch() float32 {
	if os.Getenv("WAYLAND_DISPLAY") == "" {
		return 10
	}
	return 150
}
