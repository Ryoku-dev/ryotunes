package chrome

import (
	"testing"
	"time"

	"gioui.org/layout"

	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
)

func TestUpNextDropsRowsBeforeTheSpecStrip(t *testing.T) {
	const head, row, gap, spec = 24, 44, 2, 23
	cases := []struct {
		height, upcoming, want int
	}{
		{height: 24 + 23 + 3*44 + 2*2, upcoming: 10, want: 3},
		{height: 24 + 23 + 3*44 + 2*2 - 1, upcoming: 10, want: 2},
		{height: 24 + 23 + 44, upcoming: 10, want: 1},
		{height: 24 + 23 + 43, upcoming: 10, want: 0},
		{height: 400, upcoming: 2, want: 2},
		{height: 400, upcoming: 0, want: 0},
	}
	for _, c := range cases {
		if got := upNextFit(c.height, head, row, gap, spec, c.upcoming); got != c.want {
			t.Errorf("height %d, %d to come: %d rows, want %d", c.height, c.upcoming, got, c.want)
		}
	}
}

// TestUpNextRowsTravelWhenTheQueueMovesOn: the first track stands at once;
// the next one starts its rows a slot low and they rise into place, then
// rest without asking for frames.
func TestUpNextRowsTravelWhenTheQueueMovesOn(t *testing.T) {
	h := newPlayerHarness(t)
	var block upNext
	var travel float32
	e := envWithTheme(h)
	draw := func(id string) func(layout.Context) {
		return func(gtx layout.Context) { travel = block.travelled(gtx, e, id) }
	}
	h.frame(draw("a"))
	if travel != 0 || h.wants() {
		t.Fatalf("the first track's rows stand at once (travel %v, wants frames %v)", travel, h.wants())
	}
	h.frame(draw("b"))
	if travel != 1 || !h.wants() {
		t.Fatalf("a new track starts the rows a slot low (travel %v)", travel)
	}
	h.advance(h.theme.Motion.Move / 2)
	h.frame(draw("b"))
	if travel <= 0 || travel >= 1 {
		t.Fatalf("half way, the rows should be between slots, got %v", travel)
	}
	h.advance(h.theme.Motion.Move)
	h.frame(draw("b"))
	h.advance(time.Millisecond)
	h.frame(draw("b"))
	if travel != 0 || h.wants() {
		t.Fatalf("landed rows rest (travel %v, wants frames %v)", travel, h.wants())
	}
}

func envWithTheme(h *playerHarness) *env.Env { return &env.Env{Th: h.theme} }
