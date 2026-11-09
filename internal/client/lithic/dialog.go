package lithic

import (
	"image"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
)

// Dialog is a modal cut slab over a solid scrim.
type Dialog struct {
	Open  bool
	Width unit.Dp

	scrimTag   int
	blockerTag int
	enter      Tween
	wasOpen    bool
	visible    bool
	closed     bool
	slabRect   image.Rectangle
}

func (d *Dialog) dismiss() {
	d.Open = false
}

// Layout renders the dialog and its right-aligned actions.
func (d *Dialog) Layout(gtx layout.Context, th *Theme, title string, body layout.Widget, actions ...layout.Widget) layout.Dimensions {
	window := gtx.Constraints.Max
	opening := d.Open && (!d.wasOpen || !d.visible)
	closing := !d.Open && d.wasOpen
	if opening {
		d.visible = true
		d.closed = false
		d.enter = Tween{Duration: th.Motion.DialogIn, Ease: EaseChisel}
		d.enter.Set(gtx, 0)
		d.enter.Set(gtx, 1)
	}
	if closing {
		d.enter.Ease = EaseChisel
		d.enter.Set(gtx, 0)
		d.enter.Duration = th.Motion.DialogOut
	}
	d.wasOpen = d.Open
	if !d.visible {
		d.slabRect = image.Rectangle{}
		return layout.Dimensions{Size: window}
	}

	if d.Open {
		for {
			e, ok := gtx.Event(pointer.Filter{Target: &d.scrimTag, Kinds: pointer.Press})
			if !ok {
				break
			}
			press, ok := e.(pointer.Event)
			if ok && !press.Position.Round().In(d.slabRect) {
				d.dismiss()
				d.wasOpen = false
				d.enter.Ease = EaseChisel
				d.enter.Set(gtx, 0)
				d.enter.Duration = th.Motion.DialogOut
			}
		}
		for {
			e, ok := gtx.Event(key.Filter{Name: key.NameEscape})
			if !ok {
				break
			}
			if e, ok := e.(key.Event); ok && e.State == key.Press {
				d.dismiss()
				d.wasOpen = false
				d.enter.Ease = EaseChisel
				d.enter.Set(gtx, 0)
				d.enter.Duration = th.Motion.DialogOut
			}
		}
	}

	if !d.Open {
		for {
			_, ok := gtx.Event(pointer.Filter{Target: &d.blockerTag, Kinds: pointer.Press | pointer.Release})
			if !ok {
				break
			}
		}
	}

	progress := d.enter.Value(gtx)
	if !d.Open && progress <= 0 {
		d.visible = false
		d.closed = true
		d.slabRect = image.Rectangle{}
		return layout.Dimensions{Size: window}
	}

	margin := gtx.Dp(16)
	dialogWidth := d.Width
	if dialogWidth <= 0 {
		dialogWidth = 520
	}
	width := min(gtx.Dp(dialogWidth), max(0, window.X-2*margin))
	padding := gtx.Dp(34)
	innerWidth := max(0, width-2*padding)
	maxInnerHeight := max(0, window.Y-2*margin-2*padding)

	insideCtx := overlayContentContext(gtx)
	if !d.Open {
		insideCtx = insideCtx.Disabled()
	}
	insideCtx.Constraints.Min = image.Pt(innerWidth, 0)
	insideCtx.Constraints.Max = image.Pt(innerWidth, maxInnerHeight)
	insideRecord := op.Record(gtx.Ops)
	children := make([]layout.FlexChild, 0, 5)
	if title != "" {
		children = append(children,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return Label{Style: th.T.Sentence, Color: th.R.Fg, MaxLines: 2}.Layout(gtx, th, title)
			}),
			layout.Rigid(layout.Spacer{Height: 21}.Layout),
		)
	}
	children = append(children, layout.Rigid(body))
	if len(actions) > 0 {
		children = append(children,
			layout.Rigid(layout.Spacer{Height: 34}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				row := make([]layout.FlexChild, 0, len(actions)*2-1)
				for i, action := range actions {
					if i > 0 {
						row = append(row, layout.Rigid(layout.Spacer{Width: 8}.Layout))
					}
					row = append(row, layout.Rigid(action))
				}
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceStart}.Layout(gtx, row...)
			}),
		)
	}
	insideDims := layout.Flex{Axis: layout.Vertical}.Layout(insideCtx, children...)
	insideCall := insideRecord.Stop()
	height := min(window.Y-2*margin, insideDims.Size.Y+2*padding)
	height = max(0, height)
	x := (window.X - width) / 2
	baseY := (window.Y - height) / 2
	travel := gtx.Dp(12)
	if !d.Open {
		travel = gtx.Dp(8)
	}
	y := baseY + int(float32(travel)*(1-progress))
	d.slabRect = image.Rect(x, y, x+width, y+height)

	overlay := op.Record(gtx.Ops)
	opacity := paint.PushOpacity(gtx.Ops, progress)
	if d.Open {
		catcher := clip.Rect(image.Rect(-farAway, -farAway, farAway, farAway)).Push(gtx.Ops)
		event.Op(gtx.Ops, &d.scrimTag)
		catcher.Pop()
	}
	FillCut(gtx.Ops, image.Rect(-farAway, -farAway, farAway, farAway), 0, th.R.Scrim)

	offset := op.Offset(image.Pt(x, y)).Push(gtx.Ops)
	r := image.Rect(0, 0, width, height)
	cut := gtx.Dp(th.Cut.Large)
	FillCut(gtx.Ops, r, cut, th.R.Stone4)
	StrokeCut(gtx.Ops, r, cut, max(1, gtx.Dp(1)), th.R.Vein)
	contentOffset := op.Offset(image.Pt(padding, padding)).Push(gtx.Ops)
	insideCall.Add(gtx.Ops)
	contentOffset.Pop()
	offset.Pop()
	if !d.Open {
		blocker := clip.Rect(image.Rect(-farAway, -farAway, farAway, farAway)).Push(gtx.Ops)
		event.Op(gtx.Ops, &d.blockerTag)
		blocker.Pop()
	}
	opacity.Pop()
	op.Defer(gtx.Ops, overlay.Stop())
	return layout.Dimensions{Size: window}
}

// Closed reports and clears a dialog dismissal.
func (d *Dialog) Closed(layout.Context) bool {
	if !d.closed {
		return false
	}
	d.closed = false
	return true
}
