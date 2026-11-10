package widgets

import (
	"image"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/palette"
	libtheme "github.com/ryoku-dev/ryoku/docs/lithic/library/go/theme"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
	"github.com/ryoku-dev/ryotunes/internal/model"
)

func TestGridRows(t *testing.T) {
	t.Parallel()
	var grid Grid
	cases := []struct {
		width, count, want int
	}{
		{160, 0, 0},
		{160, 3, 3},
		{341, 4, 2},
		{1_000, 11, 3},
	}
	for _, test := range cases {
		if got := grid.Rows(test.width, test.count); got != test.want {
			t.Errorf("Rows(%d, %d) = %d, want %d", test.width, test.count, got, test.want)
		}
	}
}

func TestCardContextMenuOpensAfterReleaseAtPressPoint(t *testing.T) {
	theme := look.NewTheme(palette.Signature(true), libtheme.Comfortable)
	overlays := new(look.Overlays)
	e := &env.Env{Th: theme, Overlays: overlays}
	item := model.BrowseItem{ID: "album", Kind: "album", Title: "Album"}
	var card Card
	var operations op.Ops
	var router input.Router
	now := time.Unix(100, 0)
	frame := func() {
		operations.Reset()
		gtx := layout.Context{
			Ops: &operations, Source: router.Source(), Now: now,
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Constraints{Max: image.Pt(640, 400)},
		}
		card.Layout(gtx, e, item, 176)
		overlays.Layout(gtx, theme)
		router.Frame(&operations)
		now = now.Add(time.Millisecond)
	}

	frame()
	point := f32.Pt(80, 80)
	router.Queue(pointer.Event{
		Kind: pointer.Press, Source: pointer.Mouse, PointerID: 3,
		Buttons: pointer.ButtonSecondary, Position: point,
	})
	frame()
	router.Queue(pointer.Event{Kind: pointer.Cancel, Source: pointer.Mouse, PointerID: 3, Position: point})
	frame()
	router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 3, Position: point})
	frame()
	if overlays.IsOpen(&card.menu.overlayTag) {
		t.Fatal("release after cancel opened the card menu")
	}
	router.Queue(pointer.Event{
		Kind: pointer.Press, Source: pointer.Mouse, PointerID: 4,
		Buttons: pointer.ButtonSecondary, Position: point,
	})
	frame()
	if overlays.IsOpen(&card.menu.overlayTag) {
		t.Fatal("secondary press opened the card menu before its release")
	}
	router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 4, Position: point})
	frame()
	frame()
	if !overlays.IsOpen(&card.menu.overlayTag) || overlays.Press() != point.Round() {
		t.Fatalf("card menu open = %v, press = %v, want %v", overlays.IsOpen(&card.menu.overlayTag), overlays.Press(), point.Round())
	}
}
