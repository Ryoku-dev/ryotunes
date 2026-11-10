package look

import (
	"image"
	"image/color"
	"time"

	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/anim"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/palette"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/tokens"
)

type flowChip struct {
	kit.Click
	press anim.Tween
	hover anim.Tween
	focus anim.Tween
}

// FlowChoice is a single-select set of filter plaques that wraps to fit.
type FlowChoice struct {
	Labels   []string
	Selected int

	chips                                  []flowChip
	calls                                  []op.CallOp
	sizes                                  []image.Point
	positions                              []image.Point
	selectionX, selectionY, selectionWidth anim.Tween
	changed                                int
	pending                                bool
}

// Changed reports and clears the selected-index change latch.
func (f *FlowChoice) Changed() (int, bool) {
	if !f.pending {
		return 0, false
	}
	changed := f.changed
	f.pending = false
	return changed, true
}

// Layout draws filter-style plaques, wrapping whole choices onto new rows.
func (f *FlowChoice) Layout(gtx layout.Context, th *Theme) layout.Dimensions {
	f.ensure(len(f.Labels))
	for i := range f.chips {
		if f.chips[i].Clicked(gtx) && i != f.Selected {
			f.Selected = i
			f.changed, f.pending = i, true
		}
	}
	if len(f.Labels) == 0 {
		return layout.Dimensions{}
	}
	f.Selected = min(max(f.Selected, 0), len(f.Labels)-1)

	gap := gtx.Dp(unit.Dp(tokens.SizeFilterGap))
	available := max(0, gtx.Constraints.Max.X)
	x, y, rowHeight, width := 0, 0, 0, 0
	for i, label := range f.Labels {
		child := gtx
		child.Constraints.Min = image.Point{}
		record := op.Record(gtx.Ops)
		dimensions := f.layoutChip(child, th, i, label)
		f.calls[i] = record.Stop()
		f.sizes[i] = dimensions.Size

		if x > 0 && x+dimensions.Size.X > available {
			x = 0
			y += rowHeight + gap
			rowHeight = 0
		}
		f.positions[i] = image.Pt(x, y)
		x += dimensions.Size.X + gap
		width = max(width, x-gap)
		rowHeight = max(rowHeight, dimensions.Size.Y)
	}
	height := y + rowHeight

	target := f.positions[f.Selected]
	targetWidth := f.sizes[f.Selected].X
	f.selectionX.Duration, f.selectionY.Duration, f.selectionWidth.Duration = th.Motion.Select, th.Motion.Select, th.Motion.Select
	f.selectionX.Ease, f.selectionY.Ease, f.selectionWidth.Ease = anim.EaseChisel, anim.EaseChisel, anim.EaseChisel
	f.selectionX.Set(gtx, float32(target.X))
	f.selectionY.Set(gtx, float32(target.Y))
	f.selectionWidth.Set(gtx, float32(targetWidth))
	selectionX := f.selectionX.Value(gtx)
	selectionY := f.selectionY.Value(gtx)
	selectionWidth := f.selectionWidth.Value(gtx)
	selection := image.Rect(
		int(selectionX+.5),
		int(selectionY+.5),
		int(selectionX+selectionWidth+.5),
		int(selectionY+.5)+gtx.Dp(unit.Dp(tokens.SizeFilterHeight)),
	)
	kit.FillCut(gtx.Ops, selection, gtx.Dp(th.Cut.Small), flowDisabled(gtx, th.R.Sel))

	for i, call := range f.calls {
		offset := op.Offset(f.positions[i]).Push(gtx.Ops)
		call.Add(gtx.Ops)
		offset.Pop()
	}
	return layout.Dimensions{Size: gtx.Constraints.Constrain(image.Pt(width, height))}
}

func (f *FlowChoice) layoutChip(gtx layout.Context, th *Theme, index int, label string) layout.Dimensions {
	chip := &f.chips[index]
	selected := index == f.Selected
	cut := gtx.Dp(th.Cut.Small)
	dimensions := chip.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		semantic.Button.Add(gtx.Ops)
		semantic.LabelOp(label).Add(gtx.Ops)
		semantic.SelectedOp(selected).Add(gtx.Ops)
		semantic.EnabledOp(gtx.Source.Enabled()).Add(gtx.Ops)

		press := flowTransition(gtx, &chip.press, chip.Pressed(), th.Motion.Press, th.Motion.Press)
		cut = gtx.Dp(th.Cut.Small) + int(float32(gtx.Dp(unit.Dp(tokens.SizeControlPressCutGrow)))*press+.5)
		sink := int(float32(gtx.Dp(unit.Dp(tokens.SizeControlPressSink)))*press + .5)
		hover := flowTransition(gtx, &chip.hover, chip.Hovered() && !selected, th.Motion.HoverIn, th.Motion.HoverOut)

		ink := flowDisabled(gtx, th.R.Fg2)
		fill := color.NRGBA{}
		outline := flowDisabled(gtx, th.R.Vein)
		stroke := true
		if selected {
			ink = flowDisabled(gtx, th.R.OnSel)
			fill = flowDisabled(gtx, palette.WithAlpha(th.R.OnSel, tokens.AlphaPressRow*press))
			stroke = false
		} else if hover > 0 {
			fill = flowDisabled(gtx, palette.WithAlpha(th.R.Fg2, tokens.AlphaHoverFill*hover))
		}

		contentGTX := gtx
		contentGTX.Constraints.Min = image.Point{}
		contentGTX.Constraints.Max.Y = gtx.Dp(unit.Dp(tokens.SizeFilterHeight))
		record := op.Record(gtx.Ops)
		content := layout.Inset{Left: unit.Dp(tokens.SizeFilterInset), Right: unit.Dp(tokens.SizeFilterInset)}.Layout(contentGTX, kit.RawText(th.Theme, th.T.Label, ink, label))
		call := record.Stop()
		size := gtx.Constraints.Constrain(image.Pt(content.Size.X, gtx.Dp(unit.Dp(tokens.SizeFilterHeight))))
		bounds := image.Rectangle{Max: size}
		kit.FillCut(gtx.Ops, bounds, cut, fill)
		if stroke {
			kit.StrokeCut(gtx.Ops, bounds, cut, max(1, gtx.Dp(unit.Dp(tokens.SizeHairline))), outline)
		}
		offset := op.Offset(image.Pt((size.X-content.Size.X)/2, (size.Y-content.Size.Y)/2+sink)).Push(gtx.Ops)
		call.Add(gtx.Ops)
		offset.Pop()
		return layout.Dimensions{Size: size, Baseline: content.Baseline + (size.Y-content.Size.Y)/2 + sink}
	})
	focus := flowTransition(gtx, &chip.focus, gtx.Focused(&chip.Clickable), th.Motion.FocusIn, th.Motion.FocusOut)
	kit.FocusRingAlpha(gtx, th.Theme, image.Rectangle{Max: dimensions.Size}, cut, focus)
	return dimensions
}

func (f *FlowChoice) ensure(count int) {
	if cap(f.chips) < count {
		f.chips = append(f.chips, make([]flowChip, count-len(f.chips))...)
	} else {
		f.chips = f.chips[:count]
	}
	if cap(f.calls) < count {
		f.calls = make([]op.CallOp, count)
	} else {
		f.calls = f.calls[:count]
	}
	if cap(f.sizes) < count {
		f.sizes = make([]image.Point, count)
	} else {
		f.sizes = f.sizes[:count]
	}
	if cap(f.positions) < count {
		f.positions = make([]image.Point, count)
	} else {
		f.positions = f.positions[:count]
	}
}

func flowTransition(gtx layout.Context, tween *anim.Tween, active bool, in, out time.Duration) float32 {
	tween.Duration = out
	if active {
		tween.Duration = in
	}
	tween.Ease = anim.EaseChisel
	if active {
		tween.Set(gtx, 1)
	} else {
		tween.Set(gtx, 0)
	}
	return tween.Value(gtx)
}

func flowDisabled(gtx layout.Context, ink color.NRGBA) color.NRGBA {
	if !gtx.Source.Enabled() {
		return palette.WithAlpha(ink, tokens.AlphaDisabled)
	}
	return ink
}
