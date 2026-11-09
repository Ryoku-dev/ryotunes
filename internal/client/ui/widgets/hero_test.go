package widgets

import (
	"image"
	"testing"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
)

func TestHeroFactsKeepMetadataWhole(t *testing.T) {
	t.Parallel()
	meta := []kit.Span{
		{Text: "Artist"},
		{Text: " · "},
		{Text: "8 songs", Mono: true},
		{Text: " · "},
		{Text: "54 min", Mono: true},
	}
	var hero Hero
	facts := hero.prepareFacts(meta)
	if len(facts) != 3 {
		t.Fatalf("prepareFacts returned %d facts, want 3", len(facts))
	}
	want := [][2]int{{0, 1}, {2, 3}, {4, 5}}
	for index, fact := range facts {
		if fact.start != want[index][0] || fact.end != want[index][1] {
			t.Fatalf("fact %d range = [%d,%d), want [%d,%d)", index, fact.start, fact.end, want[index][0], want[index][1])
		}
	}
}

func TestHeroActionsFoldFromRight(t *testing.T) {
	t.Parallel()
	var ops op.Ops
	gtx := layout.Context{
		Ops:         &ops,
		Constraints: layout.Exact(image.Pt(220, 48)),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
	}
	action := func(gtx layout.Context) layout.Dimensions {
		return layout.Dimensions{Size: gtx.Constraints.Constrain(image.Pt(60, 40))}
	}
	widgets := []layout.Widget{action, action, action, action, action}
	var actions HeroActions
	actions.Layout(gtx, widgets)
	if actions.Hidden(1) {
		t.Fatal("Shuffle folded before lower-priority actions")
	}
	if !actions.Hidden(2) || !actions.Hidden(3) {
		t.Fatal("Save and Download must fold before Shuffle")
	}
	if actions.Hidden(0) || actions.Hidden(4) {
		t.Fatal("Play and More must remain visible")
	}
}
