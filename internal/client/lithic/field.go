package lithic

import (
	"image"
	"image/color"

	"gioui.org/layout"
	"gioui.org/op"
)

// Field animates a mineral field when tone changes and snaps on palette changes.
type Field struct {
	tone  Tone
	from  color.NRGBA
	to    color.NRGBA
	blend Tween
	ready bool
}

// Layout paints the mineral field behind its content.
func (f *Field) Layout(gtx layout.Context, th *Theme, tone Tone, w layout.Widget) layout.Dimensions {
	target := th.R.Field(tone)
	f.blend.Duration = th.Motion.Field
	f.blend.Ease = EaseChisel
	if !f.ready {
		f.tone = tone
		f.from, f.to = target, target
		f.ready = true
		f.blend.Set(gtx, 1)
	} else if f.tone != tone {
		f.from = Mix(f.from, f.to, f.blend.Value(gtx))
		f.to = target
		f.tone = tone
		f.blend = Tween{Duration: th.Motion.Field, Ease: EaseChisel}
		f.blend.Set(gtx, 0)
		f.blend.Set(gtx, 1)
	} else if target != f.to {
		// The palette changed under a steady state: follow it at once rather
		// than keep painting the old scheme's stone.
		f.from, f.to = target, target
	}
	ink := Mix(f.from, f.to, f.blend.Value(gtx))
	child := op.Record(gtx.Ops)
	if gtx.Constraints.Max.X > 0 {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
	}
	var dims layout.Dimensions
	if w != nil {
		dims = w(gtx)
	} else {
		dims.Size = gtx.Constraints.Min
	}
	call := child.Stop()
	FillCut(gtx.Ops, image.Rectangle{Max: dims.Size}, 0, ink)
	call.Add(gtx.Ops)
	return dims
}
