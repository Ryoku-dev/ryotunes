package look

import (
	"image"
	"testing"
	"time"

	"gioui.org/io/input"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/anim"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/palette"
	libtheme "github.com/ryoku-dev/ryoku/docs/lithic/library/go/theme"
)

func TestTipClosesOnGlobalExitAndRequiresReentry(t *testing.T) {
	reduced := anim.Reduced()
	anim.SetReduce(true)
	defer anim.SetReduce(reduced)

	theme := NewTheme(palette.Signature(true), libtheme.Comfortable)
	theme.Motion.TooltipDelay = 0
	var tip Tip
	var overlays Overlays
	var operations op.Ops
	var router input.Router
	now := time.Now()
	body := layout.Spacer{Width: 40, Height: 20}.Layout
	frame := func() {
		operations.Reset()
		gtx := layout.Context{
			Ops: &operations, Source: router.Source(), Now: now,
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Exact(image.Pt(200, 120)),
		}
		tip.Layout(gtx, theme, &overlays, "Detail", body)
		overlays.Layout(gtx, theme)
		router.Frame(&operations)
		now = now.Add(time.Millisecond)
	}

	tip.hovered = true
	tip.since = now
	tip.rect = image.Rect(10, 10, 50, 30)
	overlays.pointer = image.Pt(20, 20)
	frame()
	if !tip.shown || !overlays.desired(&tip.tag) {
		t.Fatal("hovered tip did not open")
	}
	anchor := overlays.entry(&tip.tag).anchor

	overlays.pointer = image.Pt(30, 20)
	frame()
	if got := overlays.entry(&tip.tag).anchor; got != anchor {
		t.Fatalf("tip followed pointer within anchor: got %v, want %v", got, anchor)
	}

	overlays.pointer = image.Pt(100, 80)
	frame()
	if tip.hovered || tip.shown || overlays.desired(&tip.tag) {
		t.Fatal("tip remained armed after the global pointer left its widget")
	}

	overlays.pointer = image.Pt(20, 20)
	frame()
	frame()
	if tip.hovered || overlays.desired(&tip.tag) {
		t.Fatal("tip reopened without a new pointer enter")
	}
}

func TestTipClosesWhenOwnerSkipsFrame(t *testing.T) {
	reduced := anim.Reduced()
	anim.SetReduce(true)
	defer anim.SetReduce(reduced)

	theme := NewTheme(palette.Signature(true), libtheme.Comfortable)
	theme.Motion.TooltipDelay = 0
	var tip Tip
	var overlays Overlays
	var operations op.Ops
	var router input.Router
	now := time.Now()
	body := layout.Spacer{Width: 40, Height: 20}.Layout
	frame := func(layoutOwner bool) {
		operations.Reset()
		gtx := layout.Context{
			Ops: &operations, Source: router.Source(), Now: now,
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Exact(image.Pt(200, 120)),
		}
		if layoutOwner {
			tip.Layout(gtx, theme, &overlays, "Detail", body)
		}
		overlays.Layout(gtx, theme)
		router.Frame(&operations)
		now = now.Add(time.Millisecond)
	}

	tip.hovered = true
	tip.since = now
	tip.rect = image.Rect(10, 10, 50, 30)
	overlays.pointer = image.Pt(20, 20)
	frame(true)
	if !overlays.desired(&tip.tag) {
		t.Fatal("hovered tip did not open")
	}
	frame(true)
	if !overlays.desired(&tip.tag) {
		t.Fatal("tip closed while its owner was still laid out")
	}

	frame(false)
	if overlays.desired(&tip.tag) || overlays.IsOpen(&tip.tag) {
		t.Fatal("tip remained open after its owner skipped a frame")
	}
}

func TestOpeningTipClosesPreviousTip(t *testing.T) {
	var overlays Overlays
	first, second := new(int), new(int)
	body := layout.Spacer{Width: 10, Height: 10}.Layout
	overlays.open(first, image.Rect(0, 0, 10, 10), Placement{Prefer: Below}, body, true)
	overlays.open(second, image.Rect(20, 0, 30, 10), Placement{Prefer: Below}, body, true)
	if overlays.desired(first) {
		t.Fatal("opening a tip left the previous tip active")
	}
	if !overlays.desired(second) {
		t.Fatal("new tip was not active")
	}
}
