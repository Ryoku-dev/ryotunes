package chrome

import (
	"image"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryotunes/internal/client/state"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

// TestQueueRowHoverWashesLeansAndRests drives a real pointer over a queue
// row: it takes the Hub's wash and 2 dp lean and shows its remove control;
// left, it settles back and the queue asks for no more frames.
func TestQueueRowHoverWashesLeansAndRests(t *testing.T) {
	h := newPlayerHarness(t)
	track := func(id string, queued bool) model.Track {
		return model.Track{VideoID: id, Title: id, Artists: "Artist", Queued: queued}
	}
	store := state.New(protocol.Snapshot{
		Provider: model.ProviderYouTube,
		Queue: model.QueueSnapshot{
			Items:        []model.QueueItem{track("a", false), track("b", true), track("c", true)},
			CurrentIndex: 0,
		},
	})
	e := &env.Env{Th: h.theme, Overlays: new(look.Overlays), Store: store}
	var queue Queue
	draw := func(gtx layout.Context) {
		gtx.Constraints = layout.Exact(image.Pt(340, 600))
		queue.Layout(gtx, e)
	}
	h.frame(draw)
	row := queue.items[queueEntryKey{videoID: "b"}]
	if row == nil {
		t.Fatal("the queue has no row for b")
	}

	// "Now playing" head 40, its row 56, "Up next" head 40: b's middle at 164
	h.router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: f32.Pt(150, 164)})
	h.frame(draw)
	for range 20 {
		h.advance(16 * time.Millisecond)
		h.frame(draw)
	}
	unitScale := layout.Context{Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}}
	if lean := row.wash.Lean(unitScale); lean != 2 {
		t.Fatalf("a hovered row leans 2 dp, got %d", lean)
	}
	if row.remove.Description != "Remove from queue" {
		t.Fatal("a hovered row shows its remove control")
	}

	h.router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: f32.Pt(150, 580)})
	h.frame(draw)
	for range 30 {
		h.advance(16 * time.Millisecond)
		h.frame(draw)
	}
	h.advance(time.Millisecond)
	h.frame(draw)
	if row.wash.Lean(unitScale) != 0 || h.wants() {
		t.Fatalf("a row left alone rests and asks for no frame (wants %v)", h.wants())
	}
}
