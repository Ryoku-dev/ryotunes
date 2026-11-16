package sheet

import (
	"image"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
)

// Spec strip geometry (PageHeader.qml:115-139), in dp. The page head puts
// SpecStripAbove between its words and the strip.
const (
	SpecStripAbove unit.Dp = 16
	specGap        unit.Dp = 18
	specLabelGap   unit.Dp = 6
	specTracking           = 1.2
)

// Spec is one figure of a page's spec strip: its label in sentence case (set
// in spaced capitals), its value as it reads ("1204", Pad2 for counts under
// a hundred), and Lit for a value worth the accent.
type Spec struct {
	Label string
	Value string
	Lit   bool
}

// SpecStrip lays a page's figures out on one line, as the Hub's page head
// does: each label in the smallest mono, spaced capitals, fg3, then 6 on its
// value in the smallest mono, fg (sel when lit), the figures 18 apart:
// "PLAYLISTS 24   SONGS 1204   ALBUMS 88". It allocates nothing for figures
// already shown.
func SpecStrip(gtx layout.Context, th *look.Theme, specs []Spec) layout.Dimensions {
	if len(specs) == 0 {
		return layout.Dimensions{}
	}
	gtx.Constraints.Min = image.Point{}
	labelGap, gap := gtx.Dp(specLabelGap), gtx.Dp(specGap)
	x, height, baseline := 0, 0, 0
	for index, spec := range specs {
		if index > 0 {
			x += gap
		}
		offset := op.Offset(image.Pt(x, 0)).Push(gtx.Ops)
		label := kit.LayoutSpacedCaps(gtx, th.Theme, th.T.MonoSm, th.R.Fg3, specTracking, spec.Label)
		offset.Pop()
		x += label.Size.X + labelGap
		ink := th.R.Fg
		if spec.Lit {
			ink = th.R.Sel
		}
		offset = op.Offset(image.Pt(x, 0)).Push(gtx.Ops)
		value := kit.LayoutSpacedCaps(gtx, th.Theme, th.T.MonoSm, ink, 0, spec.Value)
		offset.Pop()
		x += value.Size.X
		height = max(height, label.Size.Y, value.Size.Y)
		baseline = max(baseline, label.Baseline)
	}
	return layout.Dimensions{Size: gtx.Constraints.Constrain(image.Pt(x, height)), Baseline: baseline}
}
