package lithic

import (
	"image"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
)

// Disclosure reveals optional content without making closed content interactive.
type Disclosure struct {
	reveal Tween
	ready  bool
}

// Layout reveals or closes optional content with Lithic's unfold motion.
func (d *Disclosure) Layout(gtx layout.Context, th *Theme, open bool, content layout.Widget) layout.Dimensions {
	d.reveal.Duration = th.Motion.Unfold
	d.reveal.Ease = EaseChisel
	if !d.ready {
		d.ready = true
		d.reveal.Set(gtx, 0)
		if open {
			d.reveal.Set(gtx, 1)
		}
	} else if open {
		d.reveal.Set(gtx, 1)
	} else {
		d.reveal.Set(gtx, 0)
	}
	progress := d.reveal.Value(gtx)
	if !open && progress <= 0 {
		return layout.Dimensions{Size: gtx.Constraints.Constrain(image.Point{})}
	}

	contentGTX := gtx
	contentGTX.Constraints.Min.Y = 0
	if !open {
		contentGTX = contentGTX.Disabled()
	}
	record := op.Record(gtx.Ops)
	dims := content(contentGTX)
	call := record.Stop()
	visible := min(dims.Size.Y, max(0, int(float32(dims.Size.Y)*progress+.5)))
	viewport := clip.Rect(image.Rect(0, 0, dims.Size.X, visible)).Push(gtx.Ops)
	// An opacity layer costs an offscreen pass, so only while moving.
	if progress < 1 {
		opacity := paint.PushOpacity(gtx.Ops, progress)
		call.Add(gtx.Ops)
		opacity.Pop()
	} else {
		call.Add(gtx.Ops)
	}
	viewport.Pop()
	return layout.Dimensions{Size: image.Pt(dims.Size.X, visible), Baseline: dims.Baseline}
}
