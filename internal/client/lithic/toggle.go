package lithic

import (
	"image"

	"gioui.org/io/key"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/widget"
)

// Toggle is a labelled cut track with a moving mineral knob.
type Toggle struct {
	widget.Bool
	slide, tone, hover, focus Tween
}

// Layout renders and updates the boolean toggle.
func (t *Toggle) Layout(gtx layout.Context, th *Theme, label string) layout.Dimensions {
	for {
		event, ok := gtx.Event(key.Filter{Focus: &t.Bool, Name: key.NameSpace})
		if !ok {
			break
		}
		if event, ok := event.(key.Event); ok && event.State == key.Press {
			t.Value = !t.Value
		}
	}

	original := gtx
	gtx.Constraints.Min = image.Point{}
	trackWidth, trackHeight := gtx.Dp(40), gtx.Dp(22)
	trackCut := gtx.Dp(4)
	dims := t.Bool.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		semantic.Switch.Add(gtx.Ops)
		if label != "" {
			semantic.LabelOp(label).Add(gtx.Ops)
		}

		t.slide.Duration = th.Motion.Select
		t.slide.Ease = EaseChisel
		t.tone.Duration = th.Motion.Field
		t.tone.Ease = EaseChisel
		target := float32(0)
		if t.Value {
			target = 1
		}
		t.slide.Set(gtx, target)
		t.tone.Set(gtx, target)
		position := t.slide.Value(gtx)
		tone := t.tone.Value(gtx)
		hover := hoverValue(gtx, th, &t.hover, t.Hovered())
		disabled := !original.Source.Enabled()

		var children [3]layout.FlexChild
		count := 0
		children[count] = layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			size := constrainMax(gtx, image.Pt(trackWidth, trackHeight))
			track := image.Rectangle{Max: size}
			var layer paint.OpacityStack
			if disabled {
				layer = paint.PushOpacity(gtx.Ops, disabledAlpha)
			}
			trackColor := Mix(th.R.Stone4, th.R.Sel, tone)
			trackColor = Mix(trackColor, th.R.Fg, .06*hover)
			FillCut(gtx.Ops, track, trackCut, trackColor)
			if tone < 1 {
				StrokeCut(gtx.Ops, track, trackCut, max(1, gtx.Dp(1)), WithAlpha(th.R.Vein, 1-tone))
			}

			knobSize := min(gtx.Dp(14), min(size.X, size.Y))
			knobCut := min(gtx.Dp(2), knobSize/2)
			left := min(gtx.Dp(4), max(0, size.X-knobSize))
			right := max(left, size.X-gtx.Dp(4)-knobSize)
			x := int(lerp(float32(left), float32(right), position) + 0.5)
			y := (size.Y - knobSize) / 2
			knobColor := Mix(th.R.Fg2, th.R.OnSel, position)
			offset := op.Offset(image.Pt(x, y)).Push(gtx.Ops)
			FillCut(gtx.Ops, image.Rect(0, 0, knobSize, knobSize), knobCut, knobColor)
			offset.Pop()
			if disabled {
				layer.Pop()
			}
			return layout.Dimensions{Size: size}
		})
		count++
		if label != "" {
			children[count] = layout.Rigid(layout.Spacer{Width: 8}.Layout)
			count++
			ink := th.R.Fg
			if disabled {
				ink = th.R.Fg3
			}
			children[count] = layout.Rigid(rawText(th, th.T.Label, ink, label))
			count++
		}
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children[:count]...)
	})
	focus := focusValue(original, th, &t.focus, original.Focused(&t.Bool))
	FocusRingAlpha(original, th, image.Rectangle{Max: dims.Size}, trackCut, focus)
	return dims
}
