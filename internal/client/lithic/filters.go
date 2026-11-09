package lithic

import (
	"image"
	"strconv"

	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
)

type filterButton struct {
	Click
	press, hover, focus Tween
}

// Filters owns a row of counted chips and its moving bone slab.
type Filters struct {
	buttons  []filterButton
	widths   []int
	x, width Tween
	changed  int
	pending  bool
}

func (f *Filters) ensure(n int) {
	for len(f.buttons) < n {
		f.buttons = append(f.buttons, filterButton{})
	}
	f.buttons = f.buttons[:n]
}

func (f *Filters) poll(gtx layout.Context) {
	for i := range f.buttons {
		if f.buttons[i].Clicked(gtx) {
			f.changed, f.pending = i, true
		}
	}
}

// Changed reports and clears a selected chip index.
func (f *Filters) Changed(gtx layout.Context) (int, bool) {
	f.poll(gtx)
	if !f.pending {
		return 0, false
	}
	changed := f.changed
	f.pending = false
	return changed, true
}

// Layout renders filter labels and counts with a moving selection.
func (f *Filters) Layout(gtx layout.Context, th *Theme, labels []string, counts []int, selected int) layout.Dimensions {
	gtx.Constraints.Min = image.Point{}
	f.ensure(len(labels))
	f.poll(gtx)
	if cap(f.widths) < len(labels) {
		f.widths = make([]int, len(labels))
	} else {
		f.widths = f.widths[:len(labels)]
	}
	gap := gtx.Dp(4)
	record := op.Record(gtx.Ops)
	children := make([]layout.FlexChild, 0, len(labels))
	for i, label := range labels {
		i, label := i, label
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			var cut int
			dims := f.buttons[i].Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				semantic.Button.Add(gtx.Ops)
				semantic.LabelOp(label).Add(gtx.Ops)
				semantic.SelectedOp(i == selected).Add(gtx.Ops)
				f.buttons[i].press.Duration = th.Motion.Press
				f.buttons[i].press.Ease = EaseChisel
				if f.buttons[i].Pressed() && gtx.Source.Enabled() {
					f.buttons[i].press.Set(gtx, 1)
				} else {
					f.buttons[i].press.Set(gtx, 0)
				}
				pressed := f.buttons[i].press.Value(gtx)
				cut = gtx.Dp(th.Cut.Small) + int(float32(gtx.Dp(4))*pressed+0.5)
				sink := int(float32(gtx.Dp(1))*pressed + 0.5)
				hover := hoverValue(gtx, th, &f.buttons[i].hover, f.buttons[i].Hovered() && i != selected)
				palette := interactiveColors(gtx, th, Quiet, hover)
				if i == selected {
					palette.fill = WithAlpha(th.R.OnSel, .08*pressed)
					palette.ink = disabledInk(gtx, th.R.OnSel)
					palette.stroke = false
				}

				contentGTX := gtx
				contentGTX.Constraints.Min = image.Point{}
				contentGTX.Constraints.Max.Y = gtx.Dp(32)
				m := op.Record(gtx.Ops)
				content := layout.Inset{Left: 13, Right: 13}.Layout(contentGTX, func(gtx layout.Context) layout.Dimensions {
					parts := []layout.FlexChild{layout.Rigid(rawText(th, th.T.Label, palette.ink, label))}
					if counts != nil && i < len(counts) {
						parts = append(parts,
							layout.Rigid(layout.Spacer{Width: 6}.Layout),
							layout.Rigid(rawText(th, th.T.MonoSm, palette.ink, strconv.Itoa(counts[i]))),
						)
					}
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, parts...)
				})
				call := m.Stop()
				size := constrainMax(gtx, image.Pt(content.Size.X, gtx.Dp(32)))
				r := image.Rectangle{Max: size}
				FillCut(gtx.Ops, r, cut, palette.fill)
				if palette.stroke {
					StrokeCut(gtx.Ops, r, cut, max(1, gtx.Dp(1)), palette.outline)
				}
				offset := op.Offset(image.Pt((size.X-content.Size.X)/2, (size.Y-content.Size.Y)/2+sink)).Push(gtx.Ops)
				call.Add(gtx.Ops)
				offset.Pop()
				return layout.Dimensions{Size: size}
			})
			f.widths[i] = dims.Size.X
			focus := focusValue(gtx, th, &f.buttons[i].focus, gtx.Focused(&f.buttons[i].Click.Clickable))
			FocusRingAlpha(gtx, th, image.Rectangle{Max: dims.Size}, cut, focus)
			return dims
		}))
	}
	dims := layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Gap: gap}.Layout(gtx, children...)
	content := record.Stop()

	if selected >= 0 && selected < len(f.widths) {
		targetX := selected * gap
		for i := range selected {
			targetX += f.widths[i]
		}
		f.x.Duration, f.width.Duration = th.Motion.Select, th.Motion.Select
		f.x.Ease, f.width.Ease = EaseChisel, EaseChisel
		f.x.Set(gtx, float32(targetX))
		f.width.Set(gtx, float32(f.widths[selected]))
		x := int(f.x.Value(gtx) + .5)
		width := int(f.width.Value(gtx) + .5)
		FillCut(gtx.Ops, image.Rect(x, 0, x+width, gtx.Dp(32)), gtx.Dp(th.Cut.Small), disabledInk(gtx, th.R.Sel))
	}
	content.Add(gtx.Ops)
	return dims
}
