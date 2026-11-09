package look

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

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/anim"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/palette"
	libtheme "github.com/ryoku-dev/ryoku/docs/lithic/library/go/theme"
)

func TestPlaceOverlay(t *testing.T) {
	window := image.Pt(400, 300)
	tests := []struct {
		name    string
		anchor  image.Rectangle
		content image.Point
		prefer  Side
		align   Align
		want    image.Point
		side    Side
	}{
		{"below", image.Rect(100, 50, 160, 80), image.Pt(120, 80), Below, Start, image.Pt(100, 88), Below},
		{"above", image.Rect(100, 180, 160, 210), image.Pt(120, 80), Above, Start, image.Pt(100, 92), Above},
		{"flip above", image.Rect(200, 260, 240, 280), image.Pt(100, 80), Below, Start, image.Pt(200, 172), Above},
		{"flip below", image.Rect(200, 10, 240, 30), image.Pt(100, 80), Above, Start, image.Pt(200, 38), Below},
		{"clamp left", image.Rect(-20, 80, 20, 100), image.Pt(100, 60), Below, Start, image.Pt(8, 108), Below},
		{"clamp right", image.Rect(370, 80, 390, 100), image.Pt(100, 60), Below, Start, image.Pt(292, 108), Below},
		{"clamp top", image.Rect(100, -20, 140, 0), image.Pt(100, 400), Above, Start, image.Pt(100, 8), Above},
		{"clamp bottom", image.Rect(100, 290, 140, 310), image.Pt(100, 400), Below, Start, image.Pt(100, 8), Below},
		{"end alignment", image.Rect(200, 80, 260, 100), image.Pt(100, 60), Below, End, image.Pt(160, 108), Below},
		{"larger than window", image.Rect(20, 20, 40, 40), image.Pt(500, 500), Below, Start, image.Pt(8, 8), Below},
	}
	for _, test := range tests {
		got, side := placeOverlay(window, test.anchor, test.content, 8, 8, test.prefer, test.align)
		if got != test.want || side != test.side {
			t.Errorf("%s: got %v/%v, want %v/%v", test.name, got, side, test.want, test.side)
		}
	}
}

func TestOverlayTopIgnoresPassiveEntries(t *testing.T) {
	overlays := Overlays{items: []*overlayEntry{
		{tag: "menu"},
		{tag: "search", placement: Placement{Passive: true}},
		{tag: "tip", tip: true},
	}}
	tag, ok := overlays.Top()
	if !ok || tag != "menu" {
		t.Fatalf("Top() = (%v, %v), want (menu, true)", tag, ok)
	}

	overlays.items = overlays.items[1:]
	if tag, ok := overlays.Top(); ok {
		t.Fatalf("passive-only Top() = (%v, true), want (_, false)", tag)
	}
}

func TestOverlayFocusReturnTarget(t *testing.T) {
	opener := new(struct{})
	tests := []struct {
		name  string
		entry overlayEntry
		want  any
	}{
		{"entry by default", overlayEntry{tag: "menu"}, "menu"},
		{"explicit opener", overlayEntry{tag: "menu", placement: Placement{Return: opener}}, opener},
		{"passive leaves focus", overlayEntry{tag: "search", placement: Placement{Passive: true, Return: opener}}, nil},
		{"tip leaves focus", overlayEntry{tag: "tip", tip: true, placement: Placement{Return: opener}}, nil},
	}
	for _, test := range tests {
		if got := test.entry.focusReturn(); got != test.want {
			t.Errorf("%s: focusReturn() = %v, want %v", test.name, got, test.want)
		}
	}
}

func TestOverlayRoutesInsideAndOutsidePresses(t *testing.T) {
	reduced := anim.Reduced()
	anim.SetReduce(true)
	defer anim.SetReduce(reduced)

	theme := NewTheme(palette.Signature(true), libtheme.Comfortable)
	var overlays Overlays
	var inside, underneath kit.Click
	insideClicked, underneathClicked := false, false
	content := func(gtx layout.Context) layout.Dimensions {
		dimensions := inside.Layout(gtx, layout.Spacer{Width: 100, Height: 40}.Layout)
		if inside.Clicked(gtx) {
			insideClicked = true
		}
		return dimensions
	}
	overlays.Open("menu", image.Rect(20, 20, 40, 40), Placement{Prefer: Below}, content)

	var operations op.Ops
	var router input.Router
	now := time.Now()
	frame := func() {
		operations.Reset()
		gtx := layout.Context{
			Ops: &operations, Source: router.Source(), Now: now,
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Exact(image.Pt(300, 200)),
		}
		underneath.Layout(gtx, layout.Spacer{Width: 300, Height: 200}.Layout)
		if underneath.Clicked(gtx) {
			underneathClicked = true
		}
		overlays.Layout(gtx, theme)
		router.Frame(&operations)
		now = now.Add(time.Millisecond)
	}
	click := func(point f32.Point) {
		router.Queue(
			pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: point, PointerID: 1},
			pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: point, PointerID: 1},
		)
		frame()
		router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: point, PointerID: 1})
		frame()
	}

	frame()
	click(f32.Pt(50, 60))
	if !insideClicked {
		t.Fatal("press inside overlay did not reach its clickable")
	}
	if underneathClicked {
		t.Fatal("press inside overlay reached the clickable underneath")
	}

	insideClicked = false
	click(f32.Pt(22, 50))
	if !overlays.IsOpen("menu") {
		t.Fatal("press on overlay chrome closed the overlay")
	}
	if insideClicked || underneathClicked {
		t.Fatal("press on overlay chrome was not swallowed behind content")
	}

	click(f32.Pt(250, 150))
	if overlays.IsOpen("menu") {
		t.Fatal("outside press did not close overlay")
	}
	if underneathClicked {
		t.Fatal("outside press reached the clickable underneath")
	}
}
